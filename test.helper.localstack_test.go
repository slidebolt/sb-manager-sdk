// LocalStack is a full in-process stack for integration tests.
// It starts every service and plugin in dependency order so tests
// can exercise cross-plugin behaviour without running any real infra.
//
// This file lives in package managersdk_test (not managersdk) to avoid
// import cycles: plugins import sb-manager-sdk, so sb-manager-sdk cannot
// import plugins in non-test code.
package managersdk_test

import (
	"encoding/json"
	"testing"

	amcrestapp "github.com/slidebolt/plugin-amcrest/app"
	androidtvapp "github.com/slidebolt/plugin-androidtv/app"
	automationapp "github.com/slidebolt/plugin-automation/app"
	esphomeapp "github.com/slidebolt/plugin-esphome/app"
	frigateapp "github.com/slidebolt/plugin-frigate/app"
	kasaapp "github.com/slidebolt/plugin-kasa/app"
	systemapp "github.com/slidebolt/plugin-system/app"
	wizapp "github.com/slidebolt/plugin-wiz/app"
	z2mapp "github.com/slidebolt/plugin-zigbee2mqtt/app"
	messenger "github.com/slidebolt/sb-messenger-sdk"
	scriptserver "github.com/slidebolt/sb-script/server"
	storage "github.com/slidebolt/sb-storage-sdk"
	testkit "github.com/slidebolt/sb-testkit"
)

// pluginApp is the minimal interface every plugin app satisfies.
type pluginApp interface {
	OnStart(deps map[string]json.RawMessage) (json.RawMessage, error)
	OnShutdown() error
}

// LocalStack runs all services and plugins in-process. Call NewLocalStack(t)
// to create the stack with messenger and storage ready. Call Start() or
// StartPlugins() to launch plugins — callers may seed storage before calling
// Start() so that plugins see pre-existing data during their OnStart phase.
type LocalStack struct {
	t             *testing.T
	env           *testkit.TestEnv
	configuredIDs []string
}

// NewLocalStack creates a LocalStack with messenger and storage started.
// Plugins are not yet running — call Start() or StartPlugins() after
// optionally pre-seeding storage.
func NewLocalStack(t *testing.T) *LocalStack {
	t.Helper()

	env := testkit.NewTestEnv(t)
	env.Start("messenger")
	env.Start("storage")

	return &LocalStack{t: t, env: env}
}

// Start launches sb-script and all plugins in dependency order:
//  1. sb-script (scripting engine)
//  2. plugin-system (seeds time/location entities)
//  3. plugin-automation (discovers groups from labelled entities)
//  4. remaining plugins in any order
func (ls *LocalStack) Start() *LocalStack {
	ls.t.Helper()

	ls.startScript()

	ls.startPlugin(systemapp.PluginID, systemapp.New())
	ls.startPlugin(automationapp.PluginID, automationapp.New())
	ls.startPlugin(esphomeapp.PluginID, esphomeapp.New())
	ls.startPlugin(wizapp.PluginID, wizapp.New())
	ls.startPlugin(z2mapp.PluginID, z2mapp.New())
	ls.startPlugin(amcrestapp.PluginID, amcrestapp.New())
	ls.startPlugin(androidtvapp.PluginID, androidtvapp.New())
	ls.startPlugin(frigateapp.PluginID, frigateapp.New())
	ls.startPlugin(kasaapp.PluginID, kasaapp.New())

	return ls
}

// StartPlugins launches sb-script and only the named plugins (by plugin ID).
// Unknown IDs cause the test to fail immediately.
func (ls *LocalStack) StartPlugins(names ...string) *LocalStack {
	ls.t.Helper()

	ls.startScript()

	pluginFactories := map[string]func() pluginApp{
		systemapp.PluginID:     func() pluginApp { return systemapp.New() },
		automationapp.PluginID: func() pluginApp { return automationapp.New() },
		esphomeapp.PluginID:    func() pluginApp { return esphomeapp.New() },
		wizapp.PluginID:        func() pluginApp { return wizapp.New() },
		z2mapp.PluginID:        func() pluginApp { return z2mapp.New() },
		amcrestapp.PluginID:    func() pluginApp { return amcrestapp.New() },
		androidtvapp.PluginID:  func() pluginApp { return androidtvapp.New() },
		frigateapp.PluginID:    func() pluginApp { return frigateapp.New() },
		kasaapp.PluginID:       func() pluginApp { return kasaapp.New() },
	}

	for _, name := range names {
		factory, ok := pluginFactories[name]
		if !ok {
			ls.t.Fatalf("localstack: unknown plugin %q", name)
		}
		ls.startPlugin(name, factory())
	}

	return ls
}

// ConfiguredPlugins returns the IDs of all services and plugins that have
// been started (including sb-script if running).
func (ls *LocalStack) ConfiguredPlugins() []string {
	return ls.configuredIDs
}

// Messenger returns the shared messenger client.
func (ls *LocalStack) Messenger() messenger.Messenger {
	return ls.env.Messenger()
}

// Storage returns the storage client.
func (ls *LocalStack) Storage() storage.Storage {
	return ls.env.Storage()
}

// startScript starts the sb-script engine on a dedicated messenger connection.
func (ls *LocalStack) startScript() {
	ls.t.Helper()

	scriptMsg, err := messenger.Connect(map[string]json.RawMessage{
		"messenger": ls.env.MessengerPayload(),
	})
	if err != nil {
		ls.t.Fatalf("localstack: sb-script messenger: %v", err)
	}
	svc, err := scriptserver.New(scriptMsg, ls.env.Storage())
	if err != nil {
		ls.t.Fatalf("localstack: sb-script start: %v", err)
	}
	// Flush ensures NATS has registered subscriptions before first request.
	if err := scriptMsg.Flush(); err != nil {
		ls.t.Fatalf("localstack: sb-script flush: %v", err)
	}
	ls.t.Cleanup(func() { svc.Shutdown(); scriptMsg.Close() })
	ls.configuredIDs = append(ls.configuredIDs, "sb-script")
}

// startPlugin calls OnStart on the plugin and registers OnShutdown cleanup.
func (ls *LocalStack) startPlugin(id string, p pluginApp) {
	ls.t.Helper()

	deps := map[string]json.RawMessage{
		"messenger": ls.env.MessengerPayload(),
		"storage":   ls.env.StoragePayload(),
	}
	if _, err := p.OnStart(deps); err != nil {

		ls.t.Fatalf("localstack: start %s: %v", id, err)
	}
	ls.t.Cleanup(func() { p.OnShutdown() }) //nolint:errcheck
	ls.configuredIDs = append(ls.configuredIDs, id)
}
