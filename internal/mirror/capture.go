// internal/mirror/capture.go
package mirror

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// Capture 管理采集子进程：stdout 经 io.Pipe 桥接给消费者；
// 进程意外退出时自动重启 1 次，重启成功流继续，再失败则关闭管道（EOF）。
// Stop 终止进程并关闭管道，但 Done 不会关闭（仅采集永久失败时关闭）。
type Capture struct {
	ff   string
	args []string

	mu        sync.Mutex
	cmd       *exec.Cmd
	pipeW     *io.PipeWriter
	pipeR     *io.PipeReader
	restarted bool
	stopped   bool
	done      chan struct{}
	err       error

	closeDone sync.Once // 保证 done 只被关闭一次
}

func Start(ff string, args []string) (*Capture, error) {
	c := &Capture{ff: ff, args: args, done: make(chan struct{})}
	c.pipeR, c.pipeW = io.Pipe()
	if err := c.launch(); err != nil {
		c.mu.Lock()
		c.err = fmt.Errorf("启动 ffmpeg 失败: %w", err)
		c.mu.Unlock()
		c.pipeW.Close()
		c.pipeR.Close()
		close(c.done)
		return nil, err
	}
	go c.watch()
	return c, nil
}

func (c *Capture) Stdout() io.Reader { return c.pipeR }

func (c *Capture) Done() <-chan struct{} { return c.done }

func (c *Capture) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// launch 启动子进程并把 stdout 拷贝到管道。
func (c *Capture) launch() error {
	cmd := exec.Command(c.ff, c.args...)
	cmd.Stdout = c.pipeW
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}
	c.mu.Lock()
	c.cmd = cmd
	c.mu.Unlock()
	return nil
}

// watch 等待进程退出：Stop 视为正常；意外退出则重启 1 次。
func (c *Capture) watch() {
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()
	waitErr := cmd.Wait()

	c.mu.Lock()
	stopped := c.stopped
	c.mu.Unlock()
	if stopped {
		return // Stop() 负责收尾（kill + 关管道），Done 保持开启
	}

	// 意外退出：重启 1 次
	c.mu.Lock()
	if c.restarted {
		c.err = fmt.Errorf("ffmpeg 再次退出: %v", waitErr)
		c.restarted = false
		c.mu.Unlock()
		c.finish()
		return
	}
	c.restarted = true
	c.mu.Unlock()

	if lerr := c.launch(); lerr != nil {
		c.mu.Lock()
		c.err = fmt.Errorf("ffmpeg 崩溃且重启失败: %v（原错误: %v）", lerr, waitErr)
		c.mu.Unlock()
		c.finish()
		return
	}

	// launch 期间可能发生 Stop（stopped 检查在 launch 之前的窗口内）。
	// 此时需终止刚启动的新进程，避免进程泄漏；done/pipe 由 Stop 关闭。
	c.mu.Lock()
	stopped = c.stopped
	newCmd := c.cmd
	c.mu.Unlock()
	if stopped {
		if newCmd != nil && newCmd.Process != nil {
			newCmd.Process.Kill()
		}
		newCmd.Wait() // 回收资源；stdout 拷贝到已关闭的 pipeW 会立即返回
		return
	}

	go c.watch()
}

func (c *Capture) finish() {
	c.pipeW.Close() // 读端得到 EOF
	c.closeDone.Do(func() { close(c.done) })
}

// Stop 终止子进程并关闭管道。Done 不关闭，Err() 保持 nil。
// 幂等：重复调用或采集已永久失败（finish 后）也返回 nil。
func (c *Capture) Stop() error {
	c.mu.Lock()
	c.stopped = true
	cmd := c.cmd
	c.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
	}
	if err := c.pipeW.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, os.ErrClosed) {
		return err
	}
	return nil
}
