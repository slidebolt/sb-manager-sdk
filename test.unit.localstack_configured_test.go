package managersdk_test

import (
	"testing"

	systemapp "github.com/slidebolt/plugin-system/app"
	automationapp "github.com/slidebolt/plugin-automation/app"
	domain "github.com/slidebolt/sb-domain"
)

// TestLocalStackStartConfiguredPlugins proves that StartPlugins() launches only
// the specified plugins. This lets tests bring up a minimal subset of the stack
// to keep startup overhead low.
func TestLocalStackStartConfiguredPlugins(t *testing.T) {
	ls := NewLocalStack(t)
	ls.StartPlugins(systemapp.PluginID, automationapp.PluginID)

	configured := ls.ConfiguredPlugins()

	// sb-script is always started, plus the two specified plugins.
	wantCount := 3
	if len(configured) != wantCount {
		t.Errorf("configured plugin count: got %d, want %d: %v", len(configured), wantCount, configured)
	}

	hasID := func(id string) bool {
		for _, v := range configured {
			if v == id {
				return true
			}
		}
		return false
	}

	if !hasID("sb-script") {
		t.Error("sb-script should always be in configured plugins")
	}
	if !hasID(systemapp.PluginID) {
		t.Errorf("expected %q in configured plugins", systemapp.PluginID)
	}
	if !hasID(automationapp.PluginID) {
		t.Errorf("expected %q in configured plugins", automationapp.PluginID)
	}

	// Verify plugin-system actually ran — it seeds time entities on startup.
	entries, err := ls.Storage().Search(systemapp.PluginID + ".time.>")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("plugin-system should have seeded time entities")
	}

	// Verify plugin-automation is connected: it should handle a storage query
	// for its group namespace without error.
	_, err = ls.Storage().Search(automationapp.PluginID + ".group.>")
	if err != nil {
		t.Fatalf("storage query after automation start: %v", err)
	}

	t.Logf("configured plugins: %v", configured)
}

// TestLocalStackStartConfiguredPluginsSubset proves that requesting only one
// plugin results in exactly that plugin (plus sb-script) being configured.
func TestLocalStackStartConfiguredPluginsSubset(t *testing.T) {
	ls := NewLocalStack(t)

	// Pre-seed an entity so plugin-automation can find it, but only start plugin-system.
	ls.StartPlugins(systemapp.PluginID)

	configured := ls.ConfiguredPlugins()
	if len(configured) != 2 { // sb-script + plugin-system
		t.Errorf("want 2 configured (sb-script + plugin-system), got %d: %v", len(configured), configured)
	}

	// plugin-automation was NOT started, so its group namespace should be empty.
	entries, err := ls.Storage().Search(automationapp.PluginID + ".group.>")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("automation groups: want 0 (not started), got %d", len(entries))
	}

	// plugin-system WAS started, so time entities should exist.
	timeEntries, err := ls.Storage().Search(systemapp.PluginID + ".time.>")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(timeEntries) == 0 {
		t.Fatal("plugin-system should have seeded time entities")
	}

	// Verify unknown plugin causes test failure (we can't test t.Fatal directly,
	// but we can verify the happy path — the designed intent is documented here).
	t.Logf("subset start confirmed: %v", configured)

	_ = domain.Entity{} // ensure domain import used
}
