// main_mirror_test.go — mirrorSupervisor 运行时音频降级的集成向测试。
// 用 testdata/fakecapture 假 ffmpeg（FAKE_STREAM 环境变量控制崩溃模式）：
// die 模式每次启动都崩溃，Capture 内部重启 1 次后永久失败 → Done 关闭，
// supervisor 在 grace 内检测到失败且带音频 → 去音频重试（Fallback 关闭），
// /live 源经 SwitchableReader 切到新采集（新采集的输出可读到），
// 重试后的采集再次永久失败 → Done 关闭走正常退出路径。
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dlnacast-cli/internal/mirror"
)

// buildFakeMirror 编译 testdata/fakecapture，作为假采集可执行文件。
// 复用 buildFakeCap 的编译逻辑不便（在 mirror 包内），此处独立编译。
func buildFakeMirror(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fakecapture")
	if filepath.Separator == '\\' {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakecapture")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译 fakecapture 失败: %v\n%s", err, out)
	}
	return bin
}

// syncBuf 线程安全字节缓冲。
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuf) Write(p []byte) {
	b.mu.Lock()
	b.buf.Write(p)
	b.mu.Unlock()
}

func (b *syncBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// drainLoop 持续排空 src（换源后继续读新源，把数据收进 buf）。
// io.Pipe 是同步的：没有读者会让采集子进程的 stdout 写阻塞，
// 导致 watch/cmd.Wait 永不返回、Done 永不关闭。
// 读到 EOF 不退出：可能即将 Switch 到新源，稍候重试。
// goroutine 在测试进程退出时终止（泄漏一个读循环，测试内可接受）。
func drainLoop(src *mirror.SwitchableReader) *syncBuf {
	out := &syncBuf{}
	go func() {
		b := make([]byte, 512)
		for {
			n, err := src.Read(b)
			if n > 0 {
				out.Write(b[:n])
			}
			if err != nil {
				time.Sleep(2 * time.Millisecond) // EOF：等换源，避免自旋
			}
		}
	}()
	return out
}

func TestMirrorSupervisorAudioFallback(t *testing.T) {
	fake := buildFakeMirror(t)
	os.Setenv("FAKE_STREAM", "die")
	defer os.Unsetenv("FAKE_STREAM")

	opts := mirror.Options{
		Quality:  mirror.QualityMedium,
		Encoder:  "libx264",
		AudioDev: "audio=virtual-audio-capturer", // 探测"成功"，但运行时打不开 → die 崩溃
	}
	sup, src, err := startSupervisedCapture(fake, opts, 3*time.Second)
	if err != nil {
		t.Fatalf("startSupervisedCapture: %v", err)
	}
	defer sup.Stop()

	// 全程持续排空（换源后自动读新源），避免同步管道写阻塞
	buf := drainLoop(src)

	// 降级应发生：Fallback 关闭（去音频重试 + 换源）
	select {
	case <-sup.Fallback():
	case <-time.After(15 * time.Second):
		t.Fatal("带音频且启动即崩溃时未触发音频降级")
	}

	// 重试后仍永久失败 → Done 关闭、Err 非空（正常退出路径）
	select {
	case <-sup.Done():
		if sup.Err() == nil {
			t.Error("Done 关闭后 Err 应非空")
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("重试后再失败应关闭 Done；已收数据: %q", buf.String())
	}

	// 流恢复断言：换源后的新采集输出也到达了（die 模式每次 launch 写 5 个 BAD，
	// 两次 launch + Capture 内部重启共 4 个进程 → 至少 15 个 BAD；换源生效的
	// 关键证据是 Done 前新源数据持续到达）
	if n := strings.Count(buf.String(), "BAD"); n < 10 {
		t.Errorf("换源后应读到新采集输出，BAD 总数 = %d，数据: %q", n, buf.String())
	}
}

func TestMirrorSupervisorNoAudioNoFallback(t *testing.T) {
	fake := buildFakeMirror(t)
	os.Setenv("FAKE_STREAM", "die")
	defer os.Unsetenv("FAKE_STREAM")

	opts := mirror.Options{Quality: mirror.QualityMedium, Encoder: "libx264"}
	sup, src, err := startSupervisedCapture(fake, opts, 3*time.Second)
	if err != nil {
		t.Fatalf("startSupervisedCapture: %v", err)
	}
	defer sup.Stop()
	drainLoop(src) // 必须持续排空，否则同步管道写阻塞导致 Done 永不关闭

	// 无音频：失败直接走 Done，不应触发降级
	select {
	case <-sup.Done():
		if sup.Err() == nil {
			t.Error("Err 应非空")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Done 应关闭")
	}
	select {
	case <-sup.Fallback():
		t.Error("无音频不应触发降级")
	default:
	}
}

func TestMirrorSupervisorStopNoFallback(t *testing.T) {
	// 正常 Stop（用户退出）：Done 不关闭，不触发降级
	fake := buildFakeMirror(t)
	os.Setenv("FAKE_STREAM", "die")
	defer os.Unsetenv("FAKE_STREAM")

	opts := mirror.Options{Quality: mirror.QualityMedium, Encoder: "libx264", AudioDev: "audio=virtual-audio-capturer"}
	sup, _, err := startSupervisedCapture(fake, opts, 3*time.Second)
	if err != nil {
		t.Fatalf("startSupervisedCapture: %v", err)
	}

	sup.Stop()
	select {
	case <-sup.Done():
		t.Error("Stop 后 Done 不应关闭（Done 仅采集永久失败时关闭）")
	default:
	}
	select {
	case <-sup.Fallback():
		t.Error("Stop 不应触发降级")
	default:
	}
}
