// main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
		// URL 直投也按扩展名归类，无扩展名默认 video
		typ := mediaExts[strings.ToLower(filepath.Ext(arg))]
		if typ == "" {
			typ = "video"
		}
		return &server.File{URL: arg, Title: urlTitle(arg), Type: typ}
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
		flagSub     = flag.String("s", "", "字幕文件（srt），多个用逗号分隔与媒体一一对应")
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
	if len(args) >= 1 && args[0] == "ls" {
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
		files = append(files, f)
	}

	if *flagSub != "" {
		if err := applySubtitles(files, *flagSub); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
	}

	runCast(files, *flagDevice, *flagSeek, *flagVol, *flagPort, *flagTimeout, *flagVerbose)
}

// applySubtitles 把 -s 字幕列表按逗号拆分后映射到媒体列表：
// 1 个字幕应用到第一个视频文件；多个字幕必须与媒体数量一致，按索引一一对应。
func applySubtitles(files []*server.File, subFlag string) error {
	subs := strings.Split(subFlag, ",")
	for _, sub := range subs {
		if _, err := os.Stat(sub); err != nil {
			return fmt.Errorf("字幕文件不存在: %s", sub)
		}
	}
	if len(subs) == 1 {
		for _, f := range files {
			if f.Type == "video" {
				f.SubPath = subs[0]
				return nil
			}
		}
		return fmt.Errorf("字幕数量与媒体数量不一致")
	}
	if len(subs) != len(files) {
		return fmt.Errorf("字幕数量与媒体数量不一致")
	}
	for i, f := range files {
		f.SubPath = subs[i]
	}
	return nil
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
