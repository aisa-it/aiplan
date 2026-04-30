package tracker

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

type ActHandler interface {
	Handle(activity dao.ActivityEvent) error
}

//
//func confSkipper(act dao.ActivityEvent, requestedData map[string]interface{}) dao.ActivityEvent {
//	switch act.EntityType {
//
//	case types.LayerIssue:
//		if v, ok := requestedData["tg_sender"]; ok {
//			if val, intOk := v.(int64); intOk {
//				act.SenderTg = val
//			}
//		}
//	case types.LayerDoc:
//		if v, ok := requestedData["tg_sender"]; ok {
//			if val, intOk := v.(int64); intOk {
//				act.SenderTg = val
//			}
//		}
//	default:
//		return act
//	}
//	return act
//}
