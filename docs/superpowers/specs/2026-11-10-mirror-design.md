# dlnacast-cli 屏幕镜像（mirror）设计文档

日期：2026-11-10

## 概述

为 dlnacast-cli 新增 `dlnacast mirror` 子命令：把当前电脑屏幕实时镜像到 DLNA 渲染设备。技术路线为外部 ffmpeg 做采集与硬件编码，Go 编排流媒体服务与 DLNA 投放。

## 延迟的现实约束

DLNA/AVTransport 是"推 URL 给电视播放器"模型，电视播放器固有的 1~3 秒缓冲无法从投屏端消除。本设计把投屏端（采集→编码→HTTP 推流）的延迟压到 100ms 以内，端到端延迟预计 1~3 秒，瓶颈在电视。README 如实说明。

## 命令格式

```
dlnacast mirror [flags]
  -display <n>     显示器编号（默认 0 = 主屏）
  -region x,y,w,h  只采集屏幕区域（与 -display 互斥，region 优先校验）
  -quality <p>     low / medium / high（默认 medium）
  -no-audio        不采集系统声音
  复用全局 flag：-d、-timeout、-v
```

## 数据流

```
ffmpeg 子进程
  采集（按平台）:
    Windows: 视频 gdigrab / 音频 dshow("virtual-audio-capturer"，无则降级纯视频)
    macOS:   avfoundation
    Linux:   x11grab / 音频 pulse
  编码（逐级探测回落）: h264_nvenc → h264_qsv → h264_amf → libx264(ultrafast, zerolatency)
  输出: MPEG-TS (H.264 + AAC-LC)，-flush_packets -1，短 GOP
    ↓ stdout 管道
Go HTTP 服务器新增 GET /live
  零缓冲直通：http.Flusher 每块写后 Flush；Content-Type video/mp2t
    ↓ 媒体 URL = http://<localIP>:<port>/live
SOAP SetAVTransportURI(直播 URL, upnp:class object.item.videoItem) → Play
    ↓
控制循环复用现有实现；进度条位置改为已投时长；s 键改为"重投"（重新 SetAVTransportURI 回实时点）
```

## 画质预设（-quality）

| 档 | 分辨率缩放 | 帧率 | 视频码率 | GOP |
|---|---|---|---|---|
| low | 1280x720 | 24 | 2M | 48 |
| medium | 1920x1080 | 30 | 4M | 60 |
| high | 源分辨率 | 30 | 8M | 60 |

低延迟参数：`-tune zerolatency`、`-preset ultrafast`（libx264）、短 GOP、`-muxdelay 0 -muxpreload 0`、`-flush_packets -1`。

## 采集区域

- 默认主屏（-display 0）
- -display n：多显示器（ffmpeg gdigrab 用 `-offset_x/-offset_y/-video_size` 按显示器原点采集；显示器枚举用 Go `golang.org/x/sys/windows` 或 ffmpeg `-list_devices` 探测，实现取简单者）
- -region x,y,w,h：直接传给 ffmpeg 的 offset/size 参数，与 -display 互斥

## 编解码器探测

启动时运行 `ffmpeg -hide_banner -encoders`，按 nvenc → qsv → amf → libx264 顺序选择第一个可用项；`-v` 时打印所选编码器。Windows 音频设备用 `ffmpeg -list_devices` 探测 dshow 的 "virtual-audio-capturer"；探测失败或采集失败时自动去掉音频参数重试一次。

## 包结构

```
internal/mirror/
  ├── ffmpeg.go     # ffmpeg 路径探测、参数构造（纯函数，表驱动单测）、编码器探测
  ├── capture.go    # 子进程生命周期：启动、stdout 管道、崩溃重启 1 次、停止
  └── stream.go     # /live HTTP 端点：单生产者单消费者管道直通
main.go             # mirror 子命令分发；设备发现/选择/投放复用现有代码
```

接口：

```go
// ffmpeg.go
type Quality string // "low" / "medium" / "high"
type Options struct {
    Display int    // 显示器编号
    Region  string // 非空时为 "x,y,w,h"，覆盖 Display
    Quality Quality
    NoAudio bool
}
func FindFFmpeg() (string, error)                  // PATH → 常见安装位置
func BuildArgs(ff string, opts Options) ([]string, error) // 完整 ffmpeg 参数
func ProbeEncoder(ff string) (string, error)       // 返回可用 h264 编码器名
func ProbeAudioDevice(ff string) (string, bool)    // dshow virtual-audio-capturer 探测（仅 Windows）

// capture.go
type Capture struct{ ... }
func Start(ff string, args []string) (*Capture, error)
func (c *Capture) Stdout() io.Reader
func (c *Capture) Stop() error   // kill + wait
// 崩溃后自动重启 1 次（重启期间 Stdout 读到 EOF，由 stream 层断开重连逻辑处理：直接结束投屏会话）

// stream.go
func NewLiveHandler(src io.Reader) http.Handler  // 零缓冲直通
```

## 错误处理

- ffmpeg 不存在：报错并提示 https://ffmpeg.org 下载
- 编码器探测失败：回落链末尾必为 libx264（ffbuild 默认含），链空时报错
- 音频设备缺失/打开失败：首次带音频启动失败后去音频重试一次
- ffmpeg 崩溃：自动重启 1 次；再失败则停止投屏退出，提示错误输出
- 退出（q / Ctrl+C / 轮询失败 3 次）：先 Capture.Stop()（kill ffmpeg）再 Stop 电视
- -region 格式非法：参数校验即报错

## 测试策略

- ffmpeg.go 参数构造：表驱动单测（平台 × 预设 × 音频开关 × region/display）
- capture.go：用 fake ffmpeg（脚本输出固定字节流）测启动/停止/崩溃重启
- stream.go：httptest 断言 chunked 直通（禁缓冲头、内容一致）
- 真机：电视实际镜像 + 延迟主观评估
