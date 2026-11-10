// internal/dlna/device.go
package dlna

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

type Device struct {
	Name                string
	Host                string
	ControlURL          string
	RenderingControlURL string
}

type deviceDesc struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		ServiceList  []struct {
			ServiceType string `xml:"serviceType"`
			ControlURL  string `xml:"controlURL"`
		} `xml:"serviceList>service"`
	} `xml:"device"`
}

// Describe 拉取并解析设备描述 XML。
func Describe(locationURL string, c *Client) (*Device, error) {
	resp, err := c.HTTP.Get(locationURL)
	if err != nil {
		return nil, fmt.Errorf("获取设备描述失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var desc deviceDesc
	if err := xml.Unmarshal(body, &desc); err != nil {
		return nil, fmt.Errorf("解析设备描述失败: %w", err)
	}

	u, err := url.Parse(locationURL)
	if err != nil {
		return nil, err
	}
	base := u.Scheme + "://" + u.Host

	dev := &Device{Name: desc.Device.FriendlyName, Host: u.Hostname()}
	for _, svc := range desc.Device.ServiceList {
		switch {
		case strings.Contains(svc.ServiceType, "AVTransport"):
			dev.ControlURL = absURL(base, svc.ControlURL)
		case strings.Contains(svc.ServiceType, "RenderingControl"):
			dev.RenderingControlURL = absURL(base, svc.ControlURL)
		}
	}
	if dev.ControlURL == "" {
		return nil, errors.New("设备不支持 AVTransport，无法投屏")
	}
	return dev, nil
}

func absURL(base, path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
