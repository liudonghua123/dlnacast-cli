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
	"dlnacast-cli/internal/mirror"
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
		fmt.Fprintf(os.Stderr, "用法:\n  dlnacast [flags] <media1> [media2 ...]\n  dlnacast mirror [flags]\n  dlnacast ls\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) >= 1 && args[0] == "ls" {
		runList(*flagTimeout)
		return
	}
	if len(args) >= 1 && args[0] == "mirror" {
		runMirrorCommand(args[1:], *flagDevice, *flagTimeout, *flagVerbose, *flagPort)
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

// selectDevice 扫描并选择 DLNA 设备：name 非空时按 IP 或名称子串匹配，
// 否则交互选择。找不到时返回错误。
func selectDevice(name string, timeout int, verbose bool) (*dlna.Device, error) {
	locs, err := ssdp.Discover(time.Duration(timeout) * time.Second)
	if err != nil {
		return nil, fmt.Errorf("扫描失败: %w", err)
	}
	if len(locs) == 0 {
		return nil, fmt.Errorf("未发现 DLNA 设备，请确认电视与电脑在同一局域网")
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
		return nil, fmt.Errorf("未发现可用的 DLNA 设备")
	}
	var dev *dlna.Device
	if name != "" {
		for _, d := range devices {
			if d.Host == name || strings.Contains(d.Name, name) {
				dev = d
				break
			}
		}
		if dev == nil {
			return nil, fmt.Errorf("未找到设备 %q", name)
		}
	} else {
		dev = ui.ChooseDevice(devices)
		if dev == nil {
			return nil, fmt.Errorf("已取消")
		}
	}
	return dev, nil
}

func runCast(files []*server.File, device string, seekSec, vol, port, timeout int, verbose bool) {
	dev, err := selectDevice(device, timeout, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
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

func runMirrorCommand(rest []string, device string, timeout int, verbose bool, port int) {
	fs := flag.NewFlagSet("mirror", flag.ExitOnError)
	var (
		display = fs.Int("display", 0, "显示器编号（0 = 主屏）")
		region  = fs.String("region", "", "只采集屏幕区域 x,y,w,h")
		quality = fs.String("quality", "medium", "画质 low/medium/high")
		noAudio = fs.Bool("no-audio", false, "不采集系统声音")
	)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: dlnacast [全局 flags] mirror [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(rest); err != nil {
		os.Exit(1)
	}

	opts := mirror.Options{
		Display: *display,
		Region:  *region,
		NoAudio: *noAudio,
	}
	switch mirror.Quality(*quality) {
	case mirror.QualityLow:
		opts.Quality = mirror.QualityLow
	case mirror.QualityMedium:
		opts.Quality = mirror.QualityMedium
	case mirror.QualityHigh:
		opts.Quality = mirror.QualityHigh
	default:
		fmt.Fprintf(os.Stderr, "错误: quality 必须是 low/medium/high\n")
		os.Exit(1)
	}

	ff, err := mirror.FindFFmpeg()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	enc, err := mirror.ProbeEncoder(ff)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	opts.Encoder = enc
	if verbose {
		fmt.Printf("编码器: %s\n", enc)
	}

	if !opts.NoAudio {
		if audioDev, ok := mirror.ProbeAudioDevice(ff); ok {
			opts.AudioDev = audioDev
			if verbose {
				fmt.Printf("音频设备: %s\n", audioDev)
			}
		} else {
			fmt.Println("提示: 未找到系统声音采集设备，仅镜像画面")
		}
	}

	if opts.Region != "" {
		if _, _, _, _, err := mirror.ParseRegion(opts.Region); err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			os.Exit(1)
		}
	}

	runMirror(opts, device, port, timeout, verbose)
}

func runMirror(opts mirror.Options, device string, port, timeout int, verbose bool) {
	dev, err := selectDevice(device, timeout, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("投屏到: %s (%s)\n", dev.Name, dev.Host)

	args, err := mirror.BuildArgs("ffmpeg", opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	ff, _ := mirror.FindFFmpeg() // runMirrorCommand 已保证成功
	if verbose {
		fmt.Printf("ffmpeg 参数: %s %s\n", ff, strings.Join(args, " "))
	}

	cap, err := mirror.Start(ff, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}

	srv := server.NewServer(port)
	srv.SetLiveHandler(mirror.NewLiveHandler(cap.Stdout()))
	if err := srv.Start(); err != nil {
		cap.Stop()
		fmt.Fprintf(os.Stderr, "启动本地服务失败: %v\n", err)
		os.Exit(1)
	}

	ip, err := dlna.LocalIPFor(dev.Host)
	if err != nil {
		cap.Stop()
		srv.Close()
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
	liveURL := fmt.Sprintf("http://%s:%d/live", ip, srv.Port())
	fmt.Printf("直播地址: %s\n", liveURL)

	tr := dlna.NewTransport(dev, dlna.NewClient())
	recast := func() {
		if err := tr.Load(liveURL, "屏幕镜像", "video", ""); err != nil {
			fmt.Fprintf(os.Stderr, "\n投放失败: %v\n", err)
		}
	}
	recast()

	keys, cancelKeys, err := ui.Keys()
	if err != nil {
		fmt.Fprintln(os.Stderr, "提示: 无法进入交互模式，镜像中... (Ctrl+C 退出)")
		select {}
	}
	defer cancelKeys()

	fmt.Println("按 s 重新同步到实时画面，↑↓ 音量，q 退出")
	start := time.Now()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	volume := -1
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
			case ui.KeyS:
				recast() // 重新投放，回到实时点
			case ui.KeyUp:
				if volume < 0 {
					volume, _ = tr.Volume()
				}
				volume = min(volume+10, 100)
				tr.SetVolume(volume)
			case ui.KeyDown:
				if volume < 0 {
					volume, _ = tr.Volume()
				}
				volume = max(volume-10, 0)
				tr.SetVolume(volume)
			}
		case <-cap.Done():
			fmt.Fprintf(os.Stderr, "\n采集失败: %v\n", cap.Err())
			quit = true
		case <-ticker.C:
			elapsed := int(time.Since(start).Seconds())
			_, _, playing, err := tr.Status()
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
				volume, _ = tr.Volume()
			}
			ui.RenderProgress(os.Stdout, elapsed, 0, playing, volume, "屏幕镜像")
		}
	}
	fmt.Println()
	cap.Stop()
	srv.Close()
	tr.Stop()
}
