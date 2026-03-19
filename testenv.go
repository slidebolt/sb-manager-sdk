// Package managersdk provides a test harness that simulates the SlideBolt
// manager for unit tests. It wires up mock services in dependency order
// so binaries can be tested in isolation without running infrastructure.
package managersdk

import (
	"encoding/json"
	"testing"

	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
	server "github.com/slidebolt/sb-storage-server"
)

// TestEnv simulates the manager for tests. It creates mock services
// in the correct dependency order and provides typed accessors.
type TestEnv struct {
	t                *testing.T
	msg              messenger.Messenger
	messengerPayload json.RawMessage
	sch              storage.Storage
}

// NewTestEnv creates a test environment with a shared messenger bus.
// All mock services attach to this bus, just like production.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	msg, payload, err := messenger.MockWithPayload()
	if err != nil {
		t.Fatalf("manager-sdk: create messenger mock: %v", err)
	}
	t.Cleanup(func() { msg.Close() })

	return &TestEnv{t: t, msg: msg, messengerPayload: payload}
}

// Start initializes a mock service by name. Services are started in
// the order you call them — the caller is responsible for dependency
// order, same as declaring dependsOn in production.
func (e *TestEnv) Start(service string) {
	e.t.Helper()

	switch service {
	case "messenger":
		// Already started in NewTestEnv.
	case "storage":
		// The storage server gets its own dedicated connection — matching production
		// topology where each service is a separate process. Sharing env.msg between
		// server and client causes the client's Request calls to compete with the
		// server's subscription callbacks on the same NATS connection object.
		serverMsg, err := messenger.Connect(map[string]json.RawMessage{
			"messenger": e.messengerPayload,
		})
		if err != nil {
			e.t.Fatalf("manager-sdk: storage server messenger: %v", err)
		}
		e.t.Cleanup(func() { serverMsg.Close() })

		handler, err := server.NewHandler()
		if err != nil {
			e.t.Fatalf("manager-sdk: create storage handler: %v", err)
		}
		if err := handler.Register(serverMsg); err != nil {
			e.t.Fatalf("manager-sdk: register storage handler: %v", err)
		}
		// Flush ensures the SUB command has been received by the NATS server
		// before the client sends its first request.
		if err := serverMsg.Flush(); err != nil {
			e.t.Fatalf("manager-sdk: flush storage server messenger: %v", err)
		}
		e.sch = storage.ClientFrom(e.msg)
	default:
		e.t.Fatalf("manager-sdk: unknown service %q", service)
	}
}

// Messenger returns the shared messenger client.
func (e *TestEnv) Messenger() messenger.Messenger {
	return e.msg
}

// MessengerPayload returns the raw JSON payload that services use to connect
// to the shared messenger bus (contains NATS host and port).
func (e *TestEnv) MessengerPayload() json.RawMessage {
	return e.messengerPayload
}

// Storage returns the storage client. Panics if storage was not started.
func (e *TestEnv) Storage() storage.Storage {
	if e.sch == nil {
		e.t.Fatal("manager-sdk: storage not started, call env.Start(\"storage\") first")
	}
	return e.sch
}
