package dao

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var NotifiSubscription NotifyService = NotifyService{channels: make(map[string]func(string))}

type NotifyService struct {
	channels map[string]func(string)
}

func (ns *NotifyService) Subscribe(channel string, handler func(payload string)) {
	ns.channels[channel] = handler
}

func (ns *NotifyService) Start(ctx context.Context, dsn string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := ns.pgListen(ctx, dsn)
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("NOTIFY error while subscribe, reconnect...", "err", err)
				time.Sleep(time.Second * 3)
			}
		}
	}
}

func (ns *NotifyService) pgListen(ctx context.Context, dsn string) error {
	conn, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close()

	pgConn, err := conn.Acquire(ctx)
	if err != nil {
		return err
	}
	defer pgConn.Release()

	for channel := range ns.channels {
		_, err = pgConn.Exec(ctx, fmt.Sprintf("LISTEN %s", channel))
		if err != nil {
			return err
		}
	}

	for {
		notification, err := pgConn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		fn, ok := ns.channels[notification.Channel]
		if !ok {
			slog.Warn("Notify channel not registered", "channel", notification.Channel)
		}
		fn(notification.Payload)
	}
}
