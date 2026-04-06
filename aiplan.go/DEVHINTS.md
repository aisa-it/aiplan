## Интеграция фронтенда в сборку

### Build tags

Используются два файла с взаимоисключающими build tags:

**embedFront.go** (`//go:build embedSPA`):
```go
//go:embed pwa
var frontFS embed.FS
```
- Активируется при сборке с флагом `-tags embedSPA`
- Встраивает содержимое папки `pwa` в бинарник через `embed.FS`

**externalFront.go** (`//go:build !embedSPA`):
```go
var frontFS embed.FS  // пустая
```
- Активируется по умолчанию (без тега)
- `frontFS` пустая — фронтенд обслуживается через переменную окружения `FRONT_PATH`

### Условия сборки

| Режим | Build tag | Фронтенд | Использование |
|-------|-----------|----------|---------------|
| Production (Dagger CI) | `-tags embedSPA` | Встроен в бинарник | Docker образы для деплоя |
| Development | без тега | Внешний (`FRONT_PATH`) | Локальная разработка |

### Dagger CI (dagger-ci/main.go)

```go
// Сборка фронтенда
front := m.FrontBuildEnv(version, source.Directory("aiplan-front/"))

// Сборка бэкенда с embedded SPA
builder := m.GoBuildEnv(source).
    WithExec([]string{"go", "build", "-tags", "embedSPA", ...})

// Контейнер с обоими вариантами
m.BackEnv(...).
    WithDirectory("/app/spa", front.Directory("/src/dist/pwa")).
    WithEnvVariable("FRONT_PATH", "/app/spa")
```

В production контейнере:
- Фронтенд встроен в бинарник (через `embedSPA`)
- Также копируется в `/app/spa` как fallback

### Локальная разработка

```bash
# Бэкенд (без embedded фронтенда)
go build ./cmd/aiplan
FRONT_PATH=../aiplan-front/dist/pwa ./aiplan

# Фронтенд отдельно
cd aiplan-front && yarn dev
```

## Роли участников в проекте

Старые роли:

| Значение | Роль          |
| -------- | ------------- |
| 5        | Гость         |
| 10       | Наблюдатель   |
| 15       | Участник      |
| 20       | Администратор |

Новые роли:

| Значение | Роль          |
| -------- | ------------- |
| 5        | Гость         |
| 10       | Участник      |
| 15       | Администратор |

## Поиск задач

Методы:

```
POST /api/issues/search/
POST /api/workspaces/:workspaceSlug/projects/:projectId/issues/
GET /api/workspaces/:workspaceSlug/my-issues/ LEGACY
```

### Параметры в пути

| Параметр          | Описание                               | Тип    | Стандартное значение |
| ----------------- | -------------------------------------- | ------ | -------------------- |
| `show_sub_issues` | Показывать дочерние задачи             | bool   | true                 |
| `order_by`        | Поле для сортировки                    | string | "sequence_id"        |
| `groupBy`         | Поле для группировки. **УСТАРЕЛ**      | string | ""                   |
| `offset`          | Офсет для пагинации                    | int    | -1                   |
| `limit`           | Лимит для пагинации                    | int    | 100                  |
| `desc`            | Порядок сортировки. true - по убыванию | bool   | true                 |

Поля доступные для сортировки: `id, created_at, updated_at, name, priority, target_date, sequence_id, state, labels, sub_issues_count, link_count, attachment_count, assignees, watchers, author`.

### Тело запроса

```json
{
  "authors": [""],
  "assignees": [""],
  "watchers": [""],

  "states": [""],
  "priorities": [""],
  "labels": [""],
  "workspaces": [""],
  "workspace_slugs": [""],

  "assigned_to_me": false,
  "watched_by_me": false,
  "only_active": false,

  "search_query": ""
}
```

| Параметр          | Описание                                                                           | Тип      | Стандартное значение |
| ----------------- | ---------------------------------------------------------------------------------- | -------- | -------------------- |
| `authors`         | Список id авторов                                                                  | []string | []                   |
| `states`          | Список id состояний                                                                | []string | []                   |
| `priorities`      | Список id приоритетов. "" для задач без приоритета.                                | []string | []                   |
| `labels`          | Список id тегов                                                                    | []string | []                   |
| `workspaces`      | Список id пространств                                                              | []string | []                   |
| `workspace_slugs` | Список slug пространств                                                            | []string | []                   |
| `assigned_to_me`  | Только задачи назначенные на текущего пользователя                                 | bool     | false                |
| `watched_by_me`   | Только задачи в наблюдении у текущего пользователя                                 | bool     | false                |
| `only_active`     | "Активные" задачи не в статусах "Завершена" и "Отменена"                           | bool     | false                |
| `search_query`    | Поисковой запрос для названия и описания задачи. Если пустой - поиск не происходит | string   | ""                   |

Если в запросе **не** указаны `workspaces` и `workspace_slugs` и `:workspaceSlug` в пути, то:

- Если не стафф и не суперпользователь - возвращаются все задачи из всех пространств **пользователя**.
- Если стафф или суперпользователь - возвращаются все задачи из **всех** пространств

## View Props

```json
{
  {
    "filters":
      {
        "assignees": "State",
        "created_by": "Manual"
      },
      "issueView": "list",
      "created_at": "-created_at",
      "showSubIssues": true,
      "showEmptyGroups": true,
      "state_tables_hide": {
        "stateId": true
      },
      "columns_to_show": [""]
  }
}
```

## Перенос задач

Методы:

```
POST /api/workspaces/:workspaceSlug/issues/migrate/
POST /api/workspaces/:workspaceSlug/issues/migrate/byLabel/
```

### Перенос задачи

`POST /api/workspaces/:workspaceSlug/issues/migrate/`

Параметры в URL:

| Параметр         | Описание                | Тип    |
| ---------------- | ----------------------- | ------ |
| `target_project` | ID проекта назначения   | string |
| `src_issue`      | ID исходной задачи      | string |
| `linked_issues`  | Переносить семью задачи | bool   |
| `delete_src`     | Удалять исходные задачи | bool   |

### Перенос задач по тегу

`POST /api/workspaces/:workspaceSlug/issues/migrate/byLabel/`

Параметры в URL:

| Параметр         | Описание                | Тип    |
| ---------------- | ----------------------- | ------ |
| `target_project` | ID проекта назначения   | string |
| `src_label`      | ID исходного тега       | string |
| `linked_issues`  | Переносить семью задачи | bool   |
| `delete_src`     | Удалять исходные задачи | bool   |

Если тег не назначен ни одной задаче - возвращается статус 304.

### Тело ответа

При возникновении ошибок валидации приходит тело следующего содержания:

```json
{
  "errors": [
    {
      "error": "текст ошибки",
      "src_issue_id": "id исходной задачи с ошибкой",
      "type": "тип сущности с ошибкой",
      "entities": ["массив ID сущностей с ошибкой"]
    }
  ]
}
```

### Возможные ошибки

- `target project not found`
- `issue not found`
- `issues with conflicted names`
- `label not found`
- `source author not a target project member`
- `you are not a target project member`
- `source assignees that not a members of target project`
- `source watchers that not a members of target project`
- `source state that does not exist in target project`
- `source labels that does not exist in target project`

## Кастомные поля задач (Property Templates)

Система дополнительных полей для задач на уровне проекта.

### Архитектура

Двухуровневая модель:

```
Project 1--* ProjectPropertyTemplate 1--* IssueProperty *--1 Issue
```

- **ProjectPropertyTemplate** — шаблон поля на уровне проекта (определяет какие поля доступны)
- **IssueProperty** — значение поля для конкретной задачи

### Поддерживаемые типы

| Тип       | Описание        | Значение по умолчанию |
| --------- | --------------- | --------------------- |
| `string`  | Текстовое поле  | `""`                  |
| `boolean` | Логическое поле | `false`               |

### API эндпоинты

**Шаблоны (ProjectPropertyTemplate):**

```
GET    /api/auth/workspaces/:ws/projects/:proj/property-templates/
POST   /api/auth/workspaces/:ws/projects/:proj/property-templates/       (только админ)
PATCH  /api/auth/workspaces/:ws/projects/:proj/property-templates/:id/   (только админ)
DELETE /api/auth/workspaces/:ws/projects/:proj/property-templates/:id/   (только админ)
```

**Значения (IssueProperty):**

```
GET    /api/auth/workspaces/:ws/projects/:proj/issues/:issueId/properties/
POST   /api/auth/workspaces/:ws/projects/:proj/issues/:issueId/properties/:templateId/
```

### Особенности

- **GET properties** возвращает ВСЕ шаблоны проекта со значениями (или дефолтами если не установлено)
- **POST property** — upsert: создаёт или обновляет значение
- **OnlyAdmin** — поля с этим флагом могут редактировать только админы проекта
- Валидация значений через JSON Schema

## Activity Tracker

Система генерации событий активности при изменениях сущностей.

**Функция создания записи о событии:**

`TrackEvent[E dao.IDaoAct](tracker *ActTracker, layer types.EntityLayer, activityAction string, ctx *Ctx, entity E, actor *dao.User) error`

**принимает**
- tracker *ActTracker
- layer types.EntityLayer
    - `LayerWorkspace` — пространства
    - `LayerProject` — проекты
    - `LayerIssue` — задачи
    - `LayerDoc` — документы
    - `LayerForm` — формы
    - `LayerSprint` — спринты
- activityAction string (verbs)
    - `VerbUpdated` = "updated" (обновление полей)
    - `VerbRemoved` = "removed" (удаление связанной сущности)
    - `VerbAdded`   = "added" (добавление связанной сущности (assignee, watcher, label))
    - `VerbDeleted` = "deleted" (удаление сущности)
    - `VerbCreated` = "created" (создание сущности)
    - `VerbMove`    = "move" (перемещение между родительскими сущностями)
    - `VerbCopied`  = "copied" (создание в результате копирования)

- ctx *Ctx ([контекст трекера](#контекст-трекера))
- entity E (обновленная сущность реализующая интерфейс [dao.IDaoAct](#интерфейс-daoidaoact))
- actor *dao.User (пользователь совершивший изменение)

### Контекст трекера

```go
type Ctx struct {
    New        ActivitySide  // изменяемые данные запроса (request)
    Old        ActivitySide  // изменяемые данные старой структуры (current)
    GormMap    *map[string]interface{} // данные из БД (новые значения)
    OldGormMap *map[string]interface{} // данные из БД (старые значения)
}

// GormMap и OldGormMap используется для обновления данных в базе gorm.Update()
// New и Old для корректировки названия полей при записи в лог активности 

type ActivityCtx struct {
    Tracker    *ActTracker
    TrackerCtx *Ctx
    RequestedData   DataEntity  // merge  GormMap и New
    CurrentInstance DataEntity  // merge  OldGormMap и Old
    Actor           *dao.User
    Layer           types.EntityLayer
}
```

**Ключи контекста (из activity-fields.go):**
- `UpdateScopeKey` — scope для генерации составного имени поля (например, `label_name`)
- `ParentKey` — поле родительской сущности для операций move
- `ParentTitleKey` — название родительской сущности для логов
- `EntityKey` — основная сущность для операции
- `CustomVerbKey` — кастомный глагол
- `OldTitleKey` — старое значение title при удалении
- `FieldMoveKey` — тип перемещения для поля verb ("move_workspace_to_doc", "move_doc_to_doc")

**FieldKind (суффиксы полей):**
- `KindLogValue` — отображаемое значение (`{field}_activity_val`)
- `KindScopeID` — ID области видимости (`{field}_updateScopeId`)
- `KindTransform` — функция трансформации (`{field}_func_name`)
- `KindLookup` — поле для получения из внешнего источника (`{field}_get_field`)
- `KindLogOverride` — переопределённое имя поля (`field_log`)
- `KindCustomKey` — кастомный ключ (`{field}_key`)

### интерфейс dao.IDaoAct

```go
type IDaoAct interface {
	GetId() uuid.UUID //id сущности
	GetString() string // строковое представление для записи в поле new
	GetEntityType() actField.ActivityField // тип сущности для записи в поле field
}
```

### добавление нового слоя (EntityLayer)

Добавить новый entity_type:  
дополнить triggers.sql CHECK constraint entity_fk_check новый тип
внести изменения для существующих entity_type

```sql
ALTER TABLE activity_events DROP CONSTRAINT IF EXISTS entity_fk_check;

ALTER TABLE activity_events
    ADD CONSTRAINT entity_fk_check
        CHECK (
            -- существующие слои...
            (entity_type = 7 AND          -- NEW: новый слой (например, LayerCustom)
             workspace_id IS NOT NULL AND
             project_id IS NULL AND
             issue_id IS NULL AND
             doc_id IS NULL AND
             sprint_id IS NULL AND
             form_id IS NULL AND
             custom_id IS NOT NULL)       -- новое поле в таблице
        )
```

**Примеры использования**

-  изменения роли документа

```go
ctx := tracker.NewTrackerCtx(&requestedMap, &currentMap)

ctx.New.SetKey(actField.ReaderRole.Field.AsLogValue(), fmt.Sprint(newDoc.ReaderRole))
ctx.Old.SetKey(actField.ReaderRole.Field.AsLogValue(), fmt.Sprint(oldDocMap["reader_role"]))
//AsLogValue ожидает string для записи в поля new_value^ old_value 
err := tracker.TrackEvent(trackerInst, types.LayerDoc, actField.VerbUpdated, ctx, newDoc, actor)
```

## notifications/member-role

**Роль**: Управление ролями участников и их настройками уведомлений.

**Роли участников:**

```go
const (
    ProjectAdminRole = 1 << iota   // администратор проекта
    ProjectMemberRole              // участник проекта
    ProjectGuestRole               // гость проекта

    ProjectDefaultWatcher          // наблюдатель по умолчанию
    ProjectDefaultAssigner         // исполнитель по умолчанию

    IssueAuthor                    // автор задачи
    IssueWatcher                  // наблюдатель задачи
    IssueAssigner                  // исполнитель задачи
    IssueCommentCreator           // автор комментария

    CommentMentioned              // упоминаемый в комментарии

    WorkspaceAdminRole            // администратор пространства
    WorkspaceMemberRole           // участник пространства
    WorkspaceGuestRole            // гость пространства

    DocAuthor                      // автор документа
    DocWatcher                    // наблюдатель документа
    DocEditor                     // редактор документа
    DocReader                     // читатель документа

    SprintAuthor                  // автор спринта
    SprintWatcher                 // наблюдатель спринта

    ActionAuthor                  // автор действия
)
```

**Тип MemberNotify:**

```go
type MemberNotify struct {
    user                   *dao.User
    loc                    types.TimeZone
    authorProjectSettings   *projectMemberNotifies   // настройки проекта для автора 
    memberProjectSettings   *projectMemberNotifies  // настройки проекта для участника  
    authorWorkspaceSettings *workspaceMemberNotifies // настройки пространства для автора
    memberWorkspaceSettings *workspaceMemberNotifies // настройки пространства для участника  
    roles                  Role
}
```

**UsersStep** — функция добавления пользователей в реестр:

```go
type UsersStep func(tx *gorm.DB, users UserRegistry) error
```

**Функции добавления пользователей:**
- `AddUserRole(actor *dao.User, role Role)` — добавить автора действия
- `AddIssueUsers(issue *dao.Issue)` — добавить автора, исполнителей и наблюдателей задачи
- `AddDefaultWatchers(projectId uuid.UUID)` — добавить наблюдателей по умолчанию
- `AddProjectAdmin(projectId uuid.UUID)` — добавить администраторов проекта
- `AddWorkspaceAdmins(workspaceId uuid.UUID)` — добавить администраторов пространства
- `AddDocMembers(docID uuid.UUID)` — добавить участников документа
- `AddOriginalCommentAuthor(act *dao.ActivityEvent)` — добавить автора исходного комментария
- `AddCommentMentionedUsers(comment)` — добавить упоминаемых пользователей (парсит `@username` из HTML)

## notifications/event.go

**Роль**: Сервис нотификаций — обрабатывает события активности и отправляет уведомления.

**Интерфейс EventHandler:**

```go
type EventHandler interface {
    CanHandle(event *dao.ActivityEvent) bool
    Preload(tx *gorm.DB, event *dao.ActivityEvent) error
    GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep
    GetSettingsFunc() member_role.IsNotifyFunc
    AuthorRole() member_role.Role
    FilterRecipients(event *dao.ActivityEvent, recipients []member_role.MemberNotify) []member_role.MemberNotify
}
```

**Процесс обработки события:**
1. Получить `EventHandler` по типу сущности
2. Вызвать `Preload()` для загрузки связанных данных
3. Определить получателей через `GetRecipientsSteps()`
4. Отфильтровать получателей через `FilterRecipients()`
5. Для каждого канала проверить настройки пользователя (`GetSettingsFunc()`)
6. Отправить уведомление через канал

**Примеры использования**

- Определение получателей для уведомления о задаче

```go
func (issueEvent) GetRecipientsSteps(event *dao.ActivityEvent) []member_role.UsersStep {
    return []member_role.UsersStep{
        member_role.AddUserRole(event.Issue.Author, member_role.IssueAuthor),
        member_role.AddIssueUsers(event.Issue),
        member_role.AddOriginalCommentAuthor(event),
        member_role.AddCommentMentionedUsers(event.NewIssueComment),
        member_role.AddDefaultWatchers(event.ProjectID.UUID),
    }
}
```