package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"github.com/gofrs/uuid"
)

func parentUpdate[E dao.IDaoAct](field actField.ActivityField) func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
	return func(c *ActivityCtx, entity E) ([]dao.ActivityEvent, error) {
		result := make([]dao.ActivityEvent, 0)
		issue, ok := any(entity).(dao.Issue)
		if !ok {
			return result, nil
		}

		newParentId := GetAsOrDefault[uuid.UUID](c.RequestedData, field.AsKey(), uuid.Nil)
		oldParentId := GetAsOrDefault[uuid.NullUUID](c.CurrentInstance, field.AsKey(), uuid.NullUUID{})

		if !oldParentId.Valid && newParentId.IsNil() {
			return result, nil
		}

		ids := []uuid.UUID{issue.ID}

		if oldParentId.Valid {
			ids = append(ids, oldParentId.UUID)
		}
		if !newParentId.IsNil() {
			ids = append(ids, newParentId)
		}

		var issues []dao.Issue
		if err := c.Tracker.db.Preload("Project").Where("id in (?)", ids).Find(&issues).Error; err != nil {
			return result, err
		}

		issueMap := utils.SliceToMap(&issues, func(v *dao.Issue) uuid.UUID {
			return v.ID
		})

		var changes []activityChange[dao.Issue]
		issueStr := issue.GetString()

		addChange := func(verb string, is dao.Issue) {
			change := activityChange[dao.Issue]{
				verb:   verb,
				field:  actField.SubIssue.Field,
				entity: is,
			}
			if verb == actField.VerbAdded {
				change.oldVal = nil
				change.newVal = issueStr
				change.newID = uuid.NullUUID{UUID: issue.ID, Valid: true}
			} else { // VerbRemoved
				change.oldVal = utils.ToPtr(issueStr)
				change.newVal = ""
				change.oldID = uuid.NullUUID{UUID: issue.ID, Valid: true}
			}
			changes = append(changes, change)
		}

		if !oldParentId.Valid && !newParentId.IsNil() {
			c.RequestedData[actField.Parent.Field.AsLogValue().String()] = issueMap[newParentId].GetString()
			c.CurrentInstance[actField.Parent.Field.AsLogValue().String()] = "<nil>"
			addChange(actField.VerbAdded, issueMap[newParentId])
		} else if newParentId.IsNil() && oldParentId.Valid {
			c.CurrentInstance[actField.Parent.Field.AsLogValue().String()] = issueMap[oldParentId.UUID].GetString()
			addChange(actField.VerbRemoved, issueMap[oldParentId.UUID])
		} else if !newParentId.IsNil() && oldParentId.Valid {
			entityIRem := issueMap[oldParentId.UUID]
			entityIAdd := issueMap[newParentId]
			c.CurrentInstance[actField.Parent.Field.AsLogValue().String()] = entityIRem.GetString()
			c.RequestedData[actField.Parent.Field.AsLogValue().String()] = entityIAdd.GetString()
			addChange(actField.VerbRemoved, entityIRem)
			addChange(actField.VerbAdded, entityIAdd)
		}

		events, err := buildEvents(c, changes)
		if err != nil {
			return nil, err
		}

		getNullUidFromUUID := func(id uuid.UUID) uuid.NullUUID {
			if id.IsNil() {
				return uuid.NullUUID{}
			}
			return uuid.NullUUID{UUID: id, Valid: true}
		}

		activityEvents, err := fieldUpdate[E](c, field, getNullUidFromUUID(newParentId), oldParentId, entity)
		if err != nil {
			return nil, err
		}

		return append(events, activityEvents...), nil
	}
}
