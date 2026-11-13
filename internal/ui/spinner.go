package ui

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// SpinnerFrames 是转圈动画的帧。
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner 在终端的同一行刷新状态文字，只应在标准错误连接到终端时使用。
// Println 可以在动画上方插入普通行而不打乱画面。
type Spinner struct {
	out   io.Writer
	theme Theme
	text  func() string
	every time.Duration

	mu    sync.Mutex
	frame int
	stop  chan struct{}
	done  chan struct{}
}

// NewSpinner 创建动画。text 每帧调用一次，返回动画后面的文字。
func NewSpinner(out io.Writer, theme Theme, text func() string) *Spinner {
	return &Spinner{out: out, theme: theme, text: text, every: 100 * time.Millisecond}
}

// Frame 返回第 i 帧的完整一行（不含回车与清行控制符），便于测试。
func (s *Spinner) Frame(i int) string {
	return s.theme.Accent.Render(SpinnerFrames[i%len(SpinnerFrames)]) + " " + s.text()
}

// Start 开始动画。
func (s *Spinner) Start() {
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	s.draw()
	go func() {
		defer close(s.done)
		tk := time.NewTicker(s.every)
		defer tk.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-tk.C:
				s.draw()
			}
		}
	}()
}

func (s *Spinner) draw() {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "\r\x1b[2K%s", s.Frame(s.frame))
	s.frame++
}

// Println 清除当前动画行，输出一行文字，随后动画在下一行继续。
func (s *Spinner) Println(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "\r\x1b[2K%s\n", line)
}

// Stop 停止动画并清除动画行。可重复调用。
func (s *Spinner) Stop() {
	if s.stop == nil {
		return
	}
	select {
	case <-s.stop:
		return
	default:
	}
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprint(s.out, "\r\x1b[2K")
}

// Clock 把时长格式化为 m:ss。
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}
