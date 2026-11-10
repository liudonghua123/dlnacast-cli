// fakeffmpeg 是测试用的假 ffmpeg。
// 环境变量控制行为（由测试进程继承）：
//
//	FAKE_OK=<name>   命令行含 "-c:v <name>" 时 exit 0，否则 exit 1
//	FAKE_OK=none     一律 exit 1
//	未设置            含 "-c:v libx264" 时 exit 0，否则 exit 1
package main

import (
	"os"
	"strings"
)

func main() {
	ok := "libx264"
	if v := os.Getenv("FAKE_OK"); v != "" {
		ok = v
	}
	if ok == "none" {
		os.Exit(1)
	}
	if strings.Contains(strings.Join(os.Args[1:], " "), "-c:v "+ok) {
		os.Exit(0)
	}
	os.Exit(1)
}
