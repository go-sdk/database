package dbx

import (
	"context"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-sdk/core/errx"
	"github.com/go-sdk/core/logx"
	"github.com/go-sdk/core/osx"
	"gorm.io/gorm/logger"
)

// LoggerConfig 定义 GORM 日志级别和慢查询阈值。
type LoggerConfig struct {
	SlowThreshold             time.Duration
	LogLevel                  logger.LogLevel
	IgnoreRecordNotFoundError bool
}

// DefaultLoggerConfig 返回与 core/logx 默认级别一致的 GORM 日志配置。
func DefaultLoggerConfig() LoggerConfig {
	level := logger.Warn
	if osx.IsDebug() {
		level = logger.Info
	}
	return LoggerConfig{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  level,
		IgnoreRecordNotFoundError: true,
	}
}

type gormLogger struct{ config LoggerConfig }

var _ logger.Interface = (*gormLogger)(nil)

// dbxSourceDir 当前文件所在目录，用于识别调用栈中 dbx 内部实现帧。
var dbxSourceDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file) + string(filepath.Separator)
}()

// isInternalSource 报告调用帧是否属于 dbx 或 GORM 生态内部实现；
// 测试文件视为业务帧，便于测试断言调用方位置。
func isInternalSource(file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return false
	}
	if strings.HasPrefix(file, dbxSourceDir) {
		return true
	}
	file = filepath.ToSlash(file)
	return strings.Contains(file, "/gorm.io/") || strings.Contains(file, "/go-gormigrate/") || strings.Contains(file, "/glebarez/")
}

// callerSource 返回触发 SQL 的业务调用方位置；
// 跳过 dbx 和 GORM 生态内部帧，全部为内部帧时返回空字符串。
func callerSource() string {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !isInternalSource(frame.File) {
			return frame.File + ":" + strconv.Itoa(frame.Line)
		}
		if !more {
			return ""
		}
	}
}

// NewLogger 创建从 GORM context 读取 core/logx Logger 的日志适配器。
func NewLogger(config LoggerConfig) logger.Interface {
	return &gormLogger{config: config}
}

func (l *gormLogger) LogMode(level logger.LogLevel) logger.Interface {
	copyValue := *l
	copyValue.config.LogLevel = level
	return &copyValue
}

func (l *gormLogger) Info(ctx context.Context, message string, values ...any) {
	if l.config.LogLevel >= logger.Info {
		logx.Ctx(ctx).Info().Str("source", callerSource()).Msgf(message, values...)
	}
}

func (l *gormLogger) Warn(ctx context.Context, message string, values ...any) {
	if l.config.LogLevel >= logger.Warn {
		logx.Ctx(ctx).Warn().Str("source", callerSource()).Msgf(message, values...)
	}
}

func (l *gormLogger) Error(ctx context.Context, message string, values ...any) {
	if l.config.LogLevel >= logger.Error {
		logx.Ctx(ctx).Error().Str("source", callerSource()).Msgf(message, values...)
	}
}

func (l *gormLogger) Trace(ctx context.Context, begin time.Time, sqlFunc func() (string, int64), queryErr error) {
	if l.config.LogLevel <= logger.Silent {
		return
	}
	elapsed := time.Since(begin).Truncate(time.Millisecond)
	isRecordNotFound := errx.Is(queryErr, logger.ErrRecordNotFound)
	switch {
	case queryErr != nil && l.config.LogLevel >= logger.Error && (!isRecordNotFound || !l.config.IgnoreRecordNotFoundError):
		sqlText, rows := sqlFunc()
		logx.Ctx(ctx).Error().Err(queryErr).Str("source", callerSource()).
			Str("sql", sqlText).Int64("rows", rows).Dur("elapsed", elapsed).Msg("gorm query")
	case l.config.SlowThreshold > 0 && elapsed > l.config.SlowThreshold && l.config.LogLevel >= logger.Warn:
		sqlText, rows := sqlFunc()
		logx.Ctx(ctx).Warn().Str("source", callerSource()).Str("sql", sqlText).
			Int64("rows", rows).Dur("elapsed", elapsed).
			Dur("slow_threshold", l.config.SlowThreshold).Msg("gorm slow query")
	case l.config.LogLevel == logger.Info:
		sqlText, rows := sqlFunc()
		logx.Ctx(ctx).Info().Str("source", callerSource()).Str("sql", sqlText).
			Int64("rows", rows).Dur("elapsed", elapsed).Msg("gorm query")
	}
}

// ParamsFilter 保留查询参数，由 GORM Dialector 将参数转义后写入 SQL 日志。
func (l *gormLogger) ParamsFilter(_ context.Context, sqlText string, params ...any) (string, []any) {
	return sqlText, params
}
