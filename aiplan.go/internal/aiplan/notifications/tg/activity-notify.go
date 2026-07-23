package tg

import (
	"fmt"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
)

type funcTgMsgFormat func(act *dao.ActivityEvent, af actField.ActivityField) TgMsg

func finalizeActivityTitle(msg TgMsg, actor, entity string, url *url.URL) TgMsg {
	msg.Title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		bot.EscapeMarkdown(actor),
		bot.EscapeMarkdown(msg.Title),
		bot.EscapeMarkdown(entity),
		url.String(),
	)
	return msg
}

func formatByField(
	act *dao.ActivityEvent, m map[actField.ActivityField]funcTgMsgFormat, defaultFn funcTgMsgFormat,
) (TgMsg, error) {
	var res TgMsg

	if f, ok := m[act.Field]; ok {
		res = f(act, act.Field)
	} else if defaultFn != nil {
		res = defaultFn(act, act.Field)
	}

	if res.IsEmpty() {
		return res, fmt.Errorf("%s %w, verb: %s, field: %s, id: %s", act.EntityType.String(), ErrEmptyActivity, act.Verb, act.Field.String(), act.ID)
	}

	return res, nil
}
