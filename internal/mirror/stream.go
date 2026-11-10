// internal/mirror/stream.go
package mirror

import (
	"io"
	"net/http"
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
