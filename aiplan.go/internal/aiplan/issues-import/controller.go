// Пакет для управления импортом задач (issues) из Jira.
// Содержит логику для отслеживания статуса импорта, отмены и получения списка активных импортов.
// Основные возможности:
//   - Отслеживание статуса импорта в реальном времени.
//   - Отмена активных импортов.
//   - Получение списка активных импортов для конкретного пользователя или проекта.
//   - Очистка старых завершенных импортов.
//   - Отмена импортов для конкретного workspace.
package issues_import

import (
	"log/slog"
	"os"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	filestorage "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/file-storage"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/atomic"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/context"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/entity"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications"
	"github.com/glebarez/sqlite"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Текущие импорты в системе по проектам(key проекта жиры)
var RunningImports atomic.ImportMap[*context.ImportContext] = atomic.NewImportMap[*context.ImportContext](nil)

type ImportService struct {
	memDB         *gorm.DB
	db            *gorm.DB
	storage       filestorage.FileStorage
	notifyService *notifications.EmailService

	closeCh chan bool
}

type Import struct {
	ID                uuid.UUID
	ProjectKey        string `gorm:"uniqueIndex:import_index,priority:1,where:finished=0"`
	TargetWorkspaceID string `gorm:"uniqueIndex:import_index,priority:3,where:finished=0"`
	ActorID           string `gorm:"uniqueIndex:import_index,priority:2,where:finished=0"`
	Finished          bool
	StartAt           time.Time

	Context *context.ImportContext `gorm:"-"`
}

func (i *Import) AfterSave(tx *gorm.DB) error {
	RunningImports.Put(i.Context.ID.String(), i.Context)
	return nil
}

func (i *Import) AfterFind(tx *gorm.DB) error {
	i.Context, _ = RunningImports.Get(i.ID.String())
	return nil
}

func (i *Import) AfterDelete(tx *gorm.DB) error {
	RunningImports.Delete(i.ID.String())
	return nil
}

func (i *Import) GetStatus() ImportStatus {
	globalProgress := 0
	progress := 0
	switch i.Context.Stage {
	case "fetch":
		progress = i.Context.Counters.GetFetchProgress()
		globalProgress = int(float32(progress) * 0.2)
	case "issues":
		progress = i.Context.Counters.GetMappingProgress()
		globalProgress = int(float32(progress)*0.2) + 20
	case "attachments":
		progress = i.Context.Counters.GetAttachmentsProgress()
		globalProgress = int(float32(progress)*0.2) + 40
	case "users":
		progress = i.Context.Counters.GetUsersProgress()
		globalProgress = int(float32(progress)*0.2) + 60
	case "db":
		progress = i.Context.Counters.GetDBProgress()
		globalProgress = int(float32(progress)*0.2) + 80
	}

	if i.Context.Finished {
		progress = 100
		globalProgress = 100
	}

	status := ImportStatus{
		ID:                  i.Context.ID,
		ActorID:             i.ActorID,
		ProjectKey:          i.ProjectKey,
		TotalIssues:         i.Context.Counters.TotalIssues,
		DoneIssues:          i.Context.Counters.MappedIssues.Load(),
		TotalAttachments:    i.Context.Counters.TotalAttachments,
		ImportedAttachments: i.Context.Counters.ImportedAttachments.Load(),
		Stage:               i.Context.Stage,
		StartAt:             i.Context.StartAt,
		Finished:            i.Context.Finished,
		Progress:            progress,
		GlobalProgress:      globalProgress,
		TargetWorkspaceId:   i.Context.TargetWorkspaceID,
	}

	if i.Context.Error != nil {
		status.Error = i.Context.Error.Error()
	}

	if i.Context.Finished {
		status.EndAt = &i.Context.EndAt
	}

	i.Context.BadAttachments.Range(func(i int, a *entity.Attachment) {
		status.FailedAttachments = append(status.FailedAttachments, FailedAttachment{
			Key:          a.JiraKey,
			Name:         a.JiraAttachment.Filename,
			AttachmentId: a.JiraAttachment.ID,
		})
	})

	return status
}

type ImportStatus struct {
	ID                  uuid.UUID          `json:"id"`
	ActorID             string             `json:"actor_id"`
	ProjectKey          string             `json:"project_key"`
	TotalIssues         int                `json:"total_issues"`
	DoneIssues          int32              `json:"done_issues"`
	Stage               string             `json:"stage"`
	TotalAttachments    int                `json:"total_attachments,omitempty"`
	ImportedAttachments int32              `json:"imported_attachments,omitempty"`
	FailedAttachments   []FailedAttachment `json:"failed_attachments,omitempty"`
	StartAt             time.Time          `json:"start_at"`
	EndAt               *time.Time         `json:"end_at,omitempty"`

	Progress       int `json:"progress"`
	GlobalProgress int `json:"global_progress"`

	Finished          bool   `json:"finished"`
	TargetWorkspaceId string `json:"target_workspace_id"`
	Error             string `json:"error,omitempty"`

	Actor *dto.UserLight `json:"actor_details,omitempty"`
}

type FailedAttachment struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	AttachmentId string `json:"attachment_id"`
}

func NewImportService(db *gorm.DB, storage filestorage.FileStorage, notifyService *notifications.EmailService) *ImportService {
	memDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		slog.Error("Open sqlite memory DB", "err", err)
		os.Exit(-1)
		return nil
	}

	if err := memDB.AutoMigrate(&Import{}); err != nil {
		slog.Error("Migrate Import table", "err", err)
		os.Exit(-1)
		return nil
	}

	is := ImportService{
		memDB:         memDB,
		db:            db,
		storage:       storage,
		notifyService: notifyService,
		closeCh:       make(chan bool),
	}
	go is.flushLoop()

	return &is
}

func (is *ImportService) flushLoop() {
	for {
		select {
		case <-is.closeCh:
			return
		case <-time.After(time.Minute):
			if err := is.memDB.
				Where("finished = 1").
				Where("start_at <= DATE('now', '-1 day')").
				Delete(&Import{}).Error; err != nil {
				slog.Error("Remove old finished imports")
			}
		}
	}
}

func (is *ImportService) Close() {
	is.closeCh <- true
}

func (is *ImportService) GetUserImports(userId string) ([]ImportStatus, error) {
	var imports []Import
	if err := is.memDB.
		Where("actor_id = ?", userId).
		Order("finished").
		Find(&imports).Error; err != nil {
		return nil, err
	}
	var res []ImportStatus
	for _, i := range imports {
		res = append(res, i.GetStatus())
	}
	return res, nil
}

func (is *ImportService) GetUserImportStatus(id string, actorId string) (ImportStatus, error) {
	var i Import
	if err := is.memDB.
		Where("actor_id = ?", actorId).
		Where("id = ?", id).
		Or("project_key = ?", id). // legacy api support
		First(&i).Error; err != nil {
		return ImportStatus{}, err
	}
	return i.GetStatus(), nil
}

func (is *ImportService) CanStartImport(actorId string) bool {
	var working bool
	if err := is.memDB.Select("count(*) > 0").
		Where("actor_id = ?", actorId).
		Where("finished = 0").
		Model(&Import{}).
		Find(&working).Error; err != nil {
		slog.Error("Get user working imports", "err", err)
		return false
	}
	return !working
}

func (is *ImportService) CancelImport(id string, actorId string) error {
	var i Import
	if err := is.memDB.
		Where("actor_id = ?", actorId).
		Where("id = ?", id).
		Where("finished = 0").
		Or("project_key = ?", id). // legacy api support
		First(&i).Error; err != nil {
		return err
	}
	i.Context.Cancel()
	is.memDB.Model(&Import{}).Where("id = ?", i.ID).UpdateColumn("finished", true)
	return nil
}

func (is *ImportService) GetActiveImports() ([]ImportStatus, error) {
	var imports []Import
	if err := is.memDB.
		Where("finished = 0").
		Order("start_at").
		Find(&imports).Error; err != nil {
		return nil, err
	}
	res := make([]ImportStatus, len(imports))
	for ii, i := range imports {
		s := i.GetStatus()

		var actor dao.User
		if err := is.db.Where("id = ?", i.ActorID).First(&actor).Error; err != nil {
			slog.Error("Get import actor", "id", s.ActorID, "err", err)
		} else {
			s.Actor = actor.ToLightDTO()
		}

		res[ii] = s
	}
	return res, nil
}

func (is *ImportService) CancelWorkspaceImports(workspaceId string) error {
	var imports []Import
	if err := is.memDB.
		Where("finished = 0").
		Where("target_workspace_id = ?", workspaceId).
		Order("start_at").
		Find(&imports).Error; err != nil {
		return err
	}
	for _, i := range imports {
		i.Context.Cancel()
		is.memDB.Model(&Import{}).Where("id = ?", i.ID).UpdateColumn("finished", true)
	}
	return nil
}
