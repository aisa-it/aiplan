// Определяет структуры данных для представления конфигурации просмотра (View) в приложении. Используется для сериализации и десериализации данных View в формате JSON.
//
// Основные возможности:
//   - Определение структуры ViewProps для хранения параметров просмотра, таких как отображение групп, подзадач, активные элементы, фильтры и т.д.
//   - Определение структуры ViewFilters для хранения параметров фильтрации данных просмотра.
//   - Реализация методов для преобразования структур ViewProps и ViewFilters в JSON и обратно для удобной работы с API.
package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

type ViewProps struct {
	ShowEmptyGroups bool            `json:"showEmptyGroups"`
	HideSubIssues   bool            `json:"hideSubIssues"`
	ShowOnlyActive  bool            `json:"showOnlyActive"`
	AutoSave        bool            `json:"autoSave"`
	IssueView       string          `json:"issueView" validate:"omitempty,oneof=list kanban calendar gantt_chart"`
	Filters         ViewFilters     `json:"filters"`
	GroupTablesHide map[string]bool `json:"group_tables_hide"`
	Columns         []string        `json:"columns_to_show"`
	ActiveTab       string          `json:"activeTab,omitempty"`
	PageSize        *int            `json:"page_size,omitempty" extensions:"x-nullable"`
	Draft           bool            `json:"draft"`
}

func (vp ViewProps) Value() (driver.Value, error) {
	b, err := json.Marshal(vp)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (vp *ViewProps) Scan(value interface{}) error {
	if value == nil {
		*vp = ViewProps{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	return json.Unmarshal(bytes, vp)
}

type ViewFilters struct {
	OrderBy   string `json:"order_by,omitempty"`
	GroupBy   string `json:"group_by,omitempty"`
	OrderDesc bool   `json:"orderDesc"`

	States     []string `json:"states"`
	Workspaces []string `json:"workspaces"`
	Projects   []string `json:"projects"`

	AssignedToMe bool `json:"assignedToMe"`
	WatchedToMe  bool `json:"watchedToMe"`
	AuthoredToMe bool `json:"authoredToMe"`
}
