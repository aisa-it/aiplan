package email

import (
	"fmt"
	"log/slog"
	"mime"
	"strings"
	"sync"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"golang.org/x/sync/errgroup"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"
)

type EmailService struct {
	d            *gomail.Dialer
	cfg          *config.Config
	db           *gorm.DB
	monitorExit  chan bool
	sending      bool
	sendingMutex sync.Mutex
	disabled     bool

	emailChan chan EmailMessage
	eg        errgroup.Group

	emailFrom  string
	senderName string

	an *ActivityNotification
}

func NewEmailService(cfg *config.Config, db *gorm.DB) *EmailService {
	email, emailName := parseEmailFrom(cfg.EmailFrom)

	es := &EmailService{
		d:           gomail.NewDialer(cfg.EmailHost, cfg.EmailPort, cfg.EmailUser, cfg.EmailPassword),
		cfg:         cfg,
		db:          db,
		monitorExit: make(chan bool),
		disabled:    cfg.EmailActivityDisabled,
		emailChan:   make(chan EmailMessage, cfg.EmailWorkers*50),
		emailFrom:   email,
		senderName:  emailName,
	}

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

	es.an = InitActivityNotificationService(db, es)

	return es
}

func (es *EmailService) Send(e EmailMessage) error {
	if es.disabled {
		return fmt.Errorf("email service stop")
	}
	es.emailChan <- e
	return nil
}

func (es *EmailService) sendEmail(e EmailMessage) error {
	m := gomail.NewMessage()
	m.SetHeader("From", es.formatFrom(e.Actor))
	m.SetHeader("Sender", es.emailFrom)
	m.SetHeader("To", e.To)
	m.SetHeader("Subject", e.Subject)
	m.SetBody("text/plain", e.TextContent)
	m.AddAlternative("text/html", e.Content)

	return es.d.DialAndSend(m)
}

func parseEmailFrom(s string) (email, systemName string) {
	start := strings.IndexByte(s, '<')
	if start == -1 {
		return s, ""
	}

	end := strings.IndexByte(s[start:], '>')
	if end == -1 {
		return s, ""
	}

	email = s[start+1 : start+end]

	if start > 0 {
		systemName = strings.TrimSpace(s[:start])
	}

	return email, systemName
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
				slog.Debug("email sent successfully", "to", e.To)
			}
		}()
	}
	return nil
}

func (es *EmailService) Stop() {
	slog.Info("Closing email workers")
	es.disabled = true
	close(es.emailChan)

	if err := es.eg.Wait(); err != nil {
		slog.Error("WorkerStop", "err", err)
	}

	slog.Info("Email workers successfully stopped")
}

func (es *EmailService) Close() {
	es.monitorExit <- true
}

type EmailMessage struct {
	To          string
	Subject     string
	Content     string
	TextContent string

	Actor *string
}

func (es *EmailService) formatFrom(userName *string) string {
	var user, system string

	if userName != nil {
		user = *userName
	}
	if es.senderName != "" {
		system = es.senderName
	}

	var displayName string
	if user != "" && system != "" {
		displayName = user + " (" + system + ")"
	} else if user != "" {
		displayName = user
	} else if system != "" {
		displayName = system
	}

	if displayName == "" {
		return es.emailFrom
	}

	encodedName := mime.QEncoding.Encode("UTF-8", displayName)
	return encodedName + " <" + es.emailFrom + ">"
}

type EmailError struct {
	Type string
	User string
	Err  error
}

func (e EmailError) Error() string {
	if e.User != "" {
		return "email error for user " + e.User + " (" + e.Type + "): " + e.Err.Error()
	}
	return "email error (" + e.Type + "): " + e.Err.Error()
}
