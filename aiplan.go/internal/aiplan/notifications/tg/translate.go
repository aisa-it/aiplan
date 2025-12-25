package tg

import actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"

var fieldsTranslation map[actField.ActivityField]string = map[actField.ActivityField]string{
	actField.Name.Field:          "Название",
	actField.Parent.Field:        "Родитель",
	actField.Priority.Field:      "Приоритет",
	actField.Status.Field:        "Статус",
	actField.Description.Field:   "Описание",
	actField.TargetDate.Field:    "Срок исполнения",
	actField.StartDate.Field:     "Дата начала",
	actField.CompletedAt.Field:   "Дата завершения",
	actField.Label.Field:         "Теги",
	actField.Assignees.Field:     "Исполнители",
	actField.Blocking.Field:      "Блокирует",
	actField.Blocks.Field:        "Заблокирована",
	actField.EstimatePoint.Field: "Оценки",
	actField.SubIssue.Field:      "Подзадачи",
	actField.Identifier.Field:    "Идентификатор",
	actField.Emoj.Field:          "Emoji",
	actField.Title.Field:         "Название",
}

var priorityTranslation map[string]string = map[string]string{
	"urgent": "Критический",
	"high":   "Высокий",
	"medium": "Средний",
	"low":    "Низкий",
	"":       "Не выбран",
}

var statusTranslation map[string]string = map[string]string{
	"backlog":   "Создано",
	"unstarted": "Не начато",
	"cancelled": "Отменено",
	"completed": "Завершено",
	"started":   "Начато",
}

var roleTranslation map[string]string = map[string]string{
	"5":  "Гость",
	"10": "Участник",
	"15": "Администратор",
}

func translateMap(tMap map[string]string, str *string) string {
	key := "<nil>"
	if str != nil {
		key = *str
	}
	if v, ok := tMap[key]; ok {
		return v
	}
	return ""
}
