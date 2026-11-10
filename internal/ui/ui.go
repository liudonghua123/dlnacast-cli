// internal/ui/ui.go
package ui

import (
	"fmt"
	"io"
	"os"

	"dlnacast-cli/internal/dlna"

	"golang.org/x/term"
)

type Key int

const (
	KeySpace Key = iota
	KeyQuit
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyS
	KeyN
	KeyP
	KeyOther
)

// Keys 进入 raw mode 读键。cancel 恢复终端状态。
func Keys() (<-chan Key, func(), error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, nil, fmt.Errorf("无法进入终端原始模式（需要真实终端）: %w", err)
	}

	ch := make(chan Key)
	done := make(chan struct{})
	go func() {
		defer close(ch)
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			select {
			case <-done:
				return
			case ch <- parseKey(buf, os.Stdin):
			}
		}
	}()

	cancel := func() {
		close(done)
		term.Restore(fd, oldState)
	}
	return ch, cancel, nil
}

// parseKey 解析按键字节。ANSI 方向键：ESC [ A/B/C/D。
func parseKey(buf []byte, in *os.File) Key {
	switch buf[0] {
	case 0x20:
		return KeySpace
	case 0x03, 'q':
		return KeyQuit
	case 's':
		return KeyS
	case 'n':
		return KeyN
	case 'p':
		return KeyP
	case 0x1B: // ESC 序列
		next := make([]byte, 2)
		if n, _ := io.ReadFull(in, next); n != 2 {
			return KeyOther
		}
		if next[0] != '[' {
			return KeyOther
		}
		switch next[1] {
		case 'A':
			return KeyUp
		case 'B':
			return KeyDown
		case 'C':
			return KeyRight
		case 'D':
			return KeyLeft
		}
		return KeyOther
	default:
		return KeyOther
	}
}

// ChooseDevice 交互式选择设备。非 TTY 环境直接返回第一个。
func ChooseDevice(devices []*dlna.Device) *dlna.Device {
	if len(devices) == 0 {
		return nil
	}
	if len(devices) == 1 {
		return devices[0]
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return devices[0] // 非 TTY fallback
	}
	defer term.Restore(fd, oldState)

	sel := 0
	render := func() {
		fmt.Print("\r\x1b[K选择设备（↑↓ 移动，回车确认）:\r\n\x1b[K")
		for i, d := range devices {
			cursor := "  "
			if i == sel {
				cursor = "> "
			}
			fmt.Printf("\r\x1b[K%s%s (%s)\r\n", cursor, d.Name, d.Host)
		}
		fmt.Printf("\x1b[%dA\r", len(devices)+1) // 光标回到行首
	}
	render()

	buf := make([]byte, 1)
	for {
		if _, err := os.Stdin.Read(buf); err != nil {
			return devices[sel]
		}
		switch buf[0] {
		case 0x0D, 0x0A: // Enter
			fmt.Print("\r\x1b[K\x1b[J") // 清除选择列表
			return devices[sel]
		case 0x03: // Ctrl+C
			fmt.Print("\r\x1b[K\x1b[J")
			return nil
		case 0x1B:
			next := make([]byte, 2)
			if n, _ := io.ReadFull(os.Stdin, next); n == 2 && next[0] == '[' {
				switch next[1] {
				case 'A':
					if sel > 0 {
						sel--
					}
					render()
				case 'B':
					if sel < len(devices)-1 {
						sel++
					}
					render()
				}
			}
		}
	}
}

// RenderProgress 输出单行进度条。
func RenderProgress(w io.Writer, pos, total int, playing bool, vol int, title string) {
	icon := "▶"
	if !playing {
		icon = "⏸"
	}
	pct := 0
	if total > 0 {
		pct = pos * 100 / total
	}
	barW := 20
	filled := 0
	if total > 0 {
		filled = pos * barW / total
	}
	bar := ""
	for i := 0; i < barW; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	fmt.Fprintf(w, "\r\x1b[K%s %s / %s [%s] %d%% vol:%d  %s",
		icon, hhmmss(pos), hhmmss(total), bar, pct, vol, title)
}

func hhmmss(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
