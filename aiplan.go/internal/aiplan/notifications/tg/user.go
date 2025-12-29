package tg

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types"
	actField "github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/types/activities"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gofrs/uuid"
)

type userTg struct {
	id  int64
	loc types.TimeZone

	projectDefaultWatcher  bool
	projectDefaultAssigner bool

	issueAuthor   bool
	issueWatcher  bool
	issueAssigner bool
}
