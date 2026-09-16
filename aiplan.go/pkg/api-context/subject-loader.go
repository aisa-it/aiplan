package apicontext

import (
	"errors"

	"github.com/aisa-it/aiplan/aiplan.go/pkg/apierrors"
	"github.com/aisa-it/aiplan/aiplan.go/pkg/dao"
	"gorm.io/gorm"
)

// LoadProjectSubject собирает субъект для действия в проекте вне HTTP:
// MCP-инструменты, хуки. Подгружает членство в проекте и пространстве.
//
// Не участник проекта — apierrors.ErrProjectForbidden. Членство в
// пространстве необязательно: без него пользователь просто не администратор.
func LoadProjectSubject(db *gorm.DB, user *dao.User, project *dao.Project) (*APIContext, error) {
	if user == nil || project == nil {
		return nil, apierrors.ErrProjectForbidden
	}

	var pm dao.ProjectMember
	if err := db.Where("member_id = ? AND project_id = ?", user.ID, project.ID).First(&pm).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apierrors.ErrProjectForbidden
		}
		return nil, err
	}

	var wm *dao.WorkspaceMember
	var found dao.WorkspaceMember
	if err := db.Where("member_id = ? AND workspace_id = ?", user.ID, project.WorkspaceId).First(&found).Error; err == nil {
		wm = &found
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	return NewSubject(Prefilled{
		DB:              db,
		User:            user,
		Workspace:       project.Workspace,
		WorkspaceMember: wm,
		Project:         project,
		ProjectMember:   &pm,
	}), nil
}

// LoadIssueSubject собирает субъект для действия над задачей вне HTTP.
// issue должен быть загружен с исполнителями (Preload("Assignees")) —
// движок смотрит на них.
func LoadIssueSubject(db *gorm.DB, user *dao.User, issue *dao.Issue) (*APIContext, error) {
	if user == nil || issue == nil {
		return nil, apierrors.ErrProjectForbidden
	}

	project := issue.Project
	if project == nil {
		var loaded dao.Project
		if err := db.Where("id = ?", issue.ProjectId).First(&loaded).Error; err != nil {
			return nil, err
		}
		project = &loaded
	}

	subject, err := LoadProjectSubject(db, user, project)
	if err != nil {
		return nil, err
	}
	if issue.Workspace != nil {
		subject.workspace = issue.Workspace
	}
	subject.issue.Issue = issue
	return subject, nil
}
