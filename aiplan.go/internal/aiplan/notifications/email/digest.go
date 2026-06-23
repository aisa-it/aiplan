package email

import (
	"database/sql"
	"strings"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
	"gorm.io/gorm"
)

type FieldPrerender struct {
	prerenderType prerenderType
	Verb          string
	Field         actField.ActivityField

	Value        string
	ValueComplex map[uuid.UUID]string

	Count   int
	Authors []dao.User

	Start sql.NullTime
	End   sql.NullTime

	Replace     map[string]any
	ActivityIds []uuid.UUID
	ActivityMap map[uuid.UUID]dao.ActivityEvent
}

func collectOne(act dao.ActivityEvent, m map[string][]dao.ActivityEvent) {
	key := act.Field.String()
	if prev := m[key]; len(prev) > 0 {
		m[key] = []dao.ActivityEvent{prev[0], act}
		return
	}
	m[key] = []dao.ActivityEvent{act}
}

func collectAll(act dao.ActivityEvent, m map[string][]dao.ActivityEvent) {
	key := act.Field.String()
	m[key] = append(m[key], act)
}

func collectCompositeField(str string) func(act dao.ActivityEvent, m map[string][]dao.ActivityEvent) {
	return func(act dao.ActivityEvent, m map[string][]dao.ActivityEvent) {
		if strings.HasPrefix(act.Field.String(), str) {
			m[str] = append(m[str], act)
		}
	}
}

type activityFieldCollector func(dao.ActivityEvent, map[string][]dao.ActivityEvent)

type FieldRenderer func(tx *gorm.DB, t *EmailTemplates, acts []dao.ActivityEvent, entity dao.IDaoAct) FieldPrerender

type EntityFieldConfig struct {
	Collector activityFieldCollector
	Renderer  FieldRenderer
}

func renderDigest(tx *gorm.DB, templates *EmailTemplates, activities []dao.ActivityEvent, entity dao.IDaoAct,
	fieldConfigs map[actField.ActivityField]EntityFieldConfig) (map[string]FieldPrerender, int) {

	result := make(map[string]FieldPrerender)
	totalChanges := 0

	collectors := make(map[actField.ActivityField]activityFieldCollector)
	fieldRenderMap := make(map[string]FieldRenderer)

	for field, config := range fieldConfigs {
		collectors[field] = config.Collector
		fieldRenderMap[string(field)] = config.Renderer
	}

	// собираем digest по полям
	digest := collectActivitiesByField(activities, collectors)

	for field, acts := range digest {
		if renderFunc, ok := fieldRenderMap[field]; ok {
			fp := renderFunc(tx, templates, acts, entity)
			if fp.Count > 0 {
				fp.Field = actField.ActivityField(field)
				fp.ActivityIds = utils.SliceToSlice(&acts, func(t *dao.ActivityEvent) uuid.UUID { return t.ID })
				result[field] = fp
				totalChanges += fp.Count
			}
		}
	}

	return result, totalChanges
}

func collectActivitiesByField(acts []dao.ActivityEvent, collectors map[actField.ActivityField]activityFieldCollector,
) map[string][]dao.ActivityEvent {

	result := make(map[string][]dao.ActivityEvent)

	for _, act := range acts {
		collector, ok := collectors[act.Field]
		if !ok {
			continue
		}

		collector(act, result)
	}

	return result
}
