package tg

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

func genComment[R dao.IRedactorHTML](comment *R, oldV *string, verb, titleUpdate, titleCreate, titleDelete string) TgMsg {
	msg := NewTgMsg()

	if comment != nil {
		msg.body = Stelegramf("```\n%s```",
			utils.HtmlToTg((*comment).GetRedactorHtml().Body),
		)
	} else {
		if oldV != nil {
			msg.body = Stelegramf("```\n%s```",
				utils.HtmlToTg(*oldV))
		}
	}
	msg.replace[userMentioned] = struct{}{}

	switch verb {
	case actField.VerbUpdated:
		msg.title = titleUpdate
	case actField.VerbCreated:
		msg.title = titleCreate
	case actField.VerbDeleted:
		msg.title = titleDelete
	}
	return msg
}

func genAttachment(oldV *string, newV, verb, titleCreate, titleDelete string) TgMsg {
	msg := NewTgMsg()
	switch verb {
	case actField.VerbCreated:
		msg.title = titleCreate
		msg.body += Stelegramf("*файл*: %s", newV)
	case actField.VerbDeleted:
		msg.title = titleDelete
		msg.body += Stelegramf("*файл*: ~%s~", *oldV)
	}
	return msg
}
