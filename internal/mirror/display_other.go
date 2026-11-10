//go:build !windows

package mirror

import "fmt"

// displayOrigin 非 Windows 平台只支持主屏。
func displayOrigin(n int) (int, int, error) {
	if n != 0 {
		return 0, 0, fmt.Errorf("当前平台仅支持 -display 0")
	}
	return 0, 0, nil
}

func displaySize(n int) (int, int, error) {
	return 0, 0, fmt.Errorf("当前平台需手动指定 -region")
}

// displayWorkArea 非 Windows 平台暂不支持工作区查询。
func displayWorkArea(n int) (x, y, w, h int, err error) {
	return 0, 0, 0, 0, fmt.Errorf("当前平台需手动指定 -region")
}
