// DAO (Data Access Object) - предоставляет методы для взаимодействия с базой данных проектов.
// Содержит функции для получения, создания, обновления и удаления проектов, а также для работы с связанными сущностями, такими как пользователи, рабочие пространства и оценки.
//
// Основные возможности:
//   - Получение списка проектов с возможностью фильтрации и сортировки.
//   - Получение детальной информации о проекте.
//   - Создание нового проекта.
//   - Обновление информации о проекте.
//   - Удаление проекта.
//   - Работа с оценками и оценщиками проекта.
//   - Получение списка пользователей, являющихся участниками проекта.
//   - Получение списка рабочих пространств, связанных с проектом.
//   - Работа с шаблонами задач.
package dao

import (
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/lib/pq"

	"sheff.online/aiplan/internal/aiplan/utils"

	"sheff.online/aiplan/internal/aiplan/types"

	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"sheff.online/aiplan/internal/aiplan/dto"
)

// ROLE_CHOICES = (
//     (20, "Admin"),
//     (15, "Member"),
//     (10, "Viewer"),
//     (5, "Guest"),
// )

// Running hard-deletions of projects
var deletingProjects *DeletionWatcher = NewDeletionWatcher()

// Проекты
type Project struct {
	ID        string         `gorm:"column:id;primaryKey;autoIncrement:true;unique" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	Name             string         `json:"name" validate:"projectName"`
	NameTokens       types.TsVector `json:"-" gorm:"index:project_name_tokens,type:gin"`
	Public           bool           `json:"public"`
	Identifier       string         `json:"identifier" gorm:"uniqueIndex:project_identifier_idx,priority:2,where:deleted_at is NULL" validate:"identifier"`
	CreatedById      string         `json:"created_by"`
	DefaultAssignees []string       `json:"default_assignees" gorm:"-"`
	DefaultWatchers  []string       `json:"default_watchers" gorm:"-"` // Срез строк для идентификаторов наблюдателей
	ProjectLeadId    string         `json:"project_lead"`
	UpdatedById      *string        `json:"updated_by" extensions:"x-nullable"`
	WorkspaceId      string         `json:"workspace" gorm:"uniqueIndex:project_identifier_idx,priority:1,where:deleted_at is NULL"`
	Emoji            int32          `json:"emoji,string" gorm:"default:127773"`
	LogoId           uuid.NullUUID  `json:"logo"`
	CoverImage       *string        `json:"cover_image" extensions:"x-nullable"`
	EstimateId       *string        `json:"estimate" extensions:"x-nullable"`
	RulesScript      *string        `json:"rules_script" extensions:"x-nullable"`

	Hash []byte `json:"-" gorm:"->;-:migration"`

	URL *url.URL `json:"-" gorm:"-"`

	Workspace               *Workspace      `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	ProjectLead             *User           `json:"project_lead_detail" gorm:"foreignKey:ProjectLeadId" extensions:"x-nullable"`
	DefaultAssigneesDetails []ProjectMember `json:"default_assignees_details" gorm:"foreignKey:ProjectId;associationForeignKey:ProjectId;where:IsDefaultAssignee=true"`
	DefaultWatchersDetails  []ProjectMember `json:"default_watchers_details" gorm:"foreignKey:ProjectId;associationForeignKey:ProjectId;where:IsDefaultWatcher=true"`

	TotalMembers          int            `json:"total_members,omitempty" gorm:"-"`
	IsFavorite            bool           `json:"is_favorite" gorm:"-"`
	CurrentUserMembership *ProjectMember `json:"current_user_membership,omitempty" gorm:"-" extensions:"x-nullable"`
}

// ProjectWithCount
// -migration
type ProjectWithCount struct {
	Project
	TotalMembers int `json:"total_members" gorm:"->;-:migration"`

	NameHighlighted string `json:"name_highlighted,omitempty" gorm:"->;-:migration"`
}

func (p Project) GetId() string {
	return p.ID
}

func (p Project) GetString() string {
	return p.Identifier
}

func (p Project) GetEntityType() string {
	return "project"
}

func (p Project) GetProjectId() string {
	return p.GetId()
}

func (p Project) GetWorkspaceId() string {
	return p.WorkspaceId
}

// ToLightDTO преобразует объект Project в его упрощенную версию (ProjectLight).  Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - project: объект Project, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ProjectLight: упрощенная версия объекта Project.
func (project *Project) ToLightDTO() *dto.ProjectLight {
	if project == nil {
		return nil
	}

	project.SetUrl()

	return &dto.ProjectLight{
		ID:                      project.ID,
		Name:                    project.Name,
		Public:                  project.Public,
		Identifier:              project.Identifier,
		ProjectLeadId:           project.ProjectLeadId,
		WorkspaceId:             project.WorkspaceId,
		Emoji:                   project.Emoji,
		LogoId:                  project.LogoId,
		CoverImage:              project.CoverImage,
		Url:                     types.JsonURL{project.URL},
		IsFavorite:              project.IsFavorite,
		TotalMembers:            project.TotalMembers,
		DefaultAssignees:        project.DefaultAssignees,
		DefaultWatchers:         project.DefaultWatchers,
		DefaultAssigneesDetails: utils.SliceToSlice(&project.DefaultAssigneesDetails, func(pm *ProjectMember) dto.ProjectMemberLight { return *pm.ToLightDTO() }),
		DefaultWatchersDetails:  utils.SliceToSlice(&project.DefaultWatchersDetails, func(pm *ProjectMember) dto.ProjectMemberLight { return *pm.ToLightDTO() }),
		CurrentUserMembership:   project.CurrentUserMembership.ToLightDTO(),
	}
}

// ToLightDTO преобразует объект Project в его упрощенную версию (ProjectLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Парамметры:
//   - project: объект Project, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ProjectLight: упрощенная версия объекта Project.
func (pwc *ProjectWithCount) ToLightDTO() *dto.ProjectLight {
	if pwc == nil {
		return nil
	}

	p := pwc.Project.ToLightDTO()
	p.TotalMembers = pwc.TotalMembers
	p.NameHighlighted = pwc.NameHighlighted

	return p
}

// ToDTO преобразует объект Project в его упрощенную версию (ProjectLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - project: объект Project, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ProjectLight: упрощенная версия объекта Project.
func (pwc *ProjectWithCount) ToDTO() *dto.Project {
	if pwc == nil {
		return nil
	}
	return &dto.Project{
		ProjectLight: *pwc.ToLightDTO(),
		CreatedAt:    pwc.CreatedAt,
		UpdatedAt:    pwc.UpdatedAt,
		ProjectLead:  pwc.ProjectLead.ToLightDTO(),
		RulesScript:  pwc.RulesScript,
		Workspace:    pwc.Workspace.ToLightDTO(),
	}
}

// ToDTO преобразует объект Project в его упрощенную версию (ProjectLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - project: объект Project, который нужно преобразовать.
//
// Возвращает:
//   - *dto.Project: упрощенная версия объекта Project.
func (project *Project) ToDTO() *dto.Project {
	if project == nil {
		return nil
	}

	projectDTO := dto.Project{
		ProjectLight: *project.ToLightDTO(),
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    project.UpdatedAt,
		ProjectLead:  project.ProjectLead.ToLightDTO(),
		Workspace:    project.Workspace.ToLightDTO(),
	}
	if project.CurrentUserMembership != nil && project.CurrentUserMembership.Role == types.AdminRole {
		// только для админов проекта
		projectDTO.RulesScript = project.RulesScript
	}

	return &projectDTO
}

// ChangeLead Обновляет lead проекта при обновлении роли участника проекта.
//
// Параметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//   - pm: объект ProjectMember, роль которого необходимо обновить.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обновлении роли.
func (p *Project) ChangeLead(tx *gorm.DB, pm *ProjectMember) error {
	if pm.Role != types.AdminRole {
		if err := tx.Model(pm).Update("role", types.AdminRole).Error; err != nil {
			return fmt.Errorf("member role update")
		}
	}

	if err := tx.Model(p).Updates(Project{
		ProjectLeadId: pm.Member.ID,
		ProjectLead:   pm.Member,
	}).Error; err != nil {
		return fmt.Errorf("project lead update")
	}
	return nil
}

// ProjectExtendFields
// -migration
type ProjectExtendFields struct {
	NewProject *Project `json:"-" gorm:"-" field:"project" extensions:"x-nullable"`
	OldProject *Project `json:"-" gorm:"-" field:"project" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы базы данных для данного типа модели.
func (ProjectWithCount) TableName() string {
	return "projects"
}

// BeforeCreate Обновляет lead проекта при создании нового проекта.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обновлении роли.
func (project *Project) BeforeCreate(tx *gorm.DB) error {
	if project.ProjectLeadId == "" {
		project.ProjectLeadId = project.CreatedById
	}
	return nil
}

// AfterCreate Обновляет lead проекта при создании нового проекта.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базe данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обноблении ролi.
func (project *Project) AfterCreate(tx *gorm.DB) error {
	var workspaceAdmins []WorkspaceMember
	if err := tx.Where("role = ?", types.AdminRole).Where("workspace_id = ?", project.WorkspaceId).Find(&workspaceAdmins).Error; err != nil {
		return err
	}

	for _, admin := range workspaceAdmins {
		member := ProjectMember{
			ID:             GenID(),
			ProjectId:      project.ID,
			Role:           types.AdminRole,
			WorkspaceAdmin: true,
			MemberId:       admin.MemberId,
			WorkspaceId:    project.WorkspaceId,
			ViewProps:      DefaultViewProps,
		}
		if admin.MemberId == project.CreatedById {
			member.WorkspaceAdmin = false
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
			return err
		}
	}
	return nil
}

// AfterFind Обновляет информацию о проекте после его успешного поиска в базе данных.  В частности, устанавливает статус, что пользователь является участником проекта, если он это делает.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обновлении статуса участника проекта.
func (project *Project) AfterFind(tx *gorm.DB) error {
	if userId, ok := tx.Get("userId"); ok {
		if err := tx.Select("count(*) > 0").
			Model(&ProjectFavorites{}).
			Where("user_id = ?", userId).
			Where("project_id = ?", project.ID).
			Find(&project.IsFavorite).Error; err != nil {
			return err
		}

		if err := tx.Where("member_id = ?", userId).
			Where("project_id = ?", project.ID).
			First(&project.CurrentUserMembership).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				project.CurrentUserMembership = nil
			} else {
				return err
			}
		}
	}

	if project.Workspace == nil {
		if err := tx.Unscoped().Where("id = ?", project.WorkspaceId).
			First(&project.Workspace).Error; err != nil {
			return err
		}
	}

	project.DefaultAssignees = ExtractProjectMemberIDs(project.DefaultAssigneesDetails)
	project.DefaultWatchers = ExtractProjectMemberIDs(project.DefaultWatchersDetails)

	for i, m := range project.DefaultAssigneesDetails {
		if err := tx.Where("id = ?", m.MemberId).First(&project.DefaultAssigneesDetails[i].Member).Error; err != nil {
			return err
		}
	}

	for i, m := range project.DefaultWatchersDetails {
		if err := tx.Where("id = ?", m.MemberId).First(&project.DefaultWatchersDetails[i].Member).Error; err != nil {
			return err
		}
	}

	project.SetUrl()

	return nil
}

func (project *Project) SetUrl() {
	raw := fmt.Sprintf("/%s/projects/%s/issues", project.WorkspaceId, project.ID)
	u, err := url.Parse(raw)
	if err != nil {
		slog.Error("Parse issue url", "url", raw, "err", err)
	}
	project.URL = Config.WebURL.ResolveReference(u)
}

// BeforeDelete Обновляет информацию о проекте перед его удалением.  Проверяет, является ли проект активным для текущего пользователя и удаляет его, если это так.  Также удаляет связанные данные, такие как оценки, рабочие пространства и участники проекта.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обновлении статуса участника проекта или удалении связанных данных.
func (project *Project) BeforeDelete(tx *gorm.DB) error {
	if deletingProjects.IsDeleting(project.ID) {
		return nil
	}

	deletingProjects.StartDeletion(project.ID)
	defer deletingProjects.FinishDeletion(project.ID)

	if err := tx.
		Where("project_activity_id in (?)", tx.Select("id").Where("project_id = ?", project.ID).
			Model(&ProjectActivity{})).
		Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	tx.Where("new_identifier = ? AND verb = ? AND field = ?", project.ID, "created", project.GetEntityType()).
		Model(&WorkspaceActivity{}).
		Update("new_identifier", nil)

	tx.Where("project_id = ? ", project.ID).Delete(&ProjectActivity{})

	tx.Where("new_identifier = ? ", project.ID).
		Model(&IssueActivity{}).
		Update("new_identifier", nil)

	tx.Where("old_identifier = ?", project.ID).
		Model(&IssueActivity{}).
		Update("old_identifier", nil)

	//delete asset
	if project.LogoId.Valid {
		if err := tx.Exec("UPDATE projects SET logo_id = NULL WHERE id = ?", project.ID).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", project.LogoId.UUID).
			Delete(&FileAsset{}).Error; err != nil {
			return err
		}
	}

	var issues []Issue
	if err := tx.Where("project_id = ?", project.ID).Unscoped().Find(&issues).Error; err != nil {
		return err
	}

	for _, issue := range issues {
		if err := tx.Unscoped().Set("permanentDelete", nil).Delete(&issue).Error; err != nil {
			return err
		}
	}

	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&IssueTemplate{}).Error; err != nil {
		return err
	}

	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&State{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&ProjectMember{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&Label{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&EstimatePoint{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&Estimate{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&EntityActivity{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&ProjectFavorites{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("project_id = ?", project.ID).Delete(&DeferredNotifications{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("project_id = ?", project.ID).Delete(&IssueProperty{}).Error
}

// TableName возвращает имя таблицы базы данных, соответствующей данному типу модели.  Используется для взаимодействия с базой данных через ORM.
func (Project) TableName() string {
	return "projects"
}

var defaultPageSize int = 25
var DefaultViewProps = types.ViewProps{
	ShowEmptyGroups: false,
	ShowSubIssues:   true,
	ShowOnlyActive:  false,
	AutoSave:        false,
	IssueView:       "list",
	Filters: types.ViewFilters{
		GroupBy:    "None",
		OrderBy:    "sequence_id",
		OrderDesc:  true,
		States:     []string{},
		Workspaces: []string{},
		Projects:   []string{},
	},
	Columns:         []string{},
	GroupTablesHide: make(map[string]bool),
	ActiveTab:       "all",
	PageSize:        &defaultPageSize,
}

// Участники проектов
type ProjectMember struct {
	// id uuid NOT NULL,
	ID string `gorm:"column:id;primaryKey;autoIncrement:true;unique" json:"id"`
	// created_at timestamp with time zone NOT NULL,
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone NOT NULL,
	UpdatedAt time.Time `json:"updated_at"`
	// role smallint NOT NULL,
	Role              int  `json:"role"`
	WorkspaceAdmin    bool `json:"workspace_admin"`
	IsDefaultAssignee bool `json:"is_default_assignee"`
	IsDefaultWatcher  bool `json:"is_default_watcher"`
	// created_by_id uuid,
	CreatedById *string `json:"created_by_id" extensions:"x-nullable"`
	// member_id uuid,
	MemberId string `json:"member_id" gorm:"index;uniqueIndex:project_members_idx,priority:2"`
	// project_id uuid NOT NULL,
	ProjectId string `json:"project_id" gorm:"uniqueIndex:project_members_idx,priority:1"`
	// updated_by_id uuid,
	UpdatedById *string `json:"updated_by_id" extensions:"x-nullable"`
	// workspace_id uuid NOT NULL,
	WorkspaceId                     string                `json:"workspace_id"`
	ViewProps                       types.ViewProps       `json:"view_props" gorm:"type:jsonb"`
	NotificationSettingsApp         types.ProjectMemberNS `json:"notification_settings_app" gorm:"type:jsonb"`
	NotificationAuthorSettingsApp   types.ProjectMemberNS `json:"notification_author_settings_app" gorm:"type:jsonb"`
	NotificationSettingsTG          types.ProjectMemberNS `json:"notification_settings_tg" gorm:"type:jsonb"`
	NotificationAuthorSettingsTG    types.ProjectMemberNS `json:"notification_author_settings_tg" gorm:"type:jsonb"`
	NotificationSettingsEmail       types.ProjectMemberNS `json:"notification_settings_email" gorm:"type:jsonb"`
	NotificationAuthorSettingsEmail types.ProjectMemberNS `json:"notification_author_settings_email" gorm:"type:jsonb"`
	Workspace                       *Workspace            `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Member                          *User                 `json:"member" gorm:"foreignKey:MemberId" extensions:"x-nullable"`
	Project                         *Project              `json:"project" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	CreatedBy                       *User                 `json:"created_by_detail" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`
}

// ProjectMemberExtendFields
// -migration
type ProjectMemberExtendFields struct {
	EntityMemberExtendFields

	NewProjectLead *User `json:"-" gorm:"-" field:"project_lead" extensions:"x-nullable"`
	OldProjectLead *User `json:"-" gorm:"-" field:"project_lead" extensions:"x-nullable"`

	NewDefaultAssignee *User `json:"-" gorm:"-" field:"default_assignees" extensions:"x-nullable"`
	OldDefaultAssignee *User `json:"-" gorm:"-" field:"default_assignees" extensions:"x-nullable"`

	NewDefaultWatcher *User `json:"-" gorm:"-" field:"default_watchers" extensions:"x-nullable"`
	OldDefaultWatcher *User `json:"-" gorm:"-" field:"default_watchers" extensions:"x-nullable"`
}

// EntityMemberExtendFields
// -migration
type EntityMemberExtendFields struct {
	NewRole *User `json:"-" gorm:"-" field:"role" extensions:"x-nullable"`
	OldRole *User `json:"-" gorm:"-" field:"role" extensions:"x-nullable"`

	NewMember *User `json:"-" gorm:"-" field:"member" extensions:"x-nullable"`
	OldMember *User `json:"-" gorm:"-" field:"member" extensions:"x-nullable"`
}

func (pm ProjectMember) GetId() string {
	return pm.ID
}

func (pm ProjectMember) GetString() string {
	return pm.Member.GetString()
}

func (pm ProjectMember) GetEntityType() string {
	return "member"
}

func (pm ProjectMember) GetProjectId() string {
	return pm.ProjectId
}

func (pm ProjectMember) GetWorkspaceId() string {
	return pm.WorkspaceId
}

// ToLightDTO преобразует объект Project в его упрощенную версию (ProjectLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - project: объект Project, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ProjectLight: упрощенная версия объекта Project.
func (pm *ProjectMember) ToLightDTO() *dto.ProjectMemberLight {
	if pm == nil {
		return nil
	}
	return &dto.ProjectMemberLight{
		ID:                pm.ID,
		Role:              pm.Role,
		WorkspaceAdmin:    pm.WorkspaceAdmin,
		IsDefaultAssignee: pm.IsDefaultAssignee,
		IsDefaultWatcher:  pm.IsDefaultWatcher,
		MemberId:          pm.MemberId,
		Member:            pm.Member.ToLightDTO(),
		ProjectId:         pm.ProjectId,
		Project:           pm.Project.ToLightDTO(),
	}
}

// ToDTO преобразует объект ProjectMember в его упрощенную версию (ProjectMemberLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - pm: объект ProjectMember, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ProjectMember: упрощенная версия объекта ProjectMember.
func (pm *ProjectMember) ToDTO() *dto.ProjectMember {
	if pm == nil {
		return nil
	}
	return &dto.ProjectMember{
		ProjectMemberLight:              *pm.ToLightDTO(),
		ViewProps:                       pm.ViewProps,
		NotificationSettingsApp:         pm.NotificationSettingsApp,
		NotificationAuthorSettingsApp:   pm.NotificationAuthorSettingsApp,
		NotificationSettingsTG:          pm.NotificationSettingsTG,
		NotificationAuthorSettingsTG:    pm.NotificationAuthorSettingsTG,
		NotificationSettingsEmail:       pm.NotificationSettingsEmail,
		NotificationAuthorSettingsEmail: pm.NotificationAuthorSettingsEmail,
	}
}

// BeforeDelete Обновляет информацию о проекте перед его удалением.  Проверяет, является ли проект активным для текущего пользователя и удаляет его, если это так. Также удаляет связанные данные, такие как оценки, рабочие пространства и участники проекта.
//
// Парамметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обновлении статуса участника проекта или удалении связанных данных.
func (pm *ProjectMember) BeforeDelete(tx *gorm.DB) error {
	if err := tx.Where("user_id = ?", pm.MemberId).
		Where("project_id = ?", pm.ProjectId).
		Delete(&DeferredNotifications{}).Error; err != nil {
		return err
	}

	return nil
}

// AfterUpdate Обновляет информацию о участнике проекта после обновления.  Проверяет, является ли пользователь участником проекта и удаляет его, если это так.
//
// Параметры:
//   - tx: экземпляр gorm.DB для выполнения запросов к базе данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при обновлении статуса участника проекта или удалении связанных данных.
func (pm *ProjectMember) AfterUpdate(tx *gorm.DB) error {
	if pm.Role == types.GuestRole {
		return tx.Unscoped().Where("project_id = ?", pm.ProjectId).
			Where("assignee_id = ?", pm.MemberId).
			Delete(&IssueAssignee{}).Error
	}
	return nil
}

type ProjectFavorites struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project_id" gorm:"index;uniqueIndex:project_favorites_idx,priority:1"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// user_id uuid IS_NULL:NO
	UserId string `json:"user_id" gorm:"uniqueIndex:project_favorites_idx,priority:2"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`

	Workspace *Workspace `json:"workspace" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы базы данных, соответствующей данному типу модели. Используется для взаимодействия с базой данных через ORM.
func (ProjectFavorites) TableName() string {
	return "project_favorites"
}

// ToDTO преобразует объект ProjectFavorites в его упрощенную версию (ProjectFavoritesLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Парамметры:
//   - e: объект ProjectFavorites, который нужно преобразовать.
//
// Возвращает:
//   - *dto.ProjectFavoites: упрощенная версия объекта ProjectFavoites.
func (pf *ProjectFavorites) ToDTO() *dto.ProjectFavorites {
	if pf == nil {
		return nil
	}
	return &dto.ProjectFavorites{
		ID:        pf.Id,
		ProjectId: pf.ProjectId,
		Project:   pf.Project.ToLightDTO(),
	}
}

// AllProjects возвращает список проектов, связанных с указанным пользователем и рабочим пространством.
//
// Парамметры:
//   - db: экземпляр базы данных GORM для выполнения запросов.
//   - user: идентификатор пользователя.
//
// Возвращает:
//   - *gorom.DB: экземпляр базы данных GORM для дальнейшей обработки результатов.
func AllProjects(db *gorm.DB, user string) *gorm.DB {
	project_members := db.Table("project_members").Select("count(*)").Where("project_members.project_id = projects.id")
	is_favorite := db.Table("project_favorites").Select("count(*) > 0").Where("project_favorites.project_id = projects.id").Where("user_id = ?", user)
	return db.Preload("Workspace").Preload("Workspace.Owner").
		Select("*,(?) as total_members, (?) as is_favorite", project_members, is_favorite)
}

// GetProject возвращает объект проекта по его идентификатору, рабочему пространству и идентификатору пользователя.  Функция выполняет фильтрацию по рабочему пространству и пользователю, а также предварительно загружает связанные данные, такие как рабочее пространство и lead проекта.
//
// Парамметры:
//   - db: экземпляр базы данных GORM для выполнения запросов.
//   - slug: slug рабочего пространства.
//   - user: идентификатор пользователя.
//   - prj: идентификатор проекта.
//
// Возвращает:
//   - Project: объект проекта, либо ошибка, если проект не найден или произошла ошибка при выполнении запроса.
func GetProject(db *gorm.DB, slug string, user string, prj string) (Project, error) {
	p := Project{}
	return p, db.Where("workspace_id in (?)", db.Model(&Workspace{}).Select("id").Where("slug = ?", slug)).
		Where("id in (?) or public = true", db.Table("project_members").Select("project_id").Where("member_id = ?", user)).
		Where("id = ?", prj).
		Preload("Workspace").Preload("Workspace.Owner").
		Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
		Preload("DefaultWatchersDetails", "is_default_watcher = ?", true).
		Preload("ProjectLead").
		First(&p).Error
}

// GetProjects возвращает список проектов, связанных с указанным рабочим пространством и пользователем.
// Функция выполняет фильтрацию по рабочему пространству и пользователю, а также предварителеную загрузку связанных данных.
//
// Парамметры:
//   - db: экземпляр базы данных GORM для выполнения запросов.
//   - slug: slug рабочего пространства.
//   - user: идентификатор пользователя.
//
// Возвращает:
//   - []Project: список проектов.
//   - error: ошибка, если произошла ошибка при выполнении запроса.
func GetProjects(db *gorm.DB, slug string, user string) ([]Project, error) {
	var ret []Project

	err := AllProjects(db, user).
		Where("workspace_id in (?)", db.Model(&Workspace{}).Select("id").Where("slug = ?", slug)).
		Where("id in (?) or network = 2", db.Table("project_members").Select("project_id").Where("member_id = ?", user)).
		Set("userId", user).
		Find(&ret).Error

	return ret, err
}

// GetAllUserProjects возвращает список проектов, связанных с указанным пользователем и рабочим пространством.
// Функция выполняет фильтрацию по рабочему пространству и пользователю, а также предварительную загрузку связанных данных.
//
// Парамметры:
//   - db: экземпляр базы данных GORM для выполнения запросов.
//   - user: объект пользователя.
//
// Возвращает:
//   - []Project: список проектов.
//   - error: ошибка, если произошла ошибка при выполнении запроса.
func GetAllUserProjects(db *gorm.DB, user User) ([]Project, error) {
	var ret []Project
	err := AllProjects(db, user.ID).
		Where("id in (?)", db.Table("project_members").Select("project_id").Where("member_id = ?", user.ID)).
		Find(&ret).Error

	return ret, err
}

type Estimate struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// name character varying IS_NULL:NO
	Name string `json:"name"`
	// description text IS_NULL:NO
	Description string `json:"description"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project_id"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`

	Workspace *Workspace      `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project        `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Points    []EstimatePoint `json:"points" gorm:"foreignKey:estimate_id"`
}

// TableName возвращает имя таблицы базы данных, соответствующей данному типу модели. Используется для взаимодействия с базой данных через ORM (GORM). Применяется для определения имени таблицы, в которой хранятся данные модели.
func (Estimate) TableName() string { return "estimates" }

// ToDTO преобразует объект Estimate в его упрощенную версию (EstimateLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - e: объект Estimate, который нужно преобразовать.
//
// Возвращает:
//   - *dto.Estimate: упрощенная версия объекта Estimate.
func (e *Estimate) ToDTO() *dto.Estimate {
	if e == nil {
		return nil
	}
	return &dto.Estimate{
		Id:          e.Id,
		Name:        e.Name,
		Description: e.Description,
		ProjectId:   e.ProjectId,
		Project:     e.Project.ToLightDTO(),
		Points:      utils.SliceToSlice(&e.Points, func(ep *EstimatePoint) dto.EstimatePoint { return *ep.ToDTO() }),
	}
}

type EstimatePoint struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// key integer IS_NULL:NO
	Key int `json:"key"`
	// description text IS_NULL:NO
	Description string `json:"description"`
	// value character varying IS_NULL:NO
	Value string `json:"value"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by,omitempty" extensions:"x-nullable"`
	// estimate_id uuid IS_NULL:NO
	EstimateId string `json:"estimate"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Estimate  *Estimate  `json:"estimate_detail" gorm:"foreignKey:EstimateId" extensions:"x-nullable"`
}

// ToDTO преобразует объект EstimatePoint в его упрощенную версию (EstimateLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - ep: объект EstimatePoint, который нужно преобразовать.
//
// Возвращает:
//   - *dto.EstimatePoint: упрощенная версия объекта EstimatePoint.
func (ep *EstimatePoint) ToDTO() *dto.EstimatePoint {
	if ep == nil {
		return nil
	}
	return &dto.EstimatePoint{
		Id:          ep.Id,
		Key:         ep.Key,
		Description: ep.Description,
		Value:       ep.Value,
		EstimateId:  ep.EstimateId,
		Estimate:    ep.Estimate.ToDTO(),
		ProjectId:   ep.ProjectId,
		Project:     ep.Project.ToLightDTO(),
	}
}

// TableName возвращает имя таблицы базы данных, соответствующей данному типу модели. Используется для взаимодействия с базой данных через ORM (GORM). Применяется для определения имени таблицы, в которой хранятся данные модели.
func (EstimatePoint) TableName() string { return "estimate_points" }

type ImportedProject struct {
	Id                uuid.UUID `gorm:"type:uuid;primaryKey"`
	Type              string    `gorm:"index"`
	ProjectKey        string    `gorm:"index"`
	StartAt           time.Time
	EndAt             time.Time
	TotalIssues       int
	TotalAttachments  int
	NewUsers          int
	TargetWorkspaceId string
	TargetProjectId   string `gorm:"index"`
	Successfully      bool

	TargetWorkspace Workspace `gorm:"foreignKey:TargetWorkspaceId;constraint:OnDelete:CASCADE"`
	TargetProject   Project   `gorm:"foreignKey:TargetProjectId;constraint:OnDelete:CASCADE"`
}

// Шильдик
// Шильдик
type Label struct {
	// id uuid NOT NULL,
	ID string `gorm:"column:id;primaryKey" json:"id"`
	// created_at timestamp with time zone NOT NULL,
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone NOT NULL,
	UpdatedAt time.Time `json:"updated_at"`
	// name character varying(255) COLLATE pg_catalog."default" NOT NULL,
	Name       string         `json:"name" gorm:"uniqueIndex:label_name_color_unique_idx,priority:2"`
	NameTokens types.TsVector `json:"-" gorm:"index:label_name_tokens,type:gin"`
	// description text COLLATE pg_catalog."default" NOT NULL,
	Description string `json:"description"`
	// created_by_id uuid,
	CreatedById *string `json:"created_by" extensions:"x-nullable"`
	// project_id uuid NOT NULL,
	ProjectId string `json:"project" gorm:"uniqueIndex:label_name_color_unique_idx,priority:1"`
	// updated_by_id uuid,
	UpdatedById *string `json:"updated_by" extensions:"x-nullable"`
	// workspace_id uuid NOT NULL,
	WorkspaceId string `json:"workspace"`
	// parent_id uuid,
	ParentId *string `json:"parent" extensions:"x-nullable"`
	// color character varying(255) COLLATE pg_catalog."default" NOT NULL,
	Color string `json:"color" gorm:"uniqueIndex:label_name_color_unique_idx,priority:3;default:#000000"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Parent    *Label     `json:"parent_detail,omitempty" gorm:"foreignKey:ParentId" extensions:"x-nullable"`
}

// GetId возвращает строку, представляющую собой идентификатор Issue.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: строка, представляющая собой идентификатор Issue.
func (l Label) GetId() string {
	return l.ID
}

// GetString возвращает строку из идентификатора Issue.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: строка, представляющая собой идентификатор Issue.
func (l Label) GetString() string {
	return l.Name
}

// GetEntityType возвращает строку, представляющую собой тип сущности.
//
// Парамметры:
//   - Нет
//
// Возвращает:
//   - string: тип сущности (issue).
func (l Label) GetEntityType() string {
	return "label"
}

func (l Label) GetWorkspaceId() string {
	return l.WorkspaceId
}

func (l Label) GetProjectId() string {
	return l.ProjectId
}

// ToLightDTO преобразует объект IssueComment в структуру dto.IssueCommentLight для упрощения передачи данных в клиентский код.
//
// Параметры:
//   - self: Объект IssueComment, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueCommentLight: Структура IssueCommentLight, содержащая преобразованные данные.
func (l *Label) ToLightDTO() *dto.LabelLight {
	if l == nil {
		return nil
	}
	return &dto.LabelLight{
		ID:          l.ID,
		Name:        l.Name,
		Description: l.Description,
		ProjectId:   l.ProjectId,
		Color:       l.Color,
	}
}

// LabelExtendFields
// -migration
type LabelExtendFields struct {
	NewLabel *Label `json:"-" gorm:"-" field:"label" extensions:"x-nullable"`
	OldLabel *Label `json:"-" gorm:"-" field:"label" extensions:"x-nullable"`
}

func (l *Label) BeforeDelete(tx *gorm.DB) error {
	// ProjectActivity update create to nil
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", l.ID, "created", l.GetEntityType()).
		Model(&ProjectActivity{}).Update("new_identifier", nil)
	// IssueActivity update activity to nil
	tx.Where("new_identifier = ? ", l.ID).
		Model(&IssueActivity{}).
		Update("new_identifier", nil)

	tx.Where("old_identifier = ?", l.ID).
		Model(&IssueActivity{}).
		Update("old_identifier", nil)

	//ProjectActivity delete other activity
	var activities []ProjectActivity
	if err := tx.Where("new_identifier = ? or old_identifier = ?", l.ID, l.ID).Find(&activities).Error; err != nil {
		return err
	}

	for _, activity := range activities {
		tx.Delete(&activity)
	}

	// Remove issue label
	if err := tx.Where("label_id = ?", l.ID).
		Where("project_id = ?", l.ProjectId).
		Delete(&IssueLabel{}).Error; err != nil {
		return err
	}
	return nil
}

// Состояния задач
type State struct {
	// id uuid NOT NULL,
	ID string `gorm:"column:id;primaryKey;autoIncrement:true;unique" json:"id"`
	// created_at timestamp with time zone NOT NULL,
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone NOT NULL,
	UpdatedAt time.Time `json:"updated_at"`
	// name character varying(255) COLLATE pg_catalog."default" NOT NULL,
	Name       string         `json:"name" gorm:"uniqueIndex:unique_state_idx,priority:3"`
	NameTokens types.TsVector `json:"-" gorm:"index:state_name_tokens,type:gin"`
	// description text COLLATE pg_catalog."default" NOT NULL,
	Description string `json:"description"`
	// color character varying(255) COLLATE pg_catalog."default" NOT NULL,
	Color string `json:"color" gorm:"uniqueIndex:unique_state_idx,priority:4"`
	// slug character varying(100) COLLATE pg_catalog."default" NOT NULL,
	Slug string `json:"slug"`
	// created_by_id uuid,
	CreatedById *string `json:"created_by" extensions:"x-nullable"`
	// project_id uuid NOT NULL,
	ProjectId string `json:"project" gorm:"uniqueIndex:unique_state_idx,priority:1"`
	// updated_by_id uuid,
	UpdatedById *string `json:"updated_by" extensions:"x-nullable"`
	// workspace_id uuid NOT NULL,
	WorkspaceId string `json:"workspace"`
	// sequence double precision NOT NULL,
	Sequence uint64 `json:"sequence"`
	// "group" character varying(20) COLLATE pg_catalog."default" NOT NULL,
	Group string `json:"group" gorm:"uniqueIndex:unique_state_idx,priority:2"`
	// "default" boolean NOT NULL,
	Default bool `json:"default"`

	Hash []byte `json:"-" gorm:"->;-:migration"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`

	SeqId *int `json:"group_seq_id,omitempty" gorm:"-"`
}

// Возвращает строковое представление объекта State. Используется для удобной отладки и вывода информации о состоянии задачи.
func (state State) String() string {
	return fmt.Sprintf("%s.%s (#%s)", state.Group, state.Name, state.Color)
}

// Преобразует объект State в его облегченную DTO-представление для упрощения передачи данных в интерфейс.
func (state *State) ToLightDTO() *dto.StateLight {
	if state == nil {
		return nil
	}
	return &dto.StateLight{
		ID:          state.ID,
		Name:        state.Name,
		Description: state.Description,
		Color:       state.Color,
		ProjectId:   state.ProjectId,
		WorkspaceId: state.WorkspaceId,
		Sequence:    state.Sequence,
		Group:       state.Group,
		Default:     state.Default,
	}
}

// StateExtendFields
// -migration
type StateExtendFields struct {
	NewState *State `json:"-" gorm:"-" field:"state" extensions:"x-nullable"`
	OldState *State `json:"-" gorm:"-" field:"state" extensions:"x-nullable"`
}

func (s State) GetId() string {
	return s.ID
}

func (s State) GetString() string {
	return s.Name
}

func (s State) GetEntityType() string {
	return "state"
}

func (s State) GetWorkspaceId() string {
	return s.WorkspaceId
}

func (s State) GetProjectId() string {
	return s.ProjectId
}

func (s *State) BeforeDelete(tx *gorm.DB) error {
	// ProjectActivity update create to nil
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", s.ID, "created", s.GetEntityType()).Model(&ProjectActivity{}).Update("new_identifier", nil)
	// IssueActivity update activity to nil

	tx.Where("new_identifier = ? ", s.ID).
		Model(&IssueActivity{}).
		Update("new_identifier", nil)

	tx.Where("old_identifier = ?", s.ID).
		Model(&IssueActivity{}).
		Update("old_identifier", nil)

	//ProjectActivity delete other activity
	var activities []ProjectActivity
	if err := tx.Where("new_identifier = ? or old_identifier = ?", s.ID, s.ID).Find(&activities).Error; err != nil {
		return err
	}

	for _, activity := range activities {
		tx.Delete(&activity)
	}
	return nil
}

type ProjectEntityI interface {
	WorkspaceEntityI
	GetProjectId() string
}

type ProjectActivity struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"index:project_activities_project_index,sort:desc,type:btree,priority:2;index:project_activities_actor_index,sort:desc,type:btree,priority:2;index:project_activities_mail_index,type:btree,where:notified = false"`
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
	// project_id uuid IS_NULL:YES
	ProjectId string `json:"project_id" gorm:"index:project_activities_issue_index,priority:1" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:project_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`

	UnionCustomFields string `json:"-" gorm:"-"`
	ProjectActivityExtendFields
	ActivitySender
}

func (pa ProjectActivity) GetCustomFields() string {
	return pa.UnionCustomFields
}

func (ProjectActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "new_value", "comment", "project_id", "workspace_id", "actor_id", "new_identifier", "old_identifier", "notified", "telegram_msg_ids"}
}

func (ProjectActivity) GetEntity() string {
	return "project"
}

// ProjectActivityExtendFields
// -migration
type ProjectActivityExtendFields struct {
	IssueExtendFields
	ProjectTemplateExtendFields
	ProjectMemberExtendFields
	StateExtendFields
	LabelExtendFields
}

func (ProjectActivity) TableName() string { return "project_activities" }

func (activity *ProjectActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(activity, tx)
}

func (activity *ProjectActivity) BeforeDelete(tx *gorm.DB) error {
	return tx.Where("project_activity_id = ?", activity.Id).Unscoped().Delete(&UserNotifications{}).Error
}

func (pa ProjectActivity) GetUrl() *string {
	if pa.Project.URL != nil {
		urlStr := pa.Project.URL.String()
		return &urlStr
	}
	return nil
}

func (pa ProjectActivity) SkipPreload() bool {
	if pa.Field == nil {
		return true
	}

	if pa.NewIdentifier == nil && pa.OldIdentifier == nil {
		return true
	}
	return false
}

func (pa ProjectActivity) GetField() string {
	return pointerToStr(pa.Field)
}

func (pa ProjectActivity) GetVerb() string {
	return pa.Verb
}

func (pa ProjectActivity) GetNewIdentifier() string {
	return pointerToStr(pa.NewIdentifier)
}

func (pa ProjectActivity) GetOldIdentifier() string {
	return pointerToStr(pa.OldIdentifier)

}

func (pa ProjectActivity) GetId() string {
	return pa.Id
}

func (activity *ProjectActivity) ToLightDTO() *dto.EntityActivityLight {
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
		EntityType: "project",

		NewEntity: GetActionEntity(*activity, "New"),
		OldEntity: GetActionEntity(*activity, "Old"),

		//TargetUser: activity.AffectedUser.ToLightDTO(),

		EntityUrl: activity.GetUrl(),
	}
}

//func (pa ProjectActivity) SetAffectedUser(user *User) {
//	pa.AffectedUser = user
//}

//func BuildProjectActivity[E Project | ProjectMember](entity E, t TemplateActivity) (ProjectActivity, error) {
//	var projectId, workspaceId string
//
//	switch e := any(entity).(type) {
//	case Project:
//		projectId = e.ID
//		workspaceId = e.WorkspaceId
//	case ProjectMember:
//		projectId = e.ProjectId
//		workspaceId = e.WorkspaceId
//		if t.NewIdentifier == nil {
//			t.OldIdentifier = &e.MemberId
//		} else if t.OldIdentifier == nil {
//			t.NewIdentifier = &e.MemberId
//		}
//	default:
//		return ProjectActivity{}, fmt.Errorf("unsupported entity: %T", e)
//	}
//
//	return ProjectActivity{
//		Id:            t.IdActivity,
//		Verb:          t.Verb,
//		Field:         t.Field,
//		OldValue:      t.OldValue,
//		NewValue:      t.NewValue,
//		Comment:       t.Comment,
//		ProjectId:     projectId,
//		WorkspaceId:   workspaceId,
//		ActorId:       &t.Actor.ID,
//		NewIdentifier: t.NewIdentifier,
//		OldIdentifier: t.OldIdentifier,
//		Notified:      false,
//	}, nil
//}

// IsProjectMember проверяет, является ли пользователь участником проекта.
//
// Парамметры:
//   - tx: экземпляр базы данных GORM для выполнения запросов.
//   - userId: идентификатор пользователя.
//   - projectId: идентификатор проекта.
//
// Возвращает:
//   - int: роль пользователя в проекте (например, 1 - участник, 2 - зритель), или 0, если пользователь не является участником проекта.
//   - bool: true, если пользователь является участником проекта, false в противном случае.
func IsProjectMember(tx *gorm.DB, userId string, projectId string) (int, bool) {
	var member ProjectMember
	if err := tx.
		Where("project_id = ?", projectId).
		Where("member_id = ?", userId).
		First(&member).Error; err != nil {
		return 0, false
	}
	return member.Role, true
}

// IsWorkspaceMember проверяет, является ли пользователь участником рабочего пространства.
//
// Параметры:
//   - tx: экземпляр базы данных GORM для выполнения запросов.
//   - userId: идентификатор пользователя.
//   - workspaceId: идентификатор рабочего пространства.
//
// Возвращает:
//   - bool: true, если пользователь является участником рабочего пространства, false в противном случае.
func IsWorkspaceMember(tx *gorm.DB, userId string, workspaceId string) (exist bool) {
	tx.Select("count(*) > 0").
		Model(&WorkspaceMember{}).
		Where("workspace_id = ?", workspaceId).
		Where("member_id = ?", userId).
		Find(&exist)
	return exist
}

// PreloadProjectMembersWithFilters загружает связанные данные (DefaultAssigneesDetails и DefaultWatchersDetails) для проекта, чтобы избежать нескольких запросов к базе данных.
// Это оптимизация производительности, которая позволяет загрузить все необходимые данные за один запрос.
//
// Парамметры:
//   - db: экземпляр GORM для выполнения запросов к базе данных.
//
// Возвращает:
//   - *gorom.DB: обновленный экземпляр GORM с предварительно загруженными данными.
func PreloadProjectMembersWithFilters(db *gorm.DB) *gorm.DB {
	return db.
		Preload("DefaultAssigneesDetails", "is_default_assignee = ?", true).
		Preload("DefaultWatchersDetails", "is_default_watcher = ?", true)
}

type IssueTemplate struct {
	Id          uuid.UUID `gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time
	CreatedById uuid.UUID
	UpdatedAt   time.Time
	UpdatedById uuid.UUID

	WorkspaceId uuid.UUID `gorm:"index:issue_template,priority:1"`
	ProjectId   uuid.UUID `gorm:"index:issue_template,priority:2;uniqueIndex:issue_template_name_idx,priority:1"`

	Name     string             `json:"name" gorm:"uniqueIndex:issue_template_name_idx,priority:2"`
	Template types.RedactorHTML `json:"template"`

	Workspace *Workspace
	Project   *Project
	CreatedBy *User
	UpdatedBy *User
}

func (it IssueTemplate) GetId() string {
	return it.Id.String()
}

func (it IssueTemplate) GetString() string {
	return it.Name
}

func (it IssueTemplate) GetEntityType() string {
	return "template"
}

func (it IssueTemplate) GetProjectId() string {
	return it.ProjectId.String()
}

func (it IssueTemplate) GetWorkspaceId() string {
	return it.WorkspaceId.String()
}

// TableName возвращает имя таблицы базы данных, соответствующей данному типу модели. Используется для взаимодействия с базой данных через ORM (GORM). Применяется для определения имени таблицы, в которой хранятся данные модели.
//
// Парамметры:
//   - none
//
// Возвращает:
//   - string: имя таблицы
func (IssueTemplate) TableName() string {
	return "issue_templates"
}

// ToLightDTO преобразует объект IssueTemplate в его упрощенную версию (IssueTemplateLight). Используется для возврата только необходимых данных, без необходимости загрузки всех полей.
//
// Параметры:
//   - it: объект IssueTemplate, который нужно преобразовать.
//
// Возвращает:
//   - *dto.IssueTemplate: упрощенная версия объекта IssueTemplate.
func (it *IssueTemplate) ToLightDTO() *dto.IssueTemplateLight {
	if it == nil {
		return nil
	}
	return &dto.IssueTemplateLight{
		Name:     it.Name,
		Template: it.Template,
	}
}

func (it *IssueTemplate) ToDTO() *dto.IssueTemplate {
	if it == nil {
		return nil
	}
	ttt := *it.ToLightDTO()
	return &dto.IssueTemplate{
		IssueTemplateLight: ttt,
		Id:                 it.Id,
		CreatedAt:          it.CreatedAt,
		CreatedById:        it.CreatedById,
		UpdatedAt:          it.UpdatedAt,
		UpdatedById:        it.UpdatedById,
		WorkspaceId:        it.WorkspaceId,
		ProjectId:          it.ProjectId,
	}
}

func (it *IssueTemplate) BeforeDelete(tx *gorm.DB) error {
	// ProjectActivity update create to nil
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", it.Id, "created", it.GetEntityType()).
		Model(&ProjectActivity{}).Update("new_identifier", nil)

	if err := tx.
		Where("project_activity_id in (?)", tx.Select("id").
			Where("project_id = ?", it.ProjectId).
			Where("new_identifier = ? or old_identifier = ?", it.Id, it.Id).
			Model(&ProjectActivity{})).
		Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	//ProjectActivity delete other activity
	if err := tx.Where("new_identifier = ? or old_identifier = ?", it.Id, it.Id).Delete(&ProjectActivity{}).Error; err != nil {
		return err
	}

	return nil
}

// ProjectTemplateExtendFields
// -migration
type ProjectTemplateExtendFields struct {
	NewIssueTemplate *IssueTemplate `json:"-" gorm:"-" field:"template" extensions:"x-nullable"`
	OldIssueTemplate *IssueTemplate `json:"-" gorm:"-" field:"template" extensions:"x-nullable"`
}
