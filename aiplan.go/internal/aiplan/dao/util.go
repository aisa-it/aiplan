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
		ID:              u.String(),
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

func IsUserExist(db *gorm.DB, email string) (bool, error) {
	var exists bool
	if err := db.Select("count(*) > 0").Where("email = ?", email).Model(&User{}).Find(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
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

func ExtractProjectMemberIDs(members []ProjectMember) []string {
	ids := make([]string, len(members))
	for i, member := range members {
		ids[i] = member.MemberId
	}
	return ids
}

func PrepareFilterProjectsQuery(tx *gorm.DB, userID string, workspaceIDs, projectIDs []string) *gorm.DB {
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

func GetUserNeighbors(tx *gorm.DB, userID string, workspaceIDs, projectIDs []string) *gorm.DB {
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
	idsMap := make(map[string]struct{})
	for _, id := range ids {
		idsMap[id.(string)] = struct{}{}
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

			if field == "id" && table.TableName() == "sprint_activities" {
				selectFields = append(selectFields, "id::text")
				continue
			}

			if utils.CheckInSet(fieldSet, field) {
				selectFields = append(selectFields, field)
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

		for _, project := range projects {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "project_id"}, {Name: "member_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"role": types.AdminRole, "updated_at": time.Now(), "updated_by_id": actor.MemberId}),
			}).Create(&ProjectMember{
				ID:          GenID(),
				CreatedAt:   time.Now(),
				CreatedById: &actor.MemberId,
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

func GetUserPrivilegesOverIssue(issueId string, userId string, db *gorm.DB) (*UserPrivilegesOverIssue, error) {
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

func GetUserPrivilegesOverDoc(docId string, userId string, db *gorm.DB) (*UserPrivilegesOverDoc, error) {
	var priv UserPrivilegesOverDoc
	if err := db.Raw(`select
	wm.member_id as "user_id",
  d.id as "doc_id",
  wm.role as "workspace_role",
  d.created_by_id = ? as "is_author",
  de.id is not null or wm.role >= d.editor_role as "is_editor",
  dr.id is not null or wm.role >= d.reader_role as "is_reader",
  dw.id is not null as "is_watcher"
from docs d
left join doc_editors de on d.id = de.doc_id and de.editor_id = ?
left join doc_readers dr on d.id = dr.doc_id and dr.reader_id = ?
left join doc_watchers dw on d.id = dw.doc_id and dw.watcher_id = ?
left join workspace_members wm on d.workspace_id = wm.workspace_id and wm.member_id = ?
where d.id = ?`, userId, userId, userId, userId, userId, docId).First(&priv).Error; err != nil {
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
				ID:        GenID(),
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
