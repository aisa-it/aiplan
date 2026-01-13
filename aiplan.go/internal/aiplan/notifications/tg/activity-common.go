package tg

import (
	"fmt"
	"net/url"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
)

func finalizeActivityTitle(msg TgMsg, actor, entity string, url *url.URL) TgMsg {
	msg.title = fmt.Sprintf(
		"*%s* %s [%s](%s)",
		bot.EscapeMarkdown(actor),
		bot.EscapeMarkdown(msg.title),
		bot.EscapeMarkdown(entity),
		url.String(),
	)
	return msg
}

func formatByField[T dao.ActivityI, F ~func(*T, actField.ActivityField) TgMsg](
	act *T,
	m map[actField.ActivityField]F,
	defaultFn F,
) (TgMsg, error) {
	var res TgMsg

	if (*act).GetField() == "" {
		return res, fmt.Errorf("%s field is nil", (*act).GetEntity())
	}

	af := actField.ActivityField((*act).GetField())

	if f, ok := m[af]; ok {
		res = f(act, af)
	} else if defaultFn != nil {
		res = defaultFn(act, af)
	}

	if res.IsEmpty() {
		return res, fmt.Errorf("%s activity is empty", (*act).GetEntity())
	}

	return res, nil
}
