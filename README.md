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
