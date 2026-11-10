# dlnacast-cli Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Go 实现的 DLNA 投屏 CLI：本地文件/URL 投到电视，含播放列表、字幕、运行时快捷键控制。

**Architecture:** 全手写 SSDP 发现 + SOAP(AVTransport/RenderingControl) + 本地 HTTP 文件服务(Range)。player 包编排协议层，ui 包负责终端交互。

**Tech Stack:** Go 1.27 标准库 + 唯一第三方依赖 `golang.org/x/term`（Windows raw mode）。

## Global Constraints

- 依赖：Go 标准库 + `golang.org/x/term`（不许其他第三方依赖）
- Go 工具链：`D:\apps\go\bin\go.exe` 不在 PATH，**每个 Bash 命令前必须** `export PATH="/d/apps/go/bin:$PATH"`
- module 名：`dlnacast-cli`
- 所有 SOAP 调用 5 秒超时
- 用户可见输出/错误信息用中文
- conventional commits（feat/test/chore）
- 网络协议层（ssdp/真实 SOAP）不做自动化测试，用 `go build` + 手工验收；纯逻辑（字幕转换、SOAP 构造/解析、Range、进度渲染）必须 TDD

---

### Task 1: 项目骨架

**Files:**
- Create: `go.mod`
- Create: `main.go`

**Interfaces:**
- Produces: module `dlnacast-cli`，`go build ./...` 与 `go test ./...` 可运行

- [ ] **Step 1: 初始化 module 并写最小 main.go**

```bash
export PATH="/d/apps/go/bin:$PATH"
go mod init dlnacast-cli
```

```go
// main.go
package main

import "fmt"

func main() {
	fmt.Println("dlnacast")
}
```

- [ ] **Step 2: 验证**

Run: `export PATH="/d/apps/go/bin:$PATH" && go build ./... && go vet ./...`
Expected: 无输出，退出码 0

- [ ] **Step 3: Commit**

```bash
git add go.mod main.go
git commit -m "chore: init go module"
```

---

### Task 2: srt → WebVTT 转换

**Files:**
- Create: `internal/server/subtitle.go`
- Create: `internal/server/subtitle_test.go`

**Interfaces:**
- Produces: `func SRTToVTT(srt string) (string, bool)` — 输出 vtt 全文；第二返回值 false 表示输入无有效字幕块（返回 `("", false)`）

- [ ] **Step 1: 写失败测试**

```go
// internal/server/subtitle_test.go
package server

import (
	"strings"
	"testing"
)

func TestSRTToVTT(t *testing.T) {
	tests := []struct {
		name    string
		srt     string
		wantOK  bool
		wantSub []string
	}{
		{
			name:   "basic srt",
			srt:    "1\n00:00:01,000 --> 00:00:03,000\nHello\nWorld\n\n2\n00:00:04,500 --> 00:00:06,000\nSecond",
			wantOK: true,
			wantSub: []string{
				"WEBVTT",
				"00:00:01.000 --> 00:00:03.000",
				"Hello",
				"World",
				"00:00:04.500 --> 00:00:06.000",
				"Second",
			},
		},
		{
			name:    "crlf",
			srt:     "1\r\n00:00:01,000 --> 00:00:03,000\r\nHi\r\n\r\n2\r\n00:00:04,000 --> 00:00:05,000\r\nNext",
			wantOK:  true,
			wantSub: []string{"WEBVTT", "00:00:01.000 --> 00:00:03.000", "Hi", "Next"},
		},
		{
			name:    "no index lines",
			srt:     "00:00:01,000 --> 00:00:03,000\nNoIndex",
			wantOK:  true,
			wantSub: []string{"WEBVTT", "00:00:01.000 --> 00:00:03.000", "NoIndex"},
		},
		{
			name:   "invalid",
			srt:    "not a subtitle file",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SRTToVTT(tt.srt)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("missing %q in:\n%s", sub, got)
				}
			}
		})
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/server/ -v`
Expected: FAIL，`undefined: SRTToVTT`

- [ ] **Step 3: 实现**

```go
// internal/server/subtitle.go
package server

import (
	"regexp"
	"strings"
)

var cueTiming = regexp.MustCompile(`(\d{2}:\d{2}:\d{2})[,.](\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2})[,.](\d{3})`)

// SRTToVTT 把 SRT 字幕全文转换为 WebVTT。
func SRTToVTT(srt string) (string, bool) {
	srt = strings.ReplaceAll(srt, "\r\n", "\n")
	blocks := strings.Split(srt, "\n\n")
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	found := false
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		m := cueTiming.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		found = true
		timing := m[1] + "." + m[2] + " --> " + m[3] + "." + m[4]
		lines := strings.Split(block, "\n")
		start := 0
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				start = i + 1
				break
			}
		}
		b.WriteString(timing + "\n")
		b.WriteString(strings.Join(lines[start:], "\n") + "\n\n")
	}
	if !found {
		return "", false
	}
	return b.String(), true
}
```

- [ ] **Step 4: 运行确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/server/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/subtitle.go internal/server/subtitle_test.go
git commit -m "feat: srt to webvtt conversion"
```

---

### Task 3: 本地 HTTP 媒体服务

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/server_test.go`

**Interfaces:**
- Consumes: Task 2 `SRTToVTT`
- Produces:

```go
type File struct {
	URL     string // 非空 = 远程直投，不经本地服务
	Path    string // 本地文件路径
	SubPath string // 可选 srt 字幕
	Title   string
	Type    string // "video" / "audio" / "image"
}

func NewServer(port int) *Server            // port=0 随机
func (s *Server) Start() error              // 监听 0.0.0.0:port，goroutine 中 Serve
func (s *Server) Close() error
func (s *Server) SetFiles(files []*File)    // 注册 /m/<i> 与 /m/<i>.vtt 路由（URL 文件跳过）
func (s *Server) Handler() http.Handler     // 测试用
func (s *Server) Port() int
func (s *Server) MediaURL(localIP string, f *File) string     // URL 文件原样返回
func (s *Server) SubtitleURL(localIP string, f *File) string  // 无字幕返回 ""
```

媒体路由用 `http.ServeContent`（自带 Range/206/If-Range），显式设置 Content-Type。字幕路由 srt→vtt 后以 `text/vtt; charset=utf-8` 输出。MediaURL/SubtitleURL 通过在 `s.files` 中按指针找到索引来拼 URL。

- [ ] **Step 1: 写失败测试**

```go
// internal/server/server_test.go
package server

import (
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
	s := newTestServer(t, []*File{
		{Path: "a.mp4", Type: "video"},
		{URL: "http://example.com/b.mp4", Type: "video"},
	})
	urlFiles := []*File{
		{Path: "a.mp4", Type: "video"},
		{URL: "http://example.com/b.mp4", Type: "video"},
	}
	if got := s.MediaURL("192.168.1.5", urlFiles[0]); got != "http://192.168.1.5:"+itoa(s.Port())+"/m/0" {
		t.Errorf("MediaURL = %q", got)
	}
	if got := s.MediaURL("192.168.1.5", urlFiles[1]); got != "http://example.com/b.mp4" {
		t.Errorf("URL passthrough = %q", got)
	}
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
```

（注意测试文件 import `"fmt"`。注意 `TestMediaURL` 构造的 `urlFiles` 必须与传入 `SetFiles` 的切片元素相同指针——实现时直接把 `SetFiles` 的切片保存为变量再引用其元素，避免这个坑。）

- [ ] **Step 2: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/server/ -v`
Expected: FAIL（`NewServer` 未定义）

- [ ] **Step 3: 实现**

```go
// internal/server/server.go
package server

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type File struct {
	URL     string
	Path    string
	SubPath string
	Title   string
	Type    string
}

type Server struct {
	port  int
	srv   *http.Server
	mux   *http.ServeMux
	files []*File
}

func NewServer(port int) *Server { return &Server{port: port} }

func (s *Server) Port() int { return s.port }

func (s *Server) SetFiles(files []*File) {
	s.files = files
	mux := http.NewServeMux()
	for i, f := range files {
		if f.URL != "" {
			continue
		}
		idx := i
		mux.HandleFunc(fmt.Sprintf("/m/%d", idx), s.serveMedia(idx))
		if f.SubPath != "" {
			mux.HandleFunc(fmt.Sprintf("/m/%d.vtt", idx), s.serveSubtitle(idx))
		}
	}
	s.mux = mux
}

func (s *Server) Handler() http.Handler {
	if s.mux == nil {
		return http.NewServeMux()
	}
	return s.mux
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(s.port))
	if err != nil {
		return err
	}
	if s.port == 0 {
		s.port = ln.Addr().(*net.TCPAddr).Port
	}
	s.srv = &http.Server{Handler: s}
	go s.srv.Serve(ln)
	return nil
}

func (s *Server) Close() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.mux == nil {
		http.NotFound(w, r)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) serveMedia(i int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := s.files[i]
		file, err := os.Open(f.Path)
		if err != nil {
			http.Error(w, "无法打开文件", 500)
			return
		}
		defer file.Close()
		st, err := file.Stat()
		if err != nil {
			http.Error(w, "stat failed", 500)
			return
		}
		ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(f.Path)))
		if ct == "" {
			ct = defaultMIME(f.Type)
		}
		w.Header().Set("Content-Type", ct)
		http.ServeContent(w, r, "", st.ModTime(), file)
	}
}

func defaultMIME(mediaType string) string {
	switch mediaType {
	case "image":
		return "image/jpeg"
	case "audio":
		return "audio/mpeg"
	default:
		return "video/mp4"
	}
}

func (s *Server) serveSubtitle(i int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := s.files[i]
		data, err := os.ReadFile(f.SubPath)
		if err != nil {
			http.Error(w, "read failed", 500)
			return
		}
		vtt, ok := SRTToVTT(string(data))
		if !ok {
			vtt = "WEBVTT\n"
		}
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Write([]byte(vtt))
	}
}

func (s *Server) indexOf(f *File) int {
	for i, sf := range s.files {
		if sf == f {
			return i
		}
	}
	return -1
}

// MediaURL 返回电视可访问的媒体地址；URL 直投文件原样返回。
func (s *Server) MediaURL(localIP string, f *File) string {
	if f.URL != "" {
		return f.URL
	}
	if i := s.indexOf(f); i >= 0 {
		return fmt.Sprintf("http://%s:%d/m/%d", localIP, s.port, i)
	}
	return ""
}

// SubtitleURL 返回字幕地址，无字幕时返回空串。
func (s *Server) SubtitleURL(localIP string, f *File) string {
	if f.SubPath == "" {
		return ""
	}
	if i := s.indexOf(f); i >= 0 {
		return fmt.Sprintf("http://%s:%d/m/%d.vtt", localIP, s.port, i)
	}
	return ""
}
```

- [ ] **Step 4: 运行确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/server/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/server_test.go
git commit -m "feat: local media http server with range support"
```

---

### Task 4: SSDP 发现

**Files:**
- Create: `internal/ssdp/ssdp.go`

**Interfaces:**
- Produces: `func Discover(timeout time.Duration) ([]string, error)` — 返回去重后的设备描述 LOCATION URL 列表

UDP 多播不做自动化测试，`go build` 验证 + 真机手工验收。

- [ ] **Step 1: 实现**

```go
// internal/ssdp/ssdp.go
package ssdp

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const ssdpAddr = "239.255.255.250:1900"

// Discover 发送 M-SEARCH 并收集 MediaRenderer 设备描述 URL。
func Discover(timeout time.Duration) ([]string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("ssdp listen: %w", err)
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}

	msg := strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: 239.255.255.250:1900",
		`MAN: "ssdp:discover"`,
		"MX: 2",
		"ST: urn:schemas-upnp-org:device:MediaRenderer:1",
		"", "",
	}, "\r\n")

	for i := 0; i < 3; i++ {
		if _, err := conn.WriteTo([]byte(msg), dst); err != nil {
			return nil, fmt.Errorf("ssdp send: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	seen := map[string]bool{}
	var locations []string
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	conn.SetReadDeadline(deadline)
	for time.Now().Before(deadline) {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		loc := parseLocation(string(buf[:n]))
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		locations = append(locations, loc)
	}
	return locations, nil
}

func parseLocation(resp string) string {
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "location:") {
			return strings.TrimSpace(line[len("location:"):])
		}
	}
	return ""
}
```

- [ ] **Step 2: 编译验证**

Run: `export PATH="/d/apps/go/bin:$PATH" && go build ./...`
Expected: 通过

- [ ] **Step 3: Commit**

```bash
git add internal/ssdp/ssdp.go
git commit -m "feat: ssdp discovery"
```

---

### Task 5: SOAP 客户端

**Files:**
- Create: `internal/dlna/soap.go`
- Create: `internal/dlna/soap_test.go`

**Interfaces:**
- Produces:

```go
type Client struct{ HTTP *http.Client }

func NewClient() *Client  // HTTP 超时 5 秒
func (c *Client) Call(controlURL, service, action string, args map[string]string) (map[string]string, error)
// service 为 "AVTransport" 或 "RenderingControl"；SOAP Fault 返回含 faultstring 的 error

func BuildEnvelope(service, action string, args map[string]string) string          // 测试用
func ParseResponse(service, action string, body []byte) (map[string]string, error) // 测试与 Call 用
func FormatDuration(sec int) string      // 4529 -> "1:15:29"（UPnP H:MM:SS）
func ParseUPnPTime(s string) int         // "0:01:23" -> 83；带小数 ".000" 也兼容
```

- [ ] **Step 1: 写失败测试**

```go
// internal/dlna/soap_test.go
package dlna

import (
	"strings"
	"testing"
)

func TestBuildEnvelope(t *testing.T) {
	env := BuildEnvelope("AVTransport", "Play", map[string]string{"Speed": "1"})
	for _, want := range []string{
		`xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"`,
		"<u:Play>",
		"<Speed>1</Speed>",
		"</u:Play>",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("missing %q in:\n%s", want, env)
		}
	}
}

func TestBuildEnvelopeEscape(t *testing.T) {
	env := BuildEnvelope("AVTransport", "SetAVTransportURI",
		map[string]string{"CurrentURI": "http://x/a.mp4&<b>"})
	if !strings.Contains(env, "a.mp4&amp;&lt;b&gt;") {
		t.Errorf("not escaped:\n%s", env)
	}
}

func TestParseResponse(t *testing.T) {
	body := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
  s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <Track>1</Track>
      <AbsTime>0:00:00</AbsTime>
      <RelTime>0:01:23</RelTime>
    </u:GetPositionInfoResponse>
  </s:Body>
</s:Envelope>`
	args, err := ParseResponse("AVTransport", "GetPositionInfo", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if args["RelTime"] != "0:01:23" {
		t.Errorf("RelTime = %q", args["RelTime"])
	}
	if args["Track"] != "1" {
		t.Errorf("Track = %q", args["Track"])
	}
}

func TestParseFault(t *testing.T) {
	body := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>Invalid Action</faultstring>
      <detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
        <errorCode>401</errorCode><errorDescription>Invalid Action</errorDescription>
      </UPnPError></detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`
	_, err := ParseResponse("AVTransport", "Play", []byte(body))
	if err == nil {
		t.Fatal("want fault error")
	}
	if !strings.Contains(err.Error(), "Invalid Action") {
		t.Errorf("err = %v", err)
	}
}

func TestFormatAndParseTime(t *testing.T) {
	if got := FormatDuration(4529); got != "1:15:29" {
		t.Errorf("FormatDuration = %q", got)
	}
	if got := ParseUPnPTime("1:15:29"); got != 4529 {
		t.Errorf("ParseUPnPTime = %d", got)
	}
	if got := ParseUPnPTime("0:00:00.000"); got != 0 {
		t.Errorf("ParseUPnPTime fractional = %d", got)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/dlna/ -v`
Expected: FAIL（函数未定义）

- [ ] **Step 3: 实现**

```go
// internal/dlna/soap.go
package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct{ HTTP *http.Client }

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 5 * time.Second}}
}

func BuildEnvelope(service, action string, args map[string]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:`)
	b.WriteString(action)
	b.WriteString(` xmlns:u="urn:schemas-upnp-org:service:` + service + `:1">`)
	for k, v := range args {
		b.WriteString("<" + k + ">" + xmlEscape(v) + "</" + k + ">")
	}
	b.WriteString("</u:" + action + "></s:Body></s:Envelope>")
	return b.String()
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func ParseResponse(service, action string, body []byte) (map[string]string, error) {
	var fault struct {
		Body struct {
			Fault struct {
				FaultString string `xml:"faultstring"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(body, &fault); err == nil && fault.Body.Fault.FaultString != "" {
		return nil, fmt.Errorf("SOAP fault: %s", fault.Body.Fault.FaultString)
	}

	wrapper := struct {
		Body struct {
			InnerXML []byte `xml:",innerxml"`
		} `xml:"Body"`
	}{}
	if err := xml.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	result := map[string]string{}
	decoder := xml.NewDecoder(bytes.NewReader(wrapper.Body.InnerXML))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok {
			var val string
			if err := decoder.DecodeElement(&val, &se); err == nil {
				result[se.Name.Local] = val
			}
		}
	}
	return result, nil
}

func (c *Client) Call(controlURL, service, action string, args map[string]string) (map[string]string, error) {
	env := BuildEnvelope(service, action, args)
	req, err := http.NewRequest("POST", controlURL, strings.NewReader(env))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:`+service+`:1#`+action+`"`)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		if _, perr := ParseResponse(service, action, body); perr != nil {
			return nil, fmt.Errorf("%s: HTTP %d: %v", action, resp.StatusCode, perr)
		}
		return nil, fmt.Errorf("%s: HTTP %d", action, resp.StatusCode)
	}
	return ParseResponse(service, action, body)
}

// FormatDuration 把秒格式化为 UPnP H:MM:SS。
func FormatDuration(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}

// ParseUPnPTime 把 "H:MM:SS"（可带 .mmm 小数）解析为秒。
func ParseUPnPTime(s string) int {
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	sec, _ := strconv.Atoi(parts[2])
	return h*3600 + m*60 + sec
}
```

- [ ] **Step 4: 运行确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/dlna/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dlna/soap.go internal/dlna/soap_test.go
git commit -m "feat: soap client"
```

---

### Task 6: 设备描述解析 + Transport 控制

**Files:**
- Create: `internal/dlna/device.go`
- Create: `internal/dlna/device_test.go`
- Create: `internal/dlna/transport.go`
- Create: `internal/dlna/transport_test.go`

**Interfaces:**
- Consumes: Task 5 `Client.Call` / `FormatDuration` / `ParseUPnPTime` / `xmlEscape`，Task 4 的 LOCATION URL
- Produces:

```go
type Device struct {
	Name                string // friendlyName
	Host                string // 设备 IP
	ControlURL          string // AVTransport 控制点绝对 URL
	RenderingControlURL string // RenderingControl 控制点绝对 URL
}

func Describe(locationURL string, c *Client) (*Device, error)
// GET locationURL 解析 XML；取 AVTransport 与 RenderingControl 的 controlURL（相对则拼 scheme://host）
// 找不到 AVTransport 报错（含 "AVTransport" 字样）

func NewTransport(d *Device, c *Client) *Transport
func (t *Transport) Load(mediaURL, title, mediaType, subtitleURL string) error
// SetAVTransportURI（DIDL：dc:title、upnp:class 映射 video/audio/image、
// subtitleURL 非空时附加 <res protocolInfo="http-get:*:text/vtt:*">）+ Play
func (t *Transport) Play() error
func (t *Transport) Pause() error
func (t *Transport) Stop() error
func (t *Transport) Seek(sec int) error            // REL_TIME
func (t *Transport) Status() (pos, total int, playing bool, err error)
func (t *Transport) Volume() (int, error)          // 0-100
func (t *Transport) SetVolume(level int) error
func (t *Transport) Device() *Device
```

- [ ] **Step 1: 写 Describe 失败测试**

```go
// internal/dlna/device_test.go
package dlna

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const descXML = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device>
    <friendlyName>客厅电视</friendlyName>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType>
        <controlURL>/ctl/RC</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
        <controlURL>/ctl/AVT</controlURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
        <controlURL>/ctl/CM</controlURL>
      </service>
    </serviceList>
  </device>
</root>`

func TestDescribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(descXML))
	}))
	defer srv.Close()

	dev, err := Describe(srv.URL, NewClient())
	if err != nil {
		t.Fatal(err)
	}
	if dev.Name != "客厅电视" {
		t.Errorf("Name = %q", dev.Name)
	}
	if !strings.HasSuffix(dev.ControlURL, "/ctl/AVT") || !strings.HasPrefix(dev.ControlURL, "http://") {
		t.Errorf("ControlURL = %q", dev.ControlURL)
	}
	if !strings.HasSuffix(dev.RenderingControlURL, "/ctl/RC") {
		t.Errorf("RenderingControlURL = %q", dev.RenderingControlURL)
	}
}

func TestDescribeNoAVTransport(t *testing.T) {
	badXML := `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <device><friendlyName>X</friendlyName></device>
</root>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(badXML))
	}))
	defer srv.Close()
	_, err := Describe(srv.URL, NewClient())
	if err == nil || !strings.Contains(err.Error(), "AVTransport") {
		t.Errorf("err = %v", err)
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/dlna/ -run TestDescribe -v`
Expected: FAIL（`Describe` 未定义）

- [ ] **Step 3: 实现 Describe**

```go
// internal/dlna/device.go
package dlna

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

type Device struct {
	Name                string
	Host                string
	ControlURL          string
	RenderingControlURL string
}

type deviceDesc struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		ServiceList  []struct {
			ServiceType string `xml:"serviceType"`
			ControlURL  string `xml:"controlURL"`
		} `xml:"serviceList>service"`
	} `xml:"device"`
}

// Describe 拉取并解析设备描述 XML。
func Describe(locationURL string, c *Client) (*Device, error) {
	resp, err := c.HTTP.Get(locationURL)
	if err != nil {
		return nil, fmt.Errorf("获取设备描述失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var desc deviceDesc
	if err := xml.Unmarshal(body, &desc); err != nil {
		return nil, fmt.Errorf("解析设备描述失败: %w", err)
	}

	u, err := url.Parse(locationURL)
	if err != nil {
		return nil, err
	}
	base := u.Scheme + "://" + u.Host

	dev := &Device{Name: desc.Device.FriendlyName, Host: u.Hostname()}
	for _, svc := range desc.Device.ServiceList {
		switch {
		case strings.Contains(svc.ServiceType, "AVTransport"):
			dev.ControlURL = absURL(base, svc.ControlURL)
		case strings.Contains(svc.ServiceType, "RenderingControl"):
			dev.RenderingControlURL = absURL(base, svc.ControlURL)
		}
	}
	if dev.ControlURL == "" {
		return nil, errors.New("设备不支持 AVTransport，无法投屏")
	}
	return dev, nil
}

func absURL(base, path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
```

- [ ] **Step 4: 运行 Describe 测试确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/dlna/ -run TestDescribe -v`
Expected: PASS

- [ ] **Step 5: 写 fake 渲染器 + Transport 失败测试**

fake 渲染器是复用的测试 helper（Task 8 的 player 测试也要用同款思路），放在 `transport_test.go`。它记录 action 调用顺序与参数，返回构造的响应 XML。

```go
// internal/dlna/transport_test.go
package dlna

import (
	"encoding/xml"
	"fmt"
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
		body := make([]byte, r.ContentLength)
		r.Read(body)
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
```

行为测试：

```go
// 追加到 internal/dlna/transport_test.go
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
```

- [ ] **Step 6: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/dlna/ -v`
Expected: FAIL（`NewTransport` 未定义）

- [ ] **Step 7: 实现 Transport**

```go
// internal/dlna/transport.go
package dlna

import (
	"fmt"
	"strconv"
)

type Transport struct {
	dev *Device
	c   *Client
}

func NewTransport(d *Device, c *Client) *Transport {
	return &Transport{dev: d, c: c}
}

func (t *Transport) Device() *Device { return t.dev }

func (t *Transport) call(action string, args map[string]string) (map[string]string, error) {
	return t.c.Call(t.dev.ControlURL, "AVTransport", action, args)
}

func (t *Transport) rcCall(action string, args map[string]string) (map[string]string, error) {
	return t.c.Call(t.dev.RenderingControlURL, "RenderingControl", action, args)
}

const didlTemplate = `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="0" parentID="0" restricted="1"><dc:title>%s</dc:title><upnp:class>%s</upnp:class><res>%s</res>%s</item></DIDL-Lite>`

func upnpClass(mediaType string) string {
	switch mediaType {
	case "audio":
		return "object.item.audioItem"
	case "image":
		return "object.item.imageItem"
	default:
		return "object.item.videoItem"
	}
}

// Load 投放媒体并开始播放。
func (t *Transport) Load(mediaURL, title, mediaType, subtitleURL string) error {
	extraRes := ""
	if subtitleURL != "" {
		extraRes = fmt.Sprintf(`<res protocolInfo="http-get:*:text/vtt:*">%s</res>`, subtitleURL)
	}
	didl := fmt.Sprintf(didlTemplate, title, upnpClass(mediaType), mediaURL, extraRes)

	_, err := t.call("SetAVTransportURI", map[string]string{
		"InstanceID":         "0",
		"CurrentURI":         mediaURL,
		"CurrentURIMetaData": xmlEscape(didl),
	})
	if err != nil {
		return fmt.Errorf("设置播放地址失败: %w", err)
	}
	return t.Play()
}

func (t *Transport) Play() error {
	_, err := t.call("Play", map[string]string{"InstanceID": "0", "Speed": "1"})
	return err
}

func (t *Transport) Pause() error {
	_, err := t.call("Pause", map[string]string{"InstanceID": "0"})
	return err
}

func (t *Transport) Stop() error {
	_, err := t.call("Stop", map[string]string{"InstanceID": "0"})
	return err
}

func (t *Transport) Seek(sec int) error {
	_, err := t.call("Seek", map[string]string{
		"InstanceID": "0", "Unit": "REL_TIME", "Target": FormatDuration(sec),
	})
	return err
}

// Status 返回播放位置/总时长(秒)/是否播放中。
func (t *Transport) Status() (pos, total int, playing bool, err error) {
	res, err := t.call("GetPositionInfo", map[string]string{"InstanceID": "0"})
	if err != nil {
		return 0, 0, false, err
	}
	pos = ParseUPnPTime(res["RelTime"])
	total = ParseUPnPTime(res["TrackDuration"])
	if pos == 0 { // 部分设备 RelTime 为 NOT_IMPLEMENTED，退回 AbsTime
		pos = ParseUPnPTime(res["AbsTime"])
	}

	tinfo, err := t.call("GetTransportInfo", map[string]string{"InstanceID": "0"})
	if err != nil {
		return pos, total, false, nil // 传输状态查询失败不阻塞
	}
	playing = tinfo["CurrentTransportState"] == "PLAYING"
	return pos, total, playing, nil
}

// Volume 返回 0-100 音量。
func (t *Transport) Volume() (int, error) {
	res, err := t.rcCall("GetVolume", map[string]string{"InstanceID": "0", "Channel": "Master"})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(res["CurrentVolume"])
}

// SetVolume 设置音量（0-100，自动夹取）。
func (t *Transport) SetVolume(level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	_, err := t.rcCall("SetVolume", map[string]string{
		"InstanceID": "0", "Channel": "Master", "DesiredVolume": strconv.Itoa(level),
	})
	return err
}
```

- [ ] **Step 8: 运行确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/dlna/ -v`
Expected: 全部 PASS

- [ ] **Step 9: Commit**

```bash
git add internal/dlna/
git commit -m "feat: device describe and transport control"
```

---

### Task 7: 本机出口 IP 选择

**Files:**
- Create: `internal/dlna/localip.go`

**Interfaces:**
- Produces: `func LocalIPFor(deviceIP string) (string, error)` — 返回与 deviceIP 通信时内核选择的本机出口 IP（多网卡/WSL 场景必需）

UDP connect 不发包让内核选路由，不做自动化测试（编译验证 + 真机验收）。

- [ ] **Step 1: 实现**

```go
// internal/dlna/localip.go
package dlna

import (
	"fmt"
	"net"
)

// LocalIPFor 选出与 deviceIP 通信时使用的本机 IP。
// 通过 UDP connect（不实际发包）让内核选择路由源地址，
// 天然处理多网卡（虚拟机、WSL）场景。
func LocalIPFor(deviceIP string) (string, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(deviceIP, "9"))
	if err != nil {
		return "", fmt.Errorf("无法确定到设备 %s 的本机出口 IP: %w", deviceIP, err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
```

- [ ] **Step 2: 编译验证**

Run: `export PATH="/d/apps/go/bin:$PATH" && go build ./...`
Expected: 通过

- [ ] **Step 3: Commit**

```bash
git add internal/dlna/localip.go
git commit -m "feat: local ip selection for multi-nic"
```

---

### Task 8: player 会话管理

**Files:**
- Create: `internal/player/player.go`
- Create: `internal/player/player_test.go`（含 fake 渲染器 helper，从 Task 6 的 transport_test.go 复制 `fakeRenderer` 相关代码并改为包内私有使用）

**Interfaces:**
- Consumes: Task 3 `server.NewServer/Start/Close/SetFiles/MediaURL/SubtitleURL/Port`，Task 6 `dlna.Device/NewTransport/Transport/LocalIPFor`
- Produces:

```go
type Session struct{ ... }

func NewSession(dev *dlna.Device, files []*server.File, port int) (*Session, error)
// 有本地文件时启动 server 并 SetFiles；LocalIPFor(dev.Host) 缓存本机出口 IP；
// 创建 Transport。全 URL 文件时不起本地服务。

func (s *Session) Load(i, seekSec int) error
// 边界检查（i 越界返回 error）→ 记录 currentIndex → 拼 MediaURL/SubtitleURL →
// transport.Load(url, title, type, subtitleURL) → seekSec>0 时 transport.Seek(seekSec)

func (s *Session) Play() error
func (s *Session) Pause() error
func (s *Session) Stop() error
func (s *Session) Seek(deltaSec int) error        // 相对当前 pos 跳转
func (s *Session) TogglePause() error             // 按 Status().playing 判断
func (s *Session) SetVolume(level int) error
func (s *Session) Status() (pos, total int, playing bool, err error)
func (s *Session) Volume() (int, error)
func (s *Session) Index() int                     // 当前播放索引
func (s *Session) Count() int                     // 播放列表长度
func (s *Session) Close() error                   // Stop 投屏（错误仅记录不阻塞）+ 关闭本地服务
```

- [ ] **Step 1: 写失败测试**

```go
// internal/player/player_test.go
package player

import (
	"encoding/xml"
	"fmt"
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
		body := make([]byte, r.ContentLength)
		r.Read(body)
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
		Name: "测试电视", Host: "127.0.0.1",
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
```

- [ ] **Step 2: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/player/ -v`
Expected: FAIL（`Session` 未定义）

- [ ] **Step 3: 实现 Session**

```go
// internal/player/player.go
package player

import (
	"fmt"

	"dlnacast-cli/internal/dlna"
	"dlnacast-cli/internal/server"
)

// Session 管理一次投屏会话：设备 + 播放列表 + 本地服务。
type Session struct {
	dev      *dlna.Device
	transport *dlna.Transport
	srv      *server.Server // 仅当有本地文件时非 nil
	files    []*server.File
	localIP  string
	current  int
}

func NewSession(dev *dlna.Device, files []*server.File, port int) (*Session, error) {
	s := &Session{dev: dev, files: files, current: -1}

	hasLocal := false
	for _, f := range files {
		if f.URL == "" {
			hasLocal = true
			break
		}
	}

	if hasLocal {
		s.srv = server.NewServer(port)
		if err := s.srv.Start(); err != nil {
			return nil, fmt.Errorf("启动本地服务失败: %w", err)
		}
		s.srv.SetFiles(files)
	}

	ip, err := dlna.LocalIPFor(dev.Host)
	if err != nil {
		if s.srv != nil {
			s.srv.Close()
		}
		return nil, err
	}
	s.localIP = ip

	s.transport = dlna.NewTransport(dev, dlna.NewClient())
	return s, nil
}

func (s *Session) mediaURL(f *server.File) string {
	if s.srv == nil {
		return f.URL
	}
	return s.srv.MediaURL(s.localIP, f)
}

func (s *Session) subtitleURL(f *server.File) string {
	if s.srv == nil {
		return ""
	}
	return s.srv.SubtitleURL(s.localIP, f)
}

// Load 投放第 i 个媒体，seekSec>0 时跳转到该位置。
func (s *Session) Load(i, seekSec int) error {
	if i < 0 || i >= len(s.files) {
		return fmt.Errorf("播放索引越界: %d（共 %d 个）", i, len(s.files))
	}
	s.current = i
	f := s.files[i]
	url := s.mediaURL(f)
	if url == "" {
		return fmt.Errorf("无法生成媒体地址")
	}
	if err := s.transport.Load(url, f.Title, f.Type, s.subtitleURL(f)); err != nil {
		return err
	}
	if seekSec > 0 {
		return s.transport.Seek(seekSec)
	}
	return nil
}

func (s *Session) Play() error   { return s.transport.Play() }
func (s *Session) Pause() error  { return s.transport.Pause() }
func (s *Session) Stop() error   { return s.transport.Stop() }

// Seek 相对当前位置跳转。
func (s *Session) Seek(deltaSec int) error {
	pos, _, _, err := s.Status()
	if err != nil {
		return err
	}
	target := pos + deltaSec
	if target < 0 {
		target = 0
	}
	return s.transport.Seek(target)
}

// TogglePause 按当前状态暂停或继续。
func (s *Session) TogglePause() error {
	_, _, playing, err := s.Status()
	if err != nil {
		return err
	}
	if playing {
		return s.transport.Pause()
	}
	return s.transport.Play()
}

func (s *Session) SetVolume(level int) error { return s.transport.SetVolume(level) }

func (s *Session) Status() (pos, total int, playing bool, err error) {
	return s.transport.Status()
}

func (s *Session) Volume() (int, error) { return s.transport.Volume() }

func (s *Session) Index() int { return s.current }
func (s *Session) Count() int { return len(s.files) }

// Close 停止投屏并关闭本地服务。
func (s *Session) Close() error {
	if s.transport != nil {
		s.transport.Stop() // 错误不阻塞退出
	}
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}
```

- [ ] **Step 4: 运行确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/player/ -v && go build ./...`
Expected: PASS，编译通过

- [ ] **Step 5: Commit**

```bash
git add internal/player/
git commit -m "feat: player session management"
```

---

### Task 9: 终端 UI

**Files:**
- Create: `internal/ui/ui.go`
- Create: `internal/ui/ui_test.go`
- Modify: `go.mod`（`go get golang.org/x/term`，唯一第三方依赖）

**Interfaces:**
- Consumes: Task 6 `dlna.Device`
- Produces:

```go
type Key int

const (
	KeySpace Key = iota
	KeyQuit          // q 或 Ctrl+C
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyS // 手动刷新
	KeyN // 下一个
	KeyP // 上一个
	KeyOther
)

// Keys 进入 raw mode，goroutine 逐字节读 stdin 解析按键。
// 返回按键 channel 与恢复终端的 cancel 函数。
func Keys() (<-chan Key, func(), error)

// ChooseDevice 交互式选择设备（↑↓ + Enter）。
// MakeRaw 失败（非 TTY）时 fallback 返回第一个设备。
func ChooseDevice(devices []*dlna.Device) *dlna.Device

// RenderProgress 输出单行进度（\r\x1b[K 清行重绘）。
func RenderProgress(w io.Writer, pos, total int, playing bool, vol int, title string)
```

按键字节规则：`0x20`=空格、`0x71`('q')/`0x03`(Ctrl+C)=退出、`0x73`('s')、`0x6E`('n')、`0x70`('p')、ANSI 序列 `ESC [ A/B/C/D` = ↑↓←→（读到 `0x1B` 后再读 2 字节组合判断）。

- [ ] **Step 1: 添加依赖**

```bash
export PATH="/d/apps/go/bin:$PATH"
go get golang.org/x/term && go mod tidy
```

- [ ] **Step 2: 写 RenderProgress 失败测试**

```go
// internal/ui/ui_test.go
package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderProgress(t *testing.T) {
	var b bytes.Buffer
	RenderProgress(&b, 634, 5400, true, 30, "我的视频.mp4")
	out := b.String()

	if !strings.HasPrefix(out, "\r\x1b[K") {
		t.Errorf("missing clear-line prefix: %q", out)
	}
	for _, want := range []string{"▶", "00:10:34", "01:30:00", "11%", "vol:30", "我的视频.mp4"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestRenderProgressPaused(t *testing.T) {
	var b bytes.Buffer
	RenderProgress(&b, 0, 60, false, 0, "x.mp4")
	if !strings.Contains(b.String(), "⏸") {
		t.Errorf("missing pause icon in %q", b.String())
	}
}
```

- [ ] **Step 3: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/ui/ -v`
Expected: FAIL（`RenderProgress` 未定义）

- [ ] **Step 4: 实现 ui.go**

```go
// internal/ui/ui.go
package ui

import (
	"fmt"
	"io"
	"os"

	"dlnacast-cli/internal/dlna"

	"golang.org/x/term"
)

type Key int

const (
	KeySpace Key = iota
	KeyQuit
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyS
	KeyN
	KeyP
	KeyOther
)

// Keys 进入 raw mode 读键。cancel 恢复终端状态。
func Keys() (<-chan Key, func(), error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, nil, fmt.Errorf("无法进入终端原始模式（需要真实终端）: %w", err)
	}

	ch := make(chan Key)
	done := make(chan struct{})
	go func() {
		defer close(ch)
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			select {
			case <-done:
				return
			case ch <- parseKey(buf, os.Stdin):
			}
		}
	}()

	cancel := func() {
		close(done)
		term.Restore(fd, oldState)
	}
	return ch, cancel, nil
}

// parseKey 解析按键字节。ANSI 方向键：ESC [ A/B/C/D。
func parseKey(buf []byte, in *os.File) Key {
	switch buf[0] {
	case 0x20:
		return KeySpace
	case 0x03, 'q':
		return KeyQuit
	case 's':
		return KeyS
	case 'n':
		return KeyN
	case 'p':
		return KeyP
	case 0x1B: // ESC 序列
		next := make([]byte, 2)
		if n, _ := io.ReadFull(in, next); n != 2 {
			return KeyOther
		}
		if next[0] != '[' {
			return KeyOther
		}
		switch next[1] {
		case 'A':
			return KeyUp
		case 'B':
			return KeyDown
		case 'C':
			return KeyRight
		case 'D':
			return KeyLeft
		}
		return KeyOther
	default:
		return KeyOther
	}
}

// ChooseDevice 交互式选择设备。非 TTY 环境直接返回第一个。
func ChooseDevice(devices []*dlna.Device) *dlna.Device {
	if len(devices) == 0 {
		return nil
	}
	if len(devices) == 1 {
		return devices[0]
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return devices[0] // 非 TTY fallback
	}
	defer term.Restore(fd, oldState)

	sel := 0
	render := func() {
		fmt.Print("\r\x1b[K选择设备（↑↓ 移动，回车确认）:\r\n\x1b[K")
		for i, d := range devices {
			cursor := "  "
			if i == sel {
				cursor = "> "
			}
			fmt.Printf("\r\x1b[K%s%s (%s)\r\n", cursor, d.Name, d.Host)
		}
		fmt.Printf("\x1b[%dA\r", len(devices)+1) // 光标回到行首
	}
	render()

	buf := make([]byte, 1)
	for {
		if _, err := os.Stdin.Read(buf); err != nil {
			return devices[sel]
		}
		switch buf[0] {
		case 0x0D, 0x0A: // Enter
			fmt.Print("\r\x1b[K\x1b[J") // 清除选择列表
			return devices[sel]
		case 0x03: // Ctrl+C
			fmt.Print("\r\x1b[K\x1b[J")
			return nil
		case 0x1B:
			next := make([]byte, 2)
			if n, _ := io.ReadFull(os.Stdin, next); n == 2 && next[0] == '[' {
				switch next[1] {
				case 'A':
					if sel > 0 {
						sel--
					}
					render()
				case 'B':
					if sel < len(devices)-1 {
						sel++
					}
					render()
				}
			}
		}
	}
}

// RenderProgress 输出单行进度条。
func RenderProgress(w io.Writer, pos, total int, playing bool, vol int, title string) {
	icon := "▶"
	if !playing {
		icon = "⏸"
	}
	pct := 0
	if total > 0 {
		pct = pos * 100 / total
	}
	barW := 20
	filled := 0
	if total > 0 {
		filled = pos * barW / total
	}
	bar := ""
	for i := 0; i < barW; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	fmt.Fprintf(w, "\r\x1b[K%s %s / %s [%s] %d%% vol:%d  %s",
		icon, hhmmss(pos), hhmmss(total), bar, pct, vol, title)
}

func hhmmss(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
```

- [ ] **Step 5: 运行确认通过**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./internal/ui/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ui/ go.mod go.sum
git commit -m "feat: terminal ui with key handling and progress rendering"
```

---

### Task 10: main 装配 + README

**Files:**
- Modify: `main.go`（完全替换）
- Create: `README.md`
- Create: `main_test.go`

**Interfaces:**
- Consumes: Task 3 `server.File`，Task 4 `ssdp.Discover`，Task 6 `dlna.Describe/LocalIPFor`，Task 8 `player.NewSession/Load/...`，Task 9 `ui.*`

- [ ] **Step 1: 写参数解析失败测试**

```go
// main_test.go
package main

import (
	"testing"
)

func TestClassifyMedia(t *testing.T) {
	tests := []struct {
		in      string
		wantURL bool
		wantTyp string
	}{
		{"http://example.com/a.mp4", true, "video"},
		{"https://example.com/pic.jpg", true, "image"},
		{"video.mp4", false, "video"},
		{"song.mp3", false, "audio"},
		{"photo.png", false, "image"},
		{"movie.mkv", false, "video"},
		{"C:\\dir\\video.avi", false, "video"},
		{"sound.flac", false, "audio"},
		{"unknown.xyz", false, "video"}, // 默认 video
	}
	for _, tt := range tests {
		f := classifyMedia(tt.in)
		isURL := f.URL != ""
		if isURL != tt.wantURL {
			t.Errorf("%s: isURL = %v, want %v", tt.in, isURL, tt.wantURL)
		}
		if f.Type != tt.wantTyp {
			t.Errorf("%s: type = %q, want %q", tt.in, f.Type, tt.wantTyp)
		}
	}
}
```

- [ ] **Step 2: 运行确认失败**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test . -v`
Expected: FAIL（`classifyMedia` 未定义）

- [ ] **Step 3: 实现 main.go**

```go
// main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dlnacast-cli/internal/dlna"
	"dlnacast-cli/internal/player"
	"dlnacast-cli/internal/server"
	"dlnacast-cli/internal/ssdp"
	"dlnacast-cli/internal/ui"
)

var mediaExts = map[string]string{
	".mp4": "video", ".mkv": "video", ".avi": "video", ".mov": "video",
	".wmv": "video", ".flv": "video", ".webm": "video", ".m4v": "video",
	".ts": "video", ".mpg": "video", ".mpeg": "video",
	".mp3": "audio", ".flac": "audio", ".wav": "audio", ".aac": "audio",
	".ogg": "audio", ".m4a": "audio", ".wma": "audio",
	".jpg": "image", ".jpeg": "image", ".png": "image", ".gif": "image",
	".bmp": "image", ".webp": "image",
}

// classifyMedia 把参数归类为 URL 直投或本地文件。
func classifyMedia(arg string) *server.File {
	if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
		return &server.File{URL: arg, Title: urlTitle(arg), Type: "video"}
	}
	ext := strings.ToLower(filepath.Ext(arg))
	typ := mediaExts[ext]
	if typ == "" {
		typ = "video"
	}
	return &server.File{Path: arg, Title: strings.TrimSuffix(filepath.Base(arg), filepath.Ext(arg)), Type: typ}
}

func urlTitle(u string) string {
	s := strings.TrimSuffix(u, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

func main() {
	var (
		flagDevice  = flag.String("d", "", "指定设备名称（子串匹配）或 IP")
		flagTitle   = flag.String("t", "", "媒体标题")
		flagSub     = flag.String("s", "", "字幕文件（srt）")
		flagSeek    = flag.Int("seek", 0, "起始播放位置（秒）")
		flagVol     = flag.Int("vol", -1, "初始音量（0-100）")
		flagPort    = flag.Int("port", 0, "本地 HTTP 服务端口（默认随机）")
		flagTimeout = flag.Int("timeout", 3, "设备扫描等待秒数")
		flagVerbose = flag.Bool("v", false, "详细输出")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法:\n  dlnacast [flags] <media1> [media2 ...]\n  dlnacast ls\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 || args[0] != "ls" && len(args) == 1 && args[0] == "ls" {
		// 占位
	}
	if len(args) == 1 && args[0] == "ls" {
		runList(*flagTimeout)
		return
	}
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	// 参数校验与归类
	files := make([]*server.File, 0, len(args))
	for _, arg := range args {
		f := classifyMedia(arg)
		if f.URL == "" {
			st, err := os.Stat(f.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "错误: 文件不存在: %s\n", f.Path)
				os.Exit(1)
			}
			if st.IsDir() {
				fmt.Fprintf(os.Stderr, "错误: 是目录不是文件: %s\n", f.Path)
				os.Exit(1)
			}
		}
		if *flagTitle != "" {
			f.Title = *flagTitle
		}
		if *flagSub != "" {
			if _, err := os.Stat(*flagSub); err != nil {
				fmt.Fprintf(os.Stderr, "错误: 字幕文件不存在: %s\n", *flagSub)
				os.Exit(1)
			}
			f.SubPath = *flagSub
		}
		files = append(files, f)
	}

	runCast(files, *flagDevice, *flagSeek, *flagVol, *flagPort, *flagTimeout, *flagVerbose)
}

func runList(timeout int) {
	fmt.Println("正在扫描 DLNA 设备...")
	locs, err := ssdp.Discover(time.Duration(timeout) * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扫描失败: %v\n", err)
		os.Exit(1)
	}
	if len(locs) == 0 {
		fmt.Println("未发现 DLNA 设备，请确认电视与电脑在同一局域网")
		os.Exit(1)
	}
	c := dlna.NewClient()
	for _, loc := range locs {
		if dev, err := dlna.Describe(loc, c); err == nil {
			fmt.Printf("%s (%s)\n", dev.Name, dev.Host)
		}
	}
}

func runCast(files []*server.File, device string, seekSec, vol, port, timeout int, verbose bool) {
	fmt.Println("正在扫描 DLNA 设备...")
	locs, err := ssdp.Discover(time.Duration(timeout) * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "扫描失败: %v\n", err)
		os.Exit(1)
	}
	if len(locs) == 0 {
		fmt.Fprintln(os.Stderr, "未发现 DLNA 设备，请确认电视与电脑在同一局域网")
		os.Exit(1)
	}

	c := dlna.NewClient()
	var devices []*dlna.Device
	for _, loc := range locs {
		dev, err := dlna.Describe(loc, c)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "跳过设备 %s: %v\n", loc, err)
			}
			continue
		}
		devices = append(devices, dev)
	}
	if len(devices) == 0 {
		fmt.Fprintln(os.Stderr, "未发现可用的 DLNA 设备")
		os.Exit(1)
	}

	// 设备选择
	var dev *dlna.Device
	if device != "" {
		for _, d := range devices {
			if d.Host == device || strings.Contains(d.Name, device) {
				dev = d
				break
			}
		}
		if dev == nil {
			fmt.Fprintf(os.Stderr, "错误: 未找到设备 %q\n", device)
			os.Exit(1)
		}
	} else {
		dev = ui.ChooseDevice(devices)
		if dev == nil {
			fmt.Println("已取消")
			os.Exit(0)
		}
	}
	fmt.Printf("投屏到: %s (%s)\n", dev.Name, dev.Host)

	sess, err := player.NewSession(dev, files, port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	defer sess.Close()

	if err := sess.Load(0, seekSec); err != nil {
		fmt.Fprintf(os.Stderr, "投屏失败: %v\n", err)
		os.Exit(1)
	}
	if vol >= 0 {
		if err := sess.SetVolume(vol); err != nil && verbose {
			fmt.Fprintf(os.Stderr, "设置音量失败: %v\n", err)
		}
	}
	fmt.Println("按 空格 暂停/继续，←→ 快退/快进 30s，↑↓ 音量，n/p 切换曲目，q 退出")

	keys, cancelKeys, err := ui.Keys()
	if err != nil {
		// 无终端交互能力（如被重定向）：静默播放直到 Ctrl+C
		fmt.Fprintln(os.Stderr, "提示: 无法进入交互模式，播放中... (Ctrl+C 退出)")
		select {}
	}
	defer cancelKeys()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	volume := vol
	failCount := 0
	quit := false
	for !quit {
		select {
		case k, ok := <-keys:
			if !ok {
				quit = true
				break
			}
			switch k {
			case ui.KeyQuit:
				quit = true
			case ui.KeySpace:
				sess.TogglePause()
			case ui.KeyLeft:
				sess.Seek(-30)
			case ui.KeyRight:
				sess.Seek(30)
			case ui.KeyUp:
				if volume < 0 {
					volume, _ = sess.Volume()
				}
				volume = min(volume+10, 100)
				sess.SetVolume(volume)
			case ui.KeyDown:
				if volume < 0 {
					volume, _ = sess.Volume()
				}
				volume = max(volume-10, 0)
				sess.SetVolume(volume)
			case ui.KeyN:
				if idx := sess.Index() + 1; idx < sess.Count() {
					if err := sess.Load(idx, 0); err != nil {
						fmt.Fprintf(os.Stderr, "切换失败: %v\n", err)
					}
				}
			case ui.KeyP:
				if idx := sess.Index() - 1; idx >= 0 {
					if err := sess.Load(idx, 0); err != nil {
						fmt.Fprintf(os.Stderr, "切换失败: %v\n", err)
					}
				}
			case ui.KeyS:
				// 手动刷新，下轮循环立即重绘
			}
		case <-ticker.C:
			pos, total, playing, err := sess.Status()
			if err != nil {
				failCount++
				if failCount >= 3 {
					fmt.Fprintln(os.Stderr, "\n设备可能已断开连接")
					quit = true
					break
				}
				continue
			}
			failCount = 0
			if volume < 0 {
				volume, _ = sess.Volume()
			}
			title := files[sess.Index()].Title
			ui.RenderProgress(os.Stdout, pos, total, playing, volume, title)
		}
	}
	fmt.Println()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

**修正说明**：main() 中 flag 解析后有一段多余的占位 if（`args[0] != "ls" && ...`），实现时删除，只保留：

```go
	if len(args) >= 1 && args[0] == "ls" {
		runList(*flagTimeout)
		return
	}
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}
```

另外 `strconv` import 未用到则移除；Go 1.21+ 内置 `min`/`max`，删除自定义版本（直接用内置）。`go 1.27` 的 module 声明下内置函数直接可用。

- [ ] **Step 4: 运行全部测试**

Run: `export PATH="/d/apps/go/bin:$PATH" && go test ./... && go vet ./...`
Expected: 全部 PASS，vet 无告警

- [ ] **Step 5: 写 README**

```markdown
# dlnacast-cli

DLNA 投屏命令行工具。把本地视频/图片/音频文件或 http(s) URL 投到局域网内的电视等 DLNA 渲染设备。

## 安装

    go build -o dlnacast.exe .

## 使用

    # 投本地视频
    dlnacast video.mp4

    # 投多个文件（播放列表，n/p 切换）
    dlnacast ep1.mp4 ep2.mp4 ep3.mp4

    # 投图片 / 音频
    dlnacast photo.jpg
    dlnacast song.mp3

    # 投 URL
    dlnacast http://example.com/stream.mp4

    # 带字幕（srt，自动转 VTT）
    dlnacast -s subtitle.srt movie.mp4

    # 指定设备、起始位置、音量
    dlnacast -d "客厅" -seek 120 -vol 30 movie.mp4

    # 只列出设备
    dlnacast ls

## 快捷键

    空格    暂停/继续
    ←/→    快退/快进 30 秒
    ↑/↓    音量 ±10
    n/p    下一个/上一个
    s      刷新状态
    q      退出（停止投屏）

## 注意

- 电脑与电视需在同一局域网
- Windows 首次运行如被防火墙拦截，需允许本程序访问局域网
- 本地文件通过内置 HTTP 服务提供给电视（支持拖动进度条）
```

- [ ] **Step 6: Commit**

```bash
git add main.go main_test.go README.md
git commit -m "feat: cli entrypoint with flags and control loop"
```

---

### Task 11: 真机手工验收

**Files:** 无新文件（验收记录写入 commit message 或不记录）

- [ ] **Step 1: 构建**

Run: `export PATH="/d/apps/go/bin:$PATH" && go build -o dlnacast.exe .`
Expected: 生成 `dlnacast.exe`

- [ ] **Step 2: 手工验收清单**

1. `./dlnacast.exe ls` — 能列出局域网电视（需用户环境有真机）
2. `./dlnacast.exe <某个视频>` — 电视开始播放，进度条正常刷新
3. 空格暂停/继续、←→ 快退快进、↑↓ 音量
4. 多文件播放列表 n/p 切换
5. `-s` 字幕生效（部分电视不支持 VTT，属正常兼容性问题）
6. URL 直投
7. q 退出后电视停止播放

若用户环境无真机可测，跳过本任务并在总结中说明。

