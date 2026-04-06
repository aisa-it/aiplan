package tg

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

func genComment[R dao.IRedactorHTML](comment *R, oldV *string, verb, titleUpdate, titleCreate, titleDelete string) TgMsg {
	msg := NewTgMsg()

	if comment != nil {
		msg.Body = Stelegramf("```\n%s```",
			utils.HtmlToTg((*comment).GetRedactorHtml().Body),
		)
	} else {
		if oldV != nil {
			msg.Body = Stelegramf("```\n%s```",
				utils.HtmlToTg(*oldV))
		}
	}
	msg.Replace[userMentioned] = struct{}{}

	switch verb {
	case actField.VerbUpdated:
		msg.Title = titleUpdate
	case actField.VerbCreated:
		msg.Title = titleCreate
	case actField.VerbDeleted:
		msg.Title = titleDelete
	}
	return msg
}

func genAttachment(oldV *string, newV, verb, titleCreate, titleDelete string) TgMsg {
	msg := NewTgMsg()
	switch verb {
	case actField.VerbCreated:
		msg.Title = titleCreate
		msg.Body += Stelegramf("*Файл*: %s", newV)
	case actField.VerbDeleted:
		msg.Title = titleDelete
		msg.Body += Stelegramf("*Файл*: ~%s~", *oldV)
	}
	return msg
}

func genDefault(oldV *string, newV string, af actField.ActivityField, title string) TgMsg {
	msg := NewTgMsg()

	msg.Title = title

	if oldV != nil {
		msg.Body += Stelegramf("*%s*: ~%s~ %s", types.FieldsTranslation[af], *oldV, newV)
	} else {
		msg.Body += Stelegramf("*%s*: %s", types.FieldsTranslation[af], newV)
	}
	return msg
}
