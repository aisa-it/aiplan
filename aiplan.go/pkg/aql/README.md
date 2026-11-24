# Требования к aql для создания файла конфигурации g4 antlr4

## Язык
Сравнения: =, !=, >, >=, <, <=, ~ (содержит), !~
Списки: IN (...), NOT IN (...)
Пустота: IS EMPTY, IS NOT EMPTY
Логика: AND, OR, NOT, скобки
Сортировка: ORDER BY поле [ASC|DESC], ...; без ORDER BY - по дате создания, сначала новые
Привычные имена из Jira: reporter и creator = author; issuetype = type; due = dueDate; resolved = completed; key, issuekey, id = issue; NULL = EMPTY
Имена должны работать и как в jira и аиплановские встроенные.

## Поля
|Поле|Значение|
|---|---|
|project, workspace | id или слаг проекта; пространство |
|issue | id или ключ(вид FRO-123) задачи; сравнение и списки |
|title, description | название, описание - полнотекстовый поиск |
|status, statusGroup | статус; группа статусов: backlog, unstarted, started, completed, cancelled |
|priority, labels | тип, приоритет, метки |
|assignee, author, watcher | пользователь по почте, логину или имени |
|parent | id или ключ(вид FRO-123) родителя задачи |
|sprint | id или название спринта |
|created, updated, completed, startDate, dueDate | даты |
|estimate, spent, remaining | время: 2ч 30м или 2h 30m (пока не делать)|
|worklogAuthor, worklogDate | есть запись времени этого автора за эту дату (пока не делать)|
|worklogTime | только для сортировки: сумма записей времени, подошедших под условия запроса; она же - столбец "Списано по условию" (пока не делать)|
|article | статья отчета: значение параметра "Статья отчета" у самой задачи или у ближайшего родителя; article IS EMPTY - задачи без статьи (пока не делать)|
|"Название параметра" | дополнительный параметр задачи по имени, например "Площадка" = "Пром" |
|links, blocks, blockedBy | связанные и блокирующие задачи |
|attachments, comment | есть вложения(флаг); текст комментария |

## Функции
|Функция|Значение|Пример|
|---|---|---|
|currentUser()|текущий пользователь|assignee = currentUser()|
|now(), startOfDay(), startOfWeek(), startOfMonth(), startOfYear(), endOf...()|дата; допускается сдвиг; взять функции из jira|created >= startOfMonth(-1), updated >= -7d|
|childrenOf("КЛЮЧ")|все вложенные подзадачи задачи, на любую глубину|issue IN childrenOf("WABP-16824")|
|linkedTo("КЛЮЧ")|задачи, связанные с указанной|issue IN linkedTo("WABP-1041")|
|membersOf("Команда")|участники команды с этим названием вместе с вложенными командами(пока не делать)|assignee IN membersOf("Шина")|
