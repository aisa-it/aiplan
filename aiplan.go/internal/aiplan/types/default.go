// Содержит набор дефолтных значений для различных настроек приложения. Используется для инициализации и обеспечения консистентности конфигурации.
//
// Основные возможности:
//   - Предоставляет дефолтные значения для UserSetttings, ViewProps, Theme и других типов.
//   - Упрощает инициализацию объектов конфигурации, избегая повторения одинаковых значений.
//   - Обеспечивает единую точку для определения базовых настроек приложения.
package types

import "time"

var DefaultSettings UserSettings = UserSettings{
	DeadlineNotification:  24 * time.Hour,
	TgNotificationMute:    false,
	EmailNotificationMute: false,
	AppNotificationMute:   false,
}

var defaultPageSize int = 25
var boolTrue = true
var boolFalse = true

var DefaultViewProps ViewProps = ViewProps{
	ShowEmptyGroups: false,
	ShowSubIssues:   true,
	ShowOnlyActive:  false,
	AutoSave:        false,
	IssueView:       "list",
	Filters: ViewFilters{
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

var DefaultTheme = Theme{
	System:    &boolTrue,
	Dark:      nil,
	Contrast:  nil,
	OpenInNew: &boolFalse,
}
