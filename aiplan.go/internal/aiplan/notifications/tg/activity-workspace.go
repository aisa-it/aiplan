package tg

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/utils"
	"gorm.io/gorm"
)

type funcWorkspaceMsgFormat func(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg

var (
	workspaceMap = map[actField.ActivityField]funcWorkspaceMsgFormat{
		actField.Project.Field:     workspaceProject,
		actField.Doc.Field:         workspaceDoc,
		actField.Form.Field:        workspaceForm,
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

func notifyFromWorkspaceActivity(tx *gorm.DB, act *dao.WorkspaceActivity) *ActivityTgNotification {
	var notify ActivityTgNotification
	if act.Field == nil {
		return nil
	}

	if err := preloadWorkspaceActivity(tx, act); err != nil {
		return nil
	}

	msg, err := formatWorkspaceActivity(act)
	if err != nil {
		return nil
	}

	notify.Message = msg
	notify.Users = getUserTgWorkspaceActivity(tx, act)
	notify.TableName = act.TableName()
	notify.EntityID = act.Id
	notify.AuthorTgID = act.ActivitySender.SenderTg
	return &notify
}

func preloadWorkspaceActivity(tx *gorm.DB, act *dao.WorkspaceActivity) error {
	if err := tx.Unscoped().
		Joins("Owner").
		Where("workspaces.id = ?", act.WorkspaceId).
		First(&act.Workspace).Error; err != nil {
		slog.Error("Get workspace for activity", "activityId", act.Id, "err", err)
		return fmt.Errorf("preloadWorkspaceActivity: %v", err)
	}

	return nil
}

func formatWorkspaceActivity(act *dao.WorkspaceActivity) (TgMsg, error) {
	res, err := formatByField(act, workspaceMap, nil)
	if err != nil {
		return res, err
	}

	return finalizeActivityTitle(res, act.Actor.GetName(), act.Workspace.Slug, act.Workspace.URL), nil
}

func workspaceProject(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbCreated:
		msg.title = "создал(-a) в пространстве"
		msg.body = Stelegramf("*Проект:* [%s](%s)", act.NewProject.Name, act.NewProject.URL.String())
	case actField.VerbDeleted:
		msg.title = "удалил(-a) из пространства"
		msg.body = Stelegramf("*Проект:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceDoc(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbCreated:
		msg.title = "создал(-a) в пространстве"
		msg.body = Stelegramf("*Корневой документ:* [%s](%s)", act.NewDoc.Title, act.NewDoc.URL.String())

	case actField.VerbDeleted:
		msg.title = "удалил(-a) из пространства"
		msg.body = Stelegramf("*Корневой документ:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceForm(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbCreated:
		msg.title = "создал(-a) в пространстве"
		msg.body = Stelegramf("*Форму:* [%s](%s)", act.NewForm.Title, act.NewForm.URL.String())

	case actField.VerbDeleted:
		msg.title = "удалил(-a) из пространства"
		msg.body = Stelegramf("*Форму:* ~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceDescription(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.title = "изменил(-a) в пространстве"
	msg.body += Stelegramf("*%s*: \n```\n%s```", types.FieldsTranslation[af], utils.HtmlToTg(act.NewValue))
	return msg
}

func workspaceToken(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.title = "изменил(-a) в пространстве"
	msg.body = Stelegramf("*Токен для работы интеграций*")
	return msg
}

func workspaceOwner(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.title = "изменил(-a) владельца пространства"
	msg.body += Stelegramf("~%s~ %s", getUserName(act.OldOwner), getUserName(act.NewOwner))
	return msg
}

func workspaceMember(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.title = "добавил(-a) в пространство"
		msg.body = Stelegramf("__%s__\n*Роль:* %s", getUserName(act.NewMember), types.TranslateMap(types.RoleTranslation, &act.NewValue))
	case actField.VerbRemoved:
		msg.title = "убрал(-a) из пространства"
		msg.body = Stelegramf("~%s~", getUserName(act.OldMember))

	}
	return msg
}

func workspaceIntegration(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	switch act.Verb {
	case actField.VerbAdded:
		msg.title = "добавил(-a) интеграцию в пространство"
		msg.body = Stelegramf("%s", act.NewValue)
	case actField.VerbRemoved:
		msg.title = "убрал(-a) интеграцию из пространства"
		msg.body += Stelegramf("~%s~", fmt.Sprint(*act.OldValue))
	}
	return msg
}

func workspaceRole(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.title = "изменил(-a) роль пользователя в пространстве"
	msg.body = Stelegramf("__%s__\n*Роль*: ~%s~ %s", getUserName(act.NewRole), types.TranslateMap(types.RoleTranslation, act.OldValue), types.TranslateMap(types.RoleTranslation, &act.NewValue))
	return msg
}

func workspaceName(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.title = "изменил(-a) в пространстве"
	msg.body = Stelegramf("*Имя пространства*: ~%s~ %s", fmt.Sprint(*act.OldValue), act.NewValue)

	return msg
}

func workspaceLogo(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg {
	msg := NewTgMsg()
	if act.Verb != actField.VerbUpdated {
		return msg
	}
	msg.title = "изменил(-a) в пространстве"
	msg.body = Stelegramf("*Логотип пространства*")
	return msg
}

func getUserTgWorkspaceActivity(tx *gorm.DB, act *dao.WorkspaceActivity) []userTg {
	users := make(UserRegistry)
	users.addUser(act.Actor, actionAuthor)
	addWorkspaceAdmin(tx, act.WorkspaceId, users)

	if err := users.LoadWorkspaceSettings(tx, act.WorkspaceId, actionAuthor); err != nil {
		return []userTg{}
	}
	return users.FilterActivity(act.Field, act.Verb, actField.Workspace.Field.String(), shouldWorkspaceNotify, actionAuthor)
}
