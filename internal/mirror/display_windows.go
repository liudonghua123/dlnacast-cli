//go:build windows

package mirror

import (
	"fmt"
	"syscall"
	"unsafe"
)

type rect struct {
	Left, Top, Right, Bottom int32
}

type monitorInfo struct {
	index int
	r     rect
	work  rect
}

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfo      = user32.NewProc("GetMonitorInfoW")
)

// 完整 MONITORINFOEXW 布局：cbSize=136（x64），含 szDevice[32]uint16。
type monitorInfoEx struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
	szDevice  [32]uint16
}

func listMonitors() ([]monitorInfo, error) {
	var monitors []monitorInfo
	cb := syscall.NewCallback(func(hm, hdc, prect, lparam uintptr) uintptr {
		mi := monitorInfoEx{cbSize: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		ret, _, _ := procGetMonitorInfo.Call(hm, uintptr(unsafe.Pointer(&mi)))
		if ret == 0 {
			return 1
		}
		monitors = append(monitors, monitorInfo{index: len(monitors), r: mi.rcMonitor, work: mi.rcWork})
		return 1
	})
	ret, _, err := procEnumDisplayMonitors.Call(0, 0, cb, 0)
	if ret == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors 失败: %v", err)
	}
	if len(monitors) == 0 {
		return nil, fmt.Errorf("未枚举到显示器")
	}
	return monitors, nil
}

// displayOrigin 返回第 n 个显示器的原点（n=0 主屏）。
func displayOrigin(n int) (int, int, error) {
	monitors, err := listMonitors()
	if err != nil {
		return 0, 0, err
	}
	if n < 0 || n >= len(monitors) {
		return 0, 0, fmt.Errorf("显示器编号 %d 超出范围（共 %d 个）", n, len(monitors))
	}
	return int(monitors[n].r.Left), int(monitors[n].r.Top), nil
}

// displaySize 返回第 n 个显示器的分辨率。
func displaySize(n int) (int, int, error) {
	monitors, err := listMonitors()
	if err != nil {
		return 0, 0, err
	}
	if n < 0 || n >= len(monitors) {
		return 0, 0, fmt.Errorf("显示器编号 %d 超出范围（共 %d 个）", n, len(monitors))
	}
	r := monitors[n].r
	return int(r.Right - r.Left), int(r.Bottom - r.Top), nil
}

// displayWorkArea 返回第 n 个显示器的工作区（不含任务栏）原点与尺寸。
func displayWorkArea(n int) (x, y, w, h int, err error) {
	monitors, err := listMonitors()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if n < 0 || n >= len(monitors) {
		return 0, 0, 0, 0, fmt.Errorf("显示器编号 %d 超出范围（共 %d 个）", n, len(monitors))
	}
	r := monitors[n].work
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top), nil
}
