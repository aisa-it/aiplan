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
