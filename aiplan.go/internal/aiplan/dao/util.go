// DAO (Data Access Object) - предоставляет методы для взаимодействия с базой данных.
// Содержит функции для работы с пользователями, проектами, документами и другими сущностями.
//
// Основные возможности:
//   - Работа с пользователями: создание, аутентификация, получение информации о пользователях.
//   - Работа с проектами: получение списка проектов, фильтрация проектов по различным критериям.
//   - Работа с документами: получение списка документов, фильтрация документов по различным критериям.
//   - Работа с правами доступа: определение прав доступа пользователей к различным ресурсам.
//   - Генерация UUID и паролей.
//   - Обработка текстовых данных (например, выделение упоминаний пользователей в тексте).
package dao

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"log/slog"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"github.com/sethvargo/go-password/password"
	"golang.org/x/crypto/pbkdf2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// -migration
type PaginationResponse struct {
	Count  int64 `json:"count"`
	Offset int   `json:"offset"`
	Limit  int   `json:"limit"`
	Result any   `json:"result"`

	MyEntity any `json:"my_entity,omitempty"`
}

func AddDefaultUser(db *gorm.DB, email string) {
	pass := "pbkdf2_sha256$260000$QM9bPwqeyc3Ed2LYppRoNN$BRt1aWr5wV3uqY/14k24Fnhaj1+TWExblkXUjFJKHDw=" // password123
	u := GenUUID()
	ubx := "admin"
	tm := time.Now()
	user := User{
		ID:              u,
		Email:           email,
		Password:        pass,
		Username:        &ubx,
		LastActive:      &tm,
		LastLoginTime:   &tm,
		LastLoginIp:     "0.0.0.0",
		LastLoginUagent: "golang",
		TokenUpdatedAt:  &tm,
		Theme:           types.Theme{},
		IsActive:        true,
		IsSuperuser:     true,
	}

	if err := db.Create(&user).Error; err != nil {
		log.Println(err)
	} else {
		log.Println("User created")
	}
}

func PaginationRequest(offset int, limit int, query *gorm.DB, target any) (res PaginationResponse, err error) {
	// Count query
	if err := query.Session(&gorm.Session{}).Model(target).Count(&res.Count).Error; err != nil {
		return res, err
	}

	// Data query
	if err := query.Offset(offset).Limit(limit).Find(target).Error; err != nil {
		return res, err
	}

	res.Result = target
	res.Limit = limit
	res.Offset = offset

	return res, nil
}

func GetIssueFamily(issue Issue, db *gorm.DB) (family []Issue) {
	var getChildren func(issueId uuid.UUID) []Issue

	getChildren = func(issueId uuid.UUID) []Issue {
		var children []Issue
		db.Where("parent_id = ?", issueId).Preload(clause.Associations).Find(&children)
		for _, child := range children {
			children = append(children, getChildren(child.ID)...)
		}
		return children
	}
	if parent := GetIssueRoot(issue, db); parent != nil && !parent.ID.IsNil() {
		family = append(getChildren(parent.ID), *parent)
	} else {
		family = append(getChildren(issue.ID), issue)
	}
	slices.SortFunc(family, func(a Issue, b Issue) int {
		return a.SequenceId - b.SequenceId
	})

	for i := range family {
		family[i].FetchLinkedIssues(db)
	}
	return
}

func GetIssueRoot(issue Issue, db *gorm.DB) *Issue {
	if !issue.ParentId.Valid {
		return nil
	}

	var getParent func(issue Issue) *Issue

	getParent = func(issue Issue) *Issue {
		var parent Issue
		db.Where("id = ?", issue.ParentId).Preload(clause.Associations).Find(&parent)
		if parent.ParentId.Valid {
			return getParent(parent)
		}
		return &parent
	}
	return getParent(issue)
}

func GenPassword() string {
	return password.MustGenerate(12, 6, 0, false, false)
}

// Генерация хэша пароля для базы
func GenPasswordHash(password string) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	salt := make([]rune, 32)
	for i := range salt {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		salt[i] = letters[nBig.Int64()]
	}

	return fmt.Sprintf("pbkdf2_sha256$260000$%s$%s",
		string(salt),
		base64.StdEncoding.EncodeToString(pbkdf2.Key([]byte(password), []byte(string(salt)), 260000, 32, sha256.New)),
	)
}

func GetMentionedUsers(db *gorm.DB, text types.RedactorHTML) ([]User, error) {
	reg := regexp.MustCompile(`@(\w+)`)

	res := reg.FindAllStringSubmatch(text.Body, -1)

	if len(res) == 0 {
		return nil, nil
	}

	usernames := make([]string, len(res))
	for i, r := range res {
		usernames[i] = r[1]
	}

	var users []User
	err := db.Where("username in (?)", usernames).Find(&users).Error
	return users, err
}

func ExtractProjectMemberIDs(members []ProjectMember) []uuid.UUID {
	ids := make([]uuid.UUID, len(members))
	for i, member := range members {
		ids[i] = member.MemberId
	}
	return ids
}

func PrepareFilterProjectsQuery(tx *gorm.DB, userID uuid.UUID, workspaceIDs, projectIDs []string) *gorm.DB {
	projectQuery := tx.Model(&ProjectMember{}).
		Select("project_id").
		Where("member_id = ?", userID)

	if len(workspaceIDs) > 0 {
		projectQuery = projectQuery.Where("workspace_id IN ?", workspaceIDs)
	}

	if len(projectIDs) > 0 {
		projectQuery = projectQuery.Where("project_id IN ?", projectIDs)
	}

	return projectQuery
}

func GetUserNeighbors(tx *gorm.DB, userID uuid.UUID, workspaceIDs, projectIDs []string) *gorm.DB {
	query := tx.Model(&ProjectMember{}).
		Distinct("project_members.member_id").
		Joins("JOIN project_members u on project_members.project_id = u.project_id").
		Where("u.member_id = ?", userID)

	if len(workspaceIDs) > 0 {
		query = query.Where("project_members.workspace_id IN ?", workspaceIDs)
	}

	if len(projectIDs) > 0 {
		query = query.Where("project_members.project_id IN ?", projectIDs)
	}
	return query
}

func GetUserFromProjectMember(members []User, ids []interface{}) []User {
	var users []User
	idsMap := make(map[uuid.UUID]struct{})
	for _, id := range ids {
		idsMap[id.(uuid.UUID)] = struct{}{}
	}
	for _, member := range members {
		if _, ok := idsMap[member.ID]; ok {
			users = append(users, member)
		}
	}
	return users
}

func UpdateUserLastActivityTime(tx *gorm.DB, user *User) error {
	// User table update cooldown
	if user.LastActive != nil && time.Since(*user.LastActive) <= time.Second*10 {
		return nil
	}
	return tx.Omit(clause.Associations).Model(user).UpdateColumn("last_active", time.Now()).Error
}

func SplitTSQuery(searchQuery string) string {
	searchQuery = strings.TrimSpace(searchQuery)
	if searchQuery == "" {
		return ""
	}
	words := strings.Fields(searchQuery)
	var tokens []string
	for _, word := range words {
		word = strings.Trim(word, "<:|'*)(&![]")
		if word != "" {
			tokens = append(tokens, word+":*")
		}
	}

	if len(tokens) == 0 {
		return ""
	}

	return strings.Join(tokens, " | ")
}

func GetIssuesLink(id1 uuid.UUID, id2 uuid.UUID) LinkedIssues {
	link := LinkedIssues{
		Id1: id1,
		Id2: id2,
	}
	if bytes.Compare(id2.Bytes(), id1.Bytes()) < 0 {
		link = LinkedIssues{
			Id1: id2,
			Id2: id1,
		}
	}
	return link
}

func BuildUnionSubquery(tx *gorm.DB, alias string, tab UnionableTable, tables ...UnionableTable) *gorm.DB {
	var union []string
	var args []interface{}

	for _, table := range tables {
		var selectFields []string
		fieldSet := utils.SliceToSet(table.GetFields())
		for _, field := range tab.GetFields() {
			f := strings.Split(field, "::")
			var t string
			if len(f) > 1 {
				field = f[0]
				t = "::" + f[1]
			}

			if utils.CheckInSet(fieldSet, field) {
				selectFields = append(selectFields, field+t)
			} else {
				selectFields = append(selectFields, fmt.Sprintf("NULL%s AS %s", t, field))
			}
		}

		selectFields = AddCustomFields(table, selectFields)

		q := tx.Session(&gorm.Session{DryRun: true}).
			Select(selectFields).
			Model(table).
			Find(nil).Statement

		union = append(union, "("+q.SQL.String()+")")
		args = append(args, q.Vars...)
	}

	unionSQL := strings.Join(union, " UNION ALL ")

	return tx.Table("(?) AS "+alias, gorm.Expr(unionSQL, args...)).Model(tab)
}

func SliceToSet(sl []string) map[string]interface{} {
	res := make(map[string]interface{})
	for _, s := range sl {
		res[s] = struct{}{}
	}
	return res
}

func DeleteWorkspaceMember(actor *WorkspaceMember, requestedMember *WorkspaceMember, tx *gorm.DB) error {
	// Change workspace owner on demand
	if requestedMember.Workspace.OwnerId == requestedMember.MemberId {
		if err := requestedMember.Workspace.ChangeOwner(tx, actor); err != nil {
			return err
		}
	}

	// Change role to admin for new owner
	if err := tx.Model(requestedMember).UpdateColumn("role", types.AdminRole).Error; err != nil {
		return err
	}

	// Update memberships in projects
	{
		var projects []Project
		if err := tx.Where("workspace_id = ?", requestedMember.Workspace.ID).Find(&projects).Error; err != nil {
			return err
		}

		createdByID := uuid.NullUUID{UUID: actor.MemberId, Valid: true}

		for _, project := range projects {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "project_id"}, {Name: "member_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"role": types.AdminRole, "updated_at": time.Now(), "updated_by_id": createdByID}),
			}).Create(&ProjectMember{
				ID:          GenUUID(),
				CreatedAt:   time.Now(),
				CreatedById: createdByID,
				WorkspaceId: requestedMember.Workspace.ID,
				ProjectId:   project.ID,
				Role:        types.AdminRole,
				MemberId:    requestedMember.MemberId,
			}).Error; err != nil {
				return err
			}
		}
	}

	// Migrate projects leaders to current user
	if err := tx.
		Model(&Project{}).
		Where(&Project{
			WorkspaceId:   requestedMember.Workspace.ID,
			ProjectLeadId: requestedMember.MemberId,
		}).
		Updates(Project{
			ProjectLeadId: actor.MemberId,
		}).Error; err != nil {
		return err
	}

	return tx.Omit(clause.Associations).Delete(requestedMember).Error
}

func pointerToStr(str *string) string {
	if str == nil {
		return ""
	}
	return *str
}

func GetFileAssetFromDescription(query *gorm.DB, description *string) ([]FileAsset, error) {
	var fileAssets []FileAsset
	var ids []string

	if description == nil {
		return nil, fmt.Errorf("body empty")
	}

	re := regexp.MustCompile(`/api/file/([a-f0-9-]+-\d+)`)
	matches := re.FindAllStringSubmatch(*description, -1)
	for _, match := range matches {
		ids = append(ids, match[1])
	}

	if err := query.Where("name IN (?)", ids).Find(&fileAssets).Error; err != nil {
		return nil, err
	}
	return fileAssets, nil
}

// UserPrivilegesOverIssue
// -migration
type UserPrivilegesOverIssue struct {
	UserId        string
	IssueId       string
	WorkspaceRole int
	ProjectRole   int
	IsAuthor      bool
	IsAssigner    bool
	IsWatcher     bool
}

// UserPrivilegesOverDoc
// -migration
type UserPrivilegesOverDoc struct {
	UserId        string
	DocId         string
	WorkspaceRole int
	IsAuthor      bool
	IsEditor      bool
	IsReader      bool
	IsWatcher     bool
}

func GetUserPrivilegesOverIssue(issueId string, userId uuid.UUID, db *gorm.DB) (*UserPrivilegesOverIssue, error) {
	var priv UserPrivilegesOverIssue
	if err := db.Raw(`select wm.member_id as "user_id", i.id as "issue_id", wm.role as "workspace_role", pm.role as "project_role", i.created_by_id = ? as "is_author", ia.id is not null as "is_assigner", iw.id is not null as "is_watcher" from issues i
left join issue_assignees ia on i.id = ia.issue_id and ia.assignee_id = ?
left join issue_watchers iw on i.id = iw.issue_id and iw.watcher_id = ?
left join workspace_members wm on i.workspace_id = wm.workspace_id and wm.member_id = ?
left join project_members pm on i.project_id = pm.project_id and pm.member_id = ?
where i.id = ?`, userId, userId, userId, userId, userId, issueId).First(&priv).Error; err != nil {
		return nil, err
	}
	return &priv, nil
}

func GetUserPrivilegesOverDoc(docId string, userId uuid.UUID, db *gorm.DB) (*UserPrivilegesOverDoc, error) {
	var priv UserPrivilegesOverDoc
	if err := db.Raw(`select
	wm.member_id as "user_id",
  d.id as "doc_id",
  wm.role as "workspace_role",
  d.created_by_id = ? as "is_author",
  (dar.id is not null and dar.edit is true) or wm.role >= d.editor_role as "is_editor",
  dar.id is not null  or wm.role >= d.reader_role as "is_reader",
  (dar.id is not null and dar.watch is true)  as "is_watcher"
from docs d
left join doc_access_rules dar on d.id = dar.doc_id and dar.member_id = ?
left join workspace_members wm on d.workspace_id = wm.workspace_id and wm.member_id = ?
where d.id = ?`, userId, userId, userId, docId).First(&priv).Error; err != nil {
		return nil, err
	}
	return &priv, nil
}

func GetSystemUser(tx *gorm.DB) *User {
	var user User
	username := "system"
	if err := tx.Where("username = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			user = User{
				ID:        GenUUID(),
				Email:     "aiplan@aiplan.ru",
				Password:  "",
				FirstName: "АИПлан",
				Username:  &username,
				IsActive:  true,
				IsBot:     true,
			}
			if err := tx.Create(&user).Error; err != nil {
				slog.Error("Create system user", "err", err)
				return nil
			}
		} else {
			slog.Error("Get system user", "err", err)
			return nil
		}
	}
	return &user
}

const (
	getForeignKeysSQL = `SELECT
    tc.table_name AS foreign_table_name,
    kcu.column_name AS foreign_column_name,
    ccu.table_name AS referenced_table_name,
    ccu.column_name AS referenced_column_name,
    tc.constraint_name
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
    ON tc.constraint_name = kcu.constraint_name
JOIN information_schema.constraint_column_usage AS ccu
    ON ccu.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'FOREIGN KEY'
    AND ccu.table_name = ?
    AND ccu.column_name = ?;`
)

type ForeignKey struct {
	ForeignTableName     string
	ForeignColumnName    string
	ReferencedTableName  string
	ReferencedColumnName string
	ConstraintName       string
}

func ReplaceColumnType(db *gorm.DB, table string, column string, newType string) error {
	var fks []ForeignKey
	if err := db.Raw(getForeignKeysSQL, table, column).Find(&fks).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Delete FKs
		for _, fk := range fks {
			//fmt.Printf("ALTER TABLE %s DROP CONSTRAINT %s;\n", fk.ForeignTableName, fk.ConstraintName)
			if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", fk.ForeignTableName, fk.ConstraintName)).Error; err != nil {
				return err
			}
		}

		// Change types
		//fmt.Printf("alter table %s alter column %s TYPE %s USING %s::%s;\n", table, column, newType, column, newType)
		if err := tx.Exec(fmt.Sprintf("alter table %s alter column %s TYPE %s USING %s::%s;", table, column, newType, column, newType)).Error; err != nil {
			return err
		}
		for _, fk := range fks {
			//fmt.Printf("alter table %s alter column %s TYPE %s USING %s::%s;\n", fk.ForeignTableName, fk.ForeignColumnName, newType, fk.ForeignColumnName, newType)
			if err := tx.Exec(fmt.Sprintf("alter table %s alter column %s TYPE %s USING %s::%s;", fk.ForeignTableName, fk.ForeignColumnName, newType, fk.ForeignColumnName, newType)).Error; err != nil {
				return err
			}
		}

		// Add FKs back
		for _, fk := range fks {
			//fmt.Printf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s);\n", fk.ForeignTableName, fk.ConstraintName, fk.ForeignColumnName, fk.ReferencedTableName, fk.ReferencedColumnName)
			if err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s);", fk.ForeignTableName, fk.ConstraintName, fk.ForeignColumnName, fk.ReferencedTableName, fk.ReferencedColumnName)).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// CleanInvalidUUIDs очищает невалидные UUID значения, устанавливая их в NULL
// Это предотвращает ошибки при конвертации text -> uuid
func CleanInvalidUUIDs(tx *gorm.DB, table string, column string) error {
	// Очищаем все значения которые не соответствуют формату UUID
	// Устанавливаем NULL для hex-кодированных, битых и любых невалидных значений
	sql := fmt.Sprintf(`
		UPDATE "%s"
		SET "%s" = NULL
		WHERE "%s" IS NOT NULL
		AND "%s"::text !~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
	`, table, column, column, column)

	result := tx.Exec(sql)
	if result.Error != nil {
		return fmt.Errorf("failed to clean invalid UUIDs in %s.%s: %w", table, column, result.Error)
	}

	if result.RowsAffected > 0 {
		slog.Warn("Cleaned invalid UUIDs", "table", table, "column", column, "rows", result.RowsAffected)
	}

	return nil
}

// CleanOrphanedForeignKeys очищает "осиротевшие" внешние ключи - записи, которые ссылаются на несуществующие записи в referenced таблице
func CleanOrphanedForeignKeys(tx *gorm.DB, table string, column string, referencedTable string, referencedColumn string) error {
	// Сначала проверяем, существует ли колонка в таблице
	checkColumnSQL := fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			AND table_name = '%s'
			AND column_name = '%s'
		)
	`, table, column)

	var columnExists bool
	if err := tx.Raw(checkColumnSQL).Scan(&columnExists).Error; err != nil {
		return fmt.Errorf("failed to check column existence %s.%s: %w", table, column, err)
	}

	// Если колонка не существует, просто пропускаем
	if !columnExists {
		return nil
	}

	// Устанавливаем NULL для всех записей, где внешний ключ указывает на несуществующую запись
	// Используем ::text для приведения типов на случай если referenced колонка уже UUID а текущая ещё text
	sql := fmt.Sprintf(`
		UPDATE "%s"
		SET "%s" = NULL
		WHERE "%s" IS NOT NULL
		AND NOT EXISTS (
			SELECT 1 FROM "%s" WHERE "%s"::text = "%s"."%s"::text
		)
	`, table, column, column, referencedTable, referencedColumn, table, column)

	result := tx.Exec(sql)
	if result.Error != nil {
		return fmt.Errorf("failed to clean orphaned foreign keys in %s.%s: %w", table, column, result.Error)
	}

	if result.RowsAffected > 0 {
		slog.Warn("Cleaned orphaned foreign keys", "table", table, "column", column, "referenced_table", referencedTable, "rows", result.RowsAffected)
	}

	return nil
}

// CleanAllOrphanedForeignKeys автоматически очищает все битые foreign keys из базы данных
func CleanAllOrphanedForeignKeys(tx *gorm.DB) error {
	const getAllForeignKeysSQL = `
SELECT
    kcu.table_name,
    kcu.column_name,
    ccu.table_name AS referenced_table_name,
    ccu.column_name AS referenced_column_name
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
    ON tc.constraint_name = kcu.constraint_name
    AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage AS ccu
    ON ccu.constraint_name = tc.constraint_name
    AND ccu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
    AND tc.table_schema = 'public'
ORDER BY kcu.table_name, kcu.column_name;`

	type FKInfo struct {
		TableName            string
		ColumnName           string
		ReferencedTableName  string
		ReferencedColumnName string
	}

	var foreignKeys []FKInfo
	if err := tx.Raw(getAllForeignKeysSQL).Find(&foreignKeys).Error; err != nil {
		return fmt.Errorf("failed to get foreign keys list: %w", err)
	}

	slog.Info("Found foreign keys to clean", "count", len(foreignKeys))

	for i, fk := range foreignKeys {
		if err := CleanOrphanedForeignKeys(tx, fk.TableName, fk.ColumnName, fk.ReferencedTableName, fk.ReferencedColumnName); err != nil {
			return fmt.Errorf("failed to clean orphaned foreign keys in %s.%s: %w", fk.TableName, fk.ColumnName, err)
		}

		// Прогресс логирование каждые 10 FK или на последнем
		if i%10 == 0 || i == len(foreignKeys)-1 {
			slog.Info("Cleaning FK progress", "completed", i+1, "total", len(foreignKeys))
		}
	}

	return nil
}

// DropAllForeignKeys удаляет все foreign key constraints из базы данных (без транзакции)
func DropAllForeignKeys(tx *gorm.DB) error {
	const getAllForeignKeysSQL = `
SELECT
    tc.table_name AS foreign_table_name,
    tc.constraint_name
FROM information_schema.table_constraints AS tc
WHERE tc.constraint_type = 'FOREIGN KEY'
    AND tc.table_schema = 'public';`

	type FKConstraint struct {
		ForeignTableName string
		ConstraintName   string
	}

	var constraints []FKConstraint
	if err := tx.Raw(getAllForeignKeysSQL).Find(&constraints).Error; err != nil {
		return err
	}

	slog.Info("Found foreign key constraints to drop", "count", len(constraints))

	// Увеличиваем lock_timeout и statement_timeout для длительных операций
	if err := tx.Exec("SET lock_timeout = '300s';").Error; err != nil {
		slog.Warn("Failed to set lock_timeout", "err", err)
	}
	if err := tx.Exec("SET statement_timeout = '600s';").Error; err != nil {
		slog.Warn("Failed to set statement_timeout", "err", err)
	}

	for i, fk := range constraints {
		// Экранируем имена через двойные кавычки для поддержки constraint с цифрами в начале
		sql := fmt.Sprintf("ALTER TABLE \"%s\" DROP CONSTRAINT IF EXISTS \"%s\";", fk.ForeignTableName, fk.ConstraintName)

		// Retry логика для deadlock и lock timeout
		const maxRetries = 5
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := tx.Exec(sql).Error; err != nil {
				// Проверяем на deadlock (40P01) или lock timeout (55P03)
				isDeadlock := strings.Contains(err.Error(), "40P01") || strings.Contains(err.Error(), "deadlock detected")
				isLockTimeout := strings.Contains(err.Error(), "55P03") || strings.Contains(err.Error(), "lock timeout")

				if (isDeadlock || isLockTimeout) && attempt < maxRetries {
					errType := "deadlock"
					if isLockTimeout {
						errType = "lock timeout"
					}
					slog.Warn("Retrying after "+errType,
						"constraint", fk.ConstraintName,
						"table", fk.ForeignTableName,
						"attempt", attempt,
						"maxRetries", maxRetries)
					// Увеличенная пауза перед retry
					time.Sleep(time.Duration(attempt*500) * time.Millisecond)
					lastErr = err
					continue
				}
				return fmt.Errorf("failed to drop FK constraint %s on table %s (attempt %d/%d): %w",
					fk.ConstraintName, fk.ForeignTableName, attempt, maxRetries, err)
			}
			// Успешно удалили
			if i%10 == 0 || i == len(constraints)-1 {
				slog.Info("Dropping FK constraints progress", "completed", i+1, "total", len(constraints))
			}
			break
		}
		if lastErr != nil {
			return fmt.Errorf("failed to drop FK constraint %s on table %s after %d retries: %w",
				fk.ConstraintName, fk.ForeignTableName, maxRetries, lastErr)
		}
	}
	return nil
}

// DropAllGeneratedColumns удаляет все generated columns из базы данных
// Generated columns препятствуют изменению типа referenced columns
func DropAllGeneratedColumns(tx *gorm.DB) error {
	const getAllGeneratedColumnsSQL = `
SELECT
    table_name,
    column_name
FROM information_schema.columns
WHERE table_schema = 'public'
    AND is_generated = 'ALWAYS';`

	type GeneratedColumn struct {
		TableName  string
		ColumnName string
	}

	var columns []GeneratedColumn
	if err := tx.Raw(getAllGeneratedColumnsSQL).Find(&columns).Error; err != nil {
		return err
	}

	slog.Info("Found generated columns to drop", "count", len(columns))

	// Увеличиваем lock_timeout и statement_timeout для длительных операций
	if err := tx.Exec("SET lock_timeout = '300s';").Error; err != nil {
		slog.Warn("Failed to set lock_timeout", "err", err)
	}
	if err := tx.Exec("SET statement_timeout = '600s';").Error; err != nil {
		slog.Warn("Failed to set statement_timeout", "err", err)
	}

	for i, col := range columns {
		sql := fmt.Sprintf("ALTER TABLE \"%s\" DROP COLUMN IF EXISTS \"%s\";", col.TableName, col.ColumnName)

		// Retry логика для deadlock и lock timeout
		const maxRetries = 5
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := tx.Exec(sql).Error; err != nil {
				// Проверяем на deadlock (40P01) или lock timeout (55P03)
				isDeadlock := strings.Contains(err.Error(), "40P01") || strings.Contains(err.Error(), "deadlock detected")
				isLockTimeout := strings.Contains(err.Error(), "55P03") || strings.Contains(err.Error(), "lock timeout")

				if (isDeadlock || isLockTimeout) && attempt < maxRetries {
					errType := "deadlock"
					if isLockTimeout {
						errType = "lock timeout"
					}
					slog.Warn("Retrying after "+errType,
						"column", col.ColumnName,
						"table", col.TableName,
						"attempt", attempt,
						"maxRetries", maxRetries)
					// Увеличенная пауза перед retry
					time.Sleep(time.Duration(attempt*500) * time.Millisecond)
					lastErr = err
					continue
				}
				return fmt.Errorf("failed to drop generated column %s.%s (attempt %d/%d): %w",
					col.TableName, col.ColumnName, attempt, maxRetries, err)
			}
			if i%5 == 0 || i == len(columns)-1 {
				slog.Info("Dropping generated columns progress", "completed", i+1, "total", len(columns))
			}
			break
		}
		if lastErr != nil {
			return fmt.Errorf("failed to drop generated column %s.%s after %d retries: %w",
				col.TableName, col.ColumnName, maxRetries, lastErr)
		}
	}
	return nil
}

// DropAllCheckConstraints удаляет все check constraints из базы данных (без транзакции)
func DropAllCheckConstraints(tx *gorm.DB) error {
	const getAllCheckConstraintsSQL = `
SELECT
    tc.table_name,
    tc.constraint_name
FROM information_schema.table_constraints AS tc
WHERE tc.constraint_type = 'CHECK'
    AND tc.table_schema = 'public';`

	type CheckConstraint struct {
		TableName      string
		ConstraintName string
	}

	var constraints []CheckConstraint
	if err := tx.Raw(getAllCheckConstraintsSQL).Find(&constraints).Error; err != nil {
		return err
	}

	slog.Info("Found check constraints to drop", "count", len(constraints))

	// Увеличиваем lock_timeout и statement_timeout для длительных операций
	if err := tx.Exec("SET lock_timeout = '300s';").Error; err != nil {
		slog.Warn("Failed to set lock_timeout", "err", err)
	}
	if err := tx.Exec("SET statement_timeout = '600s';").Error; err != nil {
		slog.Warn("Failed to set statement_timeout", "err", err)
	}

	for i, ck := range constraints {
		// Экранируем имена через двойные кавычки для поддержки constraint с цифрами в начале
		sql := fmt.Sprintf("ALTER TABLE \"%s\" DROP CONSTRAINT IF EXISTS \"%s\";", ck.TableName, ck.ConstraintName)

		// Retry логика для deadlock и lock timeout
		const maxRetries = 5
		var lastErr error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			if err := tx.Exec(sql).Error; err != nil {
				// Проверяем на deadlock (40P01) или lock timeout (55P03)
				isDeadlock := strings.Contains(err.Error(), "40P01") || strings.Contains(err.Error(), "deadlock detected")
				isLockTimeout := strings.Contains(err.Error(), "55P03") || strings.Contains(err.Error(), "lock timeout")

				if (isDeadlock || isLockTimeout) && attempt < maxRetries {
					errType := "deadlock"
					if isLockTimeout {
						errType = "lock timeout"
					}
					slog.Warn("Retrying after "+errType,
						"constraint", ck.ConstraintName,
						"table", ck.TableName,
						"attempt", attempt,
						"maxRetries", maxRetries)
					// Увеличенная пауза перед retry
					time.Sleep(time.Duration(attempt*500) * time.Millisecond)
					lastErr = err
					continue
				}
				return fmt.Errorf("failed to drop CHECK constraint %s on table %s (attempt %d/%d): %w",
					ck.ConstraintName, ck.TableName, attempt, maxRetries, err)
			}
			if i%10 == 0 || i == len(constraints)-1 {
				slog.Info("Dropping check constraints progress", "completed", i+1, "total", len(constraints))
			}
			break
		}
		if lastErr != nil {
			return fmt.Errorf("failed to drop CHECK constraint %s on table %s after %d retries: %w",
				ck.ConstraintName, ck.TableName, maxRetries, lastErr)
		}
	}
	return nil
}

// VacuumFull выполняет VACUUM FULL для всей базы данных
// VACUUM FULL пересоздает таблицы без фрагментации и возвращает место на диске
// ВАЖНО: VACUUM FULL блокирует таблицы, поэтому должен выполняться только во время миграции
func VacuumFull(db *gorm.DB) error {
	slog.Info("Starting VACUUM FULL")

	// VACUUM FULL нельзя выполнить внутри транзакции, поэтому используем db напрямую
	if err := db.Exec("VACUUM FULL;").Error; err != nil {
		return fmt.Errorf("failed to execute VACUUM FULL: %w", err)
	}

	slog.Info("VACUUM FULL completed successfully")
	return nil
}
