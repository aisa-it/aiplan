package cache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

const (
	workspaceSummariesNotifyChannel = "workspace_summary_changes"
)

type WorkspaceSummaryCache struct {
	db *gorm.DB
	m  sync.RWMutex
	c  map[uuid.UUID]dto.WorkspaceSummaryResponse
}

var WorkspaceSummary WorkspaceSummaryCache

func InitWorkspaceSummaryCache(db *gorm.DB) {
	WorkspaceSummary = WorkspaceSummaryCache{c: make(map[uuid.UUID]dto.WorkspaceSummaryResponse), db: db}
	dao.NotifiSubscription.Subscribe(workspaceSummariesNotifyChannel, WorkspaceSummary.notifyHandler)
}

func (wsc *WorkspaceSummaryCache) fetch(rootCtx context.Context, workspaceId uuid.UUID) error {
	ctx, span := otel.
		Tracer("aiplan").
		Start(rootCtx, "workspace_summary_cache_fetch")
	defer span.End()

	var projects []dto.ProjectLight
	var projectsRaw []dao.Project
	var sprintsRaw []dao.Sprint
	var foldersRaw []dao.SprintFolder
	var forms []dto.FormLight
	var formsRaw []dao.Form

	var sprintStatsRows []struct {
		SprintId   uuid.UUID
		AllIssues  int
		Pending    int
		InProgress int
		Completed  int
		Cancelled  int
	}

	g := errgroup.Group{}
	g.Go(func() error {
		if err := wsc.db.WithContext(ctx).Where("workspace_id = ?", workspaceId).Find(&projectsRaw).Error; err != nil {
			return err
		}
		projects = utils.SliceToSlice(&projectsRaw, func(t *dao.Project) dto.ProjectLight { return *t.ToLightDTO() })
		return nil
	})

	// Спринты группируются в папки так же, как в getSprintList (http-sprint.go).
	g.Go(func() error {
		return wsc.db.WithContext(ctx).
			Joins("SprintFolder").
			Where("sprints.workspace_id = ?", workspaceId).
			Order("start_date DESC").
			Find(&sprintsRaw).Error
	})

	g.Go(func() error {
		return wsc.db.WithContext(ctx).Where("workspace_id = ?", workspaceId).Find(&foldersRaw).Error
	})

	g.Go(func() error {
		if err := wsc.db.WithContext(ctx).Where("workspace_id = ?", workspaceId).Find(&formsRaw).Error; err != nil {
			return err
		}
		forms = utils.SliceToSlice(&formsRaw, func(t *dao.Form) dto.FormLight { return *t.ToLightDTO() })
		return nil
	})

	// Статистика по спринтам считается одним агрегирующим SQL-запросом (COUNT/SUM CASE по sprint_id),
	// без загрузки issues в память — по аналогии с business/stats.go:getSprintStats.
	g.Go(func() error {
		return wsc.db.WithContext(ctx).
			Model(&dao.SprintIssue{}).
			Select(`
				sprint_issues.sprint_id as sprint_id,
				COUNT(*) as all_issues,
				SUM(CASE WHEN s.group != 'cancelled' AND i.start_date IS NULL AND i.completed_at IS NULL OR s.group = 'backlog' THEN 1 ELSE 0 END) as pending,
				SUM(CASE WHEN s.group != 'cancelled' AND i.start_date IS NOT NULL AND i.completed_at IS NULL THEN 1 ELSE 0 END) as in_progress,
				SUM(CASE WHEN s.group != 'cancelled' AND i.completed_at IS NOT NULL THEN 1 ELSE 0 END) as completed,
				SUM(CASE WHEN s.group = 'cancelled' THEN 1 ELSE 0 END) as cancelled
			`).
			Joins("JOIN issues i ON i.id = sprint_issues.issue_id").
			Joins("JOIN states s ON s.id = i.state_id").
			Where("sprint_issues.workspace_id = ?", workspaceId).
			Group("sprint_issues.sprint_id").
			Scan(&sprintStatsRows).Error
	})

	if err := g.Wait(); err != nil {
		return err
	}

	// Сборка папок спринтов: проставляем посчитанную в SQL статистику и группируем
	// спринты по папкам так же, как это делает getSprintList (http-sprint.go).
	var sprints []dto.SprintFolder
	{
		// Статистика считалась одним агрегирующим запросом отдельно от спринтов,
		// поэтому мёржим её по sprint_id уже здесь, в памяти.
		statsBySprint := make(map[uuid.UUID]types.SprintStats, len(sprintStatsRows))
		for _, r := range sprintStatsRows {
			statsBySprint[r.SprintId] = types.SprintStats{
				AllIssues:  r.AllIssues,
				Pending:    r.Pending,
				InProgress: r.InProgress,
				Completed:  r.Completed,
				Cancelled:  r.Cancelled,
			}
		}
		for i := range sprintsRaw {
			sprintsRaw[i].Stats = statsBySprint[sprintsRaw[i].Id]
		}

		// Индекс папок по id — сюда будем раскладывать спринты. Храним указатели,
		// чтобы append ниже мутировал именно объект в карте, а не его копию.
		folderMap := make(map[uuid.UUID]*dao.SprintFolder, len(foldersRaw))
		for i := range foldersRaw {
			folderMap[foldersRaw[i].Id] = &foldersRaw[i]
		}

		// Раскладываем спринты по папкам; спринты без папки (или с папкой,
		// которая почему-то не нашлась в folderMap) собираем отдельно.
		var unassignedSprints []dao.Sprint
		for i := range sprintsRaw {
			if sprintsRaw[i].SprintFolderId.Valid {
				if folder, ok := folderMap[sprintsRaw[i].SprintFolderId.UUID]; ok {
					folder.Sprints = append(folder.Sprints, sprintsRaw[i])
				}
			} else {
				unassignedSprints = append(unassignedSprints, sprintsRaw[i])
			}
		}

		// Итоговый список папок + папка-заглушка (Id: uuid.Nil) для спринтов без папки.
		result := make([]dao.SprintFolder, 0, len(folderMap)+1)
		for _, folder := range folderMap {
			result = append(result, *folder)
		}
		if len(unassignedSprints) != 0 {
			result = append(result, dao.SprintFolder{
				Id:      uuid.Nil,
				Sprints: unassignedSprints,
			})
		}

		// Именованные папки — по алфавиту, папка-заглушка всегда последней.
		slices.SortFunc(result, func(a, b dao.SprintFolder) int {
			if a.Id == uuid.Nil && b.Id != uuid.Nil {
				return 1
			}
			if a.Id != uuid.Nil && b.Id == uuid.Nil {
				return -1
			}
			if a.Id == uuid.Nil && b.Id == uuid.Nil {
				return 0
			}
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})

		sprints = utils.SliceToSlice(&result, func(t *dao.SprintFolder) dto.SprintFolder { return *t.ToDTO() })
	}

	// Хэш содержимого сводки: склеиваем уже готовые row_hash из БД (projects/sprints/sprint_folders/forms)
	// плюс статистику по спринтам, которая не хранится в колонке, а считается отдельным агрегирующим запросом
	// выше. Куски сортируются перед хэшированием, чтобы результат не зависел от порядка строк, вернувшегося
	// из БД (запросы по projects/forms идут без ORDER BY).
	chunks := make([][]byte, 0, len(projectsRaw)+len(foldersRaw)+2*len(sprintsRaw)+len(formsRaw))
	for _, p := range projectsRaw {
		chunks = append(chunks, p.Hash)
	}
	for _, f := range foldersRaw {
		chunks = append(chunks, f.Hash)
	}
	for _, sp := range sprintsRaw {
		chunks = append(chunks, fmt.Appendf(sp.Hash, "%d_%d_%d_%d_%d",
			sp.Stats.AllIssues, sp.Stats.Pending, sp.Stats.InProgress, sp.Stats.Completed, sp.Stats.Cancelled))
	}
	for _, f := range formsRaw {
		chunks = append(chunks, f.Hash)
	}
	slices.SortFunc(chunks, bytes.Compare)

	h := sha256.New()
	for _, c := range chunks {
		h.Write(c)
	}

	wsc.m.Lock()
	wsc.c[workspaceId] = dto.WorkspaceSummaryResponse{
		Projects: projects,
		Sprints:  sprints,
		Forms:    forms,
		Hash:     h.Sum(nil),
	}
	wsc.m.Unlock()

	return nil
}

func (wsc *WorkspaceSummaryCache) notifyHandler(payload string) {
	workspaceId, err := uuid.FromString(payload)
	if err != nil {
		slog.Warn("WorkspaceSummaryCache get invalid payload", "payload", payload, "err", err)
		return
	}

	if err := wsc.fetch(context.Background(), workspaceId); err != nil {
		slog.Error("WorkspaceSummaryCache fetch new entities", "workspaceId", workspaceId, "err", err)
	}
}

func (wsc *WorkspaceSummaryCache) Load(ctx context.Context, workspaceId uuid.UUID) *dto.WorkspaceSummaryResponse {
	wsc.m.RLock()
	e, ok := wsc.c[workspaceId]
	wsc.m.RUnlock()

	if ok {
		return &e
	}

	if err := wsc.fetch(ctx, workspaceId); err != nil {
		slog.Error("WorkspaceSummaryCache fetch new entities", "workspaceId", workspaceId, "err", err)
		return nil
	}

	wsc.m.RLock()
	e, ok = wsc.c[workspaceId]
	wsc.m.RUnlock()
	if !ok {
		return nil
	}
	return &e
}
