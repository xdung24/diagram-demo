package logging

import (
	"sync"
	"time"
)

// Stream keeps a small in-memory log buffer and fans entries out to SSE subscribers.
type Stream struct {
	mu          sync.RWMutex
	subscribers map[chan string]struct{}
	entries     []string
}

func New() *Stream {
	return &Stream{subscribers: make(map[chan string]struct{})}
}

func (s *Stream) Publish(entry string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.entries) >= 200 {
		s.entries = s.entries[len(s.entries)-199:]
	}
	stamp := time.Now().Format("15:04:05")
	s.entries = append(s.entries, stamp+" "+entry)

	for ch := range s.subscribers {
		select {
		case ch <- s.entries[len(s.entries)-1]:
		default:
		}
	}
}

func (s *Stream) Subscribe() <-chan string {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan string, 20)
	s.subscribers[ch] = struct{}{}
	return ch
}

func (s *Stream) Unsubscribe(ch <-chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channel, ok := any(ch).(chan string); ok {
		delete(s.subscribers, channel)
	}
}

func (s *Stream) Snapshot() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]string, len(s.entries))
	copy(out, s.entries)
	return out
}
