package mirror

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildFake 编译 testdata/fakeffmpeg 为可执行文件，作为假 ffmpeg。
func buildFake(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fakeffmpeg")
	if filepath.Separator == '\\' {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/fakeffmpeg")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译 fake ffmpeg 失败: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("未找到仓库根目录")
		}
		dir = parent
	}
}

func TestProbeEncoderFallsBackToX264(t *testing.T) {
	// 假 ffmpeg 对 nvenc/qsv/amf 返回失败（exit 1），对 libx264 成功（exit 0）
	enc, err := ProbeEncoderWithFallback(buildFake(t), []string{"h264_nvenc", "h264_qsv", "h264_amf", "libx264"})
	if err != nil {
		t.Fatalf("ProbeEncoderWithFallback 报错: %v", err)
	}
	if enc != "libx264" {
		t.Errorf("期望回落到 libx264，得到 %s", enc)
	}
}

func TestProbeEncoderFirstAvailable(t *testing.T) {
	// 未设 FAKE_OK 时假 ffmpeg 仅对 libx264 成功（默认行为），
	// "链上第一个可用项被选中"这一语义已由 TestProbeEncoderSelectsByEnv 覆盖
	// （FAKE_OK=qsv 时选中 qsv 而非 libx264），此处不再重复。
}

func TestProbeEncoderSelectsByEnv(t *testing.T) {
	// FAKE_OK=qsv：nvenc 失败，qsv 成功
	enc, err := probeEncoderWithEnv(buildFake(t), []string{"h264_nvenc", "h264_qsv", "h264_amf", "libx264"}, "h264_qsv")
	if err != nil {
		t.Fatal(err)
	}
	if enc != "h264_qsv" {
		t.Errorf("期望 h264_qsv，得到 %s", enc)
	}
}

func TestProbeEncoderAllFail(t *testing.T) {
	// FAKE_OK=none：全部失败 → 报错
	if _, err := probeEncoderWithEnv(buildFake(t), []string{"h264_nvenc", "h264_qsv"}, "none"); err == nil {
		t.Error("全部编码器失败时应报错")
	}
}

// probeEncoderWithEnv 测试辅助：设置 FAKE_OK 环境变量后运行探测。
// ProbeEncoderWithFallback 用 os.Environ() 构造子进程环境，
// 因此测试进程上设置的环境变量会被假 ffmpeg 继承。
func probeEncoderWithEnv(ff string, chain []string, fakeOK string) (string, error) {
	orig := os.Getenv("FAKE_OK")
	os.Setenv("FAKE_OK", fakeOK)
	defer os.Setenv("FAKE_OK", orig)
	return ProbeEncoderWithFallback(ff, chain)
}
