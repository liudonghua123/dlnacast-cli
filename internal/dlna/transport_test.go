// internal/dlna/transport_test.go
package dlna

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeRenderer 记录 SOAP action 调用顺序与参数，返回构造响应。
type fakeRenderer struct {
	mu      sync.Mutex
	actions []string
	args    map[string]map[string]string
	pos     string // GetPositionInfo RelTime
	total   string // TrackDuration
	state   string // GetTransportInfo CurrentTransportState
	volume  string // GetVolume CurrentVolume
}

func newFakeRenderer() *fakeRenderer {
	return &fakeRenderer{args: map[string]map[string]string{}}
}

func (f *fakeRenderer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		action := fakeSoapAction(r.Header.Get("SOAPACTION"))
		params := fakeParseArgs(body)

		f.mu.Lock()
		f.actions = append(f.actions, action)
		f.args[action] = params
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
		w.Write([]byte(fakeResponseXML(action, resp)))
	})
}

func fakeSoapAction(hdr string) string {
	hdr = strings.Trim(hdr, `"`)
	if i := strings.LastIndex(hdr, "#"); i >= 0 {
		return hdr[i+1:]
	}
	return hdr
}

// fakeParseArgs 提取 envelope Body 内 action 元素的子元素参数。
func fakeParseArgs(body []byte) map[string]string {
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
				// 第一个元素是 action 元素，读其子元素
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

func fakeResponseXML(action string, params map[string]string) string {
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

func TestTransportLoad(t *testing.T) {
	fake := newFakeRenderer()
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	dev := &Device{Name: "T", Host: "127.0.0.1", ControlURL: srv.URL, RenderingControlURL: srv.URL}
	tr := NewTransport(dev, NewClient())

	err := tr.Load("http://192.168.1.5:8080/m/0", "测试视频", "video", "")
	if err != nil {
		t.Fatal(err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.actions) < 2 || fake.actions[0] != "SetAVTransportURI" || fake.actions[1] != "Play" {
		t.Errorf("actions = %v", fake.actions)
	}
	if uri := fake.args["SetAVTransportURI"]["CurrentURI"]; uri != "http://192.168.1.5:8080/m/0" {
		t.Errorf("CurrentURI = %q", uri)
	}
	if meta := fake.args["SetAVTransportURI"]["CurrentURIMetaData"]; !strings.Contains(meta, "测试视频") {
		t.Errorf("metadata missing title: %s", meta)
	}
}

func TestTransportSeek(t *testing.T) {
	fake := newFakeRenderer()
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	tr := NewTransport(&Device{ControlURL: srv.URL, RenderingControlURL: srv.URL}, NewClient())
	if err := tr.Seek(90); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if target := fake.args["Seek"]["Target"]; target != "0:01:30" {
		t.Errorf("Target = %q", target)
	}
	if unit := fake.args["Seek"]["Unit"]; unit != "REL_TIME" {
		t.Errorf("Unit = %q", unit)
	}
}

func TestTransportStatus(t *testing.T) {
	fake := newFakeRenderer()
	fake.pos = "0:01:23"
	fake.total = "0:10:00"
	fake.state = "PLAYING"
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	tr := NewTransport(&Device{ControlURL: srv.URL, RenderingControlURL: srv.URL}, NewClient())
	pos, total, playing, err := tr.Status()
	if err != nil {
		t.Fatal(err)
	}
	if pos != 83 || total != 600 || !playing {
		t.Errorf("pos=%d total=%d playing=%v", pos, total, playing)
	}
}

func TestTransportVolume(t *testing.T) {
	fake := newFakeRenderer()
	fake.volume = "35"
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()

	tr := NewTransport(&Device{ControlURL: srv.URL, RenderingControlURL: srv.URL}, NewClient())
	if err := tr.SetVolume(50); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	if dv := fake.args["SetVolume"]["DesiredVolume"]; dv != "50" {
		t.Errorf("DesiredVolume = %q", dv)
	}
	fake.mu.Unlock()

	vol, err := tr.Volume()
	if err != nil {
		t.Fatal(err)
	}
	if vol != 35 {
		t.Errorf("vol = %d", vol)
	}
}
