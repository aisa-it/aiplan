// Список моделей для AutoMigrate. Живёт рядом с моделями, а не в main,
// чтобы встраивающий модуль мог добавить к нему свои.
package dao

// models — модели ядра, мигрируемые при старте.
var models = []any{&ActivityEvent{}, &ActivityTelegramMessage{}, &CommentReaction{}, &DeferredNotifications{}, &Dictionary{}, &DictionaryRow{}, &Doc{}, &DocAccessRules{}, &DocAttachment{}, &DocComment{}, &DocCommentReaction{}, &DocFavorites{}, &Estimate{}, &EstimatePoint{}, &FileAsset{}, &ForeignKey{}, &Form{}, &FormAnswer{}, &FormAttachment{}, &ImportedProject{}, &Issue{}, &IssueAssignee{}, &IssueAttachment{}, &IssueBlocker{}, &IssueComment{}, &IssueDescriptionLock{}, &IssueLabel{}, &IssueLink{}, &IssueProperty{}, &IssueTemplate{}, &IssueWatcher{}, &JitsiTokenLog{}, &Label{}, &LinkedIssues{}, &NotifyService{}, &Project{}, &ProjectFavorites{}, &ProjectMember{}, &ProjectMemberWithLead{}, &ProjectPropertyTemplate{}, &ReleaseNote{}, &RulesLog{}, &SearchFilter{}, &SessionsReset{}, &Sprint{}, &SprintFolder{}, &SprintIssue{}, &SprintViews{}, &SprintWatcher{}, &State{}, &Team{}, &TeamMembers{}, &Template{}, &User{}, &UserAppNotify{}, &UserFeedback{}, &Workspace{}, &WorkspaceBackup{}, &WorkspaceFavorites{}, &WorkspaceMember{}, &WorkspaceMemberWithOwner{}}

// Models возвращает копию списка моделей ядра.
func Models() []any {
	return append([]any(nil), models...)
}
