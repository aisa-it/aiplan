package migration

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ActivitiesToOneTable struct {
	db *gorm.DB

	workspaceLayer bool
	projectLayer   bool
	issueLayer     bool
	docLayer       bool
	sprintLayer    bool
	formLayer      bool
	rootLayer      bool
}

func NewMigrateActivitiesToOneTable(db *gorm.DB) *ActivitiesToOneTable {
	return &ActivitiesToOneTable{
		db: db}
}

func (a *ActivitiesToOneTable) CheckMigrate() (bool, error) {
	if err := checkTable(a.db, &WorkspaceActivity{}, &a.workspaceLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	if err := checkTable(a.db, &ProjectActivity{}, &a.projectLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	if err := checkTable(a.db, &IssueActivity{}, &a.issueLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	if err := checkTable(a.db, &DocActivity{}, &a.docLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	if err := checkTable(a.db, &SprintActivity{}, &a.sprintLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	if err := checkTable(a.db, &FormActivity{}, &a.formLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	if err := checkTable(a.db, &RootActivity{}, &a.rootLayer); err != nil {
		return false, fmt.Errorf("%s checkMigrate: %w", a.Name(), err)
	}

	return a.workspaceLayer || a.projectLayer || a.issueLayer || a.docLayer || a.sprintLayer || a.formLayer || a.rootLayer, nil
}

func checkTable(tx *gorm.DB, model interface{}, layer *bool) error {
	silentTx := tx.Session(&gorm.Session{
		Logger: logger.Default.LogMode(logger.Silent),
	})

	err := silentTx.Model(model).Select("1").Limit(1).Find(layer).Error
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "does not exist") {
		*layer = false
		return nil
	}

	return err
}

func (a *ActivitiesToOneTable) Name() string {
	return "ActivitiesToOneTable"
}

func (a *ActivitiesToOneTable) Execute() error {
	var errs []error

	{ // UserNotifications clean
		for {
			result := a.db.Unscoped().Where("id IN (?)",
				a.db.Table("user_notifications").Select("id").Limit(1000),
			).Delete(&UserNotifications{})

			if result.RowsAffected == 0 {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}
	}

	if err := migrateActivities(a.db, IssueActivity{}.TableName(), convertIssue, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", IssueActivity{}.TableName(), err))
	}
	if err := migrateActivities(a.db, ProjectActivity{}.TableName(), convertProject, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", ProjectActivity{}.TableName(), err))
	}
	if err := migrateActivities(a.db, DocActivity{}.TableName(), convertDoc, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", DocActivity{}.TableName(), err))
	}
	if err := migrateActivities(a.db, WorkspaceActivity{}.TableName(), convertWorkspace, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", WorkspaceActivity{}.TableName(), err))
	}
	if err := migrateActivities(a.db, FormActivity{}.TableName(), convertForm, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", FormActivity{}.TableName(), err))
	}
	if err := migrateActivities(a.db, SprintActivity{}.TableName(), convertSprint, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", SprintActivity{}.TableName(), err))
	}
	if err := migrateActivities(a.db, RootActivity{}.TableName(), convertRoot, a.Name()); err != nil {
		errs = append(errs, fmt.Errorf("migrate %s: %w", RootActivity{}.TableName(), err))
	}

	for _, err := range errs {
		slog.Error("ActivitiesToOneTable", "error", err.Error())
	}

	return nil
}

func migrateActivities[T any](
	db *gorm.DB,
	tableName string,
	convertFunc func(item T) (dao.ActivityEvent, uuid.UUID, bool),
	loggerName string,
) error {
	var activities []T

	if err := db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
		return tx.Transaction(func(batchTx *gorm.DB) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))

			for _, act := range activities {
				event, id, ok := convertFunc(act)
				if !ok {
					continue
				}
				ae = append(ae, event)
				ids = append(ids, id)
			}

			if len(ae) > 0 {
				if err := batchTx.Create(&ae).Error; err != nil {
					return err
				}
				slog.Info(loggerName, "from", tableName, "batch", batch, "rows", len(ae))
			}

			if len(ids) > 0 {
				if err := batchTx.Where("id IN (?)", ids).Delete(&activities).Error; err != nil {
					return err
				}
			}

			return nil
		})
	}).Error; err != nil {
		slog.Error(loggerName, "error", err.Error())
		return err
	}
	return nil
}

// IssueActivity конвертер
func convertIssue(act IssueActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}
	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerIssue,
		WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
		ProjectID:     uuid.NullUUID{UUID: act.ProjectId, Valid: true},
		IssueID:       uuid.NullUUID{UUID: act.IssueId, Valid: true},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
	}, act.Id, true
}

// ProjectActivity конвертер
func convertProject(act ProjectActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}
	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerProject,
		WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
		ProjectID:     uuid.NullUUID{UUID: act.ProjectId, Valid: true},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
	}, act.Id, true
}

func convertDoc(act DocActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerDoc,
		WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{UUID: act.DocId, Valid: true},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
	}, act.Id, true
}

func convertWorkspace(act WorkspaceActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerWorkspace,
		WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
	}, act.Id, true
}

func convertForm(act FormActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerForm,
		WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{UUID: act.FormId, Valid: true},
		SprintID:      uuid.NullUUID{},
	}, act.Id, true
}

func convertSprint(act SprintActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerSprint,
		WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{UUID: act.SprintId, Valid: true},
	}, act.Id, true
}

func convertRoot(act RootActivity) (dao.ActivityEvent, uuid.UUID, bool) {
	if act.Field == nil {
		return dao.ActivityEvent{}, uuid.UUID{}, false
	}

	return dao.ActivityEvent{
		ID:            dao.GenUUID(),
		CreatedAt:     act.CreatedAt,
		ActorID:       act.ActorId.UUID,
		Notified:      act.Notified,
		Verb:          act.Verb,
		Field:         actField.ActivityField(*act.Field),
		OldValue:      ptrStrToString(act.OldValue),
		NewValue:      act.NewValue,
		NewIdentifier: act.NewIdentifier,
		OldIdentifier: act.OldIdentifier,
		EntityType:    types.LayerRoot,
		WorkspaceID:   uuid.NullUUID{},
		ProjectID:     uuid.NullUUID{},
		IssueID:       uuid.NullUUID{},
		DocID:         uuid.NullUUID{},
		FormID:        uuid.NullUUID{},
		SprintID:      uuid.NullUUID{},
	}, act.Id, true
}

func ptrStrToString(str *string) string {
	if str == nil {
		return ""
	}
	return *str
}
