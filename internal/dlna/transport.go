// internal/dlna/transport.go
package dlna

import (
	"fmt"
	"strconv"
)

type Transport struct {
	dev *Device
	c   *Client
}

func NewTransport(d *Device, c *Client) *Transport {
	return &Transport{dev: d, c: c}
}

func (t *Transport) Device() *Device { return t.dev }

func (t *Transport) call(action string, args map[string]string) (map[string]string, error) {
	return t.c.Call(t.dev.ControlURL, "AVTransport", action, args)
}

func (t *Transport) rcCall(action string, args map[string]string) (map[string]string, error) {
	return t.c.Call(t.dev.RenderingControlURL, "RenderingControl", action, args)
}

const didlTemplate = `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="0" parentID="0" restricted="1"><dc:title>%s</dc:title><upnp:class>%s</upnp:class><res>%s</res>%s</item></DIDL-Lite>`

func upnpClass(mediaType string) string {
	switch mediaType {
	case "audio":
		return "object.item.audioItem"
	case "image":
		return "object.item.imageItem"
	default:
		return "object.item.videoItem"
	}
}

// Load 投放媒体并开始播放。
func (t *Transport) Load(mediaURL, title, mediaType, subtitleURL string) error {
	extraRes := ""
	if subtitleURL != "" {
		extraRes = fmt.Sprintf(`<res protocolInfo="http-get:*:text/vtt:*">%s</res>`, xmlEscape(subtitleURL))
	}
	didl := fmt.Sprintf(didlTemplate, xmlEscape(title), upnpClass(mediaType), xmlEscape(mediaURL), extraRes)

	_, err := t.call("SetAVTransportURI", map[string]string{
		"InstanceID":         "0",
		"CurrentURI":         mediaURL,
		"CurrentURIMetaData": xmlEscape(didl),
	})
	if err != nil {
		return fmt.Errorf("设置播放地址失败: %w", err)
	}
	return t.Play()
}

func (t *Transport) Play() error {
	_, err := t.call("Play", map[string]string{"InstanceID": "0", "Speed": "1"})
	return err
}

func (t *Transport) Pause() error {
	_, err := t.call("Pause", map[string]string{"InstanceID": "0"})
	return err
}

func (t *Transport) Stop() error {
	_, err := t.call("Stop", map[string]string{"InstanceID": "0"})
	return err
}

func (t *Transport) Seek(sec int) error {
	_, err := t.call("Seek", map[string]string{
		"InstanceID": "0", "Unit": "REL_TIME", "Target": FormatDuration(sec),
	})
	return err
}

// Status 返回播放位置/总时长(秒)/是否播放中。
func (t *Transport) Status() (pos, total int, playing bool, err error) {
	res, err := t.call("GetPositionInfo", map[string]string{"InstanceID": "0"})
	if err != nil {
		return 0, 0, false, err
	}
	pos = ParseUPnPTime(res["RelTime"])
	total = ParseUPnPTime(res["TrackDuration"])
	if pos == 0 { // 部分设备 RelTime 为 NOT_IMPLEMENTED，退回 AbsTime
		pos = ParseUPnPTime(res["AbsTime"])
	}

	tinfo, err := t.call("GetTransportInfo", map[string]string{"InstanceID": "0"})
	if err != nil {
		return pos, total, false, nil // 传输状态查询失败不阻塞
	}
	playing = tinfo["CurrentTransportState"] == "PLAYING"
	return pos, total, playing, nil
}

// Volume 返回 0-100 音量。
func (t *Transport) Volume() (int, error) {
	res, err := t.rcCall("GetVolume", map[string]string{"InstanceID": "0", "Channel": "Master"})
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(res["CurrentVolume"])
}

// SetVolume 设置音量（0-100，自动夹取）。
func (t *Transport) SetVolume(level int) error {
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	_, err := t.rcCall("SetVolume", map[string]string{
		"InstanceID": "0", "Channel": "Master", "DesiredVolume": strconv.Itoa(level),
	})
	return err
}
