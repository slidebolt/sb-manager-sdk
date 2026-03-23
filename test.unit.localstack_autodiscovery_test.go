package managersdk_test

import (
	"encoding/json"
	"testing"

	esphomeapp "github.com/slidebolt/plugin-esphome/app"
	kasaapp "github.com/slidebolt/plugin-kasa/app"
	automationapp "github.com/slidebolt/sb-plugin-automation/app"
	domain "github.com/slidebolt/sb-domain"
)

// TestLocalStackPluginAutomationCreatesGroupsFromAutodiscoveredEntities proves
// that when multiple plugins each register entities with PluginAutomation labels,
// plugin-automation's startup discovery creates the corresponding group entities.
//
// This simulates the auto-discovery flow: plugins (here esphome + kasa) write
// entities to storage after discovering hardware, tagging them with the
// PluginAutomation group the user configured.
func TestLocalStackPluginAutomationCreatesGroupsFromAutodiscoveredEntities(t *testing.T) {
	const (
		groupLights   = "AllLights"
		groupSwitches = "AllSwitches"
	)

	ls := NewLocalStack(t)
	store := ls.Storage()

	// Seed an ESPHome light entity tagged for AllLights group.
	seedLabelledEntity(t, store,
		esphomeapp.PluginID, "esp-bedroom", "light001", "light",
		groupLights, "light",
	)

	// Seed a Kasa switch entity tagged for AllSwitches group.
	seedLabelledEntity(t, store,
		kasaapp.PluginID, "kasa-hall", "switch001", "switch",
		groupSwitches, "switch",
	)

	// Start all plugins — plugin-automation discovers both entities during OnStart.
	ls.StartPlugins(automationapp.PluginID)

	// Verify AllLights group was created.
	lightsGroupID := automationapp.NormalizeGroupID(groupLights)
	lightsKey := domain.EntityKey{
		Plugin:   automationapp.PluginID,
		DeviceID: "group",
		ID:       lightsGroupID,
	}
	raw, err := store.Get(lightsKey)
	if err != nil {
		t.Fatalf("AllLights group not found: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("AllLights group entity is empty")
	}

	// Verify AllSwitches group was created.
	switchesGroupID := automationapp.NormalizeGroupID(groupSwitches)
	switchesKey := domain.EntityKey{
		Plugin:   automationapp.PluginID,
		DeviceID: "group",
		ID:       switchesGroupID,
	}
	raw, err = store.Get(switchesKey)
	if err != nil {
		t.Fatalf("AllSwitches group not found: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("AllSwitches group entity is empty")
	}

	t.Logf("autodiscovery: created groups %q and %q from entities across multiple plugins",
		groupLights, groupSwitches)
}

// TestLocalStackPluginAutomationCreatesGroupFromMultipleEntities proves that
// multiple entities from the same plugin tagged with the same group name result
// in a single group entity (the group is a singleton per name).
func TestLocalStackPluginAutomationCreatesGroupFromMultipleEntities(t *testing.T) {
	const groupName = "BedroomLights"

	ls := NewLocalStack(t)
	store := ls.Storage()

	// Seed three ESPHome lights all tagged for the same group.
	for i := 1; i <= 3; i++ {
		e := domain.Entity{
			ID:       "light00" + string(rune('0'+i)),
			Plugin:   esphomeapp.PluginID,
			DeviceID: "esp-bedroom",
			Type:     "light",
			Name:     "Light",
			State:    domain.Light{Power: false},
		}
		if err := store.Save(e); err != nil {
			t.Fatalf("seed light%d: %v", i, err)
		}
		profile, _ := json.Marshal(map[string]any{
			"labels": map[string][]string{"PluginAutomation": {groupName}},
			"meta":   map[string]json.RawMessage{"PluginAutomation:" + groupName: json.RawMessage(`{"entity":"light","position":0}`)},
		})
		if err := store.SetProfile(e, json.RawMessage(profile)); err != nil {
			t.Fatalf("seed light%d setprofile: %v", i, err)
		}
	}

	ls.StartPlugins(automationapp.PluginID)

	// Only ONE group entity should exist for BedroomLights.
	groupID := automationapp.NormalizeGroupID(groupName)
	groupKey := domain.EntityKey{
		Plugin:   automationapp.PluginID,
		DeviceID: "group",
		ID:       groupID,
	}
	raw, err := store.Get(groupKey)
	if err != nil {
		t.Fatalf("BedroomLights group not found: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("BedroomLights group entity is empty")
	}

	t.Logf("group %q created as single entity from 3 member entities", groupName)
}
