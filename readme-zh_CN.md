# dlnacast-cli

[![Release](https://img.shields.io/github/v/release/liudonghua123/dlnacast-cli?include_prereleases&label=release)](https://github.com/liudonghua123/dlnacast-cli/releases)
[![Build](https://github.com/liudonghua123/dlnacast-cli/actions/workflows/release.yml/badge.svg)](https://github.com/liudonghua123/dlnacast-cli/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md)

DLNA 投屏命令行工具。把本地视频/图片/音频文件或 http(s) URL 投到局域网内的电视等 DLNA 渲染设备（智能电视、机顶盒等）。

跨平台（Windows / macOS / Linux），单一二进制，无运行时依赖。

## 安装

### 下载预编译版本（推荐）

从 [GitHub Releases](https://github.com/liudonghua123/dlnacast-cli/releases) 下载对应平台的压缩包（`latest` 标签随 main 分支自动构建）：

| 平台 | 文件 |
|---|---|
| Windows x64 | `dlnacast-windows-amd64.zip` |
| macOS arm64 (Apple Silicon) | `dlnacast-darwin-arm64.tar.gz` |
| macOS x64 (Intel) | `dlnacast-darwin-amd64.tar.gz` |
| Linux x64 | `dlnacast-linux-amd64.tar.gz` |
| Linux arm64 | `dlnacast-linux-arm64.tar.gz` |

### 从源码构建

需要 Go 1.27+：

    go build -o dlnacast .

## 使用

    # 投本地视频
    dlnacast video.mp4

    # 多个文件（播放列表，n/p 切换）
    dlnacast ep1.mp4 ep2.mp4 ep3.mp4

    # 投图片 / 音频
    dlnacast photo.jpg
    dlnacast song.mp3

    # 投 URL
    dlnacast http://example.com/stream.mp4

    # 带字幕（srt，自动转 VTT；多个用逗号分隔与媒体一一对应）
    dlnacast -s subtitle.srt movie.mp4
    dlnacast -s ep1.srt,ep2.srt ep1.mp4 ep2.mp4

    # 指定设备、起始位置、音量
    dlnacast -d "客厅" -seek 120 -vol 30 movie.mp4

    # 只列出设备
    dlnacast ls

### 全部参数

```
-d <name|ip>    指定设备名称（子串匹配）或 IP
-t <title>      媒体标题（默认取文件名）
-s <sub.srt>    字幕文件（srt），多个用逗号分隔与媒体一一对应
-seek <sec>     起始播放位置（秒）
-vol <0-100>    初始音量
-port <port>    本地 HTTP 服务端口（默认随机）
-timeout <sec>  设备扫描等待秒数（默认 3）
-v              详细输出
```

## 屏幕镜像

把电脑屏幕实时镜像到电视（需要本机安装 [ffmpeg](https://ffmpeg.org/download.html) 并加入 PATH）：

    dlnacast mirror

指定显示器、区域或画质档：

    # 第二个显示器
    dlnacast -v mirror -display 1

    # 屏幕区域 x,y,w,h
    dlnacast mirror -region 100,50,1280,720

    # 画质档：low（720p/2M）| medium（1080p/4M，默认）| high（原生分辨率/8M）
    dlnacast mirror -quality low

    # 只镜像画面（不采集系统声音）
    dlnacast mirror -no-audio

### 镜像参数

```
-display <n>     显示器编号（0 = 主屏，默认）
-region x,y,w,h  只采集该区域（优先于 -display）
-quality <p>     low / medium / high（默认 medium）
-no-audio        不采集系统声音
```

全局参数（`-d`、`-timeout`、`-v`、`-port`）写在 `mirror` **之前**；镜像参数写在其后。

镜像快捷键：`s` 重新同步到实时画面，`↑`/`↓` 音量，`q` 退出。

### 延迟说明

DLNA 是"推 URL 给电视播放器"模型，电视播放器固有的 1~3 秒缓冲无法从投屏端消除。投屏端管线（采集 → 硬件编码 → HTTP 推流）延迟低于 100ms，端到端延迟预计 **1~3 秒**，瓶颈在电视。系统声音需要虚拟音频采集设备（Windows 上如 `virtual-audio-capturer`）；没有时自动降级为只镜像画面。H.264 编码按 NVENC → QSV → AMF → libx264（ultrafast）顺序取第一个可用项。

## 快捷键

投屏过程中（需要真实终端）：

| 键 | 功能 |
|---|---|
| `空格` | 暂停/继续 |
| `←` / `→` | 快退/快进 30 秒 |
| `↑` / `↓` | 音量 ±10 |
| `n` / `p` | 下一个/上一个 |
| `s` | 刷新状态 |
| `q` | 退出（停止投屏） |

## 工作原理

1. SSDP（UDP 多播）发现局域网内的 DLNA MediaRenderer 设备
2. 本地文件通过内置 HTTP 服务提供给电视（支持 Range，可拖动进度条；字幕自动 srt → WebVTT）
3. SOAP 调用 AVTransport / RenderingControl 完成播放控制（暂停、快进、音量等）

## 注意

- 电脑与电视需在同一局域网
- Windows 首次运行如被防火墙拦截，需允许本程序访问局域网
- 部分电视对字幕（WebVTT）支持不佳，属设备兼容性问题
- 字符界面下（如管道/重定向）自动降级为静默播放，Ctrl+C 退出

## License

[MIT](LICENSE)
