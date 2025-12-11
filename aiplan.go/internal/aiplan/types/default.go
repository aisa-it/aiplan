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
	HideSubIssues:   false,
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
	Columns: []string{
		"priority",
		"state",
		"target_date",
		"created_at",
		"updated_at",
		"author",
		"assignees",
		"labels",
		"sub_issues_count",
		"linked_issues_count",
		"link_count",
		"attachment_count",
		"name",
	},
	GroupTablesHide: make(map[string]bool),
	ActiveTab:       "all",
	PageSize:        &defaultPageSize,
	Draft:           true,
}

var DefaultProjectMemberNS ProjectMemberNS = ProjectMemberNS{
	DisableName:          false,
	DisableDesc:          false,
	DisableState:         false,
	DisableAssignees:     false,
	DisableWatchers:      false,
	DisablePriority:      false,
	DisableParent:        false,
	DisableBlocks:        false,
	DisableBlockedBy:     false,
	DisableTargetDate:    false,
	DisableLabels:        false,
	DisableLinks:         false,
	DisableComments:      false,
	DisableAttachments:   false,
	DisableDeadline:      false,
	DisableLinked:        false,
	DisableSubIssue:      false,
	NotifyBeforeDeadline: nil,
	DisableIssueTransfer: false,
	DisableIssueNew:      false,

	DisableProjectName:            true,
	DisableProjectPublic:          true,
	DisableProjectIdentifier:      true,
	DisableProjectDefaultAssignee: true,
	DisableProjectDefaultWatcher:  true,
	DisableProjectMember:          true,
	DisableProjectOwner:           true,
	DisableProjectRole:            true,
	DisableProjectStatus:          true,
	DisableProjectLabel:           true,
	DisableProjectLogo:            true,
	DisableProjectTemplate:        true,
}

var DefaultWorkspaceMemberNS = WorkspaceMemberNS{
	DisableDocTitle:      false,
	DisableDocDesc:       false,
	DisableDocRole:       false,
	DisableDocAttachment: false,
	DisableDocComment:    false,
	DisableDocWatchers:   false,
	DisableDocCreate:     false,
	DisableDocDelete:     false,
	DisableDocMove:       false,

	DisableWorkspaceProject:     true,
	DisableWorkspaceForm:        true,
	DisableWorkspaceDoc:         true,
	DisableWorkspaceName:        true,
	DisableWorkspaceOwner:       true,
	DisableWorkspaceDesc:        true,
	DisableWorkspaceToken:       true,
	DisableWorkspaceLogo:        true,
	DisableWorkspaceMember:      true,
	DisableWorkspaceRole:        true,
	DisableWorkspaceIntegration: true,
}

var DefaultTheme = Theme{
	System:    &boolTrue,
	Dark:      nil,
	Contrast:  nil,
	OpenInNew: &boolFalse,
}
