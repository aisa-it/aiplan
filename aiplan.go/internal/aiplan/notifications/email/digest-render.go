package email

import (
	"bytes"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm"
)

type FuncFieldRender[A dao.ActivityI, E dao.IDaoAct] func(tx *gorm.DB, t *EmailTemplates, acts []A, entity E) FieldPrerender

// Generic renderDigest
func renderDigest[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB, templates *EmailTemplates, activities []A, entity E,
	fieldRenderMap map[actField.ActivityField]FuncFieldRender[A, E],
	collectors map[actField.ActivityField]activityFieldCollector[A],
) (map[string]FieldPrerender, int) {

	result := make(map[string]FieldPrerender)
	totalChanges := 0

	// собираем digest по полям
	digest := CollectActivitiesByField(activities, collectors)

	for field, acts := range digest {
		if renderFunc, ok := fieldRenderMap[actField.ActivityField(field)]; ok {
			fp := renderFunc(tx, templates, acts, entity)
			if fp.Count > 0 {
				fp.Field = actField.ActivityField(field)
				result[field] = fp
				totalChanges += fp.Count
			}
		}
	}

	return result, totalChanges
}

func renderEntityChange[A dao.ActivityI, E dao.IDaoAct](
	tx *gorm.DB, t *EmailTemplates, acts []A, current []E, key string, spec entitySpec[A, E],
) FieldPrerender {

	views, meta, count := BuildEntityChangeDigest(tx, acts, current, spec)

	ctx := collectAllCtx{
		Key:    key,
		Views:  views,
		Start:  meta.Start,
		End:    meta.End,
		Author: meta.Authors,
	}

	return t.RenderCollectAll(ctx, count)
}

func renderHead(
	templates *EmailTemplates, ddd headEntityCtx,

) string {

	var buf bytes.Buffer
	if err := templates.HeadEntity.Execute(&buf, ddd); err != nil {
		slog.Error("err", err.Error())
		return ""
	}
	return buf.String()
}
