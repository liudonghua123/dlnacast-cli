// internal/server/server_test.go
package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, files []*File) *Server {
	s := NewServer(0)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	s.SetFiles(files)
	return s
}

func TestMediaFullAndRange(t *testing.T) {
	dir := t.TempDir()
	mp4 := filepath.Join(dir, "v.mp4")
	content := strings.Repeat("0123456789", 1000) // 10KB
	if err := os.WriteFile(mp4, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t, []*File{{Path: mp4, Type: "video", Title: "v"}})

	// full GET
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/m/0", nil))
	if rec.Code != 200 || rec.Body.Len() != len(content) {
		t.Fatalf("full GET: code=%d len=%d", rec.Code, rec.Body.Len())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("content-type = %q", ct)
	}

	// range GET
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/m/0", nil)
	req2.Header.Set("Range", "bytes=100-199")
	s.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != 206 {
		t.Fatalf("range GET: code=%d want 206", rec2.Code)
	}
	if rec2.Body.Len() != 100 {
		t.Errorf("range len = %d want 100", rec2.Body.Len())
	}
	if cr := rec2.Header().Get("Content-Range"); !strings.HasPrefix(cr, "bytes 100-199/") {
		t.Errorf("content-range = %q", cr)
	}
}

func TestSubtitleRoute(t *testing.T) {
	dir := t.TempDir()
	srt := filepath.Join(dir, "s.srt")
	os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,000\nHi"), 0o644)

	s := newTestServer(t, []*File{{Path: dir + "/v.mp4", SubPath: srt, Type: "video", Title: "t"}})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/m/0.vtt", nil))
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/vtt") {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "WEBVTT") {
		t.Error("missing WEBVTT header")
	}
}

func TestURLFileNotServed(t *testing.T) {
	s := newTestServer(t, []*File{{URL: "http://example.com/a.mp4", Type: "video"}})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/m/0", nil))
	if rec.Code != 404 {
		t.Errorf("code=%d want 404", rec.Code)
	}
}

func TestMediaURL(t *testing.T) {
	// 必须与传入 SetFiles 的切片元素相同指针——保存切片再引用其元素。
	files := []*File{
		{Path: "a.mp4", Type: "video"},
		{URL: "http://example.com/b.mp4", Type: "video"},
	}
	s := newTestServer(t, files)
	if got := s.MediaURL("192.168.1.5", files[0]); got != "http://192.168.1.5:"+itoa(s.Port())+"/m/0" {
		t.Errorf("MediaURL = %q", got)
	}
	if got := s.MediaURL("192.168.1.5", files[1]); got != "http://example.com/b.mp4" {
		t.Errorf("URL passthrough = %q", got)
	}
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func TestSetLiveHandler(t *testing.T) {
	s := NewServer(0)
	s.SetLiveHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Write([]byte("LIVEBYTES"))
	}))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/live", s.Port()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "LIVEBYTES" {
		t.Errorf("body = %q", body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("Content-Type = %q", ct)
	}
}
