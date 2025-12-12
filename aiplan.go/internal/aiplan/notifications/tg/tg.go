package tg

import (
  "context"
  "log/slog"
	"os"

	tracker "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/activity-tracker"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/business"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/config"
	"github.com/go-telegram/bot"
	"gorm.io/gorm"
)

type TgService struct {
	db       *gorm.DB
	bot      *bot.Bot
	cfg      *config.Config
	tracker  *tracker.ActivitiesTracker
	disabled bool
	bl       *business.Business

	//logger      *zap.Logger
	//commands    *commands.Registry
	//dispatcher  *events.Dispatcher
	//subscribers *subscribers.Manager
	//config      *Config
	//middleware  []Middleware
}

func New(db *gorm.DB, cfg *config.Config, tracker *tracker.ActivitiesTracker, bl *business.Business) *TgService {
	if cfg.TelegramBotToken == "" {
		slog.Info("Telegram notifications disabled")
		return &TgService{disabled: true}
	}
	b, err := bot.New(cfg.TelegramBotToken)
	if err != nil {
		slog.Error("Connect to TG bot", "err", err)
		os.Exit(1)
	}

  dd, _ := b.GetMe(context.Background())
	slog.Info("TG bot connected", "username", dd.Username)
  return &TgService{
      db:       db,
      bot:      b,
      cfg:      cfg,
      tracker:  tracker,
      disabled: false,
      bl:       bl,
  }
}
