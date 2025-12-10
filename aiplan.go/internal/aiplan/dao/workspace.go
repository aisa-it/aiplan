// DAO (Data Access Object) для работы с данными рабочих пространств.  Предоставляет методы для получения, создания, обновления и удаления рабочих пространств, а также связанных с ними сущностей (членов, событий, избранного и т.д.).
//
// Основные возможности:
//   - Получение рабочих пространств по различным критериям (ID, slug, членство пользователя).
//   - Создание новых рабочих пространств.
//   - Обновление существующих рабочих пространств.
//   - Удаление рабочих пространств и связанных с ними данных.
//   - Работа с избранными рабочими пространствами.
//   - Отслеживание событий, происходящих в рабочих пространствах.
//   - Управление членством в рабочих пространствах.
//   - Получение и обновление информации о рабочих пространствах.
package dao

import (
	"fmt"
	"net/url"
	"time"

	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/lib/pq"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Рабочие пространства
type Workspace struct {
	// id uuid NOT NULL,
	ID uuid.UUID `gorm:"column:id;primaryKey;type:uuid" json:"id"`
	// created_at timestamp with time zone NOT NULL,
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone NOT NULL,
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// name character varying(255) COLLATE pg_catalog."default" NOT NULL,
	Name        string             `json:"name" validate:"workspaceName"`
	NameTokens  types.TsVector     `json:"-" gorm:"index:workspace_name_tokens,type:gin"`
	Description types.RedactorHTML `json:"description"`
	// logo character varying(200) COLLATE pg_catalog."default",
	Logo   *string       `json:"-"` // Legacy
	LogoId uuid.NullUUID `json:"logo"`
	// slug character varying(100) COLLATE pg_catalog."default" NOT NULL,
	Slug string `json:"slug" gorm:"uniqueIndex:,where:deleted_at is NULL" validate:"slug"`
	// created_by_id uuid,
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.UUID `json:"created_by_id" gorm:"type:uuid"`
	// owner_id uuid NOT NULL,
	OwnerId uuid.UUID `json:"owner_id" gorm:"type:uuid"`
	// updated_by_id uuid,
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById      uuid.NullUUID `json:"updated_by_id" gorm:"type:uuid" extensions:"x-nullable"`
	IntegrationToken string        `json:"-"`

	Hash []byte `json:"-" gorm:"->;-:migration"`

	URL *url.URL `json:"-" gorm:"-"`

	Owner     *User `json:"owner,omitempty" gorm:"foreignKey:OwnerId" extensions:"x-nullable"`
	CreatedBy *User `json:"created_by_detail" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User `json:"updated_by_detail" gorm:"foreignKey:UpdatedById;references:ID;" extensions:"x-nullable"`

	CurrentUserMembership *WorkspaceMember `json:"current_user_membership,omitempty" gorm:"-" extensions:"x-nullable"`
	LogoAsset             *FileAsset       `json:"logo_details" gorm:"foreignKey:LogoId" extensions:"x-nullable"`
	IsFavorite            bool             `json:"is_favorite" gorm:"-"`
}

func (w Workspace) GetId() uuid.UUID {
	return w.ID
}

func (w Workspace) GetWorkspaceId() uuid.UUID {
	return w.GetId()
}

func (w Workspace) GetString() string {
	return w.Name
}

func (w Workspace) GetEntityType() string {
	return actField.Workspace.String()
}

// WorkspaceExtendFields
// -migration
type WorkspaceExtendFields struct {
	NewWorkspace *Workspace `json:"-" gorm:"-" field:"workspace" extensions:"x-nullable"`
	OldWorkspace *Workspace `json:"-" gorm:"-" field:"workspace" extensions:"x-nullable"`
}

func (w *Workspace) ToLightDTO() *dto.WorkspaceLight {
	if w == nil {
		return nil
	}

	w.SetUrl()

	return &dto.WorkspaceLight{
		ID:      w.ID,
		Name:    w.Name,
		LogoId:  w.LogoId,
		Slug:    w.Slug,
		OwnerId: w.OwnerId,
		Url:     types.JsonURL{w.URL},
	}
}

func (w *Workspace) ToDTO() *dto.Workspace {
	if w == nil {
		return nil
	}

	return &dto.Workspace{
		WorkspaceLight:        *w.ToLightDTO(),
		CreatedAt:             w.CreatedAt,
		UpdatedAt:             w.UpdatedAt,
		Owner:                 w.Owner.ToLightDTO(),
		Description:           w.Description,
		CurrentUserMembership: w.CurrentUserMembership.ToDTO(),
		IsFavorite:            false,
	}
}

// WorkspaceWithCount
// -migration
type WorkspaceWithCount struct {
	Workspace
	TotalMembers  int  `json:"total_members" gorm:"->;-:migration"`
	TotalProjects int  `json:"total_projects" gorm:"->;-:migration"`
	IsFavorite    bool `json:"is_favorite" gorm:"->;-:migration"`

	NameHighlighted string `json:"name_highlighted,omitempty" gorm:"->;-:migration"`
}

func (WorkspaceWithCount) TableName() string {
	return "workspaces"
}

func (wc *WorkspaceWithCount) ToDTO() *dto.WorkspaceWithCount {
	if wc == nil {
		return nil
	}
	return &dto.WorkspaceWithCount{
		Workspace:       *wc.Workspace.ToDTO(),
		TotalMembers:    wc.TotalMembers,
		TotalProjects:   wc.TotalProjects,
		IsFavorite:      wc.IsFavorite,
		NameHighlighted: wc.NameHighlighted,
	}
}

func (workspace *Workspace) AfterFind(tx *gorm.DB) error {
	workspace.SetUrl()
	if !workspace.LogoId.Valid {
		workspace.LogoAsset = nil
	}

	if userID, ok := tx.Get("userID"); ok {
		if err := tx.Where("workspace_id = ?", workspace.ID).
			Where("member_id = ?", userID).
			First(&workspace.CurrentUserMembership).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				workspace.CurrentUserMembership = nil
			} else {
				return err
			}
		}
	}

	return nil
}

func (workspace *Workspace) SetUrl() {
	workspace.URL = Config.WebURL.JoinPath(workspace.Slug)
}

func (workspace *Workspace) BeforeDelete(tx *gorm.DB) error {

	if err := tx.
		Where("workspace_activity_id in (?)", tx.Select("id").Where("workspace_id = ?", workspace.ID).
			Model(&WorkspaceActivity{})).
		Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	tx.Where("workspace_id = ? ", workspace.ID).Delete(&WorkspaceActivity{})

	tx.Where("new_identifier = ? AND verb = ? AND field = ?", workspace.ID, "created", workspace.GetEntityType()).
		Model(&RootActivity{}).
		Update("new_identifier", nil)

	//delete asset
	if workspace.LogoId.Valid {
		if err := tx.Exec("UPDATE workspaces SET logo_id = NULL WHERE id = ?", workspace.ID).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", workspace.LogoId.UUID).
			Delete(&FileAsset{}).Error; err != nil {
			return err
		}
	}

	// delete members
	var members []WorkspaceMember
	if err := tx.Where("workspace_id = ?", workspace.ID).Find(&members).Error; err != nil {
		return err
	}
	for i, member := range members {
		member.Workspace = workspace
		members[i] = member
	}
	if err := tx.Omit(clause.Associations).Delete(&members).Error; err != nil {
		return err
	}

	// delete docs
	var docs []Doc
	if err := tx.Unscoped().
		Where("workspace_id = ?", workspace.ID).
		Where("parent_doc_id is NULL").
		Find(&docs).Error; err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	for _, doc := range docs {
		if err := tx.Unscoped().Delete(&doc).Error; err != nil {
			return err
		}
	}

	// delete projects
	var runningIds []string
	if err := tx.Model(&Project{}).Select("id").Unscoped().Where("id in (?)", deletingProjects.RunningDeletions()).Find(&runningIds).Error; err != nil {
		return err
	}
	// wait for running deletions
	deletingProjects.WaitAll(runningIds)

	var projects []Project
	if err := tx.Unscoped().
		Where("workspace_id = ?", workspace.ID).
		Find(&projects).Error; err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	for _, project := range projects {
		if err := tx.Unscoped().Delete(&project).Error; err != nil {
			return err
		}
	}

	var assets []FileAsset
	if err := tx.Where("workspace_id = ?", workspace.ID).Find(&assets).Error; err != nil {
		return err
	}
	for _, asset := range assets {
		if err := tx.Delete(&asset).Error; err != nil {
			return err
		}
	}

	// delete backups
	if err := tx.Where("workspace_id = ?", workspace.ID).Delete(&WorkspaceBackup{}).Error; err != nil {
		return err
	}

	// delete favorites
	if err := tx.Where("workspace_id = ?", workspace.ID).Delete(&WorkspaceFavorites{}).Error; err != nil {
		return err
	}

	// delete EntityActivity
	if err := tx.Unscoped().Where("workspace_id = ?", workspace.ID).Delete(&EntityActivity{}).Error; err != nil {
		return err
	}

	// delete UserNotifications
	if err := tx.Unscoped().Where("workspace_id = ?", workspace.ID).Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	// delete DeferredNotifications
	if err := tx.Unscoped().Where("workspace_id = ?", workspace.ID).Delete(&DeferredNotifications{}).Error; err != nil {
		return err
	}

	//delete forms answer
	if err := tx.Where("workspace_id = ?", workspace.ID).Delete(&FormAnswer{}).Error; err != nil {
		return err
	}

	//delete forms
	var forms []Form
	if err := tx.Where("workspace_id = ?", workspace.ID).Find(&forms).Error; err != nil {
		return err
	}

	for i := range forms {
		if err := tx.Delete(&forms[i]).Error; err != nil {
			return err
		}
	}

	//delete sprint
	var sprint []Sprint
	if err := tx.Where("workspace_id = ?", workspace.ID).Find(&sprint).Error; err != nil {
		return err
	}

	for i := range sprint {
		if err := tx.Delete(&sprint[i]).Error; err != nil {
			return err
		}
	}

	// remove from last workspaces
	return tx.Model(&User{}).Where("last_workspace_id = ?", workspace.ID).Update("last_workspace_id", nil).Error
}

func (w *Workspace) ChangeOwner(tx *gorm.DB, wm *WorkspaceMember) error {
	if wm.Role != types.AdminRole {
		if err := tx.Model(wm).Update("role", types.AdminRole).Error; err != nil {
			return fmt.Errorf("member role update")
		}
	}

	if err := tx.Model(w).Updates(Workspace{
		OwnerId: wm.MemberId,
	}).Error; err != nil {
		return fmt.Errorf("workspace owner update")
	}

	return nil
}

// Участник рабочего пространства
type WorkspaceMember struct {
	// id uuid NOT NULL,
	ID string `gorm:"column:id;primaryKey" json:"id"`
	// created_at timestamp with time zone NOT NULL,
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone NOT NULL,
	UpdatedAt time.Time `json:"updated_at"`
	// role smallint NOT NULL,
	Role int `json:"role"`
	// created_by_id uuid,
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.NullUUID `json:"created_by_id" gorm:"type:uuid" extensions:"x-nullable"`
	// member_id uuid NOT NULL,
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	MemberId uuid.UUID `json:"member_id" gorm:"type:uuid;index;uniqueIndex:workspace_members_idx,priority:2"`
	// updated_by_id uuid,
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById uuid.NullUUID `json:"updated_by_id" gorm:"type:uuid" extensions:"x-nullable"`
	// workspace_id uuid NOT NULL,
	WorkspaceId uuid.UUID `json:"workspace_id" gorm:"type:uuid;uniqueIndex:workspace_members_idx,priority:1"`

	// Признак возможности редактирования пользователя админом пространства. true только если пользователь состоит в одном пространстве.
	EditableByAdmin bool `json:"editable_by_admin" gorm:"-"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Member    *User      `json:"member" gorm:"foreignKey:MemberId" extensions:"x-nullable"`
	CreatedBy *User      `json:"created_by" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User      `json:"updated_by_detail" gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`

	NotificationSettingsApp         types.WorkspaceMemberNS `json:"notification_settings_app" gorm:"type:jsonb"`
	NotificationAuthorSettingsApp   types.WorkspaceMemberNS `json:"notification_author_settings_app" gorm:"type:jsonb"`
	NotificationSettingsTG          types.WorkspaceMemberNS `json:"notification_settings_tg" gorm:"type:jsonb"`
	NotificationAuthorSettingsTG    types.WorkspaceMemberNS `json:"notification_author_settings_tg" gorm:"type:jsonb"`
	NotificationSettingsEmail       types.WorkspaceMemberNS `json:"notification_settings_email" gorm:"type:jsonb"`
	NotificationAuthorSettingsEmail types.WorkspaceMemberNS `json:"notification_author_settings_email" gorm:"type:jsonb"`
}

func (wm WorkspaceMember) GetId() string {
	return wm.ID
}

func (wm WorkspaceMember) GetString() string {
	return wm.Member.GetString()
}

func (wm WorkspaceMember) GetEntityType() string {
	return actField.Member.String()
}

func (wm WorkspaceMember) GetWorkspaceId() uuid.UUID {
	return wm.WorkspaceId
}

func (wm *WorkspaceMember) ToLightDTO() *dto.WorkspaceMemberLight {
	if wm == nil {
		return nil
	}

	return &dto.WorkspaceMemberLight{
		ID:              wm.ID,
		Role:            wm.Role,
		EditableByAdmin: wm.EditableByAdmin,
		MemberId:        wm.MemberId,
		Member:          wm.Member.ToLightDTO(),
	}
}

func (wm *WorkspaceMember) ToDTO() *dto.WorkspaceMember {
	if wm == nil {
		return nil
	}

	return &dto.WorkspaceMember{
		WorkspaceMemberLight:            *wm.ToLightDTO(),
		NotificationSettingsApp:         wm.NotificationSettingsApp,
		NotificationAuthorSettingsApp:   wm.NotificationAuthorSettingsApp,
		NotificationSettingsTG:          wm.NotificationSettingsTG,
		NotificationAuthorSettingsTG:    wm.NotificationAuthorSettingsTG,
		NotificationSettingsEmail:       wm.NotificationSettingsEmail,
		NotificationAuthorSettingsEmail: wm.NotificationAuthorSettingsEmail,
	}
}

func (wm *WorkspaceMember) AfterCreate(tx *gorm.DB) error {
	if wm.Role == types.AdminRole {
		var projects []Project
		if err := tx.Where("workspace_id = ?", wm.WorkspaceId).Find(&projects).Error; err != nil {
			return err
		}
		if len(projects) == 0 {
			return nil
		}
		members := make([]ProjectMember, len(projects))
		for i, project := range projects {
			members[i] = ProjectMember{
				ID:             GenID(),
				ProjectId:      project.ID,
				Role:           types.AdminRole,
				WorkspaceAdmin: true,
				MemberId:       wm.MemberId,
				WorkspaceId:    project.WorkspaceId,
				ViewProps:      types.DefaultViewProps,
			}
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(members).Error
	}
	return nil
}

func (wm *WorkspaceMember) AfterFind(tx *gorm.DB) error {
	var editable bool
	if err := tx.Model(&WorkspaceMember{}).
		Select("NOT EXISTS(?)",
			tx.Model(&WorkspaceMember{}).
				Select("1").
				Where("member_id = ?", wm.MemberId).
				Where("id != ?", wm.ID),
		).
		Find(&editable).Error; err != nil {
		return err
	}
	wm.EditableByAdmin = editable
	return nil
}

func (wm *WorkspaceMember) AfterUpdate(tx *gorm.DB) error {
	if wm.Role != types.AdminRole {
		return tx.Where("member_id = ?", wm.MemberId).
			Where("workspace_id = ?", wm.WorkspaceId).
			Where("workspace_admin = true").
			Delete(&ProjectMember{}).Error
	} else {
		var projects []Project
		if err := tx.Where("workspace_id = ?", wm.WorkspaceId).Find(&projects).Error; err != nil {
			return err
		}
		members := make([]ProjectMember, len(projects))
		for i, project := range projects {
			members[i] = ProjectMember{
				ID:             GenID(),
				ProjectId:      project.ID,
				Role:           types.AdminRole,
				WorkspaceAdmin: true,
				MemberId:       wm.MemberId,
				WorkspaceId:    project.WorkspaceId,
				ViewProps:      types.DefaultViewProps,
			}
		}
		if len(members) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(members).Error
	}
}

func (wm *WorkspaceMember) BeforeDelete(tx *gorm.DB) error {

	// WorkspacetActivity update create to nil
	//tx.Where("workspace_id = ? AND new_identifier = ? AND verb = ? AND field = ?", wm.WorkspaceId, wm.MemberId, "created", wm.GetEntityType()).
	//  Model(&WorkspaceActivity{}).Update("new_identifier", nil)
	//ProjectActivity delete other activity
	//tx.Where("workspace_id = ? and verb <> ? and (new_identifier = ? or old_identifier = ?)", wm.WorkspaceId, "deleted", wm.MemberId, wm.MemberId).Delete(&WorkspaceActivity{})

	if err := tx.Exec("delete from project_favorites where user_id = ? and workspace_id = ?",
		wm.MemberId, wm.WorkspaceId).Error; err != nil {
		return err
	}

	// Delete from workspace_favorites when removing a user from a workspace
	if err := tx.Exec("delete from workspace_favorites where user_id = ? and workspace_id = ?",
		wm.MemberId, wm.WorkspaceId).Error; err != nil {
		return err
	}

	if err := tx.Exec("delete from issue_assignees where assignee_id = ? and workspace_id = ?",
		wm.MemberId, wm.WorkspaceId).Error; err != nil {
		return err
	}

	if err := tx.Exec("delete from issue_watchers where watcher_id = ? and workspace_id = ?",
		wm.MemberId, wm.WorkspaceId).Error; err != nil {
		return err
	}

	// Delete the DeferredNotifications also from all the projects
	if err := tx.Where("user_id = ?", wm.MemberId).
		Where("workspace_id = ?", wm.WorkspaceId).
		Delete(&DeferredNotifications{}).Error; err != nil {
		return err
	}

	// Delete the user also from all the projects
	var projectMember []ProjectMember
	if err := tx.Where("member_id = ?", wm.MemberId).
		Where("workspace_id = ?", wm.WorkspaceId).
		Find(&projectMember).Error; err != nil {
		return err
	}
	for _, member := range projectMember {
		if err := tx.Delete(&member).Error; err != nil {
			return err
		}
	}

	// Update last workspace id of deleted member
	{
		var workspacesMemberships []WorkspaceMember
		if err := tx.
			Where("member_id = ?", wm.MemberId).
			Not("workspace_id = ?", wm.Workspace.ID).
			Find(&workspacesMemberships).Error; err != nil {
			return err
		}

		if len(workspacesMemberships) == 0 {
			if err := tx.Model(&User{}).
				Where("id = ?", wm.MemberId).
				Update("last_workspace_id", nil).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&User{}).
				Where("id = ?", wm.MemberId).
				Update("last_workspace_id", workspacesMemberships[0].WorkspaceId).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// WorkspaceOwnerExtendFields
// -migration
type WorkspaceOwnerExtendFields struct {
	NewOwner *User `json:"-" gorm:"-" field:"owner" extensions:"x-nullable"`
	OldOwner *User `json:"-" gorm:"-" field:"owner" extensions:"x-nullable"`
}

// Избранные рабочие пространства
type WorkspaceFavorites struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	ID uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	// created_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	CreatedById uuid.NullUUID `json:"created_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace_id" gorm:"type:uuid;index;uniqueIndex:workspace_favorites_idx,priority:1"`
	// updated_by_id uuid IS_NULL:YES
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UpdatedById uuid.NullUUID `json:"updated_by_id,omitempty" gorm:"type:uuid" extensions:"x-nullable"`
	// user_id uuid IS_NULL:NO
	// Note: type:text используется потому что в существующей БД это поле имеет тип text, а не uuid
	UserId uuid.UUID `json:"user_id" gorm:"type:uuid;uniqueIndex:workspace_favorites_idx,priority:2"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	CreatedBy *User      `json:"created_by_detail" gorm:"foreignKey:CreatedById;references:ID" extensions:"x-nullable"`
	UpdatedBy *User      `json:"updated_by_detail" gorm:"foreignKey:UpdatedById;references:ID" extensions:"x-nullable"`
}

func (WorkspaceFavorites) TableName() string {
	return "workspace_favorites"
}

type WorkspaceEntityI interface {
	GetWorkspaceId() uuid.UUID
}

type WorkspaceActivity struct {
	Id        uuid.UUID `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time `json:"created_at" gorm:"index:workspace_activities_workspace_index,sort:desc,type:btree,priority:2;index:workspace_activities_actor_index,sort:desc,type:btree,priority:2;index:workspace_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	// field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId uuid.UUID `json:"workspace" gorm:"type:uuid;index:workspace_activities_workspace_index,priority:1"`
	// actor_id uuid IS_NULL:YES
	ActorId uuid.NullUUID `json:"actor,omitempty" gorm:"type:uuid;index:workspace_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`

	NewProject *Project `json:"-" gorm:"-" field:"project" extensions:"x-nullable"`

	//AffectedUser      *User  `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`
	UnionCustomFields string `json:"-" gorm:"-"`
	WorkspaceActivityExtendFields
	ActivitySender
}

func (wa WorkspaceActivity) GetCustomFields() string {
	return wa.UnionCustomFields
}

func (wa WorkspaceActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "new_value", "workspace_id", "actor_id", "new_identifier", "old_identifier", "telegram_msg_ids"}
}

func (WorkspaceActivity) GetEntity() string {
	return "workspace"
}

func (WorkspaceActivity) TableName() string { return "workspace_activities" }

func (wa WorkspaceActivity) SkipPreload() bool {
	if wa.Field == nil {
		return true
	}

	if wa.NewIdentifier == nil && wa.OldIdentifier == nil {
		return true
	}
	return false
}

func (wa WorkspaceActivity) GetField() string {
	return pointerToStr(wa.Field)
}

func (wa WorkspaceActivity) GetVerb() string {
	return wa.Verb
}

func (wa WorkspaceActivity) GetNewIdentifier() string {
	return pointerToStr(wa.NewIdentifier)
}

func (wa WorkspaceActivity) GetOldIdentifier() string {
	return pointerToStr(wa.OldIdentifier)
}

func (wa WorkspaceActivity) GetId() string {
	return wa.Id.String()
}

func (wa WorkspaceActivity) SetTgSender(id int64) {
	wa.ActivitySender.SenderTg = id
}

func (wa WorkspaceActivity) GetUrl() *string {
	if wa.Workspace.URL != nil {
		urlStr := wa.Workspace.URL.String()
		return &urlStr
	}
	return nil
}

func (activity *WorkspaceActivity) ToLightDTO() *dto.EntityActivityLight {
	if activity == nil {
		return nil
	}

	return &dto.EntityActivityLight{
		Id:         activity.Id,
		CreatedAt:  activity.CreatedAt,
		Verb:       activity.Verb,
		Field:      activity.Field,
		OldValue:   activity.OldValue,
		NewValue:   activity.NewValue,
		EntityType: "workspace",

		NewEntity: GetActionEntity(*activity, "New"),
		OldEntity: GetActionEntity(*activity, "Old"),

		EntityUrl: activity.GetUrl(),
	}
}

func (wa *WorkspaceActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(wa, tx)
}

// WorkspaceActivityExtendFields
// -migration
type WorkspaceActivityExtendFields struct {
	ProjectExtendFields
	DocExtendFields
	FormExtendFields
	SprintExtendFields
	EntityMemberExtendFields
	WorkspaceOwnerExtendFields
}

//func (wa WorkspaceActivity) SetAffectedUser(user *User) {
//	wa.AffectedUser = user
//}

func (wf *WorkspaceFavorites) ToDao() *dto.WorkspaceFavorites {
	if wf == nil {
		return nil
	}
	return &dto.WorkspaceFavorites{
		ID:          wf.ID,
		WorkspaceId: wf.WorkspaceId,
		Workspace:   wf.Workspace.ToDTO(),
	}
}

func GetWorkspace(db *gorm.DB, slug string, user string) (Workspace, error) {
	var ret Workspace
	err := db.Preload("Owner").Where("id in (?)", db.Table("workspace_members").Select("workspace_id").Where("member_id = ?", user)).
		First(&ret, "slug = ?", slug).Error
	return ret, err
}

func GetWorkspaceByID(db *gorm.DB, workspaceID string, userID uuid.UUID) (Workspace, error) {
	var workspace Workspace
	err := db.Preload("Owner").Where("id IN (?)", db.Table("workspace_members").Select("workspace_id").Where("member_id = ?", userID)).
		First(&workspace, "id = ?", workspaceID).Error

	return workspace, err
}
