package tracker

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/gofrs/uuid"
)

type activityChange[E dao.IDaoAct] struct {
	verb   string
	field  actField.ActivityField
	oldVal *string
	newVal string
	newID  uuid.NullUUID
	oldID  uuid.NullUUID
	entity E
}

func buildEvents[E dao.IDaoAct](c *ActivityCtx, changes []activityChange[E]) ([]dao.ActivityEvent, error) {
	result := make([]dao.ActivityEvent, 0, len(changes))
	for _, ch := range changes {
		ev := NewActivityEvent(ch.verb, ch.field, ch.oldVal, ch.newVal, ch.newID, ch.oldID, c.Actor)
		if err := SetEntityRefs(c.Layer, &ev, ch.entity); err != nil {
			return nil, fmt.Errorf("set entity refs failed: %w", err)
		}
		result = append(result, ev)
	}

	return result, nil
}
