// internal/mirror/ffmpeg.go
package mirror

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Quality string

const (
	QualityLow    Quality = "low"
	QualityMedium Quality = "medium"
	QualityHigh   Quality = "high"
)

type preset struct {
	w, h    int    // 采集分辨率；0,0 = 显示器原生分辨率（仅 high）
	fps     int    //
	bitrate string //
	gop     int    //
	scale   bool   // 是否追加 -vf scale 缩放
}

// 画质预设精确值：low=1280x720/24fps/2M/GOP48；medium=1920x1080/30fps/4M/GOP60；
// high=源分辨率/30fps/8M/GOP60。
var presets = map[Quality]preset{
	QualityLow:    {w: 1280, h: 720, fps: 24, bitrate: "2M", gop: 48, scale: true},
	QualityMedium: {w: 1920, h: 1080, fps: 30, bitrate: "4M", gop: 60},
	QualityHigh:   {fps: 30, bitrate: "8M", gop: 60},
}

// Options 镜像采集参数。Encoder/AudioDev 由探测步骤填充后传入 BuildArgs。
type Options struct {
	Display  int    // 显示器编号，0 = 主屏
	Region   string // 非空为 "x,y,w,h"，覆盖 Display
	Quality  Quality
	NoAudio  bool
	Encoder  string // h264 编码器名（ProbeEncoder 结果）
	AudioDev string // dshow 音频设备（如 audio=virtual-audio-capturer），空 = 无音频
}

// ParseRegion 解析 "x,y,w,h"，要求 w>0、h>0。
func ParseRegion(s string) (x, y, w, h int, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, fmt.Errorf("region 格式应为 x,y,w,h，得到 %q", s)
	}
	nums := make([]int, 4)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("region 含非数字 %q", p)
		}
		nums[i] = v
	}
	if nums[2] <= 0 || nums[3] <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("region 宽高必须为正数")
	}
	return nums[0], nums[1], nums[2], nums[3], nil
}

func qualityOf(q Quality) (preset, error) {
	p, ok := presets[q]
	if !ok {
		return preset{}, fmt.Errorf("quality 必须是 low/medium/high，得到 %q", q)
	}
	return p, nil
}

// encoderArgs 返回编码器专属参数（顺序：在 -c:v 之后）。
func encoderArgs(enc string) []string {
	switch enc {
	case "libx264":
		return []string{"-preset", "ultrafast", "-tune", "zerolatency", "-pix_fmt", "yuv420p"}
	case "h264_qsv":
		return []string{"-preset", "veryfast"}
	case "h264_nvenc":
		return []string{"-preset", "p1"}
	case "h264_amf":
		return []string{"-quality", "speed"}
	}
	return nil
}

// BuildArgs 构造完整 ffmpeg 参数（不含程序名）。纯函数，不执行 ffmpeg。
func BuildArgs(ff string, opts Options) ([]string, error) {
	if opts.Encoder == "" {
		return nil, fmt.Errorf("Encoder 未设置（需先 ProbeEncoder）")
	}
	p, err := qualityOf(opts.Quality)
	if err != nil {
		return nil, err
	}

	args := []string{"-hide_banner", "-loglevel", "error"}

	// 视频采集输入
	switch runtime.GOOS {
	case "windows":
		args = append(args, "-f", "gdigrab", "-framerate", strconv.Itoa(p.fps))
		if opts.Region != "" {
			x, y, w, h, err := ParseRegion(opts.Region)
			if err != nil {
				return nil, err
			}
			args = append(args, "-offset_x", strconv.Itoa(x), "-offset_y", strconv.Itoa(y),
				"-video_size", fmt.Sprintf("%dx%d", w, h))
		} else {
			x, y, err := displayOrigin(opts.Display)
			if err != nil {
				return nil, err
			}
			// low/medium 使用预设固定采集分辨率；high 使用显示器原生分辨率
			w, h := p.w, p.h
			if w == 0 || h == 0 {
				w, h, err = displaySize(opts.Display)
				if err != nil {
					return nil, err
				}
			}
			args = append(args, "-offset_x", strconv.Itoa(x), "-offset_y", strconv.Itoa(y),
				"-video_size", fmt.Sprintf("%dx%d", w, h))
		}
		args = append(args, "-i", "desktop")
	case "darwin":
		args = append(args, "-f", "avfoundation", "-framerate", strconv.Itoa(p.fps), "-capture_cursor", "1", "-i", "1")
	case "linux":
		args = append(args, "-f", "x11grab", "-framerate", strconv.Itoa(p.fps), "-i", ":0.0")
	default:
		return nil, fmt.Errorf("不支持的平台: %s", runtime.GOOS)
	}

	// 音频输入
	if opts.AudioDev != "" {
		switch runtime.GOOS {
		case "windows":
			args = append(args, "-f", "dshow", "-i", opts.AudioDev)
		case "linux":
			args = append(args, "-f", "pulse", "-i", "default")
		}
	}

	// 缩放（在 -c:v 之前）
	if p.scale {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", p.w))
	}

	// 编码
	if encoderArgs(opts.Encoder) == nil {
		return nil, fmt.Errorf("未知编码器: %q（需 ProbeEncoder 结果或 h264_nvenc/h264_qsv/h264_amf/libx264 之一）", opts.Encoder)
	}
	args = append(args, "-c:v", opts.Encoder)
	args = append(args, encoderArgs(opts.Encoder)...)
	args = append(args, "-b:v", p.bitrate, "-g", strconv.Itoa(p.gop))

	// 音频编码
	if opts.AudioDev != "" {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	}

	// 输出：低延迟 MPEG-TS 到 stdout
	args = append(args, "-f", "mpegts", "-muxdelay", "0", "-muxpreload", "0", "-flush_packets", "-1", "pipe:1")
	return args, nil
}

// ProbeEncoder 真实试编码探测：每个候选编码器跑一次 0.5 秒 lavfi 编码，
// 返回第一个 exit 0 的。实测结论：-encoders 列表不可靠（DLL 缺失只在运行时暴露）。
func ProbeEncoder(ff string) (string, error) {
	return ProbeEncoderWithFallback(ff, []string{"h264_nvenc", "h264_qsv", "h264_amf", "libx264"})
}

// ProbeEncoderWithFallback 按给定链试编码（可测试注入链）。
// 测试通过 FAKE_OK 环境变量控制假 ffmpeg 的成功项（环境继承见 probe_test.go）。
func ProbeEncoderWithFallback(ff string, chain []string) (string, error) {
	for _, enc := range chain {
		cmd := exec.Command(ff,
			"-hide_banner", "-loglevel", "error",
			"-f", "lavfi", "-i", "testsrc=size=320x240:rate=30", "-t", "0.5",
			"-c:v", enc, "-f", "mpegts", "-")
		if err := cmd.Run(); err == nil {
			return enc, nil
		}
	}
	return "", fmt.Errorf("没有可用的 H.264 编码器（已尝试: %s）", strings.Join(chain, " → "))
}

// ProbeAudioDevice 探测 Windows dshow 虚拟音频设备（系统声音采集）。
// 非 Windows 或未找到时返回 false，调用方降级为纯视频。
func ProbeAudioDevice(ff string) (string, bool) {
	if runtime.GOOS != "windows" {
		return "", false
	}
	out, _ := exec.Command(ff, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy").CombinedOutput()
	if strings.Contains(string(out), "virtual-audio-capturer") {
		return "audio=virtual-audio-capturer", true
	}
	return "", false
}

// FindFFmpeg 在 PATH 与常见安装位置查找 ffmpeg。
func FindFFmpeg() (string, error) {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			`C:\ffmpeg\bin\ffmpeg.exe`,
			`C:\Program Files\ffmpeg\bin\ffmpeg.exe`,
		}
	default:
		candidates = []string{"/usr/local/bin/ffmpeg", "/opt/homebrew/bin/ffmpeg"}
	}
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("未找到 ffmpeg，请安装后加入 PATH: https://ffmpeg.org/download.html")
}
