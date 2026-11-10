# dlnacast-cli 设计文档

日期：2026-11-10

## 概述

用 Go 实现的 DLNA 投屏命令行工具。支持将本地视频/图片/音频文件或 http(s) URL 投到局域网内的 DLNA 渲染设备（智能电视等）。参考 [grunjol/dlnacasts](https://github.com/grunjol/dlnacasts) 的行为（SSDP 发现 + AVTransport 控制 + 本地 HTTP 文件服务），但不复用其 Node.js 运行时，全部协议手写实现，零第三方依赖（仅 Go 标准库）。

## 命令格式

```
dlnacast [flags] <media1> [media2 ...]   # 投屏（本地文件路径或 http(s) URL，可混用）
dlnacast ls                              # 只列出发现的设备，不投屏

Flags:
  -d <name|ip>    指定设备（名称子串匹配或 IP 精确匹配），跳过交互选择
  -t <title>      媒体标题（默认取文件名/URL 尾段）
  -s <sub.srt>    字幕文件（配合视频；多文件时与视频按逗号分隔一一对应）
  -seek <sec>     起始播放位置（秒）
  -vol <0-100>    初始音量
  -port <port>    本地 HTTP 服务端口（默认随机）
  -timeout <sec>  设备扫描等待时长（默认 3 秒）
  -v              详细日志
```

## 技术方案

选型：**全手写协议实现**（方案 A）。SSDP 发现（UDP 多播）+ SOAP over HTTP（AVTransport / RenderingControl）+ 本地 HTTP 文件服务均为标准协议，手写量约 1500 行，换来零 UPnP 依赖和完全的可控性。对比过 goupnp（生成代码啰嗦、维护不活跃）和复用 Node dlnacasts（需 Node 运行时）。

### 数据流

```
参数解析 → 分类 media（本地路径 vs http URL）
  ↓ 本地文件
启动 HTTP 服务器（绑定 0.0.0.0，URL 用本机局域网 IP 拼出）
  ├─ 媒体文件：Content-Type 自动探测，支持 Range（电视拖进度条必需）
  └─ 字幕：srt → WebVTT 转换后服务（text/vtt）
  ↓
SSDP M-SEARCH（默认 3 秒窗口）→ 收集 MediaRenderer 设备 → 解析设备描述 XML
  ↓
选择设备：
  - 指定 -d → 名称子串匹配 / IP 精确匹配
  - 0 个 → 报错退出；1 个 → 直接用；多个 → 交互列表（↑↓ 回车选择）
  ↓
SOAP AVTransport: SetAVTransportURI（Metadata 含 res 链接、标题、upnp:class）
  → Play →（有字幕时 Metadata 带 subtitlefile_uri）
  ↓
控制循环：每 3 秒轮询 GetPositionInfo/GetTransportInfo 渲染进度
```

**关键细节：** 本机可能有多个网卡（虚拟机、WSL 等），构造给电视的 URL 时必须选与设备 IP 同网段的网卡地址。

## 运行时快捷键

| 键 | 功能 |
|---|---|
| `空格` | 暂停/继续 |
| `←` / `→` | 快退/快进 30s（SOAP Seek REL_TIME） |
| `↑` / `↓` | 音量 ±10（RenderingControl SetVolume，0-100） |
| `n` / `p` | 播放列表下一个/上一个 |
| `s` | 手动刷新状态 |
| `q` / `Ctrl+C` | Stop 并退出 |

进度单行刷新：`▶ 00:12:34 / 01:30:00 [██░░░░░░] 14% vol:30  我的视频.mp4`

## 包结构与职责

```
main.go                 # flag 解析、装配
internal/ssdp/          # M-SEARCH / RESPONSE（UDP 239.255.255.250:1900）
internal/dlna/
  ├── soap.go           # SOAP envelope 构造与响应解析
  ├── device.go         # 设备描述 XML 解析（friendlyName、控制 URL）
  ├── avtransport.go    # SetAVTransportURI / Play / Pause / Stop / Seek / GetPositionInfo / GetTransportInfo
  └── rendering.go      # GetVolume / SetVolume
internal/server/
  ├── server.go         # 本地 HTTP 服务、Range 处理
  └── subtitle.go       # srt → vtt 转换
internal/player/
  └── player.go         # 会话管理：设备 + 媒体 + 字幕；暴露 Pause/Resume/Seek/Volume/Status/Next/Prev
internal/ui/
  └── ui.go             # 设备选择交互、按键监听、进度渲染
```

每个包单一职责、可独立测试：ssdp 只管发现并返回设备描述 URL 列表；dlna 只管 SOAP 调用；server 只管文件服务；player 编排前三者；ui 只管终端交互。

## 错误处理

- 扫描无设备：`未发现 DLNA 设备，请确认电视与电脑在同一局域网`，退出码 1
- 电视拒绝 URI（格式不支持等）：报 SOAP Fault 原文，提示可能是格式问题
- 控制轮询连续失败 3 次：提示设备可能断开，自动停止本地服务后退出
- Windows 防火墙拦截 UDP/HTTP：给出明确提示（需要允许 Go 程序通过防火墙）
- 文件不存在/不可读：参数校验阶段即报错，不启动服务

## 测试策略

- `subtitle.go` srt→vtt：表驱动单测（纯函数）
- `soap.go`：envelope 构造断言 + 响应 XML 解析（内置 XML 样例，不起真服务）
- `server.go` Range：`httptest` 起服务断言 206 / Content-Range
- `ssdp` / 真实 SOAP 交互：需真机，手工验收，不自动化

## 明确不做（YAGNI）

- 不做设备书签/配置文件持久化
- 不做音频/图片专用控制差异（统一走 SetAVTransportURI，type 由 upnp:class 区分）
- 不做跨子网/认证设备（WPA 企业等场景）
