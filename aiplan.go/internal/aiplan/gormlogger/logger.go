// Логирование запросов GORM с возможностью трассировки, фильтрации параметров и выделения медленных запросов.
//
// Основные возможности:
//   - Логирование запросов GORM с использованием slog.
//   - Трассировка медленных запросов, превышающих заданный порог времени.
//   - Фильтрация параметров запросов для безопасной подстановки в SQL.
package gormlogger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"
	gormLog "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

type GormLogger struct {
	SlowThreshold        time.Duration
	ParameterizedQueries bool
	level                slog.Level
	logger               *slog.Logger
}

func NewGormLogger(logger *slog.Logger, slowThreshold time.Duration, paramQueries bool) *GormLogger {
	return &GormLogger{logger: logger, SlowThreshold: slowThreshold, ParameterizedQueries: paramQueries}
}

func (gl *GormLogger) LogMode(level gormLog.LogLevel) gormLog.Interface {
	return &GormLogger{level: slog.Level(level), logger: gl.logger}
}

func (gl *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	fmt.Println("info", msg, data)
}
func (gl *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	fmt.Println("warn", msg, data)
}
func (gl *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	fmt.Println("error", msg)
}
func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		l.logger.Error("",
			slog.String("file", utils.FileWithLineNum()),
			slog.String("elapsed", elapsed.String()),
			slog.Int64("rowsCount", rows),
			slog.String("err", err.Error()),
			slog.String("sql", sql),
		)
	case elapsed > l.SlowThreshold && l.SlowThreshold != 0:
		// Ignore DELETE
		if strings.Contains(sql, "DELETE") {
			return
		}

		slowLog := fmt.Sprintf("SLOW SQL >= %v", l.SlowThreshold)
		l.logger.Warn(slowLog,
			slog.String("file", utils.FileWithLineNum()),
			slog.String("elapsed", elapsed.String()),
			slog.Int64("rowsCount", rows),
			slog.String("sql", sql),
		)
	default:
		l.logger.Debug("SQL trace",
			slog.String("file", utils.FileWithLineNum()),
			slog.String("elapsed", elapsed.String()),
			slog.Int64("rowsCount", rows),
			slog.String("sql", sql),
		)
	}
}

func (l *GormLogger) ParamsFilter(ctx context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if l.ParameterizedQueries {
		return sql, nil
	}
	return sql, params
}
