// Содержит структуры данных (DTO) для представления статистики проекта.
// Используется для сериализации/десериализации данных в формате JSON.
//
// Основные возможности:
//   - Агрегированная статистика задач проекта
//   - Распределение по статусам, приоритетам, меткам
//   - Статистика по исполнителям и спринтам
//   - Временная динамика создания/завершения задач
package dto

import (
	"time"

	"github.com/gofrs/uuid"
	"github.com/labstack/echo/v4"
)

// ProjectStatsRequest представляет параметры запроса статистики проекта.
// Используется для указания какие секции статистики нужно включить в ответ.
type ProjectStatsRequest struct {
	// IncludeAssigneeStats включить статистику по исполнителям (топ-50)
	IncludeAssigneeStats bool `json:"include_assignee_stats,omitempty"`

	// IncludeLabelStats включить статистику по меткам (топ-50)
	IncludeLabelStats bool `json:"include_label_stats,omitempty"`

	// IncludeSprintStats включить статистику по спринтам (последние 50)
	IncludeSprintStats bool `json:"include_sprint_stats,omitempty"`

	// IncludeTimeline включить временную статистику (создано/завершено по месяцам за 12 месяцев)
	IncludeTimeline bool `json:"include_timeline,omitempty"`
}

func (psr *ProjectStatsRequest) FromHTTPQuery(c echo.Context) error {
	psr.IncludeAssigneeStats = true
	psr.IncludeLabelStats = true
	psr.IncludeSprintStats = true
	psr.IncludeTimeline = true
	return echo.QueryParamsBinder(c).
		Bool("include_assignee_stats", &psr.IncludeAssigneeStats).
		Bool("include_label_stats", &psr.IncludeLabelStats).
		Bool("include_sprint_stats", &psr.IncludeSprintStats).
		Bool("include_timeline", &psr.IncludeTimeline).BindError()
}

// ProjectStatsProject представляет базовую информацию о проекте в статистике.
type ProjectStatsProject struct {
	// ID идентификатор проекта
	ID uuid.UUID `json:"id"`

	// Name название проекта
	Name string `json:"name"`

	// Identifier короткий идентификатор проекта (например, "PORTAL")
	Identifier string `json:"identifier"`

	// TotalMembers общее количество участников проекта
	TotalMembers int `json:"total_members"`
}

// IssueCounters представляет общие счётчики задач.
type IssueCounters struct {
	// Total общее количество задач
	Total int `json:"total"`

	// Active количество активных задач (не completed и не cancelled)
	Active int `json:"active"`

	// Completed количество завершённых задач
	Completed int `json:"completed"`

	// Cancelled количество отменённых задач
	Cancelled int `json:"cancelled"`
}

// StateStatItem представляет статистику по одному статусу.
type StateStatItem struct {
	// StateID идентификатор статуса
	StateID uuid.UUID `json:"state_id"`

	// Name название статуса
	Name string `json:"name"`

	// Group группа статуса (backlog, unstarted, started, completed, cancelled)
	Group string `json:"group"`

	// Color цвет статуса в HEX формате
	Color string `json:"color"`

	// Count количество задач с этим статусом
	Count int `json:"count"`
}

// PriorityStats представляет распределение задач по приоритетам.
type PriorityStats struct {
	// Urgent срочные задачи
	Urgent int `json:"urgent"`

	// High высокий приоритет
	High int `json:"high"`

	// Medium средний приоритет
	Medium int `json:"medium"`

	// Low низкий приоритет
	Low int `json:"low"`

	// None без приоритета
	None int `json:"none"`
}

// StateGroupStats представляет распределение задач по группам статусов.
type StateGroupStats struct {
	// Backlog задачи в бэклоге
	Backlog int `json:"backlog"`

	// Unstarted задачи не начатые
	Unstarted int `json:"unstarted"`

	// Started задачи в работе
	Started int `json:"started"`

	// Completed завершённые задачи
	Completed int `json:"completed"`

	// Cancelled отменённые задачи
	Cancelled int `json:"cancelled"`
}

// OverdueStats представляет статистику просроченных задач.
type OverdueStats struct {
	// Count количество просроченных задач
	Count int `json:"count"`

	// OldestDate дата самой старой просроченной задачи
	OldestDate *time.Time `json:"oldest_date,omitempty" extensions:"x-nullable"`
}

// AssigneeStatItem представляет статистику задач одного исполнителя.
type AssigneeStatItem struct {
	// UserID идентификатор пользователя
	UserID uuid.UUID `json:"user_id"`

	// DisplayName отображаемое имя пользователя
	DisplayName string `json:"display_name"`

	// Active количество активных задач
	Active int `json:"active"`

	// Completed количество завершённых задач
	Completed int `json:"completed"`
}

// LabelStatItem представляет статистику по одной метке.
type LabelStatItem struct {
	// LabelID идентификатор метки
	LabelID uuid.UUID `json:"label_id"`

	// Name название метки
	Name string `json:"name"`

	// Color цвет метки в HEX формате
	Color string `json:"color"`

	// Count количество задач с этой меткой
	Count int `json:"count"`
}

// SprintStatItem представляет статистику по одному спринту.
type SprintStatItem struct {
	// SprintID идентификатор спринта
	SprintID uuid.UUID `json:"sprint_id"`

	// Name название спринта
	Name string `json:"name"`

	// Total общее количество задач в спринте
	Total int `json:"total"`

	// Completed количество завершённых задач в спринте
	Completed int `json:"completed"`
}

// MonthlyCount представляет количество за месяц.
type MonthlyCount struct {
	// Month месяц в формате "YYYY-MM"
	Month string `json:"month"`

	// Count количество
	Count int `json:"count"`
}

// TimelineStats представляет временную статистику.
type TimelineStats struct {
	// CreatedByMonth количество созданных задач по месяцам
	CreatedByMonth []MonthlyCount `json:"created_by_month"`

	// CompletedByMonth количество завершённых задач по месяцам
	CompletedByMonth []MonthlyCount `json:"completed_by_month"`
}

// ProjectStats представляет полную статистику проекта.
type ProjectStats struct {
	// Project базовая информация о проекте
	Project ProjectStatsProject `json:"project"`

	// Issues общие счётчики задач
	Issues IssueCounters `json:"issues"`

	// ByState распределение по статусам
	ByState []StateStatItem `json:"by_state"`

	// ByPriority распределение по приоритетам
	ByPriority PriorityStats `json:"by_priority"`

	// ByStateGroup распределение по группам статусов
	ByStateGroup StateGroupStats `json:"by_state_group"`

	// Overdue статистика просроченных задач
	Overdue OverdueStats `json:"overdue"`

	// AssigneeStats статистика по исполнителям (опционально, топ-50)
	AssigneeStats []AssigneeStatItem `json:"assignee_stats,omitempty" extensions:"x-nullable"`

	// LabelStats статистика по меткам (опционально, топ-50)
	LabelStats []LabelStatItem `json:"label_stats,omitempty" extensions:"x-nullable"`

	// SprintStats статистика по спринтам (опционально, последние 50)
	SprintStats []SprintStatItem `json:"sprint_stats,omitempty" extensions:"x-nullable"`

	// Timeline временная статистика (опционально, за 12 месяцев)
	Timeline *TimelineStats `json:"timeline,omitempty" extensions:"x-nullable"`
}
