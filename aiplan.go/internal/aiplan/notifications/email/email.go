package email

import (
	"bytes"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/microcosm-cc/bluemonday"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

var htmlStripPolicy *bluemonday.Policy = bluemonday.StrictPolicy()
var minifier *minify.M = minify.New()

//go:embed templates/*
var defaultTemplates embed.FS

type EmailNotification interface {
	Process()
}

type EmailMessage struct {
	To      string
	Subject string
	HTML    string
	Text    string

	replace map[string]any
}

func (es *EmailService) EmailActivity() {
	if es.cfg.EmailActivityDisabled {
		return
	}

	if es.sending {
		return
	}
	es.ProcessSprint()
}

type EmailService struct {
	d           *gomail.Dialer
	cfg         *config.Config
	db          *gorm.DB
	monitorExit chan bool
	sending     bool
	disabled    bool

	emailChan chan EmailMessage
	eg        errgroup.Group
}

func NewEmailService(cfg *config.Config, db *gorm.DB) *EmailService {
	minifier.AddFunc("text/html", html.Minify)

	es := &EmailService{
		gomail.NewDialer(cfg.EmailHost, cfg.EmailPort, cfg.EmailUser, cfg.EmailPassword),
		cfg,
		db,
		make(chan bool),
		false,
		cfg.EmailActivityDisabled,
		make(chan EmailMessage, cfg.EmailWorkers*50),
		errgroup.Group{}}
	if cfg.EmailActivityDisabled {
		slog.Warn("Email activity notifications disabled")
	}
	// insert default templates if not exists
	for i := 0; i < cfg.EmailWorkers; i++ {
		es.eg.Go(func() error {
			return es.worker(es.emailChan)
		})
	}

	es.CreateNewTemplates(db)

	return es
}

func (*EmailService) CreateNewTemplates(tx *gorm.DB) {
	dir, err := defaultTemplates.ReadDir("templates")
	if err == nil {
		for _, file := range dir {
			var exist bool
			name := strings.TrimSuffix(file.Name(), filepath.Ext(file.Name()))
			if err := tx.Model(&dao.Template{}).
				Select("EXISTS(?)",
					tx.Model(&dao.Template{}).
						Select("1").
						Where("name = ?", name),
				).
				Find(&exist).Error; err != nil {
				slog.Warn("Error check template in db", slog.String("name", name), "err", err)
				continue
			}
			if exist {
				continue
			}

			data, err := defaultTemplates.ReadFile(filepath.Join("templates", file.Name()))
			if err != nil {
				slog.Warn("Read embed template", slog.String("name", filepath.Join("templates", file.Name())), "err", err)
				continue
			}

			data, err = minifier.Bytes("text/html", data)
			if err != nil {
				slog.Warn("Error minify embed template", slog.String("name", filepath.Join("templates", file.Name())), "err", err)
			}

			if err := tx.Create(&dao.Template{
				Id:       dao.GenUUID(),
				Name:     name,
				Template: string(data),
			}).Error; err != nil {
				slog.Warn("Error insert default template", slog.String("name", name), "err", err)
			}
		}
	}
}

func (es *EmailService) Close() {
	es.monitorExit <- true
}

func (es *EmailService) Stop() {
	slog.Info("Closing email workers")
	es.disabled = true
	close(es.emailChan)

	if err := es.eg.Wait(); err != nil {
		slog.Error("Worker err:", err)
	}

	slog.Info("Email workers successfully stopped")
}

func (es *EmailService) sendEmail(e EmailMessage) error {
	m := gomail.NewMessage()
	m.SetHeader("From", es.cfg.EmailFrom)
	m.SetHeader("To", e.To)
	m.SetHeader("Subject", e.Subject)
	m.SetBody("text/plain", e.Text)
	m.AddAlternative("text/html", e.HTML)

	return es.d.DialAndSend(m)
}

func (es *EmailService) Send(e EmailMessage) error {
	if es.disabled {
		return fmt.Errorf("email service stop")
	}
	es.emailChan <- e
	return nil
}

func (es *EmailService) worker(emailChan <-chan EmailMessage) error {
	for e := range emailChan {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in email worker", "recover", r)
				}
			}()

			if err := es.sendEmail(e); err != nil {
				slog.Error("email send failed", "to", e.To, "err", err)
			} else {
				slog.Info("email sent successfully", "to", e.To)
			}
		}()
	}
	return nil
}

func (es *EmailService) UserBlockedUntil(user dao.User, until time.Time) error {
	subject := "Подозрительная активность учетной записи"

	var template dao.Template
	if err := es.db.Where("name = ?", "blocked_until").First(&template).Error; err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := template.ParsedTemplate.Execute(&buf, struct {
		Until time.Time
	}{
		Until: until,
	}); err != nil {
		return err
	}

	content, err := es.getHTML("Блокировка учетной записи", buf.String())
	if err != nil {
		return err
	}

	textContent := htmlStripPolicy.Sanitize(content)

	return es.Send(EmailMessage{
		To:      user.Email,
		Subject: subject,
		HTML:    content,
		Text:    textContent,
	})
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
