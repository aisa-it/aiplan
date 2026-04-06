package migration

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type MigrateActivityFieldsUpdate struct {
	db *gorm.DB
}

func NewMigrateActivityFieldsUpdate(db *gorm.DB) *MigrateActivityFieldsUpdate {
	return &MigrateActivityFieldsUpdate{
		db: db}
}

func (m *MigrateActivityFieldsUpdate) CheckMigrate() (bool, error) {

	var projectActivitiesExist bool
	if m.tableExists("project_activities") {
		if err := m.db.Model(&ProjectActivity{}).
			Select("EXISTS(?)",
				m.db.Model(&ProjectActivity{}).
					Select("1").
					Where("field = ? OR field = ?", "state", "labels"),
			).
			Find(&projectActivitiesExist).Error; err != nil {
			return false, fmt.Errorf("MigrateActivityFieldsUpdate checkMigrate: %s", err.Error())
		}
	}

	var issueActivitiesExist bool
	if m.tableExists("issue_activities") {
		if err := m.db.Model(&IssueActivity{}).
			Select("EXISTS(?)",
				m.db.Model(&IssueActivity{}).
					Select("1").
					Where("field = ? OR field = ?", "state", "labels"),
			).
			Find(&issueActivitiesExist).Error; err != nil {
			return false, fmt.Errorf("MigrateActivityFieldsUpdate checkMigrate: %s", err.Error())
		}
	}

	return projectActivitiesExist || issueActivitiesExist, nil
}

func (m *MigrateActivityFieldsUpdate) tableExists(tableName string) bool {
	var exists bool
	m.db.Raw(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`, tableName).Scan(&exists)
	return exists
}

func (m *MigrateActivityFieldsUpdate) Name() string {
	return "ActivityFieldsUpdate"
}

func (m *MigrateActivityFieldsUpdate) Execute() error {
	activityFieldUpdate(m.db)
	return nil
}

func activityFieldUpdate(db *gorm.DB) {
	updateInBatches[IssueActivity](db, "state", "status", "actIssueState")
	updateInBatches[ProjectActivity](db, "state", "status", "actProjectState")
	updateInBatches[IssueActivity](db, "labels", "label", "actIssueLabel")
	updateInBatches[ProjectActivity](db, "labels", "label", "actProjectLabel")
}

func updateInBatches[T interface{ GetId() uuid.UUID }](db *gorm.DB, oldValue, newValue, actionName string) {
	var activities []T
	if err := db.Where("field = ?", oldValue).FindInBatches(&activities, 30, func(tx *gorm.DB, batch int) error {
		result := tx.Model(new(T)).Where("id IN ?", utils.SliceToSlice(&activities, func(t *T) uuid.UUID {
			return (*t).GetId()
		})).Update("field", newValue)
		if result.Error != nil {
			return result.Error
		}
		slog.Info("activityFieldUpdate", "action", actionName, "batch", batch, "rows", result.RowsAffected)
		return nil
	}).Error; err != nil {
		slog.Error("activityFieldUpdate", "action", actionName, "error", err.Error())
	}
}
