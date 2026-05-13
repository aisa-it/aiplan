package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
)

type activityFieldCollector func(dao.ActivityEvent, map[string][]dao.ActivityEvent)

func collectOne(act dao.ActivityEvent, m map[string][]dao.ActivityEvent) {
	key := act.Field.String()

	if prev := m[key]; len(prev) > 0 {
		if act.CreatedAt.After(prev[0].CreatedAt) {
			m[key] = []dao.ActivityEvent{act}
		}
		return
	}

	m[key] = []dao.ActivityEvent{act}
}

func collectAll(act dao.ActivityEvent, m map[string][]dao.ActivityEvent) {
	key := act.Field.String()
	m[key] = append(m[key], act)
}

func CollectActivitiesByField(
	acts []dao.ActivityEvent, collectors map[actField.ActivityField]activityFieldCollector,
) map[string][]dao.ActivityEvent {

	result := make(map[string][]dao.ActivityEvent)

	for _, act := range acts {
		key := act.Field.String()
		collector, ok := collectors[actField.ActivityField(key)]
		if !ok {
			continue
		}

		collector(act, result)
	}

	return result
}
