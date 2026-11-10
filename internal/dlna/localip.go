// internal/dlna/localip.go
package dlna

import (
	"fmt"
	"net"
)

// LocalIPFor 选出与 deviceIP 通信时使用的本机 IP。
// 通过 UDP connect（不实际发包）让内核选择路由源地址，
// 天然处理多网卡（虚拟机、WSL）场景。
func LocalIPFor(deviceIP string) (string, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(deviceIP, "9"))
	if err != nil {
		return "", fmt.Errorf("无法确定到设备 %s 的本机出口 IP: %w", deviceIP, err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
