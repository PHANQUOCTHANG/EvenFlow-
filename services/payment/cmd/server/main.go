// Command server la diem vao cua payment-svc.
//
// TODO: hien thuc theo backlog trong docs/04-jira-backlog.md.
// Layout hexagonal chuan cua service Go: xem docs/03-cau-truc-src.md muc 2.
package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8085"
	}

	log.Info("payment dang lang nghe", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Error("server dung", "err", err)
		os.Exit(1)
	}
}
