// Package logs provides a ring buffer for storing recent log entries
// and serving them via HTTP.
package logs

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

// Entry represents a single log entry.
type Entry struct {
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Fields    []Field   `json:"fields,omitempty"`
}

// Field is a key-value pair.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Buffer stores recent log entries in a ring buffer.
type Buffer struct {
	mu      sync.RWMutex
	entries []Entry
	next    int
	size    int
}

// NewBuffer creates a ring buffer with the given capacity.
func NewBuffer(capacity int) *Buffer {
	return &Buffer{
		entries: make([]Entry, capacity),
		size:    capacity,
	}
}

// Add adds a log entry to the buffer.
func (b *Buffer) Add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.next] = e
	b.next = (b.next + 1) % b.size
}

// Recent returns the most recent log entries in order (oldest first).
func (b *Buffer) Recent(n int) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if n <= 0 || n > b.size {
		n = b.size
	}
	result := make([]Entry, 0, n)
	// Start from oldest entry in the ring
	start := b.next
	for i := 0; i < b.size && len(result) < n; i++ {
		idx := (start + i) % b.size
		if b.entries[idx].Timestamp.IsZero() {
			continue
		}
		result = append(result, b.entries[idx])
	}
	return result
}

// ZapCore returns a zapcore.Core that writes to this buffer.
func (b *Buffer) ZapCore() zapcore.Core {
	return &bufferCore{buffer: b}
}

type bufferCore struct {
	buffer *Buffer
}

func (c *bufferCore) Enabled(level zapcore.Level) bool {
	return level >= zapcore.DebugLevel
}

func (c *bufferCore) With(fields []zapcore.Field) zapcore.Core {
	return c // simplification: ignore additional fields for now
}

func (c *bufferCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *bufferCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	fs := make([]Field, 0, len(fields))
	for _, f := range fields {
		fs = append(fs, Field{Key: f.Key, Value: f.String})
	}
	c.buffer.Add(Entry{
		Timestamp: entry.Time,
		Level:     entry.Level.String(),
		Message:   entry.Message,
		Fields:    fs,
	})
	return nil
}

func (c *bufferCore) Sync() error { return nil }

// HTTPHandler serves recent log entries as JSON.
type HTTPHandler struct {
	buffer *Buffer
}

// NewHTTPHandler creates a new HTTP handler for serving logs.
func NewHTTPHandler(b *Buffer) *HTTPHandler {
	return &HTTPHandler{buffer: b}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries := h.buffer.Recent(200)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"logs": entries})
}
