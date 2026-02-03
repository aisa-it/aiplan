package email

import (
	"bytes"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
)

type EmailNotification interface {
	Process()
}

func (es *EmailService) EmailActivity() {
	if es.cfg.EmailActivityDisabled {
		return
	}

	if es.sending {
		return
	}

	templates := LoadTemplates(es.db)

	ProcessLayer(es, NewSprintPipeline(&templates))
}

// /----
func (es *EmailService) getHTML(title string, body string) (string, error) {
	return es.getHTMLWithParams(title, body, nil, nil, 0, 0)
}

func (es *EmailService) getHTMLWithParams(title string, body string, issue *dao.Issue, project *dao.Project, commentCount int, activityCount int) (string, error) {
	var template dao.Template
	if err := es.db.Where("name = ?", "body").First(&template).Error; err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, struct {
		Issue         *dao.Issue
		Title         string
		CreatedAt     time.Time
		Body          string
		CommentCount  int
		ActivityCount int
		Project       *dao.Project
	}{
		Title:         title,
		CreatedAt:     time.Now(), //TODO: timezone
		Body:          body,
		Issue:         issue,
		CommentCount:  commentCount,
		ActivityCount: activityCount,
		Project:       project,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
