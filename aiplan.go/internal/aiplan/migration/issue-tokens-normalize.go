package migration

import (
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// MigrateIssueTokensNormalize пересчитывает issues.tokens после смены формулы to_tsvector_multilang
// (нормализация normalize_fts_text в triggers.sql: дефис перед цифрой → пробел).
// Триггер gen_issue_vectors пересчитывает вектор только при смене name/description_stripped,
// поэтому старые задачи новую формулу сами не получат.
//
// Признак «старого» вектора — лексема, начинающаяся с '-': после нормализации парсер знаковых целых
// вида '-68584' не производит. Проверка идемпотентна: после пересчёта таких лексем нет и миграция молчит.
type MigrateIssueTokensNormalize struct {
	db *gorm.DB
}

func NewMigrateIssueTokensNormalize(db *gorm.DB) *MigrateIssueTokensNormalize {
	return &MigrateIssueTokensNormalize{db: db}
}

func (m *MigrateIssueTokensNormalize) Name() string {
	return "IssueTokensNormalize"
}

func (m *MigrateIssueTokensNormalize) CheckMigrate() (bool, error) {
	if !m.db.Migrator().HasTable("issues") {
		return false, nil
	}
	var exists bool
	err := m.db.Raw(`SELECT EXISTS (
		SELECT 1 FROM issues
		WHERE tokens IS NOT NULL
		  AND EXISTS (SELECT 1 FROM unnest(tokens) AS t WHERE t.lexeme LIKE '-%')
	)`).Scan(&exists).Error
	if err != nil {
		return false, fmt.Errorf("MigrateIssueTokensNormalize checkMigrate: %w", err)
	}
	return exists, nil
}

// Execute пересчитывает вектор только у задач со «старыми» лексемами, батчами по id —
// один UPDATE на всю таблицу держал бы длинную блокировку и раздувал WAL.
func (m *MigrateIssueTokensNormalize) Execute() error {
	const batchSize = 2000
	total := 0
	for {
		res := m.db.Exec(`UPDATE issues SET tokens = to_tsvector_multilang(name, description_stripped)
			WHERE id IN (
				SELECT id FROM issues
				WHERE tokens IS NOT NULL
				  AND EXISTS (SELECT 1 FROM unnest(tokens) AS t WHERE t.lexeme LIKE '-%')
				LIMIT ?
			)`, batchSize)
		if res.Error != nil {
			return fmt.Errorf("MigrateIssueTokensNormalize execute: %w", res.Error)
		}
		total += int(res.RowsAffected)
		if res.RowsAffected == 0 {
			break
		}
	}
	slog.Info("Issue tokens normalized", "issues", total)
	return nil
}
