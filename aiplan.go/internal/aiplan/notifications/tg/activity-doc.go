package tg

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

type funcDocMsgFormat func(act *dao.DocActivity, af actField.ActivityField) TgMsg

var (
	docMap = map[actField.ActivityField]funcDocMsgFormat{
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

func notifyFromDocActivity(tx *gorm.DB, act *dao.DocActivity) (*ActivityTgNotification, error) {
	if act.Field == nil {
		return nil, nil
	}

	if err := preloadDocActivity(tx, act); err != nil {
		return nil, err
	}

	msg, err := formatDocActivity(act)
	if err != nil {
		return nil, fmt.Errorf("formatDocActivity: %w", err)
	}
	steps := []UsersStep{
		addUserRole(act.Actor, actionAuthor),
		addUserRole(act.Doc.Author, docAuthor),
		addCommentMentionedUsers(act.NewDocComment),
		addDocMembers(act.DocId),
	}
	if act.Doc == nil && act.Verb == actField.VerbCreated {
		steps = append(steps, addWorkspaceAdmins(act.WorkspaceId))
	}

	plan := NotifyPlan{
		TableName:      act.TableName(),
		settings:       fromWorkspace(act.WorkspaceId),
		ActivitySender: act.ActivitySender.SenderTg,
		Entity:         actField.Doc.Field,
		AuthorRole:     actionAuthor,
		Steps:          steps,
	}

	return NewActivityTgNotification(tx, act, msg, plan), nil
}

func preloadDocActivity(tx *gorm.DB, act *dao.DocActivity) error {
	if err := tx.Unscoped().
		Joins("Workspace").
		Joins("Author").
		Preload("AccessRules.Member").
		Where("docs.id = ?", act.DocId).
		First(&act.Doc).Error; err != nil {
		return fmt.Errorf("preloadDocActivity: %v", err)
	}
	act.Workspace = act.Doc.Workspace
	act.Doc.AfterFind(tx)

	return nil
}

func formatDocActivity(act *dao.DocActivity) (TgMsg, error) {
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

func docDescription(act *dao.DocActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	msg.title = "изменил(-а) описание документа"
	msg.body = Stelegramf("```\n%s```", utils.HtmlToTg(act.NewValue))
	return msg
}

func docDoc(act *dao.DocActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	format := "*Вложенный документ*:  [%s](%s)"
	var values []any
	switch act.Verb {
	case actField.VerbCreated:
		msg.title = "создал(-a) в документе"
		values = append(values, act.NewValue, act.NewDoc.URL.String())
	case actField.VerbAdded:
		msg.title = "добавил(-a) в документ"
		values = append(values, act.NewValue, act.NewDoc.URL.String())
	case actField.VerbDeleted:
		msg.title = "удалил(-a) из документа"
		format = "*Вложенный документ*:  ~%s~"
		values = append(values, fmt.Sprint(*act.OldValue))
	case actField.VerbRemoved:
		msg.title = "убрал(-a) из документа"
		values = append(values, *act.OldValue, act.OldDoc.URL.String())
	case actField.VerbMoveDocWorkspace:
		msg.title = "сделал(-a) корневым документ"
		if act.OldValue != nil {
			format = "*Из документа*: [%s](%s)"
			values = append(values, *act.OldValue, act.OldDoc.URL.String())
		}
	case actField.VerbMoveDocDoc:
		msg.title = "переместил(-a) документ"
		format = "*Из документа*: [%s](%s)\n*В документ*: [%s](%s)"
		values = append(values, *act.OldValue, act.OldDoc.URL.String(), act.NewValue, act.NewDoc.URL.String())
	case actField.VerbMoveWorkspaceDoc:
		msg.title = "переместил(-a) документ"
		format = "*Из корневой директории*\n*В документ*: [%s](%s)"
		values = append(values, act.NewValue, act.NewDoc.URL.String())
	}
	msg.body = Stelegramf(format, values...)
	return msg
}

func docMember(act *dao.DocActivity, af actField.ActivityField) TgMsg {
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
		msg.title = "добавил(-a) пользователя в документ"
		format += "%s"
	case actField.VerbRemoved:
		msg.title = "убрал(-a) пользователя из документа"
		format += "~%s~"
	}

	msg.body = fmt.Sprintf(format, values...)
	return msg
}

func docRole(act *dao.DocActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	var format string
	values := []any{types.TranslateMap(types.RoleTranslation, act.OldValue), types.TranslateMap(types.RoleTranslation, &act.NewValue)}
	msg.title = "изменил(-a) роли в документе"
	switch af {
	case actField.ReaderRole.Field:
		format = "*Просмотр раздела:* ~%s~ %s"
	case actField.EditorRole.Field:
		format = "*Редактирование:* ~%s~ %s"
	}
	msg.body = fmt.Sprintf(format, values...)
	return msg
}

func docComment(act *dao.DocActivity, af actField.ActivityField) TgMsg {
	return genComment(act.NewDocComment, act.OldValue, act.Verb,
		"изменил(-a) комментарий в документе",
		"прокомментировал(-a) документ",
		"удалил(-a) комментарий из документа")
}

func docAttachment(act *dao.DocActivity, af actField.ActivityField) TgMsg {
	return genAttachment(act.OldValue, act.NewValue, act.Verb,
		"добавил(-a) вложение в документ",
		"удалил(-a) вложение из документа",
	)
}

func docDefault(act *dao.DocActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()

	msg.title = "изменил(-a) в документе"

	if act.OldValue != nil {
		msg.body += Stelegramf("*%s*: ~%s~ %s", types.FieldsTranslation[af], *act.OldValue, act.NewValue)
	} else {
		msg.body += Stelegramf("*%s*: %s", types.FieldsTranslation[af], act.NewValue)
	}
	return msg
}
