package server

import (
	"regexp"
	"strings"
)

var cueTiming = regexp.MustCompile(`(\d{2}:\d{2}:\d{2})[,.](\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2})[,.](\d{3})`)

// SRTToVTT 把 SRT 字幕全文转换为 WebVTT。
func SRTToVTT(srt string) (string, bool) {
	srt = strings.ReplaceAll(srt, "\r\n", "\n")
	blocks := strings.Split(srt, "\n\n")
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	found := false
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		m := cueTiming.FindStringSubmatch(block)
		if m == nil {
			continue
		}
		found = true
		timing := m[1] + "." + m[2] + " --> " + m[3] + "." + m[4]
		lines := strings.Split(block, "\n")
		start := 0
		for i, line := range lines {
			if strings.Contains(line, "-->") {
				start = i + 1
				break
			}
		}
		b.WriteString(timing + "\n")
		b.WriteString(strings.Join(lines[start:], "\n") + "\n\n")
	}
	if !found {
		return "", false
	}
	return b.String(), true
}
