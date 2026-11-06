package managersdk_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	asteriskhostapp "github.com/slidebolt/asterisk-host/app"
	asteriskapp "github.com/slidebolt/plugin-asterisk/app"
	opencodeapp "github.com/slidebolt/plugin-opencode/app"
	domain "github.com/slidebolt/sb-domain"
	mediaapp "github.com/slidebolt/sb-media/app"
	messenger "github.com/slidebolt/sb-messenger-sdk"
)

func keyOf(device, entity string) domain.EntityKey {
	return domain.EntityKey{Plugin: asteriskapp.PluginID, DeviceID: device, ID: entity}
}

// stackARI stands in for Asterisk. Everything else in these tests is real:
// asterisk-host, plugin-asterisk, sb-media, NATS, storage, RTP over UDP and HTTP.
type stackARI struct {
	srv *httptest.Server

	mu        sync.Mutex
	rtpTarget string
	channels  map[string]string // channel ID to dialed endpoint
	bridged   map[string]string // channel ID to bridge
	bridges   map[string]bool
	conn      *websocket.Conn
	writeMu   sync.Mutex
}

func newStackARI(t *testing.T) *stackARI {
	t.Helper()
	a := &stackARI{channels: map[string]string{}, bridged: map[string]string{}, bridges: map[string]bool{}}
	a.srv = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(func() {
		a.mu.Lock()
		if a.conn != nil {
			a.conn.Close()
		}
		a.mu.Unlock()
		a.srv.Close()
	})
	return a
}

func (a *stackARI) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/ari")
	if path == "/events" {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		a.mu.Lock()
		a.conn = conn
		a.mu.Unlock()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	q := r.URL.Query()
	switch {
	case path == "/endpoints/PJSIP":
		w.Write([]byte(`[{"technology":"PJSIP","resource":"201","state":"online"},
			{"technology":"PJSIP","resource":"3000","state":"online"},
			{"technology":"PJSIP","resource":"trunk1","state":"online"}]`))
		return
	case strings.HasPrefix(path, "/asterisk/config/dynamic/res_pjsip/endpoint/201"):
		w.Write([]byte(`[{"attribute":"callerid","value":"\"Kitchen Phone\" <201>"}]`))
		return
	case strings.HasPrefix(path, "/asterisk/config/"):
		w.Write([]byte(`[]`))
		return
	case path == "/asterisk/info":
		w.Write([]byte(`{"system":{"version":"22.7.0"}}`))
		return
	case r.Method == http.MethodGet:
		w.Write([]byte(`[]`))
		return
	case r.Method == http.MethodPost && path == "/bridges":
		a.bridges[q.Get("bridgeId")] = true
	case r.Method == http.MethodPost && path == "/channels/externalMedia":
		a.channels[q.Get("channelId")] = "media"
		a.rtpTarget = q.Get("external_host")
		go a.event("StasisStart", q.Get("channelId"))
	case r.Method == http.MethodPost && path == "/channels":
		a.channels[q.Get("channelId")] = q.Get("endpoint")
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/addChannel"):
		a.bridged[q.Get("channel")] = strings.Split(path, "/")[2]
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/channels/"):
		id := strings.TrimPrefix(path, "/channels/")
		delete(a.channels, id)
		delete(a.bridged, id)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/bridges/"):
		delete(a.bridges, strings.TrimPrefix(path, "/bridges/"))
	}
	w.Write([]byte(`{}`))
}

func (a *stackARI) event(typ, channel string) {
	a.mu.Lock()
	conn := a.conn
	a.mu.Unlock()
	if conn == nil {
		return
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	conn.WriteJSON(map[string]any{"type": typ, "channel": map[string]string{"id": channel}})
}

func (a *stackARI) channelFor(endpoint string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, ep := range a.channels {
		if ep == endpoint {
			return id
		}
	}
	return ""
}

func (a *stackARI) sameBridge(x, y string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	bx, bxOK := a.bridged[x]
	by, byOK := a.bridged[y]
	return bxOK && byOK && bx == by
}

func (a *stackARI) target() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rtpTarget
}

func (a *stackARI) resources() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.channels) + len(a.bridges)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func getEntity(t *testing.T, ls *LocalStack, key domain.EntityKey) (domain.Entity, bool) {
	t.Helper()
	raw, err := ls.Storage().Get(key)
	if err != nil {
		return domain.Entity{}, false
	}
	var e domain.Entity
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	return e, true
}

func clearStreamEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"MEDIA_BIND_ADDR", "MEDIA_PUBLIC_URL", "MEDIA_SIGNING_KEY", "MEDIA_RTP_HOST",
		"ASTERISK_ARI_URL", "ASTERISK_ARI_USER", "ASTERISK_ARI_PASS",
		"ASTERISK_HOST_URL", "ASTERISK_HOST_TOKEN", "ASTERISK_HOST_BIND",
		"ASTERISK_HOST_ENDPOINTS", "ASTERISK_HOST_TRUNKS", "ASTERISK_HOST_DIAL_ALLOW",
		"OPENCODE_HOST_URL", "OPENCODE_HOST_PASSWORD", "OPENCODE_AGENTS", "SPEECH_RTP_HOST",
	} {
		t.Setenv(name, "")
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return "127.0.0.1:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
}

// configureStream points the stack at the fake ARI and swaps ffmpeg for a
// passthrough, so these tests need no ffmpeg and output equals input.
func configureStream(t *testing.T, ari *stackARI) {
	t.Helper()
	clearStreamEnv(t)
	transcoder := filepath.Join(t.TempDir(), "transcoder")
	if err := os.WriteFile(transcoder, []byte("#!/bin/sh\nexec cat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mediaAddr := freeAddr(t)
	t.Setenv("MEDIA_BIND_ADDR", mediaAddr)
	t.Setenv("MEDIA_PUBLIC_URL", "http://"+mediaAddr)
	t.Setenv("MEDIA_SIGNING_KEY", strings.Repeat("k", 32))
	t.Setenv("MEDIA_RTP_HOST", "127.0.0.1")
	t.Setenv("MEDIA_RTP_PORT_MIN", "42200")
	t.Setenv("MEDIA_RTP_PORT_MAX", "42220")
	t.Setenv("MEDIA_FFMPEG_PATH", transcoder)

	hostAddr := freeAddr(t)
	t.Setenv("ASTERISK_HOST_BIND", hostAddr)
	t.Setenv("ASTERISK_HOST_URL", "http://"+hostAddr)
	t.Setenv("ASTERISK_HOST_TOKEN", "test-token")
	t.Setenv("ASTERISK_HOST_TRUNKS", "trunk1")
	t.Setenv("ASTERISK_HOST_ENDPOINTS", `{"3000":{"name":"Intercom","role":"intercom","autoAnswer":true,"listen":"on_command"}}`)
	t.Setenv("ASTERISK_ARI_URL", ari.srv.URL)
	t.Setenv("ASTERISK_ARI_USER", "u")
	t.Setenv("ASTERISK_ARI_PASS", "p")
}

func startAsteriskStack(t *testing.T, ari *stackARI) (*LocalStack, *messenger.Commands) {
	t.Helper()
	configureStream(t, ari)
	ls := NewLocalStack(t)
	ls.StartPlugins(mediaapp.ServiceID, asteriskhostapp.ServiceID, asteriskapp.PluginID)
	waitFor(t, "discovery", func() bool {
		_, ok := getEntity(t, ls, keyOf("trunk_trunk1", "registration"))
		return ok
	})
	return ls, messenger.NewCommands(ls.Messenger(), domain.LookupCommand)
}

// The same commands the Lua scripts send, through the whole stack.
func TestLocalStackPhoneThenIntercomArePatched(t *testing.T) {
	ari := newStackARI(t)
	ls, cmds := startAsteriskStack(t, ari)

	phone, _ := getEntity(t, ls, keyOf("ext_201", "registration"))
	if phone.Name != "Kitchen Phone" || phone.Type != "sip_endpoint" {
		t.Fatalf("discovered phone = %+v", phone)
	}

	if err := cmds.Send(keyOf("ext_201", "registration"), domain.SIPDial{Call: "front-door", RingSeconds: 30}); err != nil {
		t.Fatal(err)
	}
	participant := keyOf("call_front-door", "p_201")
	waitFor(t, "phone ringing", func() bool {
		e, ok := getEntity(t, ls, participant)
		return ok && e.State.(domain.SIPParticipant).Status == "ringing"
	})
	if ari.channelFor("PJSIP/3000") != "" {
		t.Fatal("intercom must not be called before the phone answers")
	}

	phoneChannel := ari.channelFor("PJSIP/201")
	ari.event("StasisStart", phoneChannel)
	waitFor(t, "phone joined", func() bool {
		e, ok := getEntity(t, ls, participant)
		return ok && e.State.(domain.SIPParticipant).Status == "joined"
	})

	if err := cmds.Send(keyOf("ext_3000", "registration"), domain.SIPDial{Call: "front-door"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "intercom ringing", func() bool { return ari.channelFor("PJSIP/3000") != "" })
	intercomChannel := ari.channelFor("PJSIP/3000")
	ari.event("StasisStart", intercomChannel)
	waitFor(t, "patched", func() bool { return ari.sameBridge(phoneChannel, intercomChannel) })

	endpoint, _ := getEntity(t, ls, keyOf("ext_3000", "registration"))
	if s := endpoint.State.(domain.SIPEndpoint); !s.InCall || s.Call != "front-door" {
		t.Fatalf("intercom = %+v", s)
	}

	if err := cmds.Send(keyOf("call_front-door", "call"), domain.SIPHangup{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "call ended", func() bool {
		e, ok := getEntity(t, ls, keyOf("call_front-door", "call"))
		return ok && e.State.(domain.SIPCall).Status == "ended"
	})
	waitFor(t, "asterisk cleanup", func() bool { return ari.resources() == 0 })
}

func TestLocalStackIntercomAudioFromCommandToURL(t *testing.T) {
	ari := newStackARI(t)
	ls, cmds := startAsteriskStack(t, ari)
	audio := keyOf("ext_3000", "audio")

	var url string
	waitFor(t, "stream url", func() bool {
		e, ok := getEntity(t, ls, audio)
		if !ok {
			return false
		}
		s := e.State.(domain.MediaStream)
		url = s.URL
		return s.Status == "idle" && url != ""
	})
	if !strings.Contains(url, "token=") {
		t.Fatal("url has no token")
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("idle status %d", resp.StatusCode)
	}
	if ari.channelFor("PJSIP/3000") != "" {
		t.Fatal("connecting to the URL must not call the intercom")
	}

	if err := cmds.Send(audio, domain.MediaStreamStart{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "external media channel", func() bool { return ari.target() != "" })
	raddr, err := net.ResolveUDPAddr("udp", ari.target())
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rtp := func(payload string) []byte {
		return append([]byte{0x80, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1}, payload...)
	}
	if _, err := conn.Write(rtp("first")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "live", func() bool {
		e, _ := getEntity(t, ls, audio)
		return e.State.(domain.MediaStream).Status == "live"
	})

	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live status %d", resp.StatusCode)
	}
	waitFor(t, "viewer count", func() bool {
		e, _ := getEntity(t, ls, audio)
		return e.State.(domain.MediaStream).Viewers == 1
	})
	if _, err := conn.Write(rtp("audio")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "audio" {
		t.Fatalf("url served %q", buf)
	}

	if err := cmds.Send(audio, domain.MediaStreamStop{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "idle", func() bool {
		e, _ := getEntity(t, ls, audio)
		return e.State.(domain.MediaStream).Status == "idle"
	})
	waitFor(t, "asterisk cleanup", func() bool { return ari.resources() == 0 })
	if e, _ := getEntity(t, ls, audio); e.State.(domain.MediaStream).URL != url {
		t.Fatal("the URL must stay the same")
	}
}

// The same chain a voice script runs, with audio replaced by a transcript event and
// the model replaced by a fake OpenCode server.
func TestLocalStackPromptReachesAgentEntity(t *testing.T) {
	clearStreamEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/session":
			w.Write([]byte(`{"id":"ses_1"}`))
		case strings.HasSuffix(r.URL.Path, "/message"):
			w.Write([]byte(`{"info":{},"parts":[{"type":"text","text":"The lights are on."}]}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	t.Setenv("OPENCODE_HOST_URL", server.URL)
	t.Setenv("OPENCODE_HOST_PASSWORD", "p")
	t.Setenv("OPENCODE_AGENTS", "voice")
	ls := NewLocalStack(t)
	ls.StartPlugins(opencodeapp.PluginID)
	key := domain.EntityKey{Plugin: opencodeapp.PluginID, DeviceID: "agent_voice", ID: "prompt"}
	waitFor(t, "agent", func() bool { _, ok := getEntity(t, ls, key); return ok })

	cmds := messenger.NewCommands(ls.Messenger(), domain.LookupCommand)
	if err := cmds.Send(key, domain.AgentPrompt{Text: "turn the lights on", Session: "voice"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "result", func() bool {
		e, _ := getEntity(t, ls, key)
		s := e.State.(domain.Agent)
		return s.Status == "idle" && s.Result == "The lights are on." && s.Prompt == "turn the lights on"
	})
}

// In production this config may be absent. sb-manager aborts startAll when a
// binary fails to start, so none of these may take the rest of the system down.
func TestLocalStackFullStartWithoutAsteriskConfig(t *testing.T) {
	clearStreamEnv(t)
	ls := NewLocalStack(t)
	ls.Start()

	waitFor(t, "server status", func() bool {
		e, ok := getEntity(t, ls, keyOf("server", "status"))
		return ok && !e.State.(domain.PBX).Connected
	})
	if entries, err := ls.Storage().Search(asteriskapp.PluginID + ".ext_3000.>"); err != nil || len(entries) != 0 {
		t.Fatalf("devices must come from Asterisk: %v %v", entries, err)
	}
}
