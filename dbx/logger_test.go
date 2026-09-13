package dbx

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-sdk/core/errx"
	"github.com/rs/zerolog"
	"gorm.io/gorm/logger"
)

func TestGORMLoggerUsesContextLogger(t *testing.T) {
	var output bytes.Buffer
	contextLogger := zerolog.New(&output).With().Str("trace-id", "trace-1").Str("span-id", "span-1").Logger()
	ctx := contextLogger.WithContext(context.Background())
	adapter := NewLogger(LoggerConfig{LogLevel: logger.Info})
	adapter.Trace(ctx, time.Now(), func() (string, int64) {
		return "SELECT * FROM users WHERE id = 42", 1
	}, nil)
	text := output.String()
	for _, expected := range []string{"trace-1", "span-1", "id = 42", `"rows":1`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("log does not contain %q: %s", expected, text)
		}
	}
}

func TestGORMLoggerRecordsError(t *testing.T) {
	var output bytes.Buffer
	contextLogger := zerolog.New(&output)
	adapter := NewLogger(LoggerConfig{LogLevel: logger.Error})
	adapter.Trace(contextLogger.WithContext(context.Background()), time.Now(), func() (string, int64) {
		return "SELECT 1", -1
	}, errx.New("query failed"))
	if text := output.String(); !strings.Contains(text, "query failed") || !strings.Contains(text, "SELECT 1") {
		t.Fatalf("unexpected error log: %s", text)
	}
}

func TestGORMLoggerSourcePointsToCaller(t *testing.T) {
	var output bytes.Buffer
	contextLogger := zerolog.New(&output)
	adapter := NewLogger(LoggerConfig{LogLevel: logger.Info})
	adapter.Trace(contextLogger.WithContext(context.Background()), time.Now(), func() (string, int64) {
		return "SELECT 1", 1
	}, nil)
	text := output.String()
	if !strings.Contains(text, "logger_test.go:") {
		t.Fatalf("source should point to the business caller instead of dbx internals: %s", text)
	}
}

func TestGORMLoggerKeepsParameters(t *testing.T) {
	adapter := NewLogger(DefaultLoggerConfig()).(*gormLogger)
	sqlText, params := adapter.ParamsFilter(context.Background(), "SELECT ?", "value")
	if sqlText != "SELECT ?" || len(params) != 1 || params[0] != "value" {
		t.Fatalf("query parameters were filtered: %q, %#v", sqlText, params)
	}
}
