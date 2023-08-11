package main

import (
	"net/http"

	httpslog "github.com/takokun778/chi-httpslog"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Logger
	logger := httpslog.NewLogger("httpslog-example", httpslog.Options{
		Concise: true,
		// Tags: map[string]string{
		// 	"version": "v1.0-81aa4244d9fc8076a",
		// 	"env":     "dev",
		// },
	})

	// Service
	r := chi.NewRouter()
	r.Use(httpslog.RequestLogger(logger, []string{"/ping"}))
	r.Use(middleware.Heartbeat("/ping"))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	r.Get("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("oh no")
	})

	r.Get("/info", func(w http.ResponseWriter, r *http.Request) {
		oplog := httpslog.LogEntry(r.Context())
		w.Header().Add("Content-Type", "text/plain")
		oplog.Info("info here")
		w.Write([]byte("info here"))
	})

	r.Get("/warn", func(w http.ResponseWriter, r *http.Request) {
		oplog := httpslog.LogEntry(r.Context())
		oplog.Warn("warn here")
		w.WriteHeader(400)
		w.Write([]byte("warn here"))
	})

	r.Get("/err", func(w http.ResponseWriter, r *http.Request) {
		oplog := httpslog.LogEntry(r.Context())
		oplog.Error("err here")
		w.WriteHeader(500)
		w.Write([]byte("err here"))
	})

	http.ListenAndServe(":5555", r)
}
