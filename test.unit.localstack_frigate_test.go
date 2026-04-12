package managersdk_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	frigateapp "github.com/slidebolt/plugin-frigate/app"
	storage "github.com/slidebolt/sb-storage-sdk"
)

func TestLocalStackFrigateDiscoveryCreatesEntities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/config":
			fmt.Fprintln(w, `{
				"cameras": {
					"indoor_monitor_02": {
						"name": "indoor_monitor_02",
						"enabled": true,
						"detect": {"enabled": true},
						"motion": {"enabled": true},
						"record": {"enabled": false},
						"snapshots": {"enabled": true},
						"review": {"alerts": {"enabled": true}, "detections": {"enabled": true}},
						"objects": {"track": ["person", "dog"]},
						"zones": {"desk": {}, "door": {}}
					}
				}
			}`)
		case "/api/events":
			fmt.Fprintln(w, `[
				{
					"id": "evt-1",
					"label": "person",
					"camera": "indoor_monitor_02",
					"start_time": 1710000000,
					"has_clip": true,
					"has_snapshot": true,
					"data": {"type": "object", "score": 0.98}
				}
			]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("FRIGATE_URL", server.URL)
	t.Setenv("FRIGATE_EVENT_LIMIT", "20")

	ls := NewLocalStack(t)
	ls.StartPlugins(frigateapp.PluginID)

	// Wait for discovery to complete (async in OnStart)
	var entries []storage.Entry
	for i := 0; i < 20; i++ {
		var err error
		entries, err = ls.Storage().Query(storage.Query{
			Pattern: frigateapp.PluginID + ".indoor_monitor_02.>",
		})
		if err == nil && len(entries) >= 10 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(entries) < 10 {
		keys := make([]string, len(entries))
		for i, e := range entries {
			keys[i] = e.Key
		}
		t.Fatalf("frigate startup created %d entries, want at least 10: %v", len(entries), keys)
	}

	raw, err := ls.Storage().Get(frigateKey{plugin: frigateapp.PluginID, device: "indoor_monitor_02", entity: "camera-state"})
	if err != nil {
		t.Fatalf("get camera entity: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal camera entity: %v", err)
	}
	if doc["type"] != "frigate_camera_status" {
		t.Fatalf("camera type: got %v want frigate_camera_status", doc["type"])
	}

	if _, err := ls.Storage().Get(frigateKey{plugin: frigateapp.PluginID, device: "indoor_monitor_02", entity: "indoor_monitor_02"}); err == nil {
		t.Fatal("unexpected legacy root camera entity plugin-frigate.indoor_monitor_02.indoor_monitor_02")
	}
}

type frigateKey struct {
	plugin string
	device string
	entity string
}

func (k frigateKey) Key() string {
	return k.plugin + "." + k.device + "." + k.entity
}
