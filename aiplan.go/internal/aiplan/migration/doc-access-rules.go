package migration

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type MigrateDocAccessRule struct {
	db *gorm.DB

	tableDocEditor  string
	tableDocReader  string
	tableDocWatcher string

	plan *MigratePlan
}

func NewMigrateDocAccessRule(db *gorm.DB) *MigrateDocAccessRule {
	return &MigrateDocAccessRule{
		db:              db,
		tableDocEditor:  "doc_editors",
		tableDocReader:  "doc_readers",
		tableDocWatcher: "doc_watchers",
	}
}

type DocReader struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	ReaderId    string  `json:"reader_id" gorm:"uniqueIndex:doc_readers_idx,priority:2"`
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	DocId       string  `json:"doc_id" gorm:"index;uniqueIndex:doc_readers_idx,priority:1"`
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	WorkspaceId string  `json:"workspace_id"`

	Workspace *dao.Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Doc       *dao.Doc       `gorm:"foreignKey:DocId" extensions:"x-nullable"`
	Reader    *dao.User      `gorm:"foreignKey:ReaderId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocReader) TableName() string { return "doc_readers" }

type DocEditor struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	EditorId    string  `json:"editor_id" gorm:"uniqueIndex:doc_editors_idx,priority:2"`
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	DocId       string  `json:"doc_id" gorm:"index;uniqueIndex:doc_editors_idx,priority:1"`
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	WorkspaceId string  `json:"workspace_id"`

	Workspace *dao.Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Doc       *dao.Doc       `gorm:"foreignKey:DocId" extensions:"x-nullable"`
	Editor    *dao.User      `gorm:"foreignKey:EditorId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocEditor) TableName() string { return "doc_editors" }

type DocWatcher struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	WatcherId   string  `json:"watcher_id" gorm:"uniqueIndex:doc_watchers_idx,priority:2"`
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	DocId       string  `json:"doc_id" gorm:"index;uniqueIndex:doc_watchers_idx,priority:1"`
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	WorkspaceId string  `json:"workspace_id"`

	Workspace *dao.Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Doc       *dao.Doc       `gorm:"foreignKey:DocId" extensions:"x-nullable"`
	Watcher   *dao.User      `gorm:"foreignKey:WatcherId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующей сущности Doc. Используется для определения имени таблицы при работе с базой данных.
func (DocWatcher) TableName() string { return "doc_watchers" }

func (m *MigrateDocAccessRule) CheckMigrate() (bool, error) {
	migratePlan, err := checkTables(m.db, m.tableDocEditor, m.tableDocReader, m.tableDocWatcher)
	if err != nil {
		return false, fmt.Errorf("MigrateDocAccessRule checkMigrate: %s", err.Error())
	}
	m.plan = migratePlan
	return len(m.plan.migrate)+len(m.plan.delete) > 0, nil
}

func (m *MigrateDocAccessRule) Name() string {
	return "DocAccessRule"
}

func (m *MigrateDocAccessRule) Execute() error {
	if len(m.plan.migrate) > 0 {
		if err := m.migrate(); err != nil {
			return err
		}
	}

	m.CheckMigrate()

	if len(m.plan.migrate) == 0 && len(m.plan.delete) > 0 {

		if err := m.db.Migrator().DropTable(m.tableDocEditor); err != nil {
			return fmt.Errorf("MigrateDocAccessRule drop table_doc_editor: %s", err.Error())
		}
		slog.Info("Migration drop", "table", m.tableDocEditor)

		if err := m.db.Migrator().DropTable(m.tableDocReader); err != nil {
			return fmt.Errorf("MigrateDocAccessRule drop table_doc_reader: %s", err.Error())
		}
		slog.Info("Migration drop", "table", m.tableDocReader)

		if err := m.db.Migrator().DropTable(m.tableDocWatcher); err != nil {
			return fmt.Errorf("MigrateDocAccessRule drop table_doc_watcher: %s", err.Error())
		}
		slog.Info("Migration drop", "table", m.tableDocWatcher)
	}
	return nil
}

func (m *MigrateDocAccessRule) migrate() error {
	batchSize := 10

	var allDocIDs []string
	if err := m.db.Model(&dao.Doc{}).
		Select("DISTINCT docs.id").
		Joins("LEFT JOIN doc_watchers w ON w.doc_id = docs.id").
		Joins("LEFT JOIN doc_editors e ON e.doc_id = docs.id").
		Joins("LEFT JOIN doc_readers r ON r.doc_id = docs.id").
		Where("w.doc_id IS NOT NULL OR e.doc_id IS NOT NULL OR r.doc_id IS NOT NULL").
		Pluck("docs.id", &allDocIDs).Error; err != nil {
		return err
	}

	slog.Info("Total documents to process", "count", len(allDocIDs), "batchSize", batchSize)

	for i := 0; i < len(allDocIDs); i += batchSize {
		end := i + batchSize
		if end > len(allDocIDs) {
			end = len(allDocIDs)
		}

		batch := allDocIDs[i:end]
		batchNumber := i/batchSize + 1
		success := 0

		for _, docID := range batch {
			if err := m.db.Transaction(func(tx *gorm.DB) error {
				return processDocument(tx, docID)
			}); err != nil {
				slog.Error("process migration error", "document", docID, "batch", batchNumber, "err", err)
				continue
			}
			success++

		}

		slog.Info("Migrate data ok", "name", m.Name(), "batch", batchNumber, "countDoc", len(batch), "success", success)

	}

	return nil
}

func processDocument(tx *gorm.DB, docID string) error {
	var doc dao.Doc
	if err := tx.Where("id = ?", docID).First(&doc).Error; err != nil {
		return err
	}

	var editors []DocEditor
	if err := tx.Where("doc_id = ?", docID).Find(&editors).Error; err != nil {
		return err
	}

	var readers []DocReader
	if err := tx.Where("doc_id = ?", docID).Find(&readers).Error; err != nil {
		return err
	}

	var watchers []DocWatcher
	if err := tx.Where("doc_id = ?", docID).Find(&watchers).Error; err != nil {
		return err
	}

	workspaceUUID := uuid.Must(uuid.FromString(doc.WorkspaceId))
	accessRulesMap := make(map[string]dao.DocAccessRules)

	for _, reader := range readers {
		if _, ok := accessRulesMap[reader.ReaderId]; !ok {
			accessRulesMap[reader.ReaderId] = dao.DocAccessRules{
				Id:          dao.GenUUID(),
				CreatedAt:   reader.CreatedAt,
				UpdatedAt:   reader.UpdatedAt,
				MemberId:    uuid.Must(uuid.FromString(reader.ReaderId)),
				CreatedById: uuid.Must(uuid.FromString(*reader.CreatedById)),
				DocId:       doc.ID,
				UpdatedById: convertToNullUUID(reader.UpdatedById),
				WorkspaceId: workspaceUUID,
				Edit:        false,
				Watch:       false,
			}
		}
	}

	for _, editor := range editors {
		if existing, ok := accessRulesMap[editor.EditorId]; ok {
			existing.Edit = true
			updateTimestamps(&existing, editor.CreatedAt, editor.UpdatedAt, editor.CreatedById, editor.UpdatedById)
			accessRulesMap[editor.EditorId] = existing
		} else {
			accessRulesMap[editor.EditorId] = dao.DocAccessRules{
				Id:          dao.GenUUID(),
				CreatedAt:   editor.CreatedAt,
				UpdatedAt:   editor.UpdatedAt,
				MemberId:    uuid.Must(uuid.FromString(editor.EditorId)),
				CreatedById: uuid.Must(uuid.FromString(*editor.CreatedById)),
				DocId:       doc.ID,
				UpdatedById: convertToNullUUID(editor.UpdatedById),
				WorkspaceId: workspaceUUID,
				Edit:        true,
				Watch:       false,
			}
		}
	}

	for _, watcher := range watchers {
		if existing, ok := accessRulesMap[watcher.WatcherId]; ok {
			existing.Watch = true
			updateTimestamps(&existing, watcher.CreatedAt, watcher.UpdatedAt, watcher.CreatedById, watcher.UpdatedById)
			accessRulesMap[watcher.WatcherId] = existing
		} else {
			accessRulesMap[watcher.WatcherId] = dao.DocAccessRules{
				Id:          dao.GenUUID(),
				CreatedAt:   watcher.CreatedAt,
				UpdatedAt:   watcher.UpdatedAt,
				MemberId:    uuid.Must(uuid.FromString(watcher.WatcherId)),
				CreatedById: uuid.Must(uuid.FromString(*watcher.CreatedById)),
				DocId:       doc.ID,
				UpdatedById: convertToNullUUID(watcher.UpdatedById),
				WorkspaceId: workspaceUUID,
				Edit:        false,
				Watch:       true,
			}
		}
	}

	for _, accessRule := range accessRulesMap {
		if err := tx.Create(&accessRule).Error; err != nil {
			return err
		}
	}

	if len(readers) > 0 {
		if err := tx.Where("doc_id = ?", docID).Delete(&DocReader{}).Error; err != nil {
			return err
		}
	}

	if len(editors) > 0 {
		if err := tx.Where("doc_id = ?", docID).Delete(&DocEditor{}).Error; err != nil {
			return err
		}
	}

	if len(watchers) > 0 {
		if err := tx.Where("doc_id = ?", docID).Delete(&DocWatcher{}).Error; err != nil {
			return err
		}
	}

	return nil
}

func updateTimestamps(rule *dao.DocAccessRules, newCreatedAt, newUpdatedAt time.Time, newCreatedByID, newUpdatedByID *string) {
	if newCreatedAt.Before(rule.CreatedAt) {
		rule.CreatedAt = newCreatedAt
		if newCreatedByID != nil {
			rule.CreatedById = uuid.Must(uuid.FromString(*newCreatedByID))
		}
	}

	if newUpdatedAt.After(rule.UpdatedAt) {
		rule.UpdatedAt = newUpdatedAt
		rule.UpdatedById = convertToNullUUID(newUpdatedByID)
	}
}

func convertToNullUUID(strPtr *string) uuid.NullUUID {
	if strPtr == nil || *strPtr == "" {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{
		UUID:  uuid.Must(uuid.FromString(*strPtr)),
		Valid: true,
	}
}
