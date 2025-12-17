package tg

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

type TelegramNotification struct {
	TgService
}

func NewTelegramNotification(service *TgService) *TelegramNotification {
	if service == nil {
		return nil
	}
	return &TelegramNotification{
		TgService: *service,
	}
}

func (t *TelegramNotification) Handle(activity dao.ActivityI) error {
	if t.Disabled {
		return nil
	}

	// Switch по конкретным типам активностей
	switch a := activity.(type) {
	case *dao.IssueActivity:
		fmt.Println("IssueActivity", a.Comment)
		//return t.handleIssue(a)
	case *dao.ProjectActivity:
		fmt.Println("ProjectActivity", a.Comment)

		//return t.handleProject(a)
	case *dao.DocActivity:
		fmt.Println("DocActivity", a.Comment)

		//return t.handleDocument(a)
	case *dao.FormActivity:
		fmt.Println("FormActivity", a.Comment)

		//return t.handleComment(a)
	case *dao.WorkspaceActivity:
		fmt.Println("WorkspaceActivity", a.Comment)

		//return t.handleWorkspace(a)
	case *dao.RootActivity:
		fmt.Println("RootActivity", a.Comment)

		//return t.handleUser(a)
	case *dao.SprintActivity:
		fmt.Println("SprintActivity", a.Comment)

		//return t.handleSystem(a)
	default:
		slog.Warn("Unknown activity type for Telegram",
			"type", fmt.Sprintf("%T", activity),
			"entity", activity.GetEntity(),
			"verb", activity.GetVerb())
		return nil
	}
	return nil
}
