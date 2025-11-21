// Пакет dao содержит методы для взаимодействия с базой данных, включая создание, чтение, обновление и удаление сущностей, связанных с задачами (issues). Он предоставляет абстракции для работы с данными, обеспечивая удобный доступ к ним и скрывая детали реализации базы данных.
//
// Основные возможности:
//   - Создание новых задач.
//   - Получение задач по различным критериям.
//   - Обновление существующих задач.
//   - Удаление задач.
//   - Работа с связанными сущностями (комментарии, метки, пользователи и т.д.).
//   - Выполнение сложных запросов и фильтраций данных.
//   - Обработка ошибок и исключений при работе с базой данных.
package dao

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dto"
	policy "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/redactor-policy"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Задачи
type Issue struct {
	ID        uuid.UUID      `gorm:"column:id;primaryKey;type:text" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// name character varying(255) COLLATE pg_catalog."default" NOT NULL,
	Name string `json:"name"`
	// priority character varying(30) COLLATE pg_catalog."default",
	Priority *string `json:"priority" extensions:"x-nullable"`

	StartDate   *types.TargetDateTimeZ `json:"start_date" extensions:"x-nullable"`
	TargetDate  *types.TargetDateTimeZ `json:"target_date" extensions:"x-nullable"`
	CompletedAt *types.TargetDateTimeZ `json:"completed_at" extensions:"x-nullable"`

	SequenceId int `json:"sequence_id" gorm:"default:1;index:,where:deleted_at is not null"`
	// created_by_id uuid,
	CreatedById string `json:"created_by"`
	// parent_id uuid,
	ParentId uuid.NullUUID `json:"parent" gorm:"type:text;index;index:issue_sort_order_index,priority:1"`
	// project_id uuid NOT NULL,
	ProjectId string `json:"project" gorm:"index:,type:hash,where:deleted_at is not null"`
	// state_id uuid,
	StateId *string `json:"state" extensions:"x-nullable"`
	// updated_by_id uuid,
	UpdatedById *string `json:"updated_by" extensions:"x-nullable"`
	// workspace_id uuid NOT NULL,
	WorkspaceId string `json:"workspace" gorm:"index:,type:hash,where:deleted_at is not null"`

	DescriptionHtml     string  `json:"description_html" gorm:"default:<p></p>"`
	DescriptionStripped *string `json:"description_stripped" extensions:"x-nullable"`
	DescriptionType     int     `json:"description_type" gorm:"default:0"`

	Tokens types.TsVector `json:"-" gorm:"index:tokens_gin,type:gin,where:deleted_at is not null;->:false"`

	// Sort order for sub issues list
	SortOrder int `json:"sort_order" gorm:"type:smallint;index:issue_sort_order_index,priority:2"`

	EstimatePoint int      `json:"estimate_point"`
	URL           *url.URL `json:"-" gorm:"-" extensions:"x-nullable"`
	ShortURL      *url.URL `json:"-" gorm:"-" extensions:"x-nullable"`

	Draft  bool `json:"draft"`
	Pinned bool `gorm:"index"`

	Parent    *Issue       `json:"parent_detail" gorm:"foreignKey:ParentId" extensions:"x-nullable"`
	Workspace *Workspace   `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	State     *State       `json:"state_detail" gorm:"foreignKey:StateId" extensions:"x-nullable"`
	Project   *Project     `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Assignees *[]User      `json:"assignee_details,omitempty" gorm:"many2many:issue_assignees;foreignKey:id;joinForeignKey:issue_id;References:id;joinReferences:assignee_id;- :migration" extensions:"x-nullable"`
	Watchers  *[]User      `json:"watcher_details,omitempty" gorm:"many2many:issue_watchers;foreignKey:id;joinForeignKey:issue_id;References:id;joinReferences:watcher_id;- :migration" extensions:"x-nullable"`
	Labels    *[]Label     `json:"label_details" gorm:"many2many:issue_labels;foreignKey:id;joinForeignKey:issue_id;References:id;joinReferences:label_id;- :migration" extensions:"x-nullable"`
	Sprints   *[]Sprint    `json:"sprints,omitempty" gorm:"many2many:sprint_issues;joinForeignKey:IssueId;joinReferences:SprintId"`
	Links     *[]IssueLink `json:"issue_link" gorm:"foreignKey:issue_id" extensions:"x-nullable"`
	Author    *User        `json:"author_detail" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`

	InlineAttachments []FileAsset `json:"issue_inline_attachments" gorm:"foreignKey:IssueId"`

	AssigneeIDs     []string    `json:"assignees" gorm:"-"`
	WatcherIDs      []string    `json:"watchers" gorm:"-"`
	LabelIDs        []string    `json:"labels" gorm:"-"`
	LinkedIssuesIDs []uuid.UUID `json:"linked_issues_ids" gorm:"-"`

	BlockerIssuesIDs []IssueBlocker `json:"blocker_issues" gorm:"-"`
	BlockedIssuesIDs []IssueBlocker `json:"blocked_issues" gorm:"-"`

	FullLoad      bool               `json:"-" gorm:"-"` // Загрузка SubIssuesCount, LinkCount, AttachmentCount, BlockerIssuesIDs и BlockedIssuesIDs полей отдельными запросами
	IssueProgress types.IssueProcess `json:"-" gorm:"-"`
}

// SubIssueExtendFields
// -migration
type SubIssueExtendFields struct {
	NewSubIssue    *Issue `json:"-" gorm:"-" field:"sub_issue" extensions:"x-nullable"`
	OldSubIssue    *Issue `json:"-" gorm:"-" field:"sub_issue" extensions:"x-nullable"`
	NewParentIssue *Issue `json:"-" gorm:"-" field:"parent" extensions:"x-nullable"`
	OldParentIssue *Issue `json:"-" gorm:"-" field:"parent" extensions:"x-nullable"`
}

// IssueExtendFields
// -migration
type IssueExtendFields struct {
	NewIssue *Issue `json:"-" gorm:"-" field:"issue::project" extensions:"x-nullable"`
	OldIssue *Issue `json:"-" gorm:"-" field:"issue::project" extensions:"x-nullable"`
}

// BlockIssueExtendFields
// -migration
type BlockIssueExtendFields struct {
	NewBlockIssue *Issue `json:"-" gorm:"-" field:"blocks" extensions:"x-nullable"`
	OldBlockIssue *Issue `json:"-" gorm:"-" field:"blocks" extensions:"x-nullable"`
}

// BlockingIssueExtendFields
// -migration
type BlockingIssueExtendFields struct {
	NewBlockingIssue *Issue `json:"-" gorm:"-" field:"blocking" extensions:"x-nullable"`
	OldBlockingIssue *Issue `json:"-" gorm:"-" field:"blocking" extensions:"x-nullable"`
}

// GetId возвращает строку, представляющую собой идентификатор сущности Issue.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: строка, представляющая собой идентификатор Issue.
func (i Issue) GetId() string {
	return i.ID.String()
}

// GetString возвращает строку из идентификатора Issue.
//
// Парамметры:
//   - self: Issue - экземпляр Issue, из которого извлекается идентификатор.
//
// Возвращает:
//   - string: строка, представляющая собой идентификатор Issue.
func (i Issue) GetString() string {
	return i.String()
}

// GetEntityType возвращает строку, представляющую собой тип сущности.
//
// Параметры:
//   - self: экземпляр Issue.
//
// Возвращает:
//   - string: строка, представляющая тип сущности (issue). Определяет, к какому типу относится сущность.
func (i Issue) GetEntityType() string {
	return "issue"
}

func (i Issue) GetWorkspaceId() string {
	return i.WorkspaceId
}

func (i Issue) GetProjectId() string {
	return i.ProjectId
}

func (i Issue) GetIssueId() string {
	return i.GetId()
}

// ToSeaarchLightDTO преобразует Issue в формат, оптимизированный для поиска.  Он извлекает необходимые поля и преобразует их в структуру dto.SearchLightweightResponse для более эффективного поиска по базе данных.
//
// Параметры:
//   - self: экземпляр Issue, который нужно преобразовать.
//
// Возвращает:
//   - dto.SearchLightweightResponse: структура, содержащая преобразованные данные Issue для поиска.
func (i IssueWithCount) ToSearchLightDTO() dto.SearchLightweightResponse {
	ii := dto.SearchLightweightResponse{
		ID:          i.ID,
		WorkspaceId: i.WorkspaceId,
		Workspace:   i.Workspace.ToLightDTO(),
		ProjectId:   i.ProjectId,
		Project:     i.Project.ToLightDTO(),
		SequenceId:  i.SequenceId,
		Name:        i.Name,
		Priority:    i.Priority,

		StartDate:   i.StartDate,
		TargetDate:  i.TargetDate,
		CompletedAt: i.CompletedAt,

		CreatedAt:       i.CreatedAt,
		UpdatedAt:       i.UpdatedAt,
		Author:          i.Author.ToLightDTO(),
		State:           i.State.ToLightDTO(),
		NameHighlighted: i.NameHighlighted,
		DescHighlighted: i.DescHighlighted,
	}

	if i.Assignees != nil {
		for _, assign := range *i.Assignees {
			ii.Assignees = append(ii.Assignees, *assign.ToLightDTO())
		}
	}

	if i.Watchers != nil {
		for _, watch := range *i.Watchers {
			ii.Watchers = append(ii.Watchers, *watch.ToLightDTO())
		}
	}

	if i.Labels != nil {
		for _, label := range *i.Labels {
			ii.Labels = append(ii.Labels, *label.ToLightDTO())
		}
	}

	return ii
}

// ToLightDTO преобразует Issue в структуру dto.IssueLight для упрощения передачи данных в клиентский код.  Функция принимает экземпляр Issue и возвращает его представление в формате dto.IssueLight.
//
// Парамметры:
//   - self: экземпляр Issue, который нужно преобразовать.
//
// Возвращает:
//   - *dto.IssueLight: структура, содержащая данные Issue в формате dto.IssueLight.
func (i *Issue) ToLightDTO() *dto.IssueLight {
	if i == nil {
		return nil
	}
	i.SetUrl()
	return &dto.IssueLight{
		Id:         i.ID.String(),
		Name:       i.Name,
		SequenceId: i.SequenceId,
		Url:        types.JsonURL{i.URL},
		ShortUrl:   types.JsonURL{i.ShortURL},
		StateId:    i.StateId,
		State:      i.State.ToLightDTO(),
		Priority:   i.Priority,
	}
}

// ToDTO преобразует Issue в структуру dto.Issue для удобства использования в API.
//
// Параметры:
//   - self: экземпляр Issue, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.Issue: структура, содержащая преобразованные данные Issue.
func (i *Issue) ToDTO() *dto.Issue {
	if i == nil {
		return nil
	}

	var parent *string
	if i.ParentId.Valid {
		parentId := i.ParentId.UUID.String()
		parent = &parentId
	}

	return &dto.Issue{
		IssueLight:          *i.ToLightDTO(),
		SequenceId:          i.SequenceId,
		CreatedAt:           i.CreatedAt,
		UpdatedAt:           i.UpdatedAt,
		Priority:            i.Priority,
		StartDate:           i.StartDate,
		TargetDate:          i.TargetDate,
		CompletedAt:         i.CompletedAt,
		ProjectId:           i.ProjectId,
		WorkspaceId:         i.WorkspaceId,
		UpdatedById:         i.UpdatedById,
		DescriptionHtml:     i.DescriptionHtml,
		DescriptionStripped: i.DescriptionStripped,
		DescriptionType:     i.DescriptionType,
		EstimatePoint:       i.EstimatePoint,
		Draft:               i.Draft,
		ParentId:            parent,
		Parent:              i.Parent.ToLightDTO(),
		Workspace:           i.Workspace.ToLightDTO(),
		Project:             i.Project.ToLightDTO(),
		Assignees:           utils.SliceToSlice(i.Assignees, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
		Watchers:            utils.SliceToSlice(i.Watchers, func(u *User) dto.UserLight { return *u.ToLightDTO() }),
		Labels:              utils.SliceToSlice(i.Labels, func(l *Label) dto.LabelLight { return *l.ToLightDTO() }),
		Links:               utils.SliceToSlice(i.Links, func(il *IssueLink) dto.IssueLinkLight { return *il.ToLightDTO() }),
		Author:              i.Author.ToLightDTO(),
		InlineAttachments:   utils.SliceToSlice(&i.InlineAttachments, func(fa *FileAsset) dto.FileAsset { return *fa.ToDTO() }),
		BlockerIssuesIDs:    utils.SliceToSlice(&i.BlockerIssuesIDs, func(ib *IssueBlocker) dto.IssueBlockerLight { return *ib.ToLightDTO() }),
		BlockedIssuesIDs:    utils.SliceToSlice(&i.BlockedIssuesIDs, func(ib *IssueBlocker) dto.IssueBlockerLight { return *ib.ToLightDTO() }),
		Sprints:             utils.SliceToSlice(i.Sprints, func(t *Sprint) dto.SprintLight { return *t.ToLightDTO() }),
	}
}

// BeforeSave - проверяет, какие поля можно обновлять в Issue.  Возвращает список разрешенных полей для обновления.
//
// Парамметры:
//   - self: экземпляр Issue, для которого проверяются права на обновление.
//
// Возвращает:
//   - []string: список строк, представляющих имена полей, которые можно обновлять.
func (Issue) FieldsAllowedForUpdate() []string {
	return []string{"name", "priority", "target_date", "start_date", "completed_at", "parent_id", "state_id", "description_html", "description_stripped", "description_type", "completed_at", "estimate_point", "updated_at", "updated_by_id", "draft", "sort_order"}
}

// BeforeSave - проверяет, какие поля можно обновлять в Issue. Возвращает список разрешенных полей для обновления.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - []string: список строк, представляющих имена полей, которые можно обновлять.
func (Issue) FieldsAllowedForAllUpdate() []string {
	return []string{"state_id", "completed_at", "updated_at", "updated_by_id"}
}

// FullTextSearch выполняет полнотекстовый поиск по Issue. Он принимает объект базы данных (tx) и поисковый запрос (search_query) в качестве параметров. Функция возвращает объект базы данных (tx) для дальнейшей обработки или выполнения запроса.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения запросов.
//   - search_query: поисковый запрос, который будет использоваться для поиска по Issue.
//
// Возвращает:
//   - *gorom.DB: объект базы данных GORM, который можно использовать для выполнения дальнейших операций.
func (Issue) FullTextSearch(tx *gorm.DB, search_query string) *gorm.DB {
	return tx.Or("issues.tokens @@ plainto_tsquery('simple', ?)", search_query).
		Or("issues.tokens @@ plainto_tsquery('russian', ?)", search_query).
		Or("issues.tokens @@ plainto_tsquery('english', ?)", search_query).
		Or("issues.tokens @@ to_tsquery('simple', ?)", SplitTSQuery(search_query)).
		Or("issues.tokens @@ to_tsquery('russian', ?)", SplitTSQuery(search_query)).
		Or("issues.tokens @@ to_tsquery('english', ?)", SplitTSQuery(search_query)).
		Or("issues.sequence_id::text like ?", strings.TrimSpace(search_query)+"%").              // Issue sequence search
		Or("CONCAT(p.identifier, '-', issues.sequence_id) = ?", strings.TrimSpace(search_query)) // Full issue num ISS-1
}

// IssueWithCount - вспомогательная структура задачи, для вытягивания счетчиков из запроса списка задач(без доп запросов)
// -migration
type IssueWithCount struct {
	Issue
	SubIssuesCount    int64 `json:"sub_issues_count" gorm:"->;-:migration"`
	LinkCount         int64 `json:"link_count" gorm:"->;-:migration"`
	AttachmentCount   int64 `json:"attachment_count" gorm:"->;-:migration"`
	LinkedIssuesCount int64 `json:"linked_issues_count" gorm:"->;-:migration"`
	CommentsCount     int64 `json:"comments_count" gorm:"->;-:migration"`

	NameHighlighted string `json:"name_highlighted,omitempty" gorm:"->;-:migration"`
	DescHighlighted string `json:"desc_highlighted,omitempty" gorm:"->;-:migration"`

	AllCount int `json:"-" gorm:"->;-:migration"`

	TsRank float64 `json:"ts_rank" gorm:"->;-:migration"` // Search debug
}

// ToDTO преобразует Issue в структуру IssueWithCount для удобства использования в API.
//
// Параметры:
//   - self: Issue - экземпляр Issue, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueWithCount: структура IssueWithCount, содержащая преобразованные данные Issue.
func (iwc *IssueWithCount) ToDTO() *dto.IssueWithCount {
	if iwc == nil {
		return nil
	}

	return &dto.IssueWithCount{
		Issue:             *iwc.Issue.ToDTO(),
		SubIssuesCount:    int(iwc.SubIssuesCount),
		LinkCount:         int(iwc.LinkCount),
		AttachmentCount:   int(iwc.AttachmentCount),
		LinkedIssuesCount: int(iwc.LinkedIssuesCount),
		CommentsCount:     int(iwc.CommentsCount),
		NameHighlighted:   iwc.NameHighlighted,
		DescHighlighted:   iwc.DescHighlighted,
	}
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности. Используется для определения имени таблицы при взаимодействии с базой данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (Issue) TableName() string { return "issues" }

// AfterFind - обрабатывает данные после получения объекта Issue из базы данных.  Обеспечивает восстановление URL,  заполняет информацию о реакции пользователей и  выполняет другие необходимые операции после извлечения данных.
//
// Парамметры:
//   - tx: объект базы данных GORM для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка во время обработки.
func (issue *Issue) AfterFind(tx *gorm.DB) error {
	_, issueStatus := tx.Get("issueProgress")
	if issueStatus {
		if issue.State != nil && issue.State.Group == "cancelled" {
			issue.IssueProgress.Status = types.Cancelled
		} else {
			if issue.StartDate == nil && issue.CompletedAt == nil {
				issue.IssueProgress.Status = types.Pending
			} else if issue.StartDate != nil && issue.CompletedAt == nil {
				issue.IssueProgress.Status = types.InProgress
			} else if issue.CompletedAt != nil {
				issue.IssueProgress.Status = types.Completed
			}

			if issue.TargetDate != nil && time.Now().After(issue.TargetDate.Time) {
				issue.IssueProgress.Overdue = true
			}
		}
	}

	if issue.Assignees != nil && len(*issue.Assignees) > 0 {
		var ids []string
		for _, assignee := range *issue.Assignees {
			ids = append(ids, assignee.ID)
		}
		issue.AssigneeIDs = ids
	} else {
		issue.AssigneeIDs = make([]string, 0)
	}

	if issue.Watchers != nil && len(*issue.Watchers) > 0 {
		var ids []string
		for _, watcher := range *issue.Watchers {
			ids = append(ids, watcher.ID)
		}
		issue.WatcherIDs = ids
	} else {
		issue.WatcherIDs = make([]string, 0)
	}

	if issue.Labels != nil && len(*issue.Labels) > 0 {
		var ids []string
		for _, label := range *issue.Labels {
			ids = append(ids, label.ID)
		}
		issue.LabelIDs = ids
	} else {
		issue.LabelIDs = make([]string, 0)
	}

	if issue.FullLoad {
		// Blockers
		if err := tx.Where("block_id = ?", issue.ID).Preload("BlockedBy.Project").Joins("BlockedBy").Find(&issue.BlockerIssuesIDs).Error; err != nil {
			return err
		}

		// Blocks
		if err := tx.Where("blocked_by_id = ?", issue.ID).Joins("Block").Find(&issue.BlockedIssuesIDs).Error; err != nil {
			return err
		}

		// Linked issues
		if err := issue.FetchLinkedIssues(tx); err != nil {
			return err
		}
	}

	issue.SetUrl()

	if issue.Workspace != nil && issue.Project != nil {
		ref, _ := url.Parse(fmt.Sprintf("/i/%s/%s/%d",
			issue.Workspace.Slug,
			issue.Project.Identifier,
			issue.SequenceId))
		issue.ShortURL = Config.WebURL.ResolveReference(ref)
	}

	return nil
}

func (issue *Issue) SetUrl() {
	raw := fmt.Sprintf("/%s/projects/%s/issues/%d", issue.WorkspaceId, issue.ProjectId, issue.SequenceId)
	u, _ := url.Parse(raw)
	issue.URL = Config.WebURL.ResolveReference(u)
}

// BeforeDelete - Вызывается перед удалением записи. Выполняет очистку связанных данных и удаление записи из базы данных.
//
// Парамметры:
//   - tx: объект базы данных GORM для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при выполнении операции.
func (issue *Issue) BeforeDelete(tx *gorm.DB) error {
	_, permanentDelete := tx.Get("permanentDelete")
	if permanentDelete {
		for _, asset := range issue.InlineAttachments {
			if err := tx.Delete(&asset).Error; err != nil {
				return err
			}
		}

		tx = tx.Unscoped().Session(&gorm.Session{})
	}

	cleanId := map[string]interface{}{"new_identifier": nil, "old_identifier": nil}
	tx.Where("(new_identifier = ? OR old_identifier = ?) AND (verb = ? OR verb = ? OR verb = ? OR verb = ?) AND field = ?", issue.ID, issue.ID, "created", "removed", "added", "copied", issue.GetEntityType()).
		Model(&ProjectActivity{}).
		Updates(cleanId)

	tx.Where("new_identifier = ? OR old_identifier = ?", issue.ID, issue.ID).
		Model(&SprintActivity{}).
		Updates(cleanId)

	tx.Where("new_identifier = ? ", issue.ID).
		Model(&IssueActivity{}).
		Update("new_identifier", nil)

	tx.Where("old_identifier = ?", issue.ID).
		Model(&IssueActivity{}).
		Update("old_identifier", nil)

	// Delete UserNotification
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	// Delete deferredNotification
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&DeferredNotifications{}).Error; err != nil {
		return err
	}

	// Delete sprintIssues
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&SprintIssue{}).Error; err != nil {
		return err
	}

	// Delete blocked by
	if err := tx.Where("blocked_by_id = ?", issue.ID).Delete(&IssueBlocker{}).Error; err != nil {
		return err
	}

	// Delete labels
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&IssueLabel{}).Error; err != nil {
		return err
	}

	// Delete activities
	var activities []IssueActivity
	if err := tx.Where("issue_id = ?", issue.ID).Find(&activities).Error; err != nil {
		return err
	}

	activityIds := utils.SliceToSlice(&activities, func(t *IssueActivity) string {
		return t.Id
	})

	if err := tx.Where("issue_activity_id in (?)", activityIds).Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}

	for _, activity := range activities {
		if err := tx.Unscoped().Delete(&activity).Error; err != nil {
			return err
		}
	}

	//// Delete activities
	//var activities []EntityActivity
	//if err := tx.Where("issue_id = ?", issue.ID).Find(&activities).Error; err != nil {
	//  return err
	//}
	//for _, activity := range activities {
	//  if err := tx.Unscoped().Delete(&activity).Error; err != nil {
	//    return err
	//  }
	//}

	// Delete comments, reaction
	var comments []IssueComment
	if err := tx.Where("issue_id = ?", issue.ID).Preload("Attachments").Find(&comments).Error; err != nil {
		return err
	}

	var commentId []uuid.UUID

	for _, comment := range comments {
		commentId = append(commentId, comment.Id)
	}

	if err := tx.Where("comment_id in ?", commentId).Delete(&CommentReaction{}).Error; err != nil {
		return err
	}

	if err := tx.Model(&IssueComment{}).
		Where("issue_id = ?", issue.ID).
		Update("reply_to_comment_id", nil).Error; err != nil {
		return err
	}

	if err := tx.Where("issue_id = ?", issue.ID).Delete(comments).Error; err != nil {
		return err
	}

	// Remove assignees
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&IssueAssignee{}).Error; err != nil {
		return err
	}

	// Remove watchers
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&IssueWatcher{}).Error; err != nil {
		return err
	}

	// Remove attachments
	var attachments []IssueAttachment
	if err := tx.Where("issue_id = ?", issue.ID).Find(&attachments).Error; err != nil {
		return err
	}
	for _, attachment := range attachments {
		if err := tx.Delete(&attachment).Error; err != nil {
			return err
		}
	}

	// Remove links
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&IssueLink{}).Error; err != nil {
		return err
	}

	// Remove blockers
	if err := tx.Where("block_id = ?", issue.ID).Or("blocked_by_id = ?", issue.ID).Delete(&IssueBlocker{}).Error; err != nil {
		return err
	}

	// Remove inline attachments
	if len(issue.InlineAttachments) == 0 {
		if err := tx.Where("issue_id = ?", issue.ID).Find(&issue.InlineAttachments).Error; err != nil {
			return err
		}
	}
	for _, attach := range issue.InlineAttachments {
		if err := tx.Delete(&attach).Error; err != nil {
			return err
		}
	}

	// Remove linked relationships
	if err := tx.Where("id1 = ? or id2 = ?", issue.ID, issue.ID).Delete(&LinkedIssues{}).Error; err != nil {

		return err
	}

	// delete RulesLog
	if err := tx.Where("issue_id = ?", issue.ID).Delete(&RulesLog{}).Error; err != nil {
		return err
	}

	// Remove children issues
	if err := tx.Model(&Issue{}).Where("parent_id = ? OR id = ?", issue.ID, issue.ID).Update("parent_id", nil).Error; err != nil {
		return err
	}
	return tx.Model(&Issue{}).Where("id = ?", issue.ID).Update("state_id", nil).Error

}

// String возвращает строку, представляющую собой идентификатор Issue.
//
// Парамметры:
//   - Нет
//
// Возвращает:
//   - string: строка, представляющая собой идентификатор Issue.
func (issue *Issue) String() string {
	if issue.Project != nil {
		return fmt.Sprintf("%s-%d", issue.Project.Identifier, issue.SequenceId)
	}
	return fmt.Sprint(issue.SequenceId)
}

// FullIssueName возвращает полное имя задачи, объединяя название проекта, номер задачи и другие релевантные данные для идентификации задачи.
//
// Парамметры:
//   - issue: экземпляр Issue, для которого необходимо сформировать полное имя.
//
// Возвращает:
//   - string: полное имя задачи в формате 'project_number'. Например: 'project-123'.
func (issue *Issue) FullIssueName() string {
	var issueID string
	if issue.Project != nil {
		issueID = fmt.Sprintf("%s-%d", issue.Project.Identifier, issue.SequenceId)
	} else {
		issueID = fmt.Sprint(issue.SequenceId)
	}

	if issue.Name != "" {
		return fmt.Sprintf("%s (%s)", issueID, issue.Name)
	}
	return issueID
}

// BeforeCreate - вызывается перед созданием новой задачи.  Функция выполняет начальную инициализацию данных задачи, например, устанавливает дефолтные значения для состояния и даты начала/окончания, если они не указаны.  Также вычисляет sequenceId и sortOrder, если они не заданы.  Функция принимает объект базы данных GORM (tx) для выполнения операций с базой данных и возвращает ошибку, если во время инициализации возникли какие-либо проблемы.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если во время инициализации возникли какие-либо проблемы, иначе nil.
func (issue *Issue) BeforeCreate(tx *gorm.DB) error {
	if issue.DescriptionStripped == nil || *issue.DescriptionStripped == "" {
		desc := policy.StripTagsPolicy.Sanitize(issue.DescriptionHtml)
		issue.DescriptionStripped = &desc
	}
	return nil
}

// BeforeSave - вызывается перед сохранением объекта в базе данных.  Выполняет предварительную обработку данных, такую как очистка HTML и вычисление ID задачи.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка во время обработки.
func (issue *Issue) BeforeSave(tx *gorm.DB) error {
	dest := tx.Statement.Dest
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() == reflect.Ptr && destValue.Elem().Kind() == reflect.Slice {
		return nil
	}

	if tx.Statement.Changed("description_html") {
		// Strip description
		var desc string
		var strippedDesc string

		if m, ok := tx.Statement.Dest.(map[string]interface{}); ok {
			var ok1 bool
			desc, ok1 = m["description_html"].(string)
			if !ok1 {
				return fmt.Errorf("incorrect description %v", m["description_html"])
			}
			strippedDesc, ok1 = m["description_stripped"].(string)
			if !ok1 {
				slog.Debug("Incorrect stripped description", "stripped_description", m["description_stripped"])
				strippedDesc = ""
			}

			if strippedDesc == "" {
				strippedDesc = policy.StripTagsPolicy.Sanitize(desc)
			} else {
				strippedDesc = policy.StripTagsPolicy.Sanitize(strippedDesc)
			}
			issue.DescriptionHtml = desc
			strippedDesc = strings.TrimSpace(strippedDesc)
			issue.DescriptionStripped = &strippedDesc

			issue.DescriptionHtml = policy.UgcPolicy.Sanitize(issue.DescriptionHtml)
		} else {
			return errors.New("incorrect issue save dest interface")
		}
	}

	return nil
}

// IsAssignee проверяет, назначен ли пользователь на данную задачу.
//
// Параметры:
//   - id: идентификатор пользователя.
//
// Возвращает:
//   - bool: true, если пользователь назначен на задачу, false в противном случае.
func (issue Issue) IsAssignee(id string) bool {
	for _, assigneeId := range issue.AssigneeIDs {
		if assigneeId == id {
			return true
		}
	}
	return false
}

// FetchLinkedIssues извлекает связанные задачи по проекту из базы данных.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при извлечении связанных задач.
func (issue *Issue) FetchLinkedIssues(tx *gorm.DB) error {
	var ids []LinkedIssues
	if err := tx.Where("id1 = ?", issue.ID).Or("id2 = ?", issue.ID).Find(&ids).Error; err != nil {
		return err
	}
	issue.LinkedIssuesIDs = make([]uuid.UUID, len(ids))

	for i, id := range ids {
		if id.Id1 == issue.ID {
			issue.LinkedIssuesIDs[i] = id.Id2
		} else {
			issue.LinkedIssuesIDs[i] = id.Id1
		}
	}
	return nil
}

// AddLinkIssue добавляет связь между текущей задачей и другой задачей.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций с базой данных.
//   - id: идентификатор связанной задачи.
//
// Возвращает:
//   - error: ошибка, если при добавлении связи произошла ошибка.
func (issue *Issue) AddLinkedIssue(tx *gorm.DB, id uuid.UUID) error {
	var sameProject bool
	if err := tx.Model(&Issue{}).
		Select("EXISTS(?)",
			tx.Model(&Issue{}).
				Select("1").
				Where("id = ?", id).
				Where("project_id = ?", issue.ProjectId).
				Where("workspace_id = ?", issue.WorkspaceId),
		).
		Find(&sameProject).Error; err != nil {
		return err
	}

	if !sameProject {
		return errors.New("not the same project")
	}

	link := GetIssuesLink(issue.ID, id)
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
		return err
	}

	issue.LinkedIssuesIDs = append(issue.LinkedIssuesIDs, id)

	return nil
}

// RemoveLinkedIssue удаляет связь между текущей задачкой и другой задачей.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций с базой данных.
//   - id: идентификатор задачи, связь с которой необходимо удалить.
//
// Возвращает:
//   - error: ошибка, если при удалении связи произошла ошибка. Если связь успешно удалена, возвращается nil.
func (issue *Issue) RemoveLinkedIssue(tx *gorm.DB, id uuid.UUID) error {
	link := GetIssuesLink(issue.ID, id)

	if err := tx.Delete(&link).Error; err != nil {
		return err
	}

	issue.LinkedIssuesIDs = slices.DeleteFunc(issue.LinkedIssuesIDs, func(curId uuid.UUID) bool {
		return curId == id
	})

	return nil
}

type IssueDescriptionLock struct {
	UserId      uuid.UUID `gorm:"index"`
	IssueId     uuid.UUID `gorm:"uniqueIndex"`
	LockedUntil time.Time `gorm:"index"`

	User  User
	Issue Issue
}

type LinkedIssues struct {
	Id1 uuid.UUID `json:"-" gorm:"primaryKey;autoIncrement:false;type:uuid;check:id1 < id2"`
	Id2 uuid.UUID `json:"-" gorm:"primaryKey;autoIncrement:false;type:uuid;index:,type:hash"`

	Issue1 Issue `json:"-" gorm:"foreignKey:Id1"`
	Issue2 Issue `json:"-" gorm:"foreignKey:Id2"`
}

// IssueLinkedExtendFields
// -migration
type IssueLinkedExtendFields struct {
	NewIssueLinked *Issue `json:"-" gorm:"-" field:"linked" extensions:"x-nullable"`
	OldIssueLinked *Issue `json:"-" gorm:"-" field:"linked" extensions:"x-nullable"`
}

// + r.Name,  // Название таблицы. Используется для определения имени таблицы в базе данных.  Это необходимо для правильной работы ORM.  Например, для выполнения запросов к базе данных.  Название таблицы должно соответствовать имени таблицы в базе данных.  Например,
func (LinkedIssues) TableName() string { return "linked_issues" }

type IssueLink struct {
	Id        string         `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// title character varying IS_NULL:YES
	Title string `json:"title,omitempty"`
	// url character varying IS_NULL:NO
	Url string `json:"url"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// issue_id uuid IS_NULL:NO
	IssueId string `json:"issue_id" gorm:"index"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project_id"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`
	// metadata jsonb IS_NULL:NO
	Metadata map[string]interface{} `json:"metadata" gorm:"serializer:json"`

	Workspace *Workspace `json:"workspace_detail,omitempty" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail,omitempty" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	CreatedBy *User      `json:"created_by_detail,omitempty" gorm:"foreignKey:CreatedById" extensions:"x-nullable"`
	Issue     *Issue     `json:"-" gorm:"foreignKey:IssueId" extensions:"x-nullable"`
}

// Возвращает имя таблицы, соответствующее текущему типу сущности. Используется для правильной работы с ORM и определения имени таблицы в базе данных.
func (IssueLink) TableName() string { return "issue_links" }

// IssueLinkExtendFields
// -migration
type IssueLinkExtendFields struct {
	NewLink *IssueLink `json:"-" gorm:"-" field:"link" extensions:"x-nullable"`
	OldLink *IssueLink `json:"-" gorm:"-" field:"link" extensions:"x-nullable"`
}

func (i IssueLink) GetId() string {
	return i.Id
}

func (i IssueLink) GetString() string {
	return i.Url
}

func (i IssueLink) GetEntityType() string {
	return "link"
}

func (i IssueLink) GetWorkspaceId() string {
	return i.WorkspaceId
}

func (i IssueLink) GetProjectId() string {
	return i.ProjectId
}

func (i IssueLink) GetIssueId() string {
	return i.IssueId
}

func (il *IssueLink) BeforeDelete(tx *gorm.DB) error {
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", il.Id, "created", "link").Model(&IssueActivity{}).Update("new_identifier", nil)
	var activities []IssueActivity
	if err := tx.Where("new_identifier = ? or old_identifier = ? ", il.Id, il.Id).Find(&activities).Error; err != nil {
		return err
	}
	for _, activity := range activities {
		tx.Delete(&activity)
	}
	return nil
}

// ToLightDTO преобразует IssueLink в структуру IssueLinkLight для упрощения передачи данных в клиентский код.
//
// Параметры:
//   - self: IssueLink - экземпляр IssueLink, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueLinkLight: структура IssueLinkLight, содержащая преобразованные данные IssueLink.
func (il *IssueLink) ToLightDTO() *dto.IssueLinkLight {
	if il == nil {
		return nil
	}

	return &dto.IssueLinkLight{
		Id:    il.Id,
		Title: il.Title,
		Url:   il.Url,

		CreatedAt: il.CreatedAt,
		UpdatedAt: il.UpdatedAt,
	}
}

type IssueAttachment struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// attributes jsonb IS_NULL:NO
	Attributes map[string]interface{} `json:"attributes" gorm:"serializer:json"`
	// asset character varying IS_NULL:NO
	AssetId  uuid.UUID `json:"asset" gorm:"type:uuid"`
	AssetOld string    `json:"-" gorm:"column:asset"` // Legacy
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// issue_id uuid IS_NULL:NO
	IssueId string `json:"issue" gorm:"index"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"-" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Asset     *FileAsset `json:"file_details" gorm:"foreignKey:AssetId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности. Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueAttachment) TableName() string { return "issue_attachments" }

// IssueAttachmentExtendFields
// -migration
type IssueAttachmentExtendFields struct {
	NewIssueAttachment *IssueAttachment `json:"-" gorm:"-" field:"attachment::issue" extensions:"x-nullable"`
	OldIssueAttachment *IssueAttachment `json:"-" gorm:"-" field:"attachment::issue" extensions:"x-nullable"`
}

func (ia IssueAttachment) GetId() string {
	return ia.Id
}

func (ia IssueAttachment) GetString() string {
	if ia.Asset != nil {
		return ia.Asset.Name
	}
	return ia.GetEntityType()
}

func (ia IssueAttachment) GetEntityType() string {
	return "attachment"
}

func (i IssueAttachment) GetWorkspaceId() string {
	return i.WorkspaceId
}

func (i IssueAttachment) GetProjectId() string {
	return i.ProjectId
}

func (i IssueAttachment) GetIssueId() string {
	return i.IssueId
}

func (attachment *IssueAttachment) AfterFind(tx *gorm.DB) error {
	return tx.Where("id = ?", attachment.AssetId).Find(&attachment.Asset).Error
}

func (attachment *IssueAttachment) BeforeDelete(tx *gorm.DB) error {
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", attachment.Id, "created", "attachment").Model(&IssueActivity{}).Update("new_identifier", nil)
	return nil
}

// ToLightDTO преобразует объект IssueAttachment в структуру dto.Attachment для удобства использования в API.
//
// Параметры:
//   - self: Объект IssueAttachment, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.Attachment: Структура dto.Attachment, содержащая преобразованные данные.
func (attachment *IssueAttachment) ToLightDTO() *dto.Attachment {
	if attachment == nil {
		return nil
	}

	return &dto.Attachment{
		Id:        attachment.Id,
		CreatedAt: attachment.CreatedAt,
		Asset:     attachment.Asset.ToDTO(),
	}
}

// AfterDelete - выполняется после удаления объекта Issue.  Функция выполняет очистку связанных данных, таких как удаление комментариев, реакций, и других связанных сущностей.
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если при выполнении операций возникают проблемы.
func (attachment *IssueAttachment) AfterDelete(tx *gorm.DB) error {
	if attachment.Asset == nil {
		if err := tx.Where("id = ?", &attachment.AssetId).First(&attachment.Asset).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
	}

	// Check if this asset used in another attachment
	if attachment.Asset != nil {
		del, err := attachment.Asset.CanBeDeleted(tx)
		if err != nil {
			return err
		}

		if del && !attachment.Asset.Id.IsNil() {
			if err := tx.Where("id = ?", attachment.Asset.Id).Delete(&FileAsset{}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

type IssueAssignee struct {
	Id        string         `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	AssigneeId  string  `json:"assignee_id" gorm:"uniqueIndex:assignees_idx,priority:2"`
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	IssueId     string  `json:"issue_id" gorm:"index;uniqueIndex:assignees_idx,priority:1"`
	ProjectId   string  `json:"project_id"`
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	WorkspaceId string  `json:"workspace_id"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Issue     *Issue     `gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Assignee  *User      `gorm:"foreignKey:AssigneeId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueAssignee) TableName() string { return "issue_assignees" }

// IssueAssigneeExtendFields
// -migration
type IssueAssigneeExtendFields struct {
	NewAssignee *User `json:"-" gorm:"-" field:"assignees" extensions:"x-nullable"`
	OldAssignee *User `json:"-" gorm:"-" field:"assignees" extensions:"x-nullable"`
}

type IssueWatcher struct {
	Id        string         `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	WatcherId   string  `json:"watcher_id" gorm:"uniqueIndex:watchers_idx,priority:2"`
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	IssueId     string  `json:"issue_id" gorm:"index;uniqueIndex:watchers_idx,priority:1"`
	ProjectId   string  `json:"project_id"`
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	WorkspaceId string  `json:"workspace_id"`

	Workspace *Workspace `gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Issue     *Issue     `gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Watcher   *User      `gorm:"foreignKey:WatcherId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueWatcher) TableName() string { return "issue_watchers" }

// IssueWatchersExtendFields
// -migration
type IssueWatchersExtendFields struct {
	NewWatcher *User `json:"-" gorm:"-" field:"watchers::issue" extensions:"x-nullable"`
	OldWatcher *User `json:"-" gorm:"-" field:"watchers::issue" extensions:"x-nullable"`
}

type IssueBlocker struct {
	Id        string         `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// block_id uuid IS_NULL:NO
	BlockId uuid.UUID `json:"block" gorm:"index"`
	// blocked_by_id uuid IS_NULL:NO
	BlockedById uuid.UUID `json:"blocked_by" gorm:"index"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by,omitempty" extensions:"x-nullable"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project_id"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"-" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Block     *Issue     `json:"blocked_issue_detail" gorm:"foreignKey:BlockId" extensions:"x-nullable"`
	BlockedBy *Issue     `json:"blocker_issue_detail" gorm:"foreignKey:BlockedById" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueBlocker) TableName() string { return "issue_blockers" }

// ToLightDTO преобразует объект IssueBlocker в структуру IssueBlockerLight для упрощения передачи данных в клиентский код.
//
// Параметры:
//   - self: Объект IssueBlocker, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueBlockerLight: Структура IssueBlockerLight, содержащая преобразованные данные.
func (ib *IssueBlocker) ToLightDTO() *dto.IssueBlockerLight {
	if ib == nil {
		return nil
	}
	return &dto.IssueBlockerLight{
		Id:          ib.Id,
		BlockId:     ib.BlockId.String(),
		BlockedById: ib.BlockedById.String(),
		Block:       ib.Block.ToLightDTO(),
		BlockedBy:   ib.BlockedBy.ToLightDTO(),
	}
}

type IssueLabel struct {
	Id        string         `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by_id,omitempty" extensions:"x-nullable"`
	// issue_id uuid IS_NULL:NO
	IssueId string `json:"issue_id" gorm:"index"`
	// label_id uuid IS_NULL:NO
	LabelId string `json:"label_id"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project_id"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace_id"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"-" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Issue     *Issue     `gorm:"foreignKey:IssueId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueLabel) TableName() string { return "issue_labels" }

type IssueComment struct {
	Id        uuid.UUID      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`

	URL *url.URL `json:"-" gorm:"-"`

	IssueId     string `json:"issue_id" gorm:"index;uniqueIndex:issue_comment_original_id_idx,priority:1"`
	ProjectId   string `json:"project_id"`
	WorkspaceId string `json:"workspace_id"`

	ActorId     *string `json:"actor_id,omitempty" gorm:"index;index:integration,priority:1" extensions:"x-nullable"`
	UpdatedById *string `json:"updated_by_id,omitempty" extensions:"x-nullable"`

	CommentHtml     types.RedactorHTML `json:"comment_html"`
	CommentStripped string             `json:"comment_stripped"`

	IntegrationMeta  string        `json:"-" gorm:"index:integration,priority:2"`
	ReplyToCommentId uuid.NullUUID `json:"reply_to_comment_id" extensions:"x-nullable"`
	OriginalComment  *IssueComment `json:"original_comment,omitempty" gorm:"foreignKey:ReplyToCommentId" extensions:"x-nullable"`

	// Id in system, from that comment was imported
	OriginalId sql.NullString `gorm:"uniqueIndex:issue_comment_original_id_idx,priority:2"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"-" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	Issue     *Issue     `gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`

	Attachments []FileAsset `json:"comment_attachments" gorm:"foreignKey:CommentId"`

	Reactions       []CommentReaction `json:"reactions" gorm:"foreignKey:CommentId"`
	ReactionSummary map[string]int    `json:"reaction_summary,omitempty" gorm:"-"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueComment) TableName() string { return "issue_comments" }

// IssueCommentExtendFields
// -migration
type IssueCommentExtendFields struct {
	NewIssueComment *IssueComment `json:"-" gorm:"-" field:"comment::issue" extensions:"x-nullable"`
}

func (i IssueComment) GetId() string {
	return i.Id.String()
}

func (i IssueComment) GetString() string {
	return fmt.Sprint(i.CommentHtml)
}

func (i IssueComment) GetEntityType() string {
	return "comment"
}

func (i IssueComment) GetWorkspaceId() string {
	return i.WorkspaceId
}

func (i IssueComment) GetProjectId() string {
	return i.ProjectId
}

func (i IssueComment) GetIssueId() string {
	return i.IssueId
}

// ToLightDTO преобразует объект IssueComment в структуру IssueCommentLight для упрощения передачи данных в клиентский код.
//
// Параметры:
//   - self: Объект IssueComment, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueCommentLight: Структура IssueCommentLight, содержащая преобразованные данные.
func (i *IssueComment) ToLightDTO() *dto.IssueCommentLight {
	if i == nil {
		return nil
	}
	i.SetUrl()
	return &dto.IssueCommentLight{
		Id:              i.Id.String(),
		CommentStripped: i.CommentStripped,
		CommentHtml:     i.CommentHtml.Body,
		URL:             types.JsonURL{i.URL},
	}
}

// ToDTO преобразует объект IssueComment в структуру IssueCommentLight для упрощения передачи данных в клиентский код.
//
// Парамметры:
//   - self: Объект IssueComment, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueCommentLight: Структура IssueCommentLight, содержащая преобразованные данные.
func (i *IssueComment) ToDTO() *dto.IssueComment {
	if i == nil {
		return nil
	}

	var a *dto.UserLight
	if i.Actor != nil {
		a = i.Actor.ToLightDTO()
	}

	return &dto.IssueComment{
		IssueCommentLight: *i.ToLightDTO(),
		CreatedAt:         i.CreatedAt,
		UpdatedAt:         i.UpdatedAt,
		UpdatedById:       i.UpdatedById,
		ActorId:           i.ActorId,
		ProjectId:         i.ProjectId,
		WorkspaceId:       i.WorkspaceId,
		IssueId:           i.IssueId,
		ReplyToCommentId:  i.ReplyToCommentId,
		OriginalComment:   i.OriginalComment.ToDTO(),
		Actor:             a,
		Attachments:       utils.SliceToSlice(&i.Attachments, func(fa *FileAsset) *dto.FileAsset { return fa.ToDTO() }),
		Reactions:         utils.SliceToSlice(&i.Reactions, func(cr *CommentReaction) *dto.CommentReaction { return cr.ToDTO() }),
		ReactionSummary:   i.ReactionSummary,
	}
}

type CommentReaction struct {
	Id        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	UserId    string    `json:"user_id"`
	CommentId uuid.UUID `json:"comment_id" gorm:"index"`
	Reaction  string    `json:"reaction"`

	User    *User         `json:"-" gorm:"foreignKey:UserId" extensions:"x-nullable"`
	Comment *IssueComment `json:"-" gorm:"foreignKey:CommentId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (CommentReaction) TableName() string { return "comment_reactions" }

// ToDTO преобразует объект IssueComment в структуру IssueCommentLight для упрощения передачи данных в клиентский код.
//
// Параметры:
//   - self: Объект IssueComment, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.IssueCommentLight: Структура IssueCommentLight, содержащая преобразованные данные.
func (cr CommentReaction) ToDTO() *dto.CommentReaction {
	return &dto.CommentReaction{
		Id:        cr.Id,
		CreatedAt: cr.CreatedAt,
		UpdatedAt: cr.UpdatedAt,
		CommentId: cr.CommentId.String(),
		UserId:    cr.UserId,
		Reaction:  cr.Reaction,
	}
}

// BeforeSave - выполняется перед сохранением объекта в базе данных.  Выполняет очистку HTML, расчет sequenceId и sortOrder, а также устанавливает начальные значения для полей состояния (start/complete).
//
// Параметры:
//   - tx: объект базы данных GORM для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если произошла ошибка при выполнении операции.
func (ic *IssueComment) BeforeSave(tx *gorm.DB) error {
	// Strip comment
	if ic.CommentStripped != "" {
		ic.CommentStripped = policy.StripTagsPolicy.Sanitize(ic.CommentStripped)
	} else {
		commentHTML := ic.CommentHtml.Body
		commentHTML = strings.ReplaceAll(commentHTML, "<p>", "\n")
		commentHTML = strings.ReplaceAll(commentHTML, "<li>", "\n")
		commentHTML = strings.TrimSpace(commentHTML)
		ic.CommentStripped = policy.StripTagsPolicy.Sanitize(commentHTML)
	}

	return nil
}

// BeforeUpdate - Вызывается перед сохранением объекта в базе данных.  Выполняет опрос полей, которые были изменены, и обновляет соответствующие поля в объекте.  Это необходимо для поддержания согласованности данных и предотвращения потери данных при обновлении.
//
// Парамметры:
//   - tx: объект базы данных GORM, используемый для выполнения операций с базой данных.
//
// Возвращает:
//   - error: ошибка, если при обновлении произошла ошибка, nil в противном случае.
func (ic *IssueComment) BeforeUpdate(tx *gorm.DB) error {
	return ic.BeforeSave(tx)
}

// AfterFind - выполняется после извлечения объекта из базы данных.  Функция обновляет URL, собирает информацию о реакциях и возвращает ошибку, если при поиске произошла ошибка.
//
// Параметры:
//   - tx: объект базы данных GORM, используемый для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если при поиске произошла ошибка, nil в противном случае.
func (i *IssueComment) AfterFind(tx *gorm.DB) error {
	i.SetUrl()

	i.ReactionSummary = make(map[string]int)
	for _, r := range i.Reactions {
		i.ReactionSummary[r.Reaction]++
	}
	return nil
}

func (i *IssueComment) SetUrl() {
	raw := fmt.Sprintf("/api/auth/workspaces/%s/projects/%s/issues/%s/comments/%s/", i.WorkspaceId, i.ProjectId, i.IssueId, i.Id)
	u, _ := url.Parse(raw)
	i.URL = Config.WebURL.ResolveReference(u)
}

// BeforeDelete - Вызывается перед удалением объекта Issue из базы данных.  Функция выполняет очистку связанных данных, таких как удаление комментариев, реакций и других связанных сущностей.
//
// Параметры:
//   - tx: объект базы данных GORM, используемый для выполнения операций.
//
// Возвращает:
//   - error: ошибка, если при удалении произошла ошибка, nil в противном случае.
func (ic *IssueComment) BeforeDelete(tx *gorm.DB) error {
	if err := tx.Where("comment_id = ?", ic.Id).Unscoped().Delete(&UserNotifications{}).Error; err != nil {
		return err
	}
	tx.Where("new_identifier = ? AND verb = ? AND field = ?", ic.Id, "created", "comment").Model(&IssueActivity{}).Update("new_identifier", nil)
	tx.Where("new_identifier = ? or old_identifier = ? ", ic.Id, ic.Id).Delete(&IssueActivity{})

	for _, attach := range ic.Attachments {
		if err := tx.Delete(&attach).Error; err != nil {
			return err
		}
	}

	if err := tx.Model(&IssueComment{}).Where("reply_to_comment_id = ?", ic.Id).Update("reply_to_comment_id", nil).Error; err != nil {
		return err
	}
	return nil
}

type IssueProperty struct {
	// created_at timestamp with time zone IS_NULL:NO
	CreatedAt time.Time `json:"created_at"`
	// updated_at timestamp with time zone IS_NULL:NO
	UpdatedAt time.Time `json:"updated_at"`
	// id uuid IS_NULL:NO
	Id string `json:"id" gorm:"primaryKey"`
	// properties jsonb IS_NULL:NO
	Properties map[string]interface{} `json:"properties" gorm:"serializer:json"`
	// created_by_id uuid IS_NULL:YES
	CreatedById *string `json:"created_by,omitempty" extensions:"x-nullable"`
	// project_id uuid IS_NULL:NO
	ProjectId string `json:"project" gorm:"index"`
	// updated_by_id uuid IS_NULL:YES
	UpdatedById *string `json:"updated_by,omitempty" extensions:"x-nullable"`
	// user_id uuid IS_NULL:NO
	UserId string `json:"user"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`

	Workspace *Workspace `json:"-" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Project   *Project   `json:"-" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	User      *User      `json:"-" gorm:"foreignKey:UserId" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (IssueProperty) TableName() string { return "issue_properties" }

// CreateIssue создает новую задачу в базе данных. Функция принимает объект задачи, объект базы данных, пользователя, текст комментария, комментарий для базы данных, ID задачи для ответа на комментарий и дополнительные метаданные.  Функция заполняет поля задачи, такие как состояние, ID последовательности и порядок сортировки. Также устанавливает начальные значения для полей состояния (start/complete).  Возвращает ошибку, если при создании задачи произошла ошибка, или nil в противном случае.
//
// Параметры:
//   - db: объект базы данных GORM для выполнения операций с базой данных.
//   - issue: объект задачи, который необходимо создать.
//
// Возвращает:
//   - error: ошибка, если при создании задачи произошла ошибка, nil в противном случае.
func CreateIssue(db *gorm.DB, issue *Issue) error {
	// State fill
	if issue.StateId == nil || *issue.StateId == "" {
		// default state or random
		var defaultState State
		if err := db.Where("project_id = ?", issue.ProjectId).Where("states.default = ?", true).First(&defaultState).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// random state
				if err := db.Where("project_id = ?", issue.ProjectId).First(&defaultState).Error; err != nil {
					return err
				}
			} else {
				return err
			}
		}
		issue.StateId = &defaultState.ID
		issue.State = &defaultState
	} else if issue.State == nil {
		if err := db.
			Where("id = ?", issue.StateId).
			Where("project_id = ?", issue.ProjectId).
			Find(&issue.State).Error; err != nil {
			return err
		}
	}

	// Calculate sequence id
	var lastId sql.NullInt64
	row := db.Model(Issue{}).
		Select("max(sequence_id)").
		Unscoped().
		Where("project_id = ?", issue.ProjectId).
		Row()
	if err := row.Scan(&lastId); err != nil {
		return err
	}

	// Just use the last ID specified (which should be the greatest) and add one to it
	if lastId.Valid {
		issue.SequenceId = int(lastId.Int64 + 1)
	} else {
		issue.SequenceId = 1
	}

	// Sort order
	if !issue.ParentId.Valid {
		issue.SortOrder = 0
	} else {
		var sortOrder int
		if err := db.Select("coalesce(max(sort_order), 0)").Model(&Issue{}).Where("parent_id = ?", issue.ParentId.UUID).Find(&sortOrder).Error; err != nil {
			return err
		}
		issue.SortOrder = sortOrder + 1
	}

	// Start timer if state in started group
	if issue.State.Group == "started" {
		issue.StartDate = &types.TargetDateTimeZ{Time: time.Now()}
	}

	if issue.State.Group == "completed" {
		issue.CompletedAt = &types.TargetDateTimeZ{Time: time.Now()}
	}

	issue.State = nil
	return db.Omit(clause.Associations).Create(issue).Error
}

type RulesLog struct {
	Id        uuid.UUID      `json:"id" gorm:"column:id;primaryKey;type:text"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// project_id uuid NOT NULL,
	ProjectId string   `json:"project" gorm:"index"`
	Project   *Project `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`
	// workspace_id uuid NOT NULL,
	WorkspaceId string     `json:"workspace"`
	Workspace   *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	// issue_id uuid IS_NULL:NO
	IssueId string `json:"issue_id" gorm:"index"`
	Issue   *Issue `json:"issue_detail" gorm:"foreignKey:IssueId" extensions:"x-nullable"`

	UserId *string `json:"userId" gorm:"index" extensions:"x-nullable"`
	User   *User   `json:"user_detail" gorm:"foreignKey:UserId" extensions:"x-nullable"`

	Time         time.Time `json:"time"`
	FunctionName *string   `json:"function_name,omitempty" extensions:"x-nullable"`
	Type         string    `json:"type" validate:"oneof=print error"`
	Msg          string    `json:"msg"`
	LuaErr       *string   `json:"lua_err,omitempty" extensions:"x-nullable"`
}

// TableName возвращает имя таблицы, соответствующее текущему типу сущности.
// Используется для правильной работы с ORM и определения имени таблицы в базе данных.
//
// Параметры:
//   - Нет
//
// Возвращает:
//   - string: имя таблицы.
func (RulesLog) TableName() string { return "rules_log" }

type IssueActivity struct {
	IssueActivityExtendFields
	ActivitySender
	Id string `json:"id" gorm:"primaryKey"`

	CreatedAt time.Time `json:"created_at" gorm:"index:issue_activities_issue_index,sort:desc,type:btree,priority:2;index:issue_activities_actor_index,sort:desc,type:btree,priority:2;index:issue_activities_mail_index,type:btree,where:notified = false"`
	// verb character varying IS_NULL:NO
	Verb string `json:"verb"`
	//field character varying IS_NULL:YES
	Field *string `json:"field,omitempty" extensions:"x-nullable"`
	// old_value text IS_NULL:YES
	OldValue *string `json:"old_value" extensions:"x-nullable"`
	// new_value text IS_NULL:YES
	NewValue string `json:"new_value" `
	// comment text IS_NULL:NO
	Comment string `json:"comment"`
	// issue_id uuid IS_NULL:YES
	IssueId string `json:"issue_id" gorm:"index:issue_activities_issue_index,priority:1" extensions:"x-nullable"`
	// project_id uuid IS_NULL:YES
	ProjectId string `json:"project_id"`
	// workspace_id uuid IS_NULL:NO
	WorkspaceId string `json:"workspace"`
	// actor_id uuid IS_NULL:YES
	ActorId *string `json:"actor,omitempty" gorm:"index:issue_activities_actor_index,priority:1" extensions:"x-nullable"`

	// new_identifier uuid IS_NULL:YES
	NewIdentifier *string `json:"new_identifier" extensions:"x-nullable"`
	// old_identifier uuid IS_NULL:YES
	OldIdentifier *string       `json:"old_identifier" extensions:"x-nullable"`
	Notified      bool          `json:"-" gorm:"default:false"`
	TelegramMsgId pq.Int64Array `json:"-" gorm:"column:telegram_msg_ids;index;type:integer[]"`

	Workspace *Workspace `json:"workspace_detail" gorm:"foreignKey:WorkspaceId" extensions:"x-nullable"`
	Actor     *User      `json:"actor_detail" gorm:"foreignKey:ActorId" extensions:"x-nullable"`
	Issue     *Issue     `json:"issue_detail" gorm:"foreignKey:IssueId" extensions:"x-nullable"`
	Project   *Project   `json:"project_detail" gorm:"foreignKey:ProjectId" extensions:"x-nullable"`

	//AffectedUser *User `json:"affected_user,omitempty" gorm:"-" extensions:"x-nullable"`

	UnionCustomFields string `json:"-" gorm:"-"`

	//NewIssueComment *IssueComment `json:"-" gorm:"-" field:"comment" extensions:"x-nullable"`
}

// IssueActivityExtendFields
// -migration
type IssueActivityExtendFields struct {
	StateExtendFields
	LabelExtendFields
	SubIssueExtendFields
	IssueAssigneeExtendFields
	IssueWatchersExtendFields
	IssueAttachmentExtendFields
	IssueLinkExtendFields
	IssueCommentExtendFields
	IssueLinkedExtendFields
	BlockIssueExtendFields
	BlockingIssueExtendFields
	ProjectExtendFields
	IssueSprintExtendFields
}

type IssueEntityI interface {
	ProjectEntityI
	GetIssueId() string
}

func (IssueActivity) TableName() string { return "issue_activities" }

// IssueActivityWithLag
// -migration
type IssueActivityWithLag struct {
	IssueActivity
	StateLag int `json:"state_lag_ms,omitempty" gorm:"->;-:migration"`
}

//func (IssueActivity) GetCustomFields(fields []string) []string {
//	return append(fields, "'issue' AS entity_type")
//}

func (IssueActivity) GetFields() []string {
	return []string{"id", "created_at", "verb", "field", "old_value", "new_value", "issue_id", "project_id", "workspace_id", "actor_id", "new_identifier", "old_identifier", "telegram_msg_ids"}
}

func (IssueActivity) GetEntity() string {
	return "issue"
}

func (ia IssueActivity) GetCustomFields() string {
	return ia.UnionCustomFields
}

func (ia IssueActivity) SkipPreload() bool {
	if ia.Field == nil {
		return true
	}

	if ia.NewIdentifier == nil && ia.OldIdentifier == nil {
		return true
	}
	return false
}

func (ia IssueActivity) GetField() string {
	return pointerToStr(ia.Field)
}

func (ia IssueActivity) GetVerb() string {
	return ia.Verb
}

func (ia IssueActivity) GetNewIdentifier() string {
	return pointerToStr(ia.NewIdentifier)
}
func (ia IssueActivity) GetOldIdentifier() string {
	return pointerToStr(ia.OldIdentifier)
}

func (ia IssueActivity) GetId() string {
	return ia.Id
}

func (ia IssueActivity) GetUrl() *string {
	if ia.Issue != nil && ia.Issue.URL != nil {
		urlStr := ia.Issue.URL.String()
		return &urlStr
	}
	return nil
}

func (activity *IssueActivity) AfterFind(tx *gorm.DB) error {
	return EntityActivityAfterFind(activity, tx)
}

func (activity *IssueActivity) BeforeDelete(tx *gorm.DB) error {
	return tx.Where("issue_activity_id = ?", activity.Id).Unscoped().Delete(&UserNotifications{}).Error
}

func (activity *IssueActivity) ToLightDTO() *dto.EntityActivityLight {
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
		EntityType: "issue",

		NewEntity: GetActionEntity(*activity, "New"),
		OldEntity: GetActionEntity(*activity, "Old"),

		//TargetUser: activity.AffectedUser.ToLightDTO(),

		EntityUrl: activity.GetUrl(),
	}
}

func (activity *IssueActivity) ToDTO() *dto.EntityActivityFull {
	if activity == nil {
		return nil
	}

	return &dto.EntityActivityFull{
		EntityActivityLight: *activity.ToLightDTO(),
		Workspace:           activity.Workspace.ToLightDTO(),
		Actor:               activity.Actor.ToLightDTO(),
		Issue:               activity.Issue.ToLightDTO(),
		Project:             activity.Project.ToLightDTO(),
		NewIdentifier:       activity.NewIdentifier,
		OldIdentifier:       activity.OldIdentifier,

		//NewEntity:           GetActionEntity(*activity, "New"),
		//OldEntity:           GetActionEntity(*activity, "Old"),
		//TargetUser:          activity.AffectedUser.ToLightDTO(),
	}
}

func (activity *IssueActivityWithLag) ToDTOWithLag() *dto.EntityActivityFull {
	if activity == nil {
		return nil
	}
	d := activity.ToDTO()
	d.StateLag = activity.StateLag
	return d
}

// ToDTO преобразует объект RulesLog в структуру dto.RulesLog для упрощения передачи данных в клиентский код.
//
// Параметры:
//   - rl: объект RulesLog, который необходимо преобразовать.
//
// Возвращает:
//   - *dto.RulesLog: структура dto.RulesLog, содержащая преобразованные данные.
func (rl *RulesLog) ToDTO() *dto.RulesLog {
	if rl == nil {
		return nil
	}

	return &dto.RulesLog{
		Id:           rl.Id.String(),
		CreatedAt:    rl.CreatedAt,
		Project:      rl.Project.ToLightDTO(),
		Workspace:    rl.Workspace.ToLightDTO(),
		Issue:        rl.Issue.ToLightDTO(),
		User:         rl.User.ToLightDTO(),
		Time:         rl.Time,
		FunctionName: rl.FunctionName,
		Type:         rl.Type,
		Msg:          rl.Msg,
		LuaErr:       rl.LuaErr,
	}
}
