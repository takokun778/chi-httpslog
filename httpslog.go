package httpslog

import (
	"context"
	"fmt"
	"io/ioutil"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewLogger(serviceName string, opts ...Options) *slog.Logger {
	if len(opts) > 0 {
		Configure(opts[0])
	} else {
		Configure(DefaultOptions)
	}

	logger := slog.With("service", strings.ToLower(serviceName))
	if len(DefaultOptions.Tags) > 0 {
		logger = logger.With("tags", DefaultOptions.Tags)
	}

	return logger
}

func RequestLogger(logger *slog.Logger, skipPaths ...[]string) func(next http.Handler) http.Handler {
	return chi.Chain(
		middleware.RequestID,
		Handler(logger, skipPaths...),
		middleware.Recoverer,
	).Handler
}

func Handler(logger *slog.Logger, optSkipPaths ...[]string) func(next http.Handler) http.Handler {
	var f middleware.LogFormatter = &requestLogger{logger}

	skipPaths := map[string]struct{}{}
	if len(optSkipPaths) > 0 {
		for _, path := range optSkipPaths[0] {
			skipPaths[path] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			// Skip the logger if the path is in the skip list
			if len(skipPaths) > 0 {
				_, skip := skipPaths[r.URL.Path]
				if skip {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Log the request
			entry := f.NewLogEntry(r)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			buf := newLimitBuffer(512)
			ww.Tee(buf)

			t1 := time.Now()
			defer func() {
				var respBody []byte
				if ww.Status() >= 400 {
					respBody, _ = ioutil.ReadAll(buf)
				}
				entry.Write(ww.Status(), ww.BytesWritten(), ww.Header(), time.Since(t1), respBody)
			}()

			next.ServeHTTP(ww, middleware.WithLogEntry(r, entry))
		}
		return http.HandlerFunc(fn)
	}
}

type requestLogger struct {
	Logger *slog.Logger
}

func (l *requestLogger) NewLogEntry(r *http.Request) middleware.LogEntry {
	entry := &RequestLoggerEntry{}
	msg := fmt.Sprintf("Request: %s %s", r.Method, r.URL.Path)
	entry.Logger = l.Logger.With("httpRequest", requestLogFields(r, true))
	if !DefaultOptions.Concise {
		entry.Logger.With("httpRequest", requestLogFields(r, DefaultOptions.Concise)).Info(msg)
	}
	return entry
}

type RequestLoggerEntry struct {
	Logger *slog.Logger
	msg    string
}

func (l *RequestLoggerEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	msg := fmt.Sprintf("Response: %d %s", status, statusLabel(status))
	if l.msg != "" {
		msg = fmt.Sprintf("%s - %s", msg, l.msg)
	}

	responseLog := map[string]interface{}{
		"status":  status,
		"bytes":   bytes,
		"elapsed": float64(elapsed.Nanoseconds()) / 1000000.0, // in milliseconds
	}

	if !DefaultOptions.Concise {
		if status >= 400 {
			body, _ := extra.([]byte)
			responseLog["body"] = string(body)
		}
		if len(header) > 0 {
			responseLog["header"] = headerLogField(header)
		}
	}

	switch statusLevel(status) {
	case slog.LevelError:
		l.Logger.With("httpResponse", responseLog).Error(msg)
	case slog.LevelWarn:
		l.Logger.With("httpResponse", responseLog).Warn(msg)
	case slog.LevelInfo:
	default:
		l.Logger.With("httpResponse", responseLog).Info(msg)
	}
}

func (l *RequestLoggerEntry) Panic(v interface{}, stack []byte) {
	stacktrace := "#"
	stacktrace = string(stack)

	l.Logger = l.Logger.
		With("stacktrace", stacktrace).
		With("panic", fmt.Sprintf("%+v", v))

	l.msg = fmt.Sprintf("%+v", v)

	middleware.PrintPrettyStack(v)
}

func requestLogFields(r *http.Request, concise bool) map[string]interface{} {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	requestURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)

	requestFields := map[string]interface{}{
		"requestURL":    requestURL,
		"requestMethod": r.Method,
		"requestPath":   r.URL.Path,
		"remoteIP":      r.RemoteAddr,
		"proto":         r.Proto,
	}
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		requestFields["requestID"] = reqID
	}

	if concise {
		return requestFields
	}

	requestFields["scheme"] = scheme

	if len(r.Header) > 0 {
		requestFields["header"] = headerLogField(r.Header)
	}

	return requestFields
}

func headerLogField(header http.Header) map[string]string {
	headerField := map[string]string{}
	for k, v := range header {
		k = strings.ToLower(k)
		switch {
		case len(v) == 0:
			continue
		case len(v) == 1:
			headerField[k] = v[0]
		default:
			headerField[k] = fmt.Sprintf("[%s]", strings.Join(v, "], ["))
		}
		if k == "authorization" || k == "cookie" || k == "set-cookie" {
			headerField[k] = "***"
		}

		for _, skip := range DefaultOptions.SkipHeaders {
			if k == skip {
				headerField[k] = "***"
				break
			}
		}
	}
	return headerField
}

func statusLevel(status int) slog.Level {
	switch {
	case status <= 0:
		return slog.LevelWarn
	case status < 400: // for codes in 100s, 200s, 300s
		return slog.LevelInfo
	case status >= 400 && status < 500:
		return slog.LevelWarn
	case status >= 500:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func statusLabel(status int) string {
	switch {
	case status >= 100 && status < 300:
		return "OK"
	case status >= 300 && status < 400:
		return "Redirect"
	case status >= 400 && status < 500:
		return "Client Error"
	case status >= 500:
		return "Server Error"
	default:
		return "Unknown"
	}
}

func LogEntry(ctx context.Context) *slog.Logger {
	entry, ok := ctx.Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry)
	if !ok || entry == nil {
		return slog.Default()
	} else {
		return entry.Logger
	}
}

func LogEntrySetField(ctx context.Context, key, value string) {
	if entry, ok := ctx.Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry); ok {
		entry.Logger = entry.Logger.With(key, value)
	}
}

func LogEntrySetFields(ctx context.Context, fields map[string]interface{}) {
	if entry, ok := ctx.Value(middleware.LogEntryCtxKey).(*RequestLoggerEntry); ok {
		entry.Logger = entry.Logger.With(fields)
	}
}
