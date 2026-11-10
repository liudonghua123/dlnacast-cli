package mirror

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildFakeCap 编译 testdata/fakecapture，返回可执行路径。
// fakecapture 行为（环境变量控制）：
//
//	FAKE_STREAM=ok     向 stdout 持续写 "LIVE"（每 20ms 一次），直到被 kill
//	FAKE_STREAM=crash  写 5 次 "BAD" 后 exit 1；若 FAKE_RESTART=1 则重启后行为同 ok
//	                     （重启由被测的 capture.go 负责，fake 自身 crash 后不再写）
//	FAKE_STREAM=die    写 5 次 "BAD" 后 exit 1（每次都失败）
func buildFakeCap(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fakecapture")
	if filepath.Separator == '\\' {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakecapture")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译 fakecapture 失败: %v\n%s", err, out)
	}
	return bin
}

func readAllTimeout(t *testing.T, r io.Reader, dur time.Duration) []byte {
	t.Helper()
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		tmp := make([]byte, 256)
		for {
			n, err := r.Read(tmp)
			buf.Write(tmp[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done: // EOF，正常返回
	case <-time.After(dur): // 超时：返回已读到的数据（流仍在继续）
	}
	return buf.Bytes()
}

func TestCaptureRunsAndStops(t *testing.T) {
	cap, err := Start(buildFakeCap(t), nil)
	if err != nil {
		t.Fatalf("Start 报错: %v", err)
	}
	// 读到至少一段数据
	r := cap.Stdout()
	deadline := time.Now().Add(3 * time.Second)
	n := 0
	for time.Now().Before(deadline) {
		tmp := make([]byte, 8)
		m, err := r.Read(tmp)
		n += m
		if err != nil {
			break
		}
		if n >= 4 {
			break
		}
	}
	if n < 4 {
		t.Fatalf("未读到足够数据: %d", n)
	}
	if err := cap.Stop(); err != nil {
		t.Errorf("Stop 报错: %v", err)
	}
}

func TestCaptureCrashRestartOnce(t *testing.T) {
	// fake: FAKE_STREAM=crash —— 第一次运行写 5 次 "BAD" 后 exit 1，
	// 并把状态文件留作"已崩溃过"的标记；capture 重启后 fake 读到状态文件转为 ok 模式。
	// 状态文件路径作为第一个参数传入 fake。
	fake := buildFakeCap(t)
	stateFile := filepath.Join(t.TempDir(), "state")
	os.Setenv("FAKE_STREAM", "crash")
	defer os.Unsetenv("FAKE_STREAM")
	cap, err := Start(fake, []string{stateFile})
	if err != nil {
		t.Fatalf("Start 报错: %v", err)
	}
	data := readAllTimeout(t, cap.Stdout(), 8*time.Second)
	if len(data) < 10 {
		t.Fatalf("重启后应有持续输出（含崩溃前的 BAD），实际 %d 字节", len(data))
	}
	if !strings.Contains(string(data), "BAD") || !strings.Contains(string(data), "LIVE") {
		t.Errorf("输出应先 BAD（首次崩溃）后 LIVE（重启成功），实际: %q", string(data[:min(len(data), 64)]))
	}
	cap.Stop()
	select {
	case <-cap.Done():
		t.Errorf("重启成功后 Done 不应关闭")
	default:
	}
}

func TestCaptureCrashTwiceGivesEOF(t *testing.T) {
	// fake: die 模式（每次都 exit 1），重启 1 次后放弃 → EOF + Done 关闭
	fake := buildFakeCap(t)
	os.Setenv("FAKE_STREAM", "die")
	defer os.Unsetenv("FAKE_STREAM")
	cap, err := Start(fake, nil)
	if err != nil {
		t.Fatalf("Start 报错: %v", err)
	}
	data := readAllTimeout(t, cap.Stdout(), 8*time.Second)
	if len(data) == 0 {
		t.Error("崩溃前应有输出")
	}
	select {
	case <-cap.Done():
	default:
		t.Error("重启耗尽后 Done 应关闭")
	}
	if cap.Err() == nil {
		t.Error("Err() 应返回失败原因")
	}
}
