package mirror

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type slowReader struct {
	chunks [][]byte
	i      int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.i >= len(r.chunks) {
		return 0, io.EOF
	}
	n := copy(p, r.chunks[r.i])
	r.i++
	return n, nil
}

func TestLiveHandlerPassthrough(t *testing.T) {
	src := &slowReader{chunks: [][]byte{[]byte("AAAA"), []byte("BBBB"), []byte("CC")}}
	h := NewLiveHandler(src)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/live", nil))

	if rec.Code != 200 {
		t.Errorf("状态码 = %d，期望 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("Content-Type = %q，期望 video/mp2t", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q，期望 no-store", cc)
	}
	if got := rec.Body.String(); got != "AAAABBBBCC" {
		t.Errorf("响应体 = %q，期望 %q", got, "AAAABBBBCC")
	}
}

func TestLiveHandlerClientDisconnect(t *testing.T) {
	// 客户端断开时 handler 应结束（不永久阻塞）
	pr, pw := io.Pipe()
	defer pw.Close()
	h := NewLiveHandler(pr)
	rec := httptest.NewRecorder()
	// 用带超时的请求：模拟断开——关闭读端
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/live", nil))
		close(done)
	}()
	pw.Write([]byte("XYZ"))
	pw.Close() // 源 EOF → handler 返回
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler 未在源 EOF 后返回")
	}
}

func TestLiveHandlerFlushesImmediately(t *testing.T) {
	// 第一块数据写入后立即 Flush（不缓冲到 handler 返回）
	pr, pw := io.Pipe()
	h := NewLiveHandler(pr)
	rec := httptest.NewRecorder()
	go func() {
		pw.Write([]byte("DATA"))
		time.Sleep(100 * time.Millisecond)
		pw.Close()
	}()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/live", nil))
	if !strings.Contains(rec.Body.String(), "DATA") {
		t.Error("数据未直通")
	}
	if !rec.Flushed {
		t.Error("应调用 Flush（零缓冲）")
	}
}
