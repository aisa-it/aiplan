package tg

import (
	"fmt"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
)

var (
	workspaceMap = map[actField.ActivityField]funcTgMsgFormat{
		actField.Project.Field:     workspaceProject,
		actField.Doc.Field:         workspaceDoc,
		actField.Form.Field:        workspaceForm,
		actField.Sprint.Field:      workspaceSprint,
		actField.Description.Field: workspaceDescription,

		actField.Token.Field:       workspaceToken,
		actField.Owner.Field:       workspaceOwner,
		actField.Member.Field:      workspaceMember,
		actField.Integration.Field: workspaceIntegration,

		actField.Role.Field: workspaceRole,
		actField.Name.Field: workspaceName,
		actField.Logo.Field: workspaceLogo,
	}
)

func FormatWorkspaceActivity(act *dao.ActivityEvent) (TgMsg, error) {
	res, err := formatByField(act, workspaceMap, nil)
	if err != nil {
		return res, err
	}

	return finalizeActivityTitle(res, act.Actor.GetName(), act.Workspace.Slug, act.Workspace.URL), nil
}

func workspaceProject(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	pef := act.WorkspaceActivityExtendFields.ProjectExtendFields
	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) в пространстве"
		msg.Body = Stelegramf("*Проект:* [%s](%s)", pef.NewProject.Name, pef.NewProject.URL.String())
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из пространства"
		msg.Body = Stelegramf("*Проект:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceDoc(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	def := act.WorkspaceActivityExtendFields.DocExtendFields
	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) в пространстве"
		msg.Body = Stelegramf("*Корневой документ:* [%s](%s)", def.NewDoc.Title, def.NewDoc.URL.String())

	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из пространства"
		msg.Body = Stelegramf("*Корневой документ:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceForm(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) в пространстве"
		msg.Body = Stelegramf("*Форму:* [%s](%s)", act.NewForm.Title, act.NewForm.URL.String())

	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из пространства"
		msg.Body = Stelegramf("*Форму:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceSprint(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbCreated:
		msg.Title = "создал(-a) в пространстве"
		msg.Body = Stelegramf("*Спринт:* [%s](%s)", act.NewSprint.GetFullName(), act.NewSprint.URL.String())
	case actField.VerbDeleted:
		msg.Title = "удалил(-a) из пространства"
		msg.Body = Stelegramf("*Спринт:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceDescription(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) в пространстве"
	msg.Body += Stelegramf("*%s*: \n```\n%s```", types.FieldsTranslation[af], utils.HtmlToTg(act.NewValue))
	return msg
}

func workspaceToken(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) в пространстве"
	msg.Body = Stelegramf("*Токен для работы интеграций*")
	return msg
}

func workspaceOwner(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) лидера пространства"
	msg.Body += Stelegramf("~%s~ %s", getUserName(act.OldOwner), getUserName(act.NewOwner))
	return msg
}

func workspaceMember(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) в пространство"
		msg.Body = Stelegramf("__%s__\n*Роль:* %s", getUserName(act.NewMember), types.TranslateMap(types.RoleTranslation, &act.NewValue))
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) из пространства"
		msg.Body = Stelegramf("~%s~", getUserName(act.OldMember))

	}
	return msg
}

func workspaceIntegration(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.Title = "добавил(-a) интеграцию в пространство"
		msg.Body = Stelegramf("%s", act.NewValue)
	case actField.VerbRemoved:
		msg.Title = "убрал(-a) интеграцию из пространства"
		msg.Body += Stelegramf("~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceRole(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) роль пользователя в пространстве"
	msg.Body = Stelegramf("__%s__\n*Роль*: ~%s~ %s", getUserName(act.NewRole), types.TranslateMap(types.RoleTranslation, act.OldValue), types.TranslateMap(types.RoleTranslation, &act.NewValue))
	return msg
}

func workspaceName(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) в пространстве"
	msg.Body = Stelegramf("*Имя пространства*: ~%s~ %s", fmt.Sprint(*act.OldValue), act.NewValue)

	return msg
}

func workspaceLogo(act *dao.ActivityEvent, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.Title = "изменил(-a) в пространстве"
	msg.Body = Stelegramf("*Логотип пространства*")
	return msg
}
