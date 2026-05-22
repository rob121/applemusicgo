package server

import (
	_ "embed"
	"net/http"

	"github.com/rob121/applemusicgo/api"
)

//go:embed swagger-ui.html
var swaggerUIHTML []byte

func registerSwaggerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPI)
	mux.HandleFunc("GET /swagger", redirectSwagger)
	mux.HandleFunc("GET /swagger/", serveSwaggerUI)
	mux.HandleFunc("GET /docs", redirectSwagger)
	mux.HandleFunc("GET /docs/", redirectSwagger)
}

func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(api.OpenAPI)
}

func redirectSwagger(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/swagger/", http.StatusFound)
}

func serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(swaggerUIHTML)
}
