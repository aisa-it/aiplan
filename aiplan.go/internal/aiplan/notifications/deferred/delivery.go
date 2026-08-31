package deferred

import (
	"encoding/json"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

type deliveryMap map[string]map[string]dao.DeliveryStatus

func loadDeliveryMap(payload json.RawMessage) deliveryMap {
	var p struct {
		Delivery deliveryMap `json:"delivery"`
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &p); err != nil {
			slog.Error("v2 loadDeliveryMap: failed to unmarshal payload", "err", err)
		}
	}
	if p.Delivery == nil {
		return make(deliveryMap)
	}
	return p.Delivery
}

func (d deliveryMap) ensureUser(userID string) map[string]dao.DeliveryStatus {
	entry, ok := d[userID]
	if !ok {
		entry = make(map[string]dao.DeliveryStatus)
		d[userID] = entry
	}
	return entry
}

func (d deliveryMap) hasPending() bool {
	for _, userDelivery := range d {
		for _, status := range userDelivery {
			if status == dao.DeliveryNotAttempted || (!status.IsDelivered() && !status.IsExhausted()) {
				return true
			}
		}
	}
	return false
}

func pendingChannelsFor(requested []string, userDelivery map[string]dao.DeliveryStatus) []string {
	var pending []string
	for _, ch := range requested {
		status, ok := userDelivery[ch]
		if !ok || status == dao.DeliveryNotAttempted || (!status.IsDelivered() && !status.IsExhausted()) {
			pending = append(pending, ch)
		}
	}
	return pending
}

func nextDeliveryStatus(current dao.DeliveryStatus, err error) dao.DeliveryStatus {
	if err != nil {
		return current + 1
	}
	return dao.DeliverySuccess
}
