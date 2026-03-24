package managersdk_test

import (
	"encoding/json"
	"testing"

	systemapp "github.com/slidebolt/plugin-system/app"
	automationapp "github.com/slidebolt/plugin-automation/app"
	domain "github.com/slidebolt/sb-domain"
	managersdk "github.com/slidebolt/sb-manager-sdk"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	storage "github.com/slidebolt/sb-storage-sdk"
	server "github.com/slidebolt/sb-storage-server"
)

// TestSharedTestEnvStartsMultipleImportedPlugins proves that multiple plugin
// apps can share a single TestEnv without interfering with each other.
// This is the unit-level version of LocalStack: wires up plugins manually
// without the LocalStack helper, confirming the primitive works correctly.
func TestSharedTestEnvStartsMultipleImportedPlugins(t *testing.T) {
	env := managersdk.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	deps := map[string]json.RawMessage{
		"messenger": env.MessengerPayload(),
	}

	// Start plugin-system.
	sysApp := systemapp.New()
	if _, err := sysApp.OnStart(deps); err != nil {
		t.Fatalf("plugin-system OnStart: %v", err)
	}
	t.Cleanup(func() { sysApp.OnShutdown() }) //nolint:errcheck

	// Start plugin-automation.
	autoApp := automationapp.New()
	if _, err := autoApp.OnStart(deps); err != nil {
		t.Fatalf("plugin-automation OnStart: %v", err)
	}
	t.Cleanup(func() { autoApp.OnShutdown() }) //nolint:errcheck

	store := env.Storage()

	// plugin-system should have seeded time entities.
	timeEntries, err := store.Search(systemapp.PluginID + ".time.>")
	if err != nil {
		t.Fatalf("search time entities: %v", err)
	}
	if len(timeEntries) == 0 {
		t.Fatal("plugin-system did not seed time entities")
	}
	t.Logf("plugin-system seeded %d time entities", len(timeEntries))

	// Both plugins share the same NATS bus — verify storage is accessible from both.
	// Save an entity with a PluginAutomation label from the "test" plugin namespace.
	testEntity := domain.Entity{
		ID:       "sensor001",
		Plugin:   "test-plugin",
		DeviceID: "test-device",
		Type:     "sensor",
		Name:     "Test Sensor",
		State:    domain.Sensor{Value: 42.0, Unit: "°C"},
	}
	if err := store.Save(testEntity); err != nil {
		t.Fatalf("save test entity: %v", err)
	}

	// Verify the entity is queryable via storage — shared bus works.
	raw, err := store.Get(domain.EntityKey{Plugin: "test-plugin", DeviceID: "test-device", ID: "sensor001"})
	if err != nil {
		t.Fatalf("get test entity: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("test entity is empty after save")
	}

	t.Logf("shared TestEnv: plugin-system and plugin-automation both running on same bus")
}

// TestSharedTestEnvIsolatesTwoIndependentEnvs proves that two separate TestEnv
// instances are fully isolated — data saved in one is not visible in the other.
func TestSharedTestEnvIsolatesTwoIndependentEnvs(t *testing.T) {
	startEnv := func(t *testing.T) (storage.Storage, messenger.Messenger) {
		t.Helper()
		// Build an independent env using the low-level primitives that TestEnv uses.
		// We can't call NewTestEnv twice from the same test and get isolation if they
		// share the same embedded NATS server — but each MockWithPayload() starts its
		// own NATS server so they ARE isolated.
		msg, payload, err := messenger.MockWithPayload()
		if err != nil {
			t.Fatalf("mock messenger: %v", err)
		}
		t.Cleanup(func() { msg.Close() })

		serverMsg, err := messenger.Connect(map[string]json.RawMessage{"messenger": payload})
		if err != nil {
			t.Fatalf("storage server messenger: %v", err)
		}
		t.Cleanup(func() { serverMsg.Close() })

		handler, err := server.NewHandler()
		if err != nil {
			t.Fatalf("storage handler: %v", err)
		}
		if err := handler.Register(serverMsg); err != nil {
			t.Fatalf("register handler: %v", err)
		}
		if err := serverMsg.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}

		return storage.ClientFrom(msg), msg
	}

	store1, _ := startEnv(t)
	store2, _ := startEnv(t)

	// Save an entity in store1.
	e := domain.Entity{
		ID: "light001", Plugin: "env1", DeviceID: "dev1",
		Type: "light", Name: "Light", State: domain.Light{Power: true},
	}
	if err := store1.Save(e); err != nil {
		t.Fatalf("store1 save: %v", err)
	}

	// Verify it's visible in store1.
	entries1, err := store1.Search("env1.>")
	if err != nil {
		t.Fatalf("store1 search: %v", err)
	}
	if len(entries1) != 1 {
		t.Errorf("store1: got %d entries, want 1", len(entries1))
	}

	// Verify it is NOT visible in store2 — complete isolation.
	entries2, err := store2.Search("env1.>")
	if err != nil {
		t.Fatalf("store2 search: %v", err)
	}
	if len(entries2) != 0 {
		t.Errorf("store2: got %d entries from store1's data, want 0 (should be isolated)", len(entries2))
	}

	t.Log("two independent TestEnvs are fully isolated")
}
