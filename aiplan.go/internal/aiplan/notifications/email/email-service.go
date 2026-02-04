package email

import (
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	memNotify "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/notifications/member-role"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
	"golang.org/x/sync/errgroup"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"
)

var minifier *minify.M = minify.New()

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

type EmailMessage struct {
	To      string
	Subject string
	HTML    string
	Text    string

	replace map[string]any
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

type emailPlan struct {
	TableName  string
	Entity     actField.ActivityField
	AuthorRole memNotify.Role
}

type EmailContext struct {
	Settings memNotify.MemberSettings
	Steps    []memNotify.UsersStep
	Plan     *emailPlan
}
