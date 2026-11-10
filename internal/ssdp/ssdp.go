// internal/ssdp/ssdp.go
package ssdp

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const ssdpAddr = "239.255.255.250:1900"

// Discover 发送 M-SEARCH 并收集 MediaRenderer 设备描述 URL。
func Discover(timeout time.Duration) ([]string, error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("ssdp listen: %w", err)
	}
	defer conn.Close()

	dst, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}

	msg := strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: 239.255.255.250:1900",
		`MAN: "ssdp:discover"`,
		"MX: 2",
		"ST: urn:schemas-upnp-org:device:MediaRenderer:1",
		"", "",
	}, "\r\n")

	for i := 0; i < 3; i++ {
		if _, err := conn.WriteTo([]byte(msg), dst); err != nil {
			return nil, fmt.Errorf("ssdp send: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	seen := map[string]bool{}
	var locations []string
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	conn.SetReadDeadline(deadline)
	for time.Now().Before(deadline) {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		loc := parseLocation(string(buf[:n]))
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		locations = append(locations, loc)
	}
	return locations, nil
}

func parseLocation(resp string) string {
	for _, line := range strings.Split(resp, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "location:") {
			return strings.TrimSpace(line[len("location:"):])
		}
	}
	return ""
}
