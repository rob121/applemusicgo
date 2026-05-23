package server

import (
	_ "embed"
	"net/http"
)

//go:embed web-ui.html
var webUIHTML []byte

func registerWebRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", serveWebUI)
}

func serveWebUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(webUIHTML)
}
