// Пакет aiplan предоставляет структуры данных и функции для работы с планами и задачами в системе.
// Он включает в себя модели для создания и управления проектами, рабочими пространствами, а также для фильтрации и поиска задач.
//
// Основные возможности:
//   - Создание и настройка проектов.
//   - Создание и настройка рабочих пространств.
//   - Фильтрация задач по различным критериям (пространства, проекты, поиск).
//   - Подготовка поисковых запросов для эффективного поиска задач.
//   - Обработка даты и времени для представления дней.
//   - Работа с идентификаторами задач.
package aiplan

import (
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

type CreateProjectRequest struct {
	Name               string   `json:"name" validate:"projectName"`
	Public             bool     `json:"public"`
	Identifier         string   `json:"identifier" validate:"identifier"`
	DefaultAssigneeIds []string `json:"default_assignees"`
	DefaultWatcherIds  []string `json:"default_watchers"`
	ProjectLeadId      string   `json:"project_lead"`
	Emoji              int32    `json:"emoji,string"`
	CoverImage         *string  `json:"cover_image"`
	EstimateId         *string  `json:"estimate"`
	RulesScript        *string  `json:"rules_script"`
}

func (req *CreateProjectRequest) Bind(project *dao.Project) {
	project.Name = req.Name
	project.Public = req.Public
	project.Identifier = req.Identifier
	project.DefaultAssignees = req.DefaultAssigneeIds
	project.DefaultWatchers = req.DefaultWatcherIds
	projectLeadUUID, _ := uuid.FromString(req.ProjectLeadId)
	project.ProjectLeadId = projectLeadUUID
	project.Emoji = req.Emoji
	project.CoverImage = req.CoverImage
	project.EstimateId = req.EstimateId
	project.RulesScript = req.RulesScript
}

type CreateWorkspaceRequest struct {
	Name    string  `json:"name" validate:"workspaceName"`
	Logo    *string `json:"logo"`
	Slug    string  `json:"slug" validate:"slug"`
	OwnerId string  `json:"owner_id"`
}

func (req *CreateWorkspaceRequest) Bind(workspace *dao.Workspace) {
	workspace.Name = req.Name
	workspace.Logo = req.Logo
	workspace.Slug = req.Slug
	ownerUUID, _ := uuid.FromString(req.OwnerId)
	workspace.OwnerId = ownerUUID
}

type ReactionRequest struct {
	Reaction string `json:"reaction" validate:"required"`
}
type FilterParams struct {
	WorkspaceIDs []string `json:"workspace_ids,omitempty"`
	ProjectIDs   []string `json:"project_ids,omitempty"`
	SearchQuery  string   `json:"search_query,omitempty"`
}

func ExtractFilterRequest(c echo.Context) (*dao.User, FilterParams, int, int, error) {
	user := c.(AuthContext).User

	offset := -1
	limit := 100
	if err := echo.QueryParamsBinder(c).
		Int("offset", &offset).
		Int("limit", &limit).BindError(); err != nil {
		return user, FilterParams{}, offset, limit, EError(c, err)
	}

	var param FilterParams
	if err := c.Bind(&param); err != nil {
		return user, FilterParams{}, offset, limit, EError(c, err)
	}

	return user, param, offset, limit, nil
}

func PrepareSearchRequest(query string) string {
	query = strings.ReplaceAll(query, `\`, `\\`)
	query = strings.ReplaceAll(query, `%`, `\%`)
	query = strings.ReplaceAll(query, `_`, `\_`)

	return "%" + strings.ToLower(query) + "%"
}

type DayRequest time.Time

func (d *DayRequest) UnmarshalText(text []byte) error {
	t, err := time.Parse("02012006", string(text))
	if err != nil {
		return err
	}
	*d = DayRequest(t)
	return nil
}
