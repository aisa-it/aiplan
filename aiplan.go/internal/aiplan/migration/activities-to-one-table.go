package migration

import (
	"fmt"
	"log/slog"
	"strings"

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
	{ //issue layer
		var activities []IssueActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}
				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
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
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", IssueActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&IssueActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	{ // project layer
		var activities []ProjectActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}
				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
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
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", ProjectActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&ProjectActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	{ // workspace layer
		var activities []WorkspaceActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}

				newVal := act.NewValue
				if *act.Field == actField.Form.Field.String() && act.Verb == actField.VerbCreated && act.NewForm != nil {
					newVal = act.NewForm.Title
				}

				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
					NewValue:      newVal,
					NewIdentifier: act.NewIdentifier,
					OldIdentifier: act.OldIdentifier,
					EntityType:    types.LayerWorkspace,
					WorkspaceID:   uuid.NullUUID{UUID: act.WorkspaceId, Valid: true},
					ProjectID:     uuid.NullUUID{},
					IssueID:       uuid.NullUUID{},
					DocID:         uuid.NullUUID{},
					FormID:        uuid.NullUUID{},
					SprintID:      uuid.NullUUID{},
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", WorkspaceActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&WorkspaceActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	{ // doc layer
		var activities []DocActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}
				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
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
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", DocActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&DocActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	{ // form layer
		var activities []FormActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}

				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
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
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", FormActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&FormActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	{ // sprint layer
		var activities []SprintActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}
				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
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
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", SprintActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&SprintActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	{ // root layer
		var activities []RootActivity
		if err := a.db.FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
			ae := make([]dao.ActivityEvent, 0, len(activities))
			ids := make([]uuid.UUID, 0, len(activities))
			for _, act := range activities {
				if act.Field == nil {
					continue
				}
				ae = append(ae, dao.ActivityEvent{
					ID:            dao.GenUUID(),
					CreatedAt:     act.CreatedAt,
					ActorID:       act.ActorId.UUID,
					Notified:      act.Notified,
					Verb:          act.Verb,
					Field:         actField.ActivityField(*act.Field),
					OldValue:      act.OldValue,
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
				})

				ids = append(ids, act.Id)
			}

			if len(ae) > 0 {
				result := tx.Create(&ae)
				if result.Error != nil {
					return result.Error
				}
				slog.Info(a.Name(), "from", RootActivity{}.TableName(), "batch", batch, "rows", result.RowsAffected)
			}

			if err := tx.Where("id IN (?)", ids).Delete(&RootActivity{}).Error; err != nil {
				slog.Error(err.Error())
				return err
			}

			return nil
		}).Error; err != nil {
			slog.Error(a.Name(), "error", err.Error())
		}
	}

	return nil
}
