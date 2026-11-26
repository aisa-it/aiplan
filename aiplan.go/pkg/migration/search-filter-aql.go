package migration

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/aql"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// MigrateSearchFilterAQL заполняет aql сохранённых фильтров по старым условиям filter.
// Даты переводятся в часовом поясе автора фильтра.
type MigrateSearchFilterAQL struct {
	db *gorm.DB
}

func NewMigrateSearchFilterAQL(db *gorm.DB) *MigrateSearchFilterAQL {
	return &MigrateSearchFilterAQL{db: db}
}

func (m *MigrateSearchFilterAQL) Name() string {
	return "SearchFilterAQL"
}

func (m *MigrateSearchFilterAQL) CheckMigrate() (bool, error) {
	if !m.db.Migrator().HasColumn(&dao.SearchFilter{}, "AQL") {
		return false, nil
	}
	var exists bool
	if err := m.db.Raw(`SELECT EXISTS (SELECT 1 FROM search_filters WHERE aql IS NULL OR aql = '')`).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("MigrateSearchFilterAQL checkMigrate: %w", err)
	}
	return exists, nil
}

func (m *MigrateSearchFilterAQL) Execute() error {
	type row struct {
		ID uuid.UUID
		// без типа gorm принимает структуру за связь и не может её разобрать
		Filter       types.IssuesListFilters `gorm:"type:jsonb"`
		UserTimezone types.TimeZone
	}
	var rows []row
	if err := m.db.Raw(`
		SELECT sf.id, sf.filter, COALESCE(u.user_timezone, '') AS user_timezone
		FROM search_filters sf
		LEFT JOIN users u ON u.id = sf.author_id
		WHERE sf.aql IS NULL OR sf.aql = ''`).Scan(&rows).Error; err != nil {
		return fmt.Errorf("MigrateSearchFilterAQL select: %w", err)
	}
	for _, r := range rows {
		query := aql.ParseOldFilters(r.Filter, (*time.Location)(&r.UserTimezone))
		if err := m.db.Model(&dao.SearchFilter{}).Where("id = ?", r.ID).UpdateColumn("aql", query).Error; err != nil {
			return fmt.Errorf("MigrateSearchFilterAQL update: %w", err)
		}
	}
	slog.Info("Search filters AQL filled", "filters", len(rows))
	return nil
}
