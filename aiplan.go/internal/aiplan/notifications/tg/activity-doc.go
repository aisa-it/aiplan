package tg

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

var (
	docMap = map[actField.ActivityField]funcTgMsgFormat{
		actField.Description.Field: docDescription,
		actField.Doc.Field:         docDoc,

		actField.Readers.Field:  docMember,
		actField.Watchers.Field: docMember,
		actField.Editors.Field:  docMember,

		actField.ReaderRole.Field: docRole,
		actField.EditorRole.Field: docRole,

		actField.Comment.Field:    docComment,
		actField.Attachment.Field: docAttachment,

		actField.Title.Field: docDefault,
	}
)

func FormatDocActivity(act *dao.ActivityEvent) (TgMsg, error) {
	res, err := formatByField(act, docMap, nil)
	if err != nil {
		return res, err
	}

	docTitle := act.Doc.Title
	if act.Doc.ParentDocID.Valid {
		docTitle = Stelegramf("...%s", act.Doc.Title)
	}

	entity := fmt.Sprintf("%s/%s", act.Workspace.Slug, docTitle)
	res = finalizeActivityTitle(res, act.Actor.GetName(), entity, act.Doc.URL)

	return res, nil
}

func docDescription(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.Title = "изменил(-а) описание документа"
	msg.Body = Stelegramf("```\n%s```", utils.HtmlToTg(act.NewValue))
	return msg
}

func docDoc(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	format := "*Вложенный документ*:  [%s](%s)"
	def := act.DocActivityExtendFields.DocExtendFields
	var values []any
	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) в документе"
		values = append(values, act.NewValue, def.NewDoc.URL.String())
	case actField.VerbAdded:
		msg.Title = "добавил(-a) в документ"
		values = append(values, act.NewValue, def.NewDoc.URL.String())
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из документа"
		format = "*Вложенный документ*:  ~%s~"
		values = append(values, fmt.Sprint(*act.OldValue))
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) из документа"
		values = append(values, *act.OldValue, def.OldDoc.URL.String())
	case actField.VerbMoveDocWorkspace:
		msg.Title = "сделал(-a) корневым документ"
		if act.OldValue != nil {
			format = "*Из документа*: [%s](%s)"
			values = append(values, *act.OldValue, def.OldDoc.URL.String())
		}
	case actField.VerbMoveDocDoc:
		msg.Title = "переместил(-a) документ"
		format = "*Из документа*: [%s](%s)\n*В документ*: [%s](%s)"
		values = append(values, *act.OldValue, def.OldDoc.URL.String(), act.NewValue, def.NewDoc.URL.String())
	case actField.VerbMoveWorkspaceDoc:
		msg.Title = "переместил(-a) документ"
		format = "*Из корневой директории*\n*В документ*: [%s](%s)"
		values = append(values, act.NewValue, def.NewDoc.URL.String())
	}
	msg.Body = Stelegramf(format, values...)
	return msg
}

func docMember(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	user := utils.GetFirstOrNil(act.NewDocEditor, act.NewDocReader, act.NewDocWatcher, act.OldDocEditor, act.OldDocReader, act.OldDocWatcher)
	if user == nil {
		return msg
	}
	var format string
	values := []any{getUserName(user)}

	switch af {
	case actField.Readers.Field:
		format = "Права *Просмотр*: "
	case actField.Watchers.Field:
		format = "*Наблюдатель*: "
	case actField.Editors.Field:
		format = "Права *Редактирование*: "
	}

	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) пользователя в документ"
		format += "%s"
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) пользователя из документа"
		format += "~%s~"
	}

	msg.Body = fmt.Sprintf(format, values...)
	return msg
}

func docRole(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	var format string
	values := []any{types.TranslateMap(types.RoleTranslation, act.OldValue), types.TranslateMap(types.RoleTranslation, &act.NewValue)}
	msg.Title = "изменил(-a) роли в документе"
	switch af {
	case actField.ReaderRole.Field:
		format = "*Просмотр раздела:* ~%s~ %s"
	case actField.EditorRole.Field:
		format = "*Редактирование:* ~%s~ %s"
	}
	msg.Body = fmt.Sprintf(format, values...)
	return msg
}

func docComment(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return genComment(act.NewDocComment, act.OldValue, act.Verb,
		"изменил(-a) комментарий в документе",
		"прокомментировал(-a) документ",
		"удалил(-a) комментарий из документа")
}

func docAttachment(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	return genAttachment(act.OldValue, act.NewValue, act.Verb,
		"добавил(-a) вложение в документ",
		"удалил(-a) вложение из документа",
	)
}

func docDefault(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	msg.Title = "изменил(-a) в документе"

	if act.OldValue != nil {
		msg.Body += Stelegramf("*%s*: ~%s~ %s", types.FieldsTranslation[af], *act.OldValue, act.NewValue)
	} else {
		msg.Body += Stelegramf("*%s*: %s", types.FieldsTranslation[af], act.NewValue)
	}
	return msg
}
