// Пакет предоставляет функциональность для отправки различных уведомлений пользователям, таких как приглашения в рабочие пространства, уведомления о смене пароля, уведомления об импорте проектов из Jira и т.д.  Поддерживает персонализацию уведомлений с использованием шаблонов HTML и отправку как в виде HTML, так и в виде простого текста.  Также включает в себя обработку ошибок и логирование событий.
//
// Основные возможности:
//   - Отправка уведомлений по email с использованием шаблонов.
//   - Поддержка различных типов уведомлений (приглашения, сброс пароля, импорт Jira).
//   - Персонализация содержимого уведомлений с использованием данных пользователя и проекта.
//   - Логирование ошибок и событий для отслеживания работы системы уведомлений.
package notifications

import (
	"bytes"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/microcosm-cc/bluemonday"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"
	"sheff.online/aiplan/internal/aiplan/config"
	"sheff.online/aiplan/internal/aiplan/dao"

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

func (es *EmailService) EmailActivity() {
	if es.cfg.EmailActivityDisabled {
		return
	}

	if es.sending {
		return
	}

	newEmailNotifyIssue(es).Process()
	newEmailNotifyProject(es).Process()
	newEmailNotifyDoc(es).Process()
}

type EmailService struct {
	d           *gomail.Dialer
	cfg         *config.Config
	db          *gorm.DB
	monitorExit chan bool
	sending     bool
	disabled    bool

	emailChan chan mail
	eg        errgroup.Group
}

type mail struct {
	To          string
	Subject     string
	Content     string
	TextContent string
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
		make(chan mail),
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
			if err := tx.Select("count(*) > 0").
				Table("templates").
				Where("name = ?", name).Find(&exist).Error; err != nil {
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
				Id:       dao.GenID(),
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

func (es *EmailService) sendEmail(e mail) error {
	m := gomail.NewMessage()
	m.SetHeader("From", es.cfg.EmailFrom)
	m.SetHeader("To", e.To)
	m.SetHeader("Subject", e.Subject)
	m.SetBody("text/plain", e.TextContent)
	m.AddAlternative("text/html", e.Content)

	return es.d.DialAndSend(m)
}

func (es *EmailService) Send(e mail) error {
	if es.disabled {
		return fmt.Errorf("email service stop")
	}
	es.emailChan <- e
	return nil
}

func (es *EmailService) worker(emailChan <-chan mail) error {
	for {
		select {
		case e, ok := <-emailChan:
			if !ok {
				return nil
			}
			if err := es.sendEmail(e); err != nil {
				slog.Error("email send err", e.To, err)
			}
		}
	}
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

	return es.Send(mail{
		To:          user.Email,
		Subject:     subject,
		Content:     content,
		TextContent: textContent,
	})
}
