package managersdk_test

import (
	"testing"

	amcrestapp "github.com/slidebolt/plugin-amcrest/app"
	androidtvapp "github.com/slidebolt/plugin-androidtv/app"
	esphomeapp "github.com/slidebolt/plugin-esphome/app"
	frigateapp "github.com/slidebolt/plugin-frigate/app"
	kasaapp "github.com/slidebolt/plugin-kasa/app"
	systemapp "github.com/slidebolt/plugin-system/app"
	wizapp "github.com/slidebolt/plugin-wiz/app"
	z2mapp "github.com/slidebolt/plugin-zigbee2mqtt/app"
	automationapp "github.com/slidebolt/plugin-automation/app"
)

// TestLocalStackReportConfiguredPlugins proves that after Start() every
// expected plugin ID appears in ConfiguredPlugins().
func TestLocalStackReportConfiguredPlugins(t *testing.T) {
	ls := NewLocalStack(t)
	ls.Start()

	configured := ls.ConfiguredPlugins()

	wantIDs := []string{
		"sb-script",
		systemapp.PluginID,
		automationapp.PluginID,
		esphomeapp.PluginID,
		wizapp.PluginID,
		z2mapp.PluginID,
		amcrestapp.PluginID,
		androidtvapp.PluginID,
		frigateapp.PluginID,
		kasaapp.PluginID,
	}

	idSet := make(map[string]bool, len(configured))
	for _, id := range configured {
		idSet[id] = true
	}

	for _, want := range wantIDs {
		if !idSet[want] {
			t.Errorf("expected plugin %q in configured plugins, got: %v", want, configured)
		}
	}

	t.Logf("configured plugins (%d): %v", len(configured), configured)
}
