// internal/mirror/stream.go
package mirror

import (
	"io"
	"net/http"
	"sync/atomic"
)

// NewLiveHandler 返回 /live 直播端点：把 src 的 MPEG-TS 字节零缓冲直通给客户端。
// 每块写入后立即 Flush；源 EOF 或客户端断开时结束。
// 单生产者（ffmpeg 管道）单消费者（一台电视）模型。
func NewLiveHandler(src io.Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "no-store")
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return // 客户端断开
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if err != nil {
				return // 源 EOF（采集结束/崩溃未恢复）
			}
		}
	})
}

// SwitchableReader 可运行时切换底层源的 io.Reader（atomic.Pointer 切换）。
// 用于音频降级重试：/live handler 持有它，采集重试后 Switch 到新源，
// 已连接的客户端无感续读。
type SwitchableReader struct {
	cur atomic.Pointer[io.Reader] // 指向 io.Reader 的指针，便于原子换源
}

func NewSwitchableReader(r io.Reader) *SwitchableReader {
	s := &SwitchableReader{}
	s.cur.Store(&r)
	return s
}

// Switch 换底层源。后续 Read 走新源；旧源由调用方负责关闭。
// 运行中换源在旧源阻塞读时需要等旧源 EOF/返回后才能生效到新读请求
// （handler 是单消费者顺序读，读到 EOF 时下一次 Read 已用新源）。
func (s *SwitchableReader) Switch(r io.Reader) {
	s.cur.Store(&r)
}

func (s *SwitchableReader) Read(p []byte) (int, error) {
	return (*s.cur.Load()).Read(p)
}
