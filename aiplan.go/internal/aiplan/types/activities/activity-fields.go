package activities

import (
	"database/sql/driver"
	"fmt"
)

type TrackKey string

const (
	updateScope  = "updateScope"
	parentKey    = "parent_key"
	entity       = "entity"
	entityParent = "entityParent"
	customVerb   = "custom_verb"
	oldTitle     = "old_title"
	parentTitle  = "parent_title"
	newEntity    = "new_entity"
	oldEntity    = "old_entity"
	fieldMove    = "field_move"
)

const (
	// KindLogValue — суффикс для значения поля, записываемого в лог activity.
	//  Используется для хранения отображаемого значения (而不是 исходного).
	//  - key: "{field}_activity_val"
	//  - Пример: "attachment_activity_val" = "file.pdf"
	KindLogValue FieldKind = "activity_val"

	// KindScopeID — суффикс для ID (области видимости).
	//  Используется для хранения идентификатора, к которому относится поле.
	//  - key: "{field}_updateScopeId"
	//  - Пример: state_updateScopeId = "uuid"
	KindScopeID FieldKind = "updateScopeId"

	// KindTransform — суффикс для функции трансформации значения поля.
	//  Используется для хранения имени функции-форматера значения.
	//  - key: "{field}_func_name"
	//  - Пример: "start_date_func_name" = "formatDate"
	KindTransform FieldKind = "func_name"

	// KindLookup — суффикс для поля, значение которого нужно получить из внешнего источника.
	//  Используется для получения связанных данных по ID из другой сущности.
	//  - key: "{field}_get_field"
	//  - Пример: "project_lead_get_field" -> получить email по user_id
	KindLookup FieldKind = "get_field"

	// KindLogOverride — переопределённое имя поля для записи в лог.
	//  Используется когда фактическое поле одно, а в лог пишется другое.
	//  - key: "field_log"
	//  - Пример: фактическое поле "description_html", в лог пишется как "description"
	KindLogOverride FieldKind = "field_log"

	// KindCustomKey — суффикс для кастомного ключа поля.
	//  Используется для указания альтернативного имени поля в запросе.
	//  - key: "{field}_key"
	//  - Пример: "entity_key" = "custom_entity_name"
	KindCustomKey FieldKind = "key"

	// KindEmpty — пустой суффикс (без суффикса).
	//  Используется по умолчанию, когда поле не имеет дополнительного суффикса.
	//  - key: "{field}" (без суффикса)
	KindEmpty FieldKind = ""
)

type FieldKey struct {
	Field ActivityField
	Kind  FieldKind
}

type FieldKind string

func (k FieldKind) AsField() ActivityField {
	return ActivityField(k)
}

func (fk FieldKey) String() string {
	if fk.Kind == "" {
		return fk.Field.String()
	}
	return fmt.Sprintf("%s_%s", fk.Field.String(), fk.Kind)
}

func NewKey[A ~string](str A) FieldKey {
	return FieldKey{ActivityField(str), KindEmpty}
}

var (

	// UpdateScopeKey — префикс scope'а для формирования составного имени поля.
	//  Используется для генерации ключей вида "{scope}_{field}".
	//  - key: "updateScope"
	//  - Пример: scope="label" + val="name" → "label_name"
	UpdateScopeKey = NewKey(updateScope)

	// ParentKey — ключ поля родительской сущности.
	//  Используется в операциях move для указания какое поле является родительским.
	//  - key: "parent_key"
	//  - Пример: значение "project_id" означает что parent это project
	ParentKey = NewKey(parentKey)

	// EntityParentKey — ключ родительской сущности.
	//  Используется для указания какой объект является родительским для текущей сущности.
	//  - key: "entityParent"
	//  - Пример: при перемещении issue в project, здесь указывается project_id
	EntityParentKey = NewKey(entityParent)

	// ParentTitleKey — название/заголовок родительской сущности.
	//  Используется для отображения имени родителя в логах и уведомлениях.
	//  - key: "parent_title"
	//  - Пример: "Project Name" или "Sprint 42"
	ParentTitleKey = NewKey(parentTitle)

	// NewEntityKey — новая сущность или значение после изменения.
	//  Используется в операциях move/перемещения для указания нового расположения.
	//  - key: "new_entity"
	//  - Пример: new_parent_id или new_project_id
	NewEntityKey = NewKey(newEntity)

	// OldEntityKey — старая сущность или значение до изменения.
	//  Используется в операциях move/перемещения для указания предыдущего расположения.
	//  - key: "old_entity"
	//  - Пример: old_parent_id или old_project_id
	OldEntityKey = NewKey(oldEntity)

	// FieldMoveKey — поле перемещения (какое именно поле перемещается).
	//  Используется для указания типа перемещения (внутри сущности или между сущностями).
	//  - key: "field_move"
	//  - Пример: "status", "project_id", "sprint"
	FieldMoveKey = NewKey(fieldMove)

	// EntityKey — основная сущность для операции.
	//  Используется для указания над какой сущностью совершается действие.
	//  - key: "entity"
	EntityKey = NewKey(entity)

	// CustomVerbKey — кастомный глагол вместо стандартного (added/updated/deleted).
	//  Используется для создания специфичных activity events.
	//  - key: "custom_verb"
	//  - Пример: "copied", "moved"
	CustomVerbKey = NewKey(customVerb)

	// OldTitleKey — старое значение title при удалении сущности.
	//  Используется для записи в лог что именно было удалено.
	// - key: "old_title"
	OldTitleKey = NewKey(oldTitle)
)

type ActivityField string

func (f ActivityField) Value() (driver.Value, error) {
	return string(f), nil
}

func (f *ActivityField) Scan(value interface{}) error {
	if value == nil {
		*f = ActivityField("")
		return nil
	}

	switch v := value.(type) {
	case string:
		*f = ActivityField(v)
	case []byte:
		*f = ActivityField(v)
	case fmt.Stringer:
		*f = ActivityField(v.String())
	default:
		return fmt.Errorf("cannot scan type %T into ActivityField: %v", value, value)
	}

	return nil
}

func New[E ~string](field E) ActivityField {
	return ActivityField(field)
}

func (a ActivityField) String() string {
	return string(a)
}

// AsLogValue — для добавления суффикса к ActivityField
// из константы KindLogValue
//   - Пример: { ActivityField }_activity_val
func (a ActivityField) AsLogValue() FieldKey {
	return FieldKey{Field: a, Kind: KindLogValue}
}

// WithFunc — для добавления суффикса к ActivityField
// из константы KindTransform
//   - Пример: { ActivityField }_func_name
func (a ActivityField) WithFunc() FieldKey {
	return FieldKey{Field: a, Kind: KindTransform}
}

// LookupFrom — для добавления суффикса к ActivityField
// из константы KindLookup
//   - Пример: { ActivityField }_get_field
func (a ActivityField) LookupFrom() FieldKey {
	return FieldKey{Field: a, Kind: KindLookup}
}

// LogAs — для добавления суффикса к ActivityField
// из константы KindLogOverride
//   - Пример: { ActivityField }_field_log
func (a ActivityField) LogAs() FieldKey {
	return FieldKey{Field: a, Kind: KindLogOverride}
}

// WithScopeID — для добавления суффикса к ActivityField
// из константы KindScopeID
//   - передавать uuid
//   - Пример: { ActivityField }_updateScopeId
func (a ActivityField) WithScopeID() FieldKey {
	return FieldKey{Field: a, Kind: KindScopeID}
}

// WithKey — для добавления суффикса к ActivityField
// из константы KindCustomKey
//   - Пример: { ActivityField }_key
func (a ActivityField) WithKey() FieldKey {
	return FieldKey{Field: a, Kind: KindCustomKey}
}

// AsKey — для без суффикса ActivityField как ключ
// из константы KindEmpty
//   - Пример:ActivityField
func (a ActivityField) AsKey() FieldKey {
	return FieldKey{Field: a, Kind: KindEmpty}
}

type FieldMapping struct {
	Req   string
	Field ActivityField
}

func makeFieldMap(actFs ...FieldMapping) map[string]string {
	m := make(map[string]string)
	for _, act := range actFs {
		if act.Req != "" {
			m[act.Req] = act.Field.String()
		}
	}
	return m
}

var (
	fieldChange = makeFieldMap(
		Comment, DescriptionHtml, Status,
		Readers, Editors, Watchers, Assignees,
		Linked, Issues, Blocks, Blocking)

	Issue = FieldMapping{"", "issue"}
	Doc   = FieldMapping{"", "doc"}

	Comment     = FieldMapping{"comment_html", "comment"}
	CommentHtml = FieldMapping{"comment_html", "comment_html"}

	Readers   = FieldMapping{"readers_list", "readers"}
	Editors   = FieldMapping{"editors_list", "editors"}
	Watchers  = FieldMapping{"watchers_list", "watchers"}
	Assignees = FieldMapping{"assignees_list", "assignees"}
	Linked    = FieldMapping{"linked_issues_ids", "linked"}
	Issues    = FieldMapping{"issue_list", "issues"}

	Blocks   = FieldMapping{"blocks_list", "blocks"}
	Blocking = FieldMapping{"blockers_list", "blocking"}

	Attachment = FieldMapping{"", "attachment"}

	Description     = FieldMapping{"description", "description"}
	DescriptionHtml = FieldMapping{"description_html", "description"}

	Title            = FieldMapping{"title", "title"}
	Emoj             = FieldMapping{"emoji", "emoji"}
	Public           = FieldMapping{"public", "public"}
	Identifier       = FieldMapping{"identifier", "identifier"}
	ProjectLead      = FieldMapping{"project_lead", "project_lead"}
	Priority         = FieldMapping{"priority", "priority"}
	Role             = FieldMapping{"role", "role"}
	ReaderRole       = FieldMapping{"reader_role", "reader_role"}
	EditorRole       = FieldMapping{"editor_role", "editor_role"}
	Status           = FieldMapping{"state", "status"}
	DefaultAssignees = FieldMapping{"default_assignees", "default_assignees"}
	DefaultWatchers  = FieldMapping{"default_watchers", "default_watchers"}

	SubIssue = FieldMapping{"", "sub_issue"}

	Token         = FieldMapping{"integration_token", "integration_token"}
	Owner         = FieldMapping{"owner_id", "owner"}
	Logo          = FieldMapping{"logo", "logo"}
	Parent        = FieldMapping{"parent", "parent"}
	Default       = FieldMapping{"default", "default"}
	EstimatePoint = FieldMapping{"estimate_point", "estimate_point"}
	Url           = FieldMapping{"url", "url"}
	DocSort       = FieldMapping{"doc_sort", "doc_sort"}
	Name          = FieldMapping{"name", "name"}
	Template      = FieldMapping{"template", "template"}
	Color         = FieldMapping{"color", "color"}
	TargetDate    = FieldMapping{"target_date", "target_date"}
	StartDate     = FieldMapping{"start_date", "start_date"}
	CompletedAt   = FieldMapping{"completed_at", "completed_at"}
	EndDate       = FieldMapping{"end_date", "end_date"}
	Label         = FieldMapping{"labels_list", "label"}
	AuthRequire   = FieldMapping{"auth_require", "auth_require"}
	Fields        = FieldMapping{"fields", "fields"}
	Group         = FieldMapping{"group", "group"}
	Sprint        = FieldMapping{"sprint", "sprint"}

	Member = FieldMapping{"", "member"}

	Link      = FieldMapping{"", "link"} //
	LinkTitle = FieldMapping{"", "link_title"}
	LinkUrl   = FieldMapping{"", "link_url"}

	LabelName  = FieldMapping{"", "label_name"} // todo <<<
	LabelColor = FieldMapping{"", "label_color"}

	StatusColor       = FieldMapping{"", "status_color"} // todo <<<
	StatusGroup       = FieldMapping{"", "status_group"}
	StatusDescription = FieldMapping{"", "status_description"}
	StatusName        = FieldMapping{"", "status_name"}
	StatusDefault     = FieldMapping{"", "status_default"}

	TemplateName     = FieldMapping{"", "template_name"} // todo <<<
	TemplateTemplate = FieldMapping{"", "template_template"}

	Project  = FieldMapping{"", "project"}
	Deadline = FieldMapping{"", "deadline"}

	Form           = FieldMapping{"", "form"}
	Integration    = FieldMapping{"", "integration"}
	WorkspaceOwner = FieldMapping{"", "owner"}
	IssueTransfer  = FieldMapping{"", "issue_transfer"}
	Workspace      = FieldMapping{"", "workspace"}
)

const (
	EntityUpdatedActivity = "entity.updated"
	EntityCreateActivity  = "entity.create"
	EntityDeleteActivity  = "entity.delete"
	EntityAddActivity     = "entity.add"
	EntityRemoveActivity  = "entity.remove"
	EntityMoveActivity    = "entity.move"

	VerbUpdated = "updated"
	VerbRemoved = "removed"
	VerbAdded   = "added"
	VerbDeleted = "deleted"
	VerbCreated = "created"
	VerbMove    = "move"
	VerbCopied  = "copied"

	VerbMoveDocWorkspace = "move_doc_to_workspace"
	VerbMoveDocDoc       = "move_doc_to_doc"
	VerbMoveWorkspaceDoc = "move_workspace_to_doc"
)

func ReqFieldMapping(in string) string {
	if v, ok := fieldChange[in]; ok {
		return v
	}
	return in
}
