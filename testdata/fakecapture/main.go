// fakecapture 是测试用的假采集进程。
// 第一个参数：状态文件路径（可选，仅 crash 模式使用）。行为：
//
//	环境变量 FAKE_STREAM=ok：每 20ms 向 stdout 写 "LIVE"，直到被 kill
//	FAKE_STREAM=crash：向 stdout 写 5 次 "BAD" 后 exit 1；
//	  若状态文件（第一个参数）不存在则先写入状态文件；
//	  重启后的进程读到状态文件则转为 ok 模式
//	FAKE_STREAM=die：无论参数如何，每次启动都写 5 次 "BAD" 后 exit 1
//	默认（无 FAKE_STREAM）：ok 模式
package main

import (
	"os"
	"time"
)

func main() {
	mode := os.Getenv("FAKE_STREAM")
	if mode == "" {
		mode = "ok"
	}
	stateFile := ""
	if len(os.Args) > 1 {
		stateFile = os.Args[1]
	}

	if mode == "crash" && stateFile != "" {
		if _, err := os.Stat(stateFile); err == nil {
			mode = "ok" // 重启的进程：转为 ok
		} else {
			os.WriteFile(stateFile, []byte("1"), 0o644)
		}
	}

	if mode == "crash" || mode == "die" {
		for i := 0; i < 5; i++ {
			os.Stdout.WriteString("BAD")
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(1)
	}

	for {
		os.Stdout.WriteString("LIVE")
		time.Sleep(20 * time.Millisecond)
	}
}
