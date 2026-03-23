package managersdk

import (
	"fmt"
	"sync"
	"testing"
	"time"

	messenger "github.com/slidebolt/sb-messenger-sdk"
)

// SpyMessage is a captured NATS message with timing metadata.
type SpyMessage struct {
	At      time.Duration // offset from spy creation
	Subject string
	Data    []byte
}

func (m SpyMessage) String() string {
	data := string(m.Data)
	if len(data) > 120 {
		data = data[:120] + "..."
	}
	return fmt.Sprintf("%6dms %s %s", m.At.Milliseconds(), m.Subject, data)
}

// MessageSpy subscribes to a NATS pattern and records every message.
// Use it in tests to see exactly what flows through the bus.
type MessageSpy struct {
	t       *testing.T
	pattern string
	start   time.Time
	mu      sync.Mutex
	msgs    []SpyMessage
	sub     messenger.Subscription
}

// Spy creates a MessageSpy that subscribes to pattern on the test bus.
// All matching messages are recorded and logged via t.Logf.
// The subscription is automatically cleaned up when the test ends.
func (e *TestEnv) Spy(pattern string) *MessageSpy {
	e.t.Helper()

	spy := &MessageSpy{
		t:       e.t,
		pattern: pattern,
		start:   time.Now(),
	}

	sub, err := e.msg.Subscribe(pattern, func(m *messenger.Message) {
		sm := SpyMessage{
			At:      time.Since(spy.start),
			Subject: m.Subject,
			Data:    make([]byte, len(m.Data)),
		}
		copy(sm.Data, m.Data)

		spy.mu.Lock()
		spy.msgs = append(spy.msgs, sm)
		spy.mu.Unlock()

		spy.t.Logf("[spy] %s", sm)
	})
	if err != nil {
		e.t.Fatalf("manager-sdk: spy subscribe %q: %v", pattern, err)
	}
	if err := e.msg.Flush(); err != nil {
		e.t.Fatalf("manager-sdk: spy flush: %v", err)
	}
	spy.sub = sub
	e.t.Cleanup(func() { sub.Unsubscribe() })
	return spy
}

// Messages returns a snapshot of all captured messages.
func (s *MessageSpy) Messages() []SpyMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SpyMessage, len(s.msgs))
	copy(out, s.msgs)
	return out
}

// MessagesFor returns captured messages whose subject contains substr.
func (s *MessageSpy) MessagesFor(substr string) []SpyMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []SpyMessage
	for _, m := range s.msgs {
		if containsString(m.Subject, substr) {
			out = append(out, m)
		}
	}
	return out
}

// Count returns the total number of captured messages.
func (s *MessageSpy) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.msgs)
}

// Dump logs all captured messages as a summary table.
func (s *MessageSpy) Dump() {
	s.mu.Lock()
	msgs := make([]SpyMessage, len(s.msgs))
	copy(msgs, s.msgs)
	s.mu.Unlock()

	s.t.Logf("[spy] %d messages on %q:", len(msgs), s.pattern)
	for _, m := range msgs {
		s.t.Logf("[spy]   %s", m)
	}
}

func containsString(s, substr string) bool {
	return len(substr) == 0 || len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
