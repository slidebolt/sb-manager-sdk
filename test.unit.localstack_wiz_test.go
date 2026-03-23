package managersdk_test

import (
	"encoding/json"
	"testing"

	wizapp "github.com/slidebolt/plugin-wiz/app"
	domain "github.com/slidebolt/sb-domain"
	automationapp "github.com/slidebolt/sb-plugin-automation/app"
)

// TestLocalStackPluginAutomationCreatesGroupFromAutodiscoveredWizEntity proves
// that when plugin-wiz discovers a light and tags it with a PluginAutomation
// label, plugin-automation creates the corresponding group during startup discovery.
//
// The entity is pre-seeded (simulating what plugin-wiz would store after UDP
// discovery) before the automation plugin starts.
func TestLocalStackPluginAutomationCreatesGroupFromAutodiscoveredWizEntity(t *testing.T) {
	const (
		groupName = "LivingRoom"
		deviceID  = "wiz_device_abc123"
		entityID  = "wiz_light_abc123"
	)

	ls := NewLocalStack(t)
	store := ls.Storage()

	// Seed a wiz light entity tagged for the LivingRoom group.
	// This mirrors what plugin-wiz stores after UDP discovery.
	entity := domain.Entity{
		ID:       entityID,
		Plugin:   wizapp.PluginID,
		DeviceID: deviceID,
		Type:     "light",
		Name:     "Wiz Living Room",
		Commands: []string{"light_turn_on", "light_turn_off", "light_set_brightness"},
		State:    domain.Light{Power: false, Brightness: 100},
	}
	if err := store.Save(entity); err != nil {
		t.Fatalf("seed wiz entity: %v", err)
	}
	profile, _ := json.Marshal(map[string]any{
		"labels": map[string][]string{"PluginAutomation": {groupName}},
		"meta":   map[string]json.RawMessage{"PluginAutomation:" + groupName: json.RawMessage(`{"entity":"light","position":0}`)},
	})
	if err := store.SetProfile(entity, json.RawMessage(profile)); err != nil {
		t.Fatalf("seed wiz entity setprofile: %v", err)
	}

	// Start only plugin-automation — it discovers the pre-seeded wiz entity.
	ls.StartPlugins(automationapp.PluginID)

	// Verify plugin-automation created the LivingRoom group.
	groupID := automationapp.NormalizeGroupID(groupName)
	groupKey := domain.EntityKey{
		Plugin:   automationapp.PluginID,
		DeviceID: "group",
		ID:       groupID,
	}
	raw, err := store.Get(groupKey)
	if err != nil {
		t.Fatalf("group %q not found: %v", groupID, err)
	}
	if len(raw) == 0 {
		t.Fatalf("group %q entity is empty", groupID)
	}

	t.Logf("group %q created from auto-discovered wiz entity %q", groupName, entityID)
}

// TestLocalStackPluginAutomationCreatesGroupsFromMultipleAutodiscoverySources
// proves that entities from different plugins (wiz + another device type) tagged
// with different group names each produce their own group entity.
func TestLocalStackPluginAutomationCreatesGroupsFromMultipleAutodiscoverySources(t *testing.T) {
	const (
		wizGroup  = "WizLights"
		kasaGroup = "KasaSwitches"
	)

	ls := NewLocalStack(t)
	store := ls.Storage()

	// Seed a wiz light tagged for WizLights group.
	wizEntity := domain.Entity{
		ID:       "wiz_light_001",
		Plugin:   wizapp.PluginID,
		DeviceID: "wiz_device_001",
		Type:     "light",
		Name:     "Wiz Bedroom",
		State:    domain.Light{Power: false},
	}
	if err := store.Save(wizEntity); err != nil {
		t.Fatalf("seed wiz entity: %v", err)
	}
	wizProfile, _ := json.Marshal(map[string]any{
		"labels": map[string][]string{"PluginAutomation": {wizGroup}},
		"meta":   map[string]json.RawMessage{"PluginAutomation:" + wizGroup: json.RawMessage(`{"entity":"light","position":0}`)},
	})
	if err := store.SetProfile(wizEntity, json.RawMessage(wizProfile)); err != nil {
		t.Fatalf("seed wiz entity setprofile: %v", err)
	}

	// Seed a second entity (different plugin namespace) tagged for KasaSwitches group.
	kasaEntity := domain.Entity{
		ID:       "kasa_switch_001",
		Plugin:   "plugin-kasa",
		DeviceID: "kasa_device_001",
		Type:     "switch",
		Name:     "Kasa Hall Switch",
		State:    domain.Switch{Power: false},
	}
	if err := store.Save(kasaEntity); err != nil {
		t.Fatalf("seed kasa entity: %v", err)
	}
	kasaProfile, _ := json.Marshal(map[string]any{
		"labels": map[string][]string{"PluginAutomation": {kasaGroup}},
		"meta":   map[string]json.RawMessage{"PluginAutomation:" + kasaGroup: json.RawMessage(`{"entity":"switch","position":0}`)},
	})
	if err := store.SetProfile(kasaEntity, json.RawMessage(kasaProfile)); err != nil {
		t.Fatalf("seed kasa entity setprofile: %v", err)
	}

	// Start plugin-automation — discovers both groups simultaneously.
	ls.StartPlugins(automationapp.PluginID)

	// Both groups should now exist.
	for _, groupName := range []string{wizGroup, kasaGroup} {
		groupID := automationapp.NormalizeGroupID(groupName)
		groupKey := domain.EntityKey{
			Plugin:   automationapp.PluginID,
			DeviceID: "group",
			ID:       groupID,
		}
		raw, err := store.Get(groupKey)
		if err != nil {
			t.Errorf("group %q not found: %v", groupName, err)
			continue
		}
		if len(raw) == 0 {
			t.Errorf("group %q entity is empty", groupName)
		}
		t.Logf("group %q created from multiple autodiscovery sources", groupName)
	}

	// Verify exactly two groups were created.
	entries, err := store.Search(automationapp.PluginID + ".group.>")
	if err != nil {
		t.Fatalf("search groups: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 group entities, got %d", len(entries))
	}
}
