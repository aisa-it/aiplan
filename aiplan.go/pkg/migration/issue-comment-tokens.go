package migration

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

// MigrateIssueCommentTokens заполняет issue_comments.tokens у комментариев, созданных до появления
// колонки: новые получают вектор от триггера, поэтому NULL есть только у старых.
type MigrateIssueCommentTokens struct {
	db *gorm.DB
}

func NewMigrateIssueCommentTokens(db *gorm.DB) *MigrateIssueCommentTokens {
	return &MigrateIssueCommentTokens{db: db}
}

func (m *MigrateIssueCommentTokens) Name() string {
	return "IssueCommentTokens"
}

func (m *MigrateIssueCommentTokens) CheckMigrate() (bool, error) {
	if !m.db.Migrator().HasColumn("issue_comments", "tokens") {
		return false, nil
	}
	var exists bool
	if err := m.db.Raw(`SELECT EXISTS (SELECT 1 FROM issue_comments WHERE tokens IS NULL)`).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("MigrateIssueCommentTokens checkMigrate: %w", err)
	}
	return exists, nil
}

// Execute идёт батчами от новых комментариев к старым, чтобы свежие раньше попали в поиск.
// Каждый батч — короткая транзакция; следующий ищется по индексу created_at от места,
// где остановился предыдущий, а не с начала таблицы.
func (m *MigrateIssueCommentTokens) Execute() error {
	const batchSize = 2000
	last := time.Now()
	total := 0
	for {
		var batch []struct {
			Id        uuid.UUID
			CreatedAt time.Time
		}
		err := m.db.Table("issue_comments").Select("id", "created_at").
			Where("created_at <= ? AND tokens IS NULL", last).
			Order("created_at DESC").Limit(batchSize).Find(&batch).Error
		if err != nil {
			return fmt.Errorf("MigrateIssueCommentTokens execute: %w", err)
		}
		if len(batch) == 0 {
			break
		}
		ids := make([]uuid.UUID, 0, len(batch))
		for _, c := range batch {
			ids = append(ids, c.Id)
		}
		res := m.db.Exec(`UPDATE issue_comments SET tokens = to_tsvector_multilang('', comment_stripped)
			WHERE id IN (?) AND tokens IS NULL`, ids)
		if res.Error != nil {
			return fmt.Errorf("MigrateIssueCommentTokens execute: %w", res.Error)
		}
		total += int(res.RowsAffected)
		last = batch[len(batch)-1].CreatedAt
	}
	slog.Info("Issue comment tokens filled", "comments", total)
	return nil
}
