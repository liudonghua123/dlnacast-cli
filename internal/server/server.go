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
	URL     string // 非空 = 远程直投，不经本地服务
	Path    string // 本地文件路径
	SubPath string // 可选 srt 字幕
	Title   string
	Type    string // "video" / "audio" / "image"
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

// SetLiveHandler 注册 /live 直播端点（mirror 用）。须在 Start 前调用；
// 与 SetFiles 互斥使用（mirror 会话没有媒体文件）。
func (s *Server) SetLiveHandler(h http.Handler) {
	if s.mux == nil {
		s.mux = http.NewServeMux()
	}
	s.mux.Handle("/live", h)
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
