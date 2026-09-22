package engine

// Action — предметное действие над сущностью.
//
// Права выражаются через действия, а не через метод и путь роута:
// путь знает только ядро, а движок должен рассуждать в терминах
// предметной области. Кроме того, один и тот же движок обслуживает
// HTTP и MCP, где никаких путей нет.
//
// Список покрывает точки расширения, а не каждый роут: служебные ручки
// (настройки уведомлений «для себя», тариф, сводка) правилами движка не
// управляются. Для задач покрытие полное — каждый роут issue-группы
// обязан иметь действие, это проверяется при старте сервера.
type Action string

// Задача: просмотр и жизненный цикл.
const (
	ActionIssueView Action = "issue.view"
	// ActionIssueSearch — поиск и списки задач. Отличается от issue.view:
	// проверяется до того, как известна конкретная задача, и работает в
	// паре с VisibilityPolicy.
	ActionIssueSearch   Action = "issue.search"
	ActionIssueCreate   Action = "issue.create"
	ActionIssueDelete   Action = "issue.delete"
	ActionIssueBulkEdit Action = "issue.bulk.edit"

	// ActionIssueUpdate — право редактировать задачу вообще.
	// Проверяется до разбора тела запроса; отдельные поля проверяются
	// действиями issue.*.set уже в обработчике.
	ActionIssueUpdate Action = "issue.update"

	ActionIssueViewActivity Action = "issue.activity.view"
	ActionIssueExport       Action = "issue.export"
	ActionIssueMigrate      Action = "issue.migrate"
)

// Задача: отдельные поля. Проверяются после разбора тела запроса,
// когда известно, что именно меняется.
const (
	ActionIssueSetState     Action = "issue.state.set"
	ActionIssueSetAssignees Action = "issue.assignees.set"
	ActionIssueSetWatchers  Action = "issue.watchers.set"
	ActionIssueSetLabels    Action = "issue.labels.set"
	ActionIssueSetProperty  Action = "issue.property.set"
	ActionIssueSetParent    Action = "issue.parent.set"
	ActionIssueSetSprint    Action = "issue.sprint.set"

	// Связи, изменяемые вместе с задачей. Отличаются от
	// issue.relation.manage: то — отдельные ручки управления связями,
	// а это правка состава связей в теле запроса на изменение задачи.
	ActionIssueSetBlockers  Action = "issue.blockers.set"
	ActionIssueSetLinked    Action = "issue.linked.set"
	ActionIssueSetSubIssues Action = "issue.subissues.set"
)

// Задача: вложенные сущности.
const (
	ActionIssueCommentView   Action = "issue.comment.view"
	ActionIssueCommentCreate Action = "issue.comment.create"
	ActionIssueCommentUpdate Action = "issue.comment.update"
	ActionIssueCommentDelete Action = "issue.comment.delete"
	ActionIssueCommentReact  Action = "issue.comment.react"

	ActionIssueAttachmentView   Action = "issue.attachment.view"
	ActionIssueAttachmentAdd    Action = "issue.attachment.add"
	ActionIssueAttachmentDelete Action = "issue.attachment.delete"

	// ActionIssueLinkManage — внешние ссылки задачи.
	ActionIssueLinkManage Action = "issue.link.manage"
	// ActionIssueRelationManage — связи между задачами:
	// подзадачи, родитель, блокировки, связанные.
	ActionIssueRelationManage Action = "issue.relation.manage"

	ActionIssuePin             Action = "issue.pin"
	ActionIssueDescriptionLock Action = "issue.description.lock"
)

// Проект. Помимо самого проекта движок управляет его справочниками:
// статусы и скрипт правил определяют бизнес-процесс, поэтому вынесены
// отдельными действиями.
const (
	ActionProjectView     Action = "project.view"
	ActionProjectCreate   Action = "project.create"
	ActionProjectUpdate   Action = "project.update"
	ActionProjectDelete   Action = "project.delete"
	ActionProjectAdmin    Action = "project.admin"
	ActionProjectArchive  Action = "project.archive"
	ActionProjectStats    Action = "project.stats.view"
	ActionProjectActivity Action = "project.activity.view"

	ActionProjectMemberView   Action = "project.member.view"
	ActionProjectMemberManage Action = "project.member.manage"

	// ActionProjectStateManage — статусы и их граф переходов.
	ActionProjectStateManage Action = "project.state.manage"
	// ActionProjectRulesManage — скрипт правил проекта.
	ActionProjectRulesManage Action = "project.rules.manage"

	ActionProjectLabelManage      Action = "project.label.manage"
	ActionProjectEstimateManage   Action = "project.estimate.manage"
	ActionProjectTemplateManage   Action = "project.template.manage"
	ActionProjectPropertyManage   Action = "project.property.manage"
	ActionProjectDictionaryView   Action = "project.dictionary.view"
	ActionProjectDictionaryManage Action = "project.dictionary.manage"
	ActionProjectViewManage       Action = "project.view.manage"

	// ActionProjectJoin — вступление в проекты пространства.
	ActionProjectJoin Action = "project.join"
	// ActionProjectSelfSettings — личные настройки участника в проекте (уведомления).
	ActionProjectSelfSettings Action = "project.self.settings"
	// ActionProjectRulesLogView — журнал срабатываний скрипта правил.
	ActionProjectRulesLogView Action = "project.rules.log.view"
)

// Пространство.
const (
	ActionWorkspaceView     Action = "workspace.view"
	ActionWorkspaceCreate   Action = "workspace.create"
	ActionWorkspaceUpdate   Action = "workspace.update"
	ActionWorkspaceDelete   Action = "workspace.delete"
	ActionWorkspaceAdmin    Action = "workspace.admin"
	ActionWorkspaceActivity Action = "workspace.activity.view"

	ActionWorkspaceMemberView   Action = "workspace.member.view"
	ActionWorkspaceMemberManage Action = "workspace.member.manage"
	ActionWorkspaceInvite       Action = "workspace.invite"

	ActionWorkspaceIntegrationManage Action = "workspace.integration.manage"
	ActionWorkspaceTokenManage       Action = "workspace.token.manage"
	ActionWorkspaceTokenView         Action = "workspace.token.view"
	ActionWorkspaceBackup            Action = "workspace.backup"
	ActionWorkspaceBackupView        Action = "workspace.backup.view"
	ActionWorkspaceImport            Action = "workspace.import"

	// ActionWorkspaceSelfSettings — личные настройки участника в пространстве:
	// уведомления, избранное.
	ActionWorkspaceSelfSettings Action = "workspace.self.settings"
)

// Спринт.
const (
	// ActionSprintList — список спринтов пространства (спринта в запросе нет).
	ActionSprintList         Action = "sprint.list"
	ActionSprintView         Action = "sprint.view"
	ActionSprintCreate       Action = "sprint.create"
	ActionSprintUpdate       Action = "sprint.update"
	ActionSprintDelete       Action = "sprint.delete"
	ActionSprintAdmin        Action = "sprint.admin"
	ActionSprintActivity     Action = "sprint.activity.view"
	ActionSprintIssueManage  Action = "sprint.issue.manage"
	ActionSprintWatchManage  Action = "sprint.watcher.manage"
	ActionSprintSearchIssues Action = "sprint.issue.search"
	// ActionSprintViewManage — личное представление спринта пользователя.
	ActionSprintViewManage   Action = "sprint.view.manage"
	ActionSprintFolderManage Action = "sprint.folder.manage"
)

// Документ.
const (
	// ActionDocList и ActionDocCreateRoot — корень дерева документов
	// пространства (документа в запросе нет).
	ActionDocList       Action = "doc.list"
	ActionDocCreateRoot Action = "doc.create.root"
	ActionDocView       Action = "doc.view"
	ActionDocCreate     Action = "doc.create"
	// ActionDocAccessManage — смена ролей и списков доступа документа.
	ActionDocAccessManage   Action = "doc.access.manage"
	ActionDocUpdate         Action = "doc.update"
	ActionDocDelete         Action = "doc.delete"
	ActionDocMove           Action = "doc.move"
	ActionDocActivity       Action = "doc.activity.view"
	ActionDocHistoryView    Action = "doc.history.view"
	ActionDocCommentView    Action = "doc.comment.view"
	ActionDocCommentCreate  Action = "doc.comment.create"
	ActionDocCommentUpdate  Action = "doc.comment.update"
	ActionDocCommentDelete  Action = "doc.comment.delete"
	ActionDocCommentReact   Action = "doc.comment.react"
	ActionDocAttachmentView Action = "doc.attachment.view"
	ActionDocAttachmentAdd  Action = "doc.attachment.add"
	ActionDocAttachmentDel  Action = "doc.attachment.delete"
)

// Форма.
const (
	ActionFormView          Action = "form.view"
	ActionFormCreate        Action = "form.create"
	ActionFormUpdate        Action = "form.update"
	ActionFormDelete        Action = "form.delete"
	ActionFormAnswer        Action = "form.answer"
	ActionFormAnswerView    Action = "form.answer.view"
	ActionFormAttachmentAdd Action = "form.attachment.add"
)
