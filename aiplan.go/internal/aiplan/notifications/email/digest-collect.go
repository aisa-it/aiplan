package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
)

type activityFieldCollector[T dao.ActivityI] func(T, map[string][]T)

func collectOne[T dao.ActivityI](act T, m map[string][]T) {
	key := act.GetField()

	if prev := m[key]; len(prev) > 0 {
		if act.GetCreatedAt().After(prev[0].GetCreatedAt()) {
			m[key] = []T{act}
		}
		return
	}

	m[key] = []T{act}
}

func collectAll[T dao.ActivityI](act T, m map[string][]T) {
	key := act.GetField()
	m[key] = append(m[key], act)
}

func CollectActivitiesByField[T dao.ActivityI](
	acts []T, collectors map[actField.ActivityField]activityFieldCollector[T],
) map[string][]T {

	result := make(map[string][]T)

	for _, act := range acts {
		key := act.GetField()
		collector, ok := collectors[actField.ActivityField(key)]
		if !ok {
			continue
		}

		collector(act, result)
	}

	return result
}
