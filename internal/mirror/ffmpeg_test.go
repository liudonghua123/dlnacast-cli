package mirror

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestParseRegion(t *testing.T) {
	cases := []struct {
		in         string
		x, y, w, h int
		wantErr    bool
	}{
		{"100,50,800,600", 100, 50, 800, 600, false},
		{"0,0,1920,1080", 0, 0, 1920, 1080, false},
		{"100,50", 0, 0, 0, 0, true},
		{"a,b,c,d", 0, 0, 0, 0, true},
		{"0,0,0,600", 0, 0, 0, 0, true},
		{"0,0,800,-5", 0, 0, 0, 0, true},
	}
	for _, c := range cases {
		x, y, w, h, err := ParseRegion(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseRegion(%q) 期望报错，得到 nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRegion(%q) 意外报错: %v", c.in, err)
			continue
		}
		if x != c.x || y != c.y || w != c.w || h != c.h {
			t.Errorf("ParseRegion(%q) = %d,%d,%d,%d，期望 %d,%d,%d,%d", c.in, x, y, w, h, c.x, c.y, c.w, c.h)
		}
	}
}

func TestBuildArgsWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅验证 windows 分支")
	}
	// 默认（裁切 16:9，不含任务栏）：工作区居中裁切 16:9。
	// 用 displayWorkArea 实际值拼断言，不依赖开发机分辨率。
	// taskbar 用例假定整屏也比 16:9 窄（本机 1440x900 即如此）——
	// 若在超宽屏上跑，taskbar 用例的算式仍与实现一致（同款 16:9 裁切算式）。
	wx, wy, ww, wh, err := displayWorkArea(0)
	if err != nil {
		t.Fatalf("displayWorkArea(0) 报错: %v", err)
	}
	cx, cy, cw, ch := wx, wy, ww, wh
	if cw*9 > ch*16 {
		cw = ch * 16 / 9
		cx += (ww - cw) / 2
	} else if cw*9 < ch*16 {
		ch = cw * 9 / 16
		cy += (wh - ch) / 2
	}
	defaultGeom := []string{"-offset_x", strconv.Itoa(cx), "-offset_y", strconv.Itoa(cy),
		"-video_size", fmt.Sprintf("%dx%d", cw, ch)}
	sw, sh, err := displaySize(0)
	if err != nil {
		t.Fatalf("displaySize(0) 报错: %v", err)
	}
	cases := []struct {
		name    string
		opts    Options
		wantSub []string // 参数序列中必须按序出现的片段
		notWant []string // 不得出现的片段
	}{
		{
			name: "medium 默认主屏 libx264（工作区 16:9 裁切+缩放 1920）",
			opts: Options{Quality: QualityMedium, Encoder: "libx264"},
			wantSub: append([]string{"-f", "gdigrab", "-framerate", "30"}, append(defaultGeom,
				[]string{"-vf", "scale=1920:-2",
					"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-pix_fmt", "yuv420p",
					"-b:v", "4M", "-g", "60",
					"-f", "mpegts", "-muxdelay", "0", "-muxpreload", "0", "-flush_packets", "-1", "pipe:1"}...)...),
			notWant: []string{"dshow"},
		},
		{
			name:    "low 带缩放（工作区 16:9 裁切+缩放 1280）",
			opts:    Options{Quality: QualityLow, Encoder: "h264_qsv"},
			wantSub: []string{"-framerate", "24", "-video_size", fmt.Sprintf("%dx%d", cw, ch), "-vf", "scale=1280:-2", "-c:v", "h264_qsv", "-preset", "veryfast", "-b:v", "2M", "-g", "48"},
		},
		{
			// taskbar=true 采集整屏（origin+size），默认仍裁切 16:9：
			// 整屏比 16:9 窄 → 裁高度 sw*9/16，offset_y 居中
			name:    "taskbar 包含任务栏（整屏后裁切）",
			opts:    Options{Quality: QualityMedium, Encoder: "libx264", Taskbar: true},
			wantSub: []string{"-offset_x", strconv.Itoa(wx), "-offset_y", strconv.Itoa(wy + (sh-sw*9/16)/2), "-video_size", fmt.Sprintf("%dx%d", sw, sw*9/16)},
		},
		{
			// taskbar + no-fill：整屏原比例
			name:    "taskbar + no-fill（整屏原比例）",
			opts:    Options{Quality: QualityMedium, Encoder: "libx264", Taskbar: true, NoFill: true},
			wantSub: []string{"-offset_x", strconv.Itoa(wx), "-offset_y", strconv.Itoa(wy), "-video_size", fmt.Sprintf("%dx%d", sw, sh)},
		},
		{
			name:    "region 覆盖 display",
			opts:    Options{Region: "100,50,800,600", Quality: QualityHigh, Encoder: "h264_qsv"},
			wantSub: []string{"-offset_x", "100", "-offset_y", "50", "-video_size", "800x600"},
			notWant: []string{"-vf"},
		},
		{
			name:    "带音频",
			opts:    Options{Quality: QualityMedium, Encoder: "libx264", AudioDev: "audio=virtual-audio-capturer"},
			wantSub: []string{"-f", "dshow", "-i", "audio=virtual-audio-capturer", "-c:a", "aac", "-b:a", "128k"},
		},
		{
			name:    "nvenc 参数",
			opts:    Options{Quality: QualityMedium, Encoder: "h264_nvenc"},
			wantSub: []string{"-c:v", "h264_nvenc", "-preset", "p1"},
			notWant: []string{"-pix_fmt"},
		},
		{
			name:    "amf 参数",
			opts:    Options{Quality: QualityMedium, Encoder: "h264_amf"},
			wantSub: []string{"-c:v", "h264_amf", "-quality", "speed"},
		},
	}
	for _, c := range cases {
		args, err := BuildArgs("ffmpeg", c.opts)
		if err != nil {
			t.Fatalf("%s: BuildArgs 报错: %v", c.name, err)
		}
		got := strings.Join(args, " ")
		pos := 0
		for _, seg := range c.wantSub {
			i := strings.Index(got[pos:], seg)
			if i < 0 {
				t.Errorf("%s: 参数缺少片段 %q（按序），实际: %s", c.name, seg, got)
				break
			}
			pos += i + len(seg)
		}
		for _, seg := range c.notWant {
			if strings.Contains(got, seg) {
				t.Errorf("%s: 参数不应包含 %q，实际: %s", c.name, seg, got)
			}
		}
		if args[0] == "ffmpeg" {
			t.Errorf("%s: 返回值不应包含程序名 ffmpeg", c.name)
		}
	}
}

func TestBuildArgsErrors(t *testing.T) {
	if _, err := BuildArgs("ffmpeg", Options{Quality: QualityMedium}); err == nil {
		t.Error("Encoder 为空应报错")
	}
	if _, err := BuildArgs("ffmpeg", Options{Quality: "ultra", Encoder: "libx264"}); err == nil {
		t.Error("非法 quality 应报错")
	}
	if _, err := BuildArgs("ffmpeg", Options{Quality: QualityMedium, Encoder: "libx264", Region: "bad"}); err == nil {
		t.Error("非法 region 应报错")
	}
	if _, err := BuildArgs("ffmpeg", Options{Quality: QualityMedium, Encoder: "h264_unknown"}); err == nil {
		t.Error("未知编码器应报错")
	}
}

func TestBuildArgsHighNoScale(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("仅验证 windows 分支")
	}
	args, err := BuildArgs("ffmpeg", Options{Quality: QualityHigh, Encoder: "libx264"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(args, " ")
	if strings.Contains(got, "-vf") {
		t.Errorf("high 档不应有缩放参数: %s", got)
	}
}
