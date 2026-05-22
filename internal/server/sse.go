package server

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/rob121/applemusicgo/internal/music"
)

type sseHub struct {
	mu      sync.Mutex
	clients []http.ResponseWriter
	state   music.PlayerState
}

func (h *sseHub) setState(state music.PlayerState) {
	h.mu.Lock()
	h.state = state
	h.mu.Unlock()
}

func (h *sseHub) broadcast(data music.PlayerState) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := "data: " + string(payload) + "\n\n"

	h.mu.Lock()
	defer h.mu.Unlock()
	var alive []http.ResponseWriter
	for _, w := range h.clients {
		if _, err := w.Write([]byte(msg)); err != nil {
			continue
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		alive = append(alive, w)
	}
	h.clients = alive
}

func (h *sseHub) addClient(w http.ResponseWriter) {
	h.mu.Lock()
	h.clients = append(h.clients, w)
	state := h.state
	h.mu.Unlock()

	if state != nil {
		payload, _ := json.Marshal(state)
		_, _ = w.Write([]byte("data: " + string(payload) + "\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func (h *sseHub) removeClient(w http.ResponseWriter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	var alive []http.ResponseWriter
	for _, c := range h.clients {
		if c != w {
			alive = append(alive, c)
		}
	}
	h.clients = alive
}
