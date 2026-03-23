package managersdk_test

import (
	"encoding/json"
	"testing"

	systemapp "github.com/slidebolt/plugin-system/app"
	automationapp "github.com/slidebolt/sb-plugin-automation/app"
	domain "github.com/slidebolt/sb-domain"
	storage "github.com/slidebolt/sb-storage-sdk"
)

// TestLocalStackStartsPluginSystem proves that NewLocalStack + Start() correctly
// boots plugin-system, which seeds time entities into storage on startup.
func TestLocalStackStartsPluginSystem(t *testing.T) {
	ls := NewLocalStack(t)
	ls.Start()

	store := ls.Storage()

	// plugin-system seeds time entities during OnStart. Verify at least
	// the "hour" entity exists under plugin-system.time.hour.
	key := domain.EntityKey{
		Plugin:   systemapp.PluginID,
		DeviceID: "time",
		ID:       "hour",
	}
	raw, err := store.Get(key)
	if err != nil {
		t.Fatalf("plugin-system time entity not found: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("plugin-system time entity is empty")
	}

	// Verify the full set of time entities.
	entries, err := store.Search(systemapp.PluginID + ".time.>")
	if err != nil {
		t.Fatalf("search plugin-system time entities: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected plugin-system to seed time entities, got none")
	}
	t.Logf("plugin-system seeded %d time entities", len(entries))
}

// TestLocalStackPluginAutomationCreatesGroupFromTaggedPluginSystemEntity proves
// the cross-plugin automation flow: a plugin-system entity tagged with a
// PluginAutomation label causes plugin-automation to create a group entity
// during its OnStart discovery pass.
func TestLocalStackPluginAutomationCreatesGroupFromTaggedPluginSystemEntity(t *testing.T) {
	const groupName = "Morning"
	groupID := automationapp.NormalizeGroupID(groupName)

	ls := NewLocalStack(t)
	store := ls.Storage()

	// Seed a plugin-system entity with a PluginAutomation group label.
	// plugin-automation's discoverGroups() reads this during OnStart.
	entity := domain.Entity{
		ID:       "hour",
		Plugin:   systemapp.PluginID,
		DeviceID: "time",
		Type:     "time",
		Name:     "Hour",
		State:    systemapp.Time{Hour: 7},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("seed entity: %v", err)
	}

	profile, _ := json.Marshal(map[string]any{
		"labels": map[string][]string{
			"PluginAutomation": {groupName},
		},
		"meta": map[string]json.RawMessage{
			"PluginAutomation:" + groupName: json.RawMessage(`{"entity":"light","position":0}`),
		},
	})
	if err := store.SetProfile(entity, json.RawMessage(profile)); err != nil {
		t.Fatalf("seed entity setprofile: %v", err)
	}

	// Now start all plugins — automation's OnStart calls discoverGroups synchronously.
	ls.Start()

	// Verify plugin-automation created the group entity.
	groupKey := domain.EntityKey{
		Plugin:   automationapp.PluginID,
		DeviceID: "group",
		ID:       groupID,
	}
	raw, err := store.Get(groupKey)
	if err != nil {
		t.Fatalf("group entity %q not found: %v", groupID, err)
	}
	if len(raw) == 0 {
		t.Fatalf("group entity %q is empty", groupID)
	}
	t.Logf("plugin-automation created group %q from tagged plugin-system entity", groupName)

	// Verify the entity key format follows plugin-automation.group.<id>
	entries, err := store.Search(automationapp.PluginID + ".group.>")
	if err != nil {
		t.Fatalf("search group entities: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one group entity")
	}
	t.Logf("total group entities: %d", len(entries))
}

// seedLabelledEntity saves an entity with a PluginAutomation label and meta.
func seedLabelledEntity(t *testing.T, store storage.Storage, plugin, device, id, typ, groupName, entityType string) {
	t.Helper()
	e := domain.Entity{
		ID:       id,
		Plugin:   plugin,
		DeviceID: device,
		Type:     typ,
		Name:     id,
		State:    domain.Light{Power: false},
	}
	if err := store.Save(e); err != nil {
		t.Fatalf("seedLabelledEntity %s: %v", id, err)
	}

	profile, _ := json.Marshal(map[string]any{
		"labels": map[string][]string{
			"PluginAutomation": {groupName},
		},
		"meta": map[string]json.RawMessage{
			"PluginAutomation:" + groupName: json.RawMessage(`{"entity":"` + entityType + `","position":0}`),
		},
	})
	if err := store.SetProfile(e, json.RawMessage(profile)); err != nil {
		t.Fatalf("seedLabelledEntity setprofile %s: %v", id, err)
	}
}
