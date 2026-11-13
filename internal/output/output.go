// Package output 负责 json、md、table 三种输出格式（D-31）与错误输出（D-32）。
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/rebang-today/rebang-cli/internal/errs"
)

// Format 输出格式。
type Format string

const (
	JSON  Format = "json"
	MD    Format = "md"
	Table Format = "table"
)

// Resolve 计算最终输出格式：命令行参数 > 配置文件 > 自动（终端为 table，否则 json）。
func Resolve(flag, configured string, isTTY bool) (Format, error) {
	for _, v := range []string{flag, configured} {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "":
			continue
		case "json":
			return JSON, nil
		case "md", "markdown":
			return MD, nil
		case "table":
			return Table, nil
		default:
			return "", errs.Usage("不支持的输出格式 %q，可选 json、md、table", v)
		}
	}
	if isTTY {
		return Table, nil
	}
	return JSON, nil
}

// IsTerminal 判断文件是否连接到终端。
func IsTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// TerminalWidth 返回终端宽度，无法获取时返回 0。
func TerminalWidth(f *os.File) int {
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// Column 描述表格的一列。
type Column struct {
	Title string
	// Max 是 table 格式下的最大显示宽度，0 表示不限制。
	Max int
	// Shrink 表示终端过窄时优先压缩这一列。
	Shrink bool
}

// Result 是一次查询的输出：JSON 原样输出 Data，table 与 md 使用 Columns 和 Rows。
type Result struct {
	Data    any
	Columns []Column
	Rows    [][]string
	// Markdown 可选，自定义 md 输出；为空时输出 Markdown 表格。
	Markdown func() string
	// Footer 是 table 与 md 格式下附加在末尾的说明。
	Footer string
}

// Writer 绑定输出目标与格式。
type Writer struct {
	Out    io.Writer
	Format Format
	// Width 为终端宽度，0 表示不截断。
	Width int
}

// Write 按格式输出结果。
func (w Writer) Write(r Result) error {
	switch w.Format {
	case JSON:
		return WriteJSON(w.Out, r.Data)
	case MD:
		if r.Markdown != nil {
			_, err := io.WriteString(w.Out, strings.TrimRight(r.Markdown(), "\n")+"\n")
			return err
		}
		s := RenderMarkdownTable(r.Columns, r.Rows)
		if r.Footer != "" {
			s += "\n" + r.Footer + "\n"
		}
		_, err := io.WriteString(w.Out, s)
		return err
	default:
		s := RenderTable(r.Columns, r.Rows, w.Width)
		if r.Footer != "" {
			s += r.Footer + "\n"
		}
		_, err := io.WriteString(w.Out, s)
		return err
	}
}

// WriteJSON 输出带缩进的 JSON，不转义 HTML 字符。
func WriteJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// RenderTable 渲染对齐的文本表格，正确处理中文宽度。
func RenderTable(cols []Column, rows [][]string, termWidth int) string {
	if len(rows) == 0 {
		return "（无数据）\n"
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = runewidth.StringWidth(c.Title)
	}
	cells := make([][]string, len(rows))
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for i := range cols {
			v := ""
			if i < len(row) {
				v = singleLine(row[i])
			}
			if m := cols[i].Max; m > 0 && runewidth.StringWidth(v) > m {
				v = runewidth.Truncate(v, m, "…")
			}
			cells[r][i] = v
			if w := runewidth.StringWidth(v); w > widths[i] {
				widths[i] = w
			}
		}
	}
	// 终端过窄时压缩最宽的一列。
	if termWidth > 0 {
		total := 0
		for _, w := range widths {
			total += w + 2
		}
		if over := total - termWidth; over > 0 {
			widest := -1
			for i := range widths {
				if cols[i].Shrink && (widest < 0 || widths[i] > widths[widest]) {
					widest = i
				}
			}
			if widest < 0 {
				widest = 0
				for i := range widths {
					if widths[i] > widths[widest] {
						widest = i
					}
				}
			}
			if nw := widths[widest] - over; nw >= 10 {
				widths[widest] = nw
				for r := range cells {
					if runewidth.StringWidth(cells[r][widest]) > nw {
						cells[r][widest] = runewidth.Truncate(cells[r][widest], nw, "…")
					}
				}
			}
		}
	}
	var b strings.Builder
	line := func(vals []string) {
		for i, v := range vals {
			if i == len(vals)-1 {
				b.WriteString(v)
			} else {
				b.WriteString(runewidth.FillRight(v, widths[i]))
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.Title
	}
	line(titles)
	for _, row := range cells {
		line(row)
	}
	return b.String()
}

// RenderMarkdownTable 渲染 Markdown 表格。
func RenderMarkdownTable(cols []Column, rows [][]string) string {
	var b strings.Builder
	b.WriteString("|")
	for _, c := range cols {
		b.WriteString(" " + EscapeMD(c.Title) + " |")
	}
	b.WriteString("\n|")
	for range cols {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, row := range rows {
		b.WriteString("|")
		for i := range cols {
			v := ""
			if i < len(row) {
				v = row[i]
			}
			b.WriteString(" " + EscapeMD(singleLine(v)) + " |")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// EscapeMD 转义表格单元格中的竖线。
func EscapeMD(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// MDLink 生成 Markdown 链接。
func MDLink(text, url string) string {
	text = strings.NewReplacer("[", `\[`, "]", `\]`).Replace(singleLine(text))
	if url == "" {
		return text
	}
	return fmt.Sprintf("[%s](%s)", text, strings.ReplaceAll(url, ")", "%29"))
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}

// WriteError 输出结构化错误。asJSON 为 true 时输出 JSON（供 Agent 解析），否则输出可读文本。
func WriteError(out io.Writer, err error, asJSON bool) {
	e := errs.From(err)
	if asJSON {
		_ = WriteJSON(out, struct {
			Error *errs.Error `json:"error"`
		}{e})
		return
	}
	fmt.Fprintf(out, "错误：%s\n", e.Error())
	if e.Hint != "" {
		fmt.Fprintf(out, "提示：%s\n", e.Hint)
	}
}
