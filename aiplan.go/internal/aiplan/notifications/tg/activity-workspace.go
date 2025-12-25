package tg

import (
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"gorm.io/gorm"
)

type funcWorkspaceMsgFormat func(act *dao.WorkspaceActivity, af actField.ActivityField) TgMsg

var (
	workspaceMap = map[actField.ActivityField]funcWorkspaceMsgFormat{
		//actField.Description.Field: docDescription,
		//actField.Doc.Field:         docDoc,
		//
		//actField.Readers.Field:  docMember,
		//actField.Watchers.Field: docMember,
		//actField.Editors.Field:  docMember,
		//
		//actField.ReaderRole.Field: docRole,
		//actField.EditorRole.Field: docRole,
		//
		//actField.Comment.Field:    docComment,
		//actField.Attachment.Field: docAttachment,
		//
		//actField.Title.Field: docDefault,
	}
)

func notifyFromWorkspaceActivity(tx *gorm.DB, act *dao.WorkspaceActivity) *ActivityTgNotification {
	var notify ActivityTgNotification
	if act.Field == nil {
		return nil
	}

	if err := preloadDocActivity(tx, act); err != nil {
		return nil
	}

	msg, err := formatDocActivity(act)
	if err != nil {
		return nil
	}

	notify.Message = msg
	notify.Users = getUserTgDocActivity(tx, act)
	notify.TableName = act.TableName()
	notify.EntityID = act.Id
	notify.AuthorTgID = act.ActivitySender.SenderTg
	return &notify
}
