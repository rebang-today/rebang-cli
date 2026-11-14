package tui

import (
	"io"
	"os"
	"sync"
)

// 鼠标指针形状（OSC 22）。kitty、Ghostty、foot 等终端支持，其他终端会忽略。
// 恢复时发送空值，表示回到终端自己的默认指针（通常是文本 I 形光标），
// 而不是指定 default/text，以免覆盖用户在终端里的设置。
const (
	pointerHand  = "\x1b]22;pointer\x1b\\"
	pointerReset = "\x1b]22;\x1b\\"
)

// lockedFile 包装标准输出：Bubble Tea 的渲染器每帧一次 Write，
// 指针序列与帧写入互斥，保证控制序列不会插进一帧的中间。
// 嵌入 *os.File 以保留 Fd，Bubble Tea 依赖它识别终端与窗口大小。
type lockedFile struct {
	*os.File
	mu sync.Mutex
}

func (f *lockedFile) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.File.Write(p)
}

// setPointer 在指针状态变化时写出控制序列；没有配置输出时什么都不做。
func (m *model) setPointer(on bool) {
	if m.opt.Pointer == nil || on == m.pointerOn {
		return
	}
	m.pointerOn = on
	seq := pointerReset
	if on {
		seq = pointerHand
	}
	_, _ = io.WriteString(m.opt.Pointer, seq)
}
