package httpslog

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

var DefaultOptions = Options{
	LogLevel:        "info",
	LevelFieldName:  "level",
	Concise:         false,
	Tags:            nil,
	SkipHeaders:     nil,
	TimeFieldFormat: time.RFC3339Nano,
	TimeFieldName:   "timestamp",
}

type Options struct {
	LogLevel        string
	LevelFieldName  string
	Concise         bool
	Tags            map[string]string
	SkipHeaders     []string
	TimeFieldFormat string
	TimeFieldName   string
}

func Configure(opts Options) {
	if opts.LogLevel == "" {
		opts.LogLevel = "info"
	}

	if opts.LevelFieldName == "" {
		opts.LevelFieldName = "level"
	}

	if opts.TimeFieldFormat == "" {
		opts.TimeFieldFormat = time.RFC3339Nano
	}

	if opts.TimeFieldName == "" {
		opts.TimeFieldName = "timestamp"
	}

	for i, header := range opts.SkipHeaders {
		opts.SkipHeaders[i] = strings.ToLower(header)
	}

	DefaultOptions = opts

	logLevel := slog.LevelInfo

	switch strings.ToLower(opts.LogLevel) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))
}
