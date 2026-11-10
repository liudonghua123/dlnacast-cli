// internal/player/player_test.go
package player

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"dlnacast-cli/internal/dlna"
	"dlnacast-cli/internal/server"
)

// fakeRenderer：与 Task 6 transport_test.go 同款（不同包需复制）。
type fakeRenderer struct {
	mu      sync.Mutex
	actions []string
	args    map[string]map[string]string
	pos     string
	total   string
	state   string
	volume  string
	lastURI string // 记录最近一次 SetAVTransportURI 的 CurrentURI
}

func newFakeRenderer() *fakeRenderer {
	return &fakeRenderer{args: map[string]map[string]string{}}
}

func (f *fakeRenderer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		action := fakeAction(r.Header.Get("SOAPACTION"))
		params := fakeParams(body)

		f.mu.Lock()
		f.actions = append(f.actions, action)
		f.args[action] = params
		if action == "SetAVTransportURI" {
			f.lastURI = params["CurrentURI"]
		}
		f.mu.Unlock()

		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		var resp map[string]string
		switch action {
		case "GetPositionInfo":
			resp = map[string]string{"RelTime": f.pos, "TrackDuration": f.total}
		case "GetTransportInfo":
			resp = map[string]string{"CurrentTransportState": f.state}
		case "GetVolume":
			resp = map[string]string{"CurrentVolume": f.volume}
		}
		w.Write([]byte(fakeRespXML(action, resp)))
	})
}

func fakeAction(hdr string) string {
	hdr = strings.Trim(hdr, `"`)
	if i := strings.LastIndex(hdr, "#"); i >= 0 {
		return hdr[i+1:]
	}
	return hdr
}

func fakeParams(body []byte) map[string]string {
	m := map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	inBody := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			if se.Name.Local == "Body" {
				inBody = true
				continue
			}
			if inBody {
				for {
					tok2, err2 := dec.Token()
					if err2 != nil {
						break
					}
					if se2, ok := tok2.(xml.StartElement); ok {
						var v string
						if dec.DecodeElement(&v, &se2) == nil {
							m[se2.Name.Local] = v
						}
					} else if _, ok := tok2.(xml.EndElement); ok {
						return m
					}
				}
			}
		}
	}
	return m
}

func fakeRespXML(action string, params map[string]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:`)
	b.WriteString(action + `Response xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">`)
	for k, v := range params {
		b.WriteString("<" + k + ">" + v + "</" + k + ">")
	}
	b.WriteString("</u:" + action + "Response></s:Body></s:Envelope>")
	return b.String()
}

// --- Session 测试 ---

func newTestSession(t *testing.T, files []*server.File) (*Session, *fakeRenderer) {
	fake := newFakeRenderer()
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(func() { srv.Close() })

	dev := &dlna.Device{
		Name:                "测试电视",
		Host:                "127.0.0.1",
		ControlURL:          srv.URL,
		RenderingControlURL: srv.URL,
	}
	s, err := NewSession(dev, files, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, fake
}

func TestSessionLoadLocalFile(t *testing.T) {
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "v.mp4")
	os.WriteFile(mp4, []byte("video"), 0o644)

	files := []*server.File{{Path: mp4, Title: "v", Type: "video"}}
	s, fake := newTestSession(t, files)

	if err := s.Load(0, 0); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if !strings.HasPrefix(fake.lastURI, "http://127.0.0.1:") || !strings.HasSuffix(fake.lastURI, "/m/0") {
		t.Errorf("lastURI = %q", fake.lastURI)
	}
}

func TestSessionLoadURLFile(t *testing.T) {
	files := []*server.File{{URL: "http://example.com/a.mp4", Title: "a", Type: "video"}}
	s, fake := newTestSession(t, files)

	if err := s.Load(0, 0); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.lastURI != "http://example.com/a.mp4" {
		t.Errorf("lastURI = %q", fake.lastURI)
	}
}

func TestSessionLoadIndexOutOfRange(t *testing.T) {
	files := []*server.File{{URL: "http://example.com/a.mp4", Type: "video"}}
	s, _ := newTestSession(t, files)
	if err := s.Load(1, 0); err == nil {
		t.Error("want error for out-of-range index")
	}
	if err := s.Load(-1, 0); err == nil {
		t.Error("want error for negative index")
	}
}

func TestSessionTogglePause(t *testing.T) {
	files := []*server.File{{URL: "http://example.com/a.mp4", Type: "video"}}
	s, fake := newTestSession(t, files)
	s.Load(0, 0)

	// 播放中 → TogglePause 应发 Pause
	fake.mu.Lock()
	fake.state = "PLAYING"
	fake.mu.Unlock()
	if err := s.TogglePause(); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	last := fake.actions[len(fake.actions)-1]
	fake.mu.Unlock()
	if last != "Pause" {
		t.Errorf("last action = %q, want Pause", last)
	}

	// 暂停中 → TogglePause 应发 Play
	fake.mu.Lock()
	fake.state = "PAUSED_PLAYBACK"
	fake.mu.Unlock()
	if err := s.TogglePause(); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	last = fake.actions[len(fake.actions)-1]
	fake.mu.Unlock()
	if last != "Play" {
		t.Errorf("last action = %q, want Play", last)
	}
}

func TestSessionCloseSendsStop(t *testing.T) {
	files := []*server.File{{URL: "http://example.com/a.mp4", Type: "video"}}
	s, fake := newTestSession(t, files)
	s.Load(0, 0)
	s.Close()

	fake.mu.Lock()
	defer fake.mu.Unlock()
	found := false
	for _, a := range fake.actions {
		if a == "Stop" {
			found = true
		}
	}
	if !found {
		t.Errorf("no Stop action in %v", fake.actions)
	}
}

func TestSessionStatusAndVolume(t *testing.T) {
	files := []*server.File{{URL: "http://example.com/a.mp4", Type: "video"}}
	s, fake := newTestSession(t, files)
	s.Load(0, 0)

	fake.mu.Lock()
	fake.pos = "0:00:10"
	fake.total = "0:01:00"
	fake.state = "PLAYING"
	fake.volume = "40"
	fake.mu.Unlock()

	pos, total, playing, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	if pos != 10 || total != 60 || !playing {
		t.Errorf("pos=%d total=%d playing=%v", pos, total, playing)
	}

	vol, err := s.Volume()
	if err != nil {
		t.Fatal(err)
	}
	if vol != 40 {
		t.Errorf("vol = %d", vol)
	}
}
