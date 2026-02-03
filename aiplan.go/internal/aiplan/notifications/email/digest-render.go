package email

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm"
)

type FuncFieldRender[A dao.ActivityI, E dao.IDaoAct] func(tx *gorm.DB, t *EmailTemplates, acts []A, entity E) (string, int)

// Generic renderDigest
func renderDigest[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB, templates *EmailTemplates, activities []A, entity E,
	fieldRenderMap map[actField.ActivityField]FuncFieldRender[A, E],
	collectors map[actField.ActivityField]activityFieldCollector[A],
) (map[string]fieldPrerender, int) {

	result := make(map[string]fieldPrerender)
	totalChanges := 0

	// собираем digest по полям
	digest := CollectActivitiesByField(activities, collectors)

	for field, acts := range digest {
		if renderFunc, ok := fieldRenderMap[actField.ActivityField(field)]; ok {
			val, cnt := renderFunc(tx, templates, acts, entity)
			if cnt > 0 {
				result[field] = fieldPrerender{
					Value: val,
					Count: cnt,
				}
				totalChanges += cnt
			}
		}
	}

	return result, totalChanges
}
