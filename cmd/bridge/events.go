package main

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// SyncEvent is a structured status event emitted to stdout during sync.
// These events are consumed by the menu bar app for real-time progress display.
type SyncEvent struct {
	Event       string `json:"event"`
	Time        string `json:"time"`
	Phase       string `json:"phase,omitempty"`
	Notes       int    `json:"notes,omitempty"`
	Items       int    `json:"items,omitempty"`
	DurationMs  int64  `json:"duration_ms"`
	NotesSynced int    `json:"notes_synced"`
	TagsSynced  int    `json:"tags_synced"`
	QueueItems  int    `json:"queue_items"`
	Error       string `json:"error,omitempty"`
}

// EventEmitter writes structured JSON status events to an io.Writer.
// It is safe for concurrent use and safe to call on a nil receiver.
type EventEmitter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewEventEmitter creates an EventEmitter that writes JSON lines to w.
func NewEventEmitter(w io.Writer) *EventEmitter {
	return &EventEmitter{w: w}
}

// Emit writes a SyncEvent as a newline-delimited JSON line.
// The Time field is set automatically to the current UTC time.
// Safe to call on a nil receiver (no-op).
func (e *EventEmitter) Emit(event *SyncEvent) {
	if e == nil {
		return
	}
	event.Time = time.Now().UTC().Format(time.RFC3339)
	e.mu.Lock()
	defer e.mu.Unlock()
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = e.w.Write(data)
}
