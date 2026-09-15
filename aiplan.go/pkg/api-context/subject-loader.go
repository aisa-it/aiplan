package apicontext

import (
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm"
)

// LoadIssueSubject собирает субъект для действия над задачей вне HTTP:
// MCP-инструменты, хуки загрузки файлов. Подгружает членство в проекте и
// пространстве и сам проект; issue должен быть загружен с исполнителями
// (Preload("Assignees")) — движок смотрит на них.
//
// Не участник проекта — apierrors.ErrProjectForbidden. Членство в
// пространстве необязательно: без него пользователь просто не администратор.
func LoadIssueSubject(db *gorm.DB, user *dao.User, issue *dao.Issue) (*APIContext, error) {
	if user == nil || issue == nil {
		return nil, apierrors.ErrProjectForbidden
	}

	var pm dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, issue.ProjectId).First(&pm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrProjectForbidden
		}
		return nil, err
	}

	var wm *dao.WorkspaceMember
	var found dao.WorkspaceMember
	if err := db.Where("member_id = ? AND workspace_id = ?", user.ID, issue.WorkspaceId).First(&found).Error; err == nil {
		wm = &found
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	project := issue.Project
	if project == nil {
		var loaded dao.Project
		if err := db.Where("id = ?", issue.ProjectId).First(&loaded).Error; err != nil {
			return nil, err
		}
		project = &loaded
	}

	return NewSubject(Prefilled{
		DB:              db,
		User:            user,
		Workspace:       issue.Workspace,
		WorkspaceMember: wm,
		Project:         project,
		ProjectMember:   &pm,
		Issue:           issue,
	}), nil
}
