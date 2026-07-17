package main

import (
	"fmt"
	"net/http"
	"time"
)

func (a *App) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := a.subscribe()
	defer a.unsubscribe(ch)
	if _, err := fmt.Fprint(w, ": conectado\n\n"); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	revalidate := time.NewTimer(5 * time.Minute)
	defer revalidate.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if _, err := fmt.Fprint(w, "event: change\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-revalidate.C:
			return
		}
	}
}

func (a *App) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	a.mu.Lock()
	if a.subscribers == nil {
		a.subscribers = map[chan struct{}]struct{}{}
	}
	a.subscribers[ch] = struct{}{}
	a.mu.Unlock()
	return ch
}

func (a *App) unsubscribe(ch chan struct{}) {
	a.mu.Lock()
	delete(a.subscribers, ch)
	a.mu.Unlock()
}

func (a *App) notify() {
	a.mu.Lock()
	for ch := range a.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	a.mu.Unlock()
}
