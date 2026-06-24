package cache

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
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
	var sprints []dto.SprintLight
	var forms []dto.FormLight

	g := errgroup.Group{}
	g.Go(func() error {
		var projectsRaw []dao.Project
		fmt.Println(ctx)
		if err := wsc.db.WithContext(ctx).Where("workspace_id = ?", workspaceId).Find(&projectsRaw).Error; err != nil {
			return err
		}
		projects = utils.SliceToSlice(&projectsRaw, func(t *dao.Project) dto.ProjectLight { return *t.ToLightDTO() })
		return nil
	})

	g.Go(func() error {
		var sprintsRaw []dao.Sprint
		fmt.Println(ctx)
		if err := wsc.db.WithContext(ctx).Where("workspace_id = ?", workspaceId).Find(&sprintsRaw).Error; err != nil {
			return err
		}
		sprints = utils.SliceToSlice(&sprintsRaw, func(t *dao.Sprint) dto.SprintLight { return *t.ToLightDTO() })
		return nil
	})

	g.Go(func() error {
		var formsRaw []dao.Form
		fmt.Println(ctx)
		if err := wsc.db.WithContext(ctx).Where("workspace_id = ?", workspaceId).Find(&formsRaw).Error; err != nil {
			return err
		}
		forms = utils.SliceToSlice(&formsRaw, func(t *dao.Form) dto.FormLight { return *t.ToLightDTO() })
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	wsc.m.Lock()
	wsc.c[workspaceId] = dto.WorkspaceSummaryResponse{
		Projects: projects,
		Sprints:  sprints,
		Forms:    forms,
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
