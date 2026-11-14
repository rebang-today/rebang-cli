package cli

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/rebang-today/rebang-cli/internal/api"
	"github.com/rebang-today/rebang-cli/internal/errs"
	"github.com/rebang-today/rebang-cli/internal/output"
	"github.com/rebang-today/rebang-cli/internal/summary"
	"github.com/rebang-today/rebang-cli/internal/ui"
)

// SummaryResult 是 summary 命令的 JSON 输出：一句话总结与服务端原样返回的 detail。
type SummaryResult struct {
	SummaryShort string          `json:"summary_short"`
	Detail       json.RawMessage `json:"detail"`
}

// summaryMaxWidth 是 table 格式下正文的最大宽度，过宽的终端里长行不便阅读。
const summaryMaxWidth = 100

func newSummaryCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "summary <item_key>",
		Short: "查看条目的 AI 总结（需登录）",
		Long: "查看条目的 AI 总结：一句话总结，以及要点、经过、观点、网友怎么看等分区。\n\n" +
			"item_key 来自 rebang list 或 rebang search 的 JSON 输出（-o json）。不是所有条目都有总结，没有时退出码为 8。",
		Example: "  rebang list --node <节点ID> -o json   # 取 item_key\n  rebang summary <item_key>\n  rebang summary <item_key> -o json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := strings.TrimSpace(args[0])
			if utf8.RuneCountInString(key) != 32 {
				return errs.New(errs.ExitUsage, errs.CodeInvalidArgument, "item_key 应为 32 位", "从 rebang list -o json 或 rebang search -o json 的 item_key 字段获取")
			}
			if err := requireLogin(); err != nil {
				return err
			}
			in, err := a.Client().Insight(cmd.Context(), key)
			if api.IsNotFound(err) {
				return errs.New(errs.ExitNoSummary, errs.CodeSummaryNotFound, "该条目暂无 AI 总结", "不是所有条目都有总结，可以换一条再试")
			}
			if err != nil {
				return api.MapError(err)
			}
			detail := in.Detail
			if len(detail) == 0 {
				detail = json.RawMessage("null")
			}
			sections := summary.Sections(in.ParseDetail())
			short := ui.CleanText(in.SummaryShort)
			return a.Writer().Write(output.Result{
				Data:     SummaryResult{SummaryShort: in.SummaryShort, Detail: detail},
				Markdown: func() string { return summaryMarkdown(short, sections) },
				Pretty: func(t ui.Theme, width int) string {
					return summaryPretty(t, width, short, sections)
				},
			})
		},
	}
}

func summaryPretty(t ui.Theme, width int, short string, sections []summary.Line) string {
	if width <= 0 || width > summaryMaxWidth {
		width = summaryMaxWidth
	}
	st := summary.Style{
		Heading: func(s string) string { return t.Accent.Render(s) },
		Label:   func(s string) string { return t.Strong.Render(s) },
		Bold:    func(s string) string { return t.Strong.Render(s) },
		Quote:   func(s string) string { return t.Dim.Render(s) },
		Dim:     func(s string) string { return t.Dim.Render(s) },
	}
	lines := []summary.Line{{Kind: summary.Heading, Text: "AI 总结"}, {Kind: summary.Para, Text: short}}
	if len(sections) > 0 {
		lines = append(lines, summary.Line{Kind: summary.Blank})
		lines = append(lines, sections...)
	}
	return strings.Join(summary.Render(lines, width, st), "\n") + "\n"
}

func summaryMarkdown(short string, sections []summary.Line) string {
	var b strings.Builder
	b.WriteString("## AI 总结\n\n" + short + "\n")
	for _, l := range sections {
		ind := strings.Repeat(" ", l.Indent)
		label := l.Label
		if label != "" && l.Text != "" && !strings.HasSuffix(label, "：") {
			label += " "
		}
		switch l.Kind {
		case summary.Blank:
		case summary.Heading:
			b.WriteString("\n### " + l.Text + "\n\n")
		case summary.Bullet:
			b.WriteString(ind + "- " + label + l.Text + "\n")
		case summary.Quote:
			b.WriteString(ind + "> “" + l.Text + "”\n")
		default:
			b.WriteString(ind + label + l.Text + "\n")
		}
	}
	return b.String()
}
