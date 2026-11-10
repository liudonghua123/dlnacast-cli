// internal/player/player.go
package player

import (
	"fmt"

	"dlnacast-cli/internal/dlna"
	"dlnacast-cli/internal/server"
)

// Session 管理一次投屏会话：设备 + 播放列表 + 本地服务。
type Session struct {
	dev       *dlna.Device
	transport *dlna.Transport
	srv       *server.Server // 仅当有本地文件时非 nil
	files     []*server.File
	localIP   string
	current   int
}

func NewSession(dev *dlna.Device, files []*server.File, port int) (*Session, error) {
	s := &Session{dev: dev, files: files, current: -1}

	hasLocal := false
	for _, f := range files {
		if f.URL == "" {
			hasLocal = true
			break
		}
	}

	if hasLocal {
		s.srv = server.NewServer(port)
		s.srv.SetFiles(files)
		if err := s.srv.Start(); err != nil {
			return nil, fmt.Errorf("启动本地服务失败: %w", err)
		}
	}

	ip, err := dlna.LocalIPFor(dev.Host)
	if err != nil {
		if s.srv != nil {
			s.srv.Close()
		}
		return nil, err
	}
	s.localIP = ip

	s.transport = dlna.NewTransport(dev, dlna.NewClient())
	return s, nil
}

func (s *Session) mediaURL(f *server.File) string {
	if s.srv == nil {
		return f.URL
	}
	return s.srv.MediaURL(s.localIP, f)
}

func (s *Session) subtitleURL(f *server.File) string {
	if s.srv == nil {
		return ""
	}
	return s.srv.SubtitleURL(s.localIP, f)
}

// Load 投放第 i 个媒体，seekSec>0 时跳转到该位置。
func (s *Session) Load(i, seekSec int) error {
	if i < 0 || i >= len(s.files) {
		return fmt.Errorf("播放索引越界: %d（共 %d 个）", i, len(s.files))
	}
	s.current = i
	f := s.files[i]
	url := s.mediaURL(f)
	if url == "" {
		return fmt.Errorf("无法生成媒体地址")
	}
	if err := s.transport.Load(url, f.Title, f.Type, s.subtitleURL(f)); err != nil {
		return err
	}
	if seekSec > 0 {
		return s.transport.Seek(seekSec)
	}
	return nil
}

func (s *Session) Play() error  { return s.transport.Play() }
func (s *Session) Pause() error { return s.transport.Pause() }
func (s *Session) Stop() error  { return s.transport.Stop() }

// Seek 相对当前位置跳转。
func (s *Session) Seek(deltaSec int) error {
	pos, _, _, err := s.Status()
	if err != nil {
		return err
	}
	target := pos + deltaSec
	if target < 0 {
		target = 0
	}
	return s.transport.Seek(target)
}

// TogglePause 按当前状态暂停或继续。
func (s *Session) TogglePause() error {
	_, _, playing, err := s.Status()
	if err != nil {
		return err
	}
	if playing {
		return s.transport.Pause()
	}
	return s.transport.Play()
}

func (s *Session) SetVolume(level int) error { return s.transport.SetVolume(level) }

func (s *Session) Status() (pos, total int, playing bool, err error) {
	return s.transport.Status()
}

func (s *Session) Volume() (int, error) { return s.transport.Volume() }

func (s *Session) Index() int { return s.current }
func (s *Session) Count() int { return len(s.files) }

// Close 停止投屏并关闭本地服务。
func (s *Session) Close() error {
	if s.transport != nil {
		s.transport.Stop() // 错误不阻塞退出
	}
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}
