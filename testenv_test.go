package managersdk

import (
	"testing"

	messenger "github.com/slidebolt/sb-messenger-sdk"
)

func TestEnvMessenger(t *testing.T) {
	env := NewTestEnv(t)
	env.Start("messenger")

	msg := env.Messenger()
	done := make(chan string, 1)

	msg.Subscribe("test.ping", func(m *messenger.Message) {
		done <- string(m.Data)
	})

	msg.Publish("test.ping", []byte("pong"))

	if got := <-done; got != "pong" {
		t.Errorf("got %q, want %q", got, "pong")
	}
}

func TestEnvSchema(t *testing.T) {
	env := NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	sch := env.Storage()

	item := testKV{K: "esphome.lightstrip.light001", Brightness: 255}
	if err := sch.Save(item); err != nil {
		t.Fatal(err)
	}

	got, err := sch.Get(keyStr("esphome.lightstrip.light001"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "" {
		t.Fatal("expected non-empty data")
	}

	entries, err := sch.Search("esphome.>")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}
}

// Test helpers — minimal Keyed types.
type keyStr string

func (k keyStr) Key() string { return string(k) }

type testKV struct {
	K          string `json:"-"`
	Brightness int    `json:"brightness"`
}

func (t testKV) Key() string { return t.K }
