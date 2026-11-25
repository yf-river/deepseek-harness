// 邮件汇总与投递。正文口径遵循 /data/ida/docs/tapd/20261102-cwt发送邮件 的指南：
// 只写本批次真实数据，时间统一按北京时间展示，失败原因只写到接口证据支持的层次。

package robot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// cst 是展示用时区。CWT 的 API 时间字段是 UTC RFC3339。
var cst = time.FixedZone("CST", 8*3600)

// Verdict 按全体流程的终态给批次定性。
//
// 三种结论必须分清，否则会误导：
//   - PASS    全部通过
//   - FAILED  确有主流程跑到终态并失败（CWT 侧判定为 failed）
//   - PARTIAL 有流程没拿到结果（提交超时、执行器异常、仍在中途），这不等于测试失败
//
// 关键区别：StatusError 是"我们没跑成"，不是"被测系统不合格"。
// 把它算成 FAILED 会让人以为产品有问题，实际可能是网络或调度问题。
func Verdict(results []RunSummary) string {
	if len(results) == 0 {
		return "PARTIAL"
	}
	anyIncomplete := false
	anyBusinessFail := false
	for _, item := range results {
		switch item.Status {
		case StatusPassed:
			// 通过
		case StatusFailed:
			anyBusinessFail = true
		default:
			// error / 未进入终态：执行器侧没拿到结果
			anyIncomplete = true
		}
	}
	switch {
	case anyIncomplete:
		return "PARTIAL"
	case anyBusinessFail:
		return "FAILED"
	default:
		return "PASS"
	}
}

// FormatDuration 把 CWT 给的 Go duration 字符串收敛成「5分22秒」这类好读的形式。
func FormatDuration(raw string) string {
	if raw == "" {
		return "-"
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return raw
	}
	parsed = parsed.Round(time.Second)
	minutes := int(parsed / time.Minute)
	seconds := int((parsed % time.Minute) / time.Second)
	if minutes == 0 {
		return fmt.Sprintf("%d秒", seconds)
	}
	if seconds == 0 {
		return fmt.Sprintf("%d分", minutes)
	}
	return fmt.Sprintf("%d分%d秒", minutes, seconds)
}

// formatTime 把 RFC3339（UTC）换算成北京时间，只到分钟。
func formatTime(raw string) string {
	if raw == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		if parsed, err = time.Parse(time.RFC3339, raw); err != nil {
			return raw
		}
	}
	return parsed.In(cst).Format("2006-01-02 15:04")
}

// displayName 返回主流程的展示名。name 表里没有时退回内置名称，最后才用原始 id，
// 避免邮件里出现「ida」这种小写机器标识。
func displayName(id string, names map[string]string) string {
	if name := names[id]; name != "" {
		return name
	}
	builtin := map[string]string{"ida": "IDA", "mira": "Mira", "dpm": "DPM"}
	if name := builtin[id]; name != "" {
		return name
	}
	return strings.ToUpper(id)
}

// statusLabel 把状态翻译成邮件里读得懂的中文。
func statusLabel(status string) string {
	switch status {
	case StatusPassed:
		return "通过"
	case StatusFailed:
		return "失败"
	case StatusError:
		return "未取得结果"
	case StatusRunning:
		return "执行中"
	case StatusQueued:
		return "排队中"
	}
	return status
}

// RenderBody 生成邮件正文（Markdown）。不含任何凭据、Token 或完整请求响应。
func RenderBody(results []RunSummary, names map[string]string, startedAt, endedAt time.Time) string {
	var out bytes.Buffer
	pass, failed, incomplete := 0, 0, 0
	for _, item := range results {
		switch item.Status {
		case StatusPassed:
			pass++
		case StatusFailed:
			failed++
		default:
			incomplete++
		}
	}

	// 汇总行按三类分开写：把「没跑成」和「跑了但失败」混成一个数字，
	// 收件人会误以为被测系统出了问题。
	fmt.Fprintf(&out, "本次主流程巡检完成：%d 条通过", pass)
	if failed > 0 {
		fmt.Fprintf(&out, "，%d 条失败", failed)
	}
	if incomplete > 0 {
		fmt.Fprintf(&out, "，%d 条未取得结果", incomplete)
	}
	out.WriteString("。\n\n")
	if incomplete > 0 {
		out.WriteString("> 未取得结果指提交或等待环节没跑完，不代表被测功能有问题。\n\n")
	}

	fmt.Fprintf(&out, "执行时间：%s—%s（北京时间）。\n\n",
		startedAt.In(cst).Format("2006-01-02 15:04"),
		endedAt.In(cst).Format("2006-01-02 15:04"))

	out.WriteString("| 主流程 | 结果 | 耗时 | 已通过验证数 | 详情 |\n")
	out.WriteString("|---|---|---|---:|---|\n")
	for _, item := range results {
		name := displayName(item.ID, names)
		detail := "-"
		if item.DetailURL != "" {
			detail = fmt.Sprintf("[查看报告](%s)", item.DetailURL)
		}
		fmt.Fprintf(&out, "| %s | %s | %s | %d | %s |\n",
			name, statusLabel(item.Status), FormatDuration(item.Duration), item.Validated, detail)
	}

	// 失败与未取得结果分成两段。前者是被测系统的结论，后者是执行链路的问题，
	// 排查方向完全不同，混在一起会浪费排查时间。
	var failures []string
	var incompleteLines []string
	for _, item := range results {
		name := displayName(item.ID, names)
		switch item.Status {
		case StatusFailed:
			if item.Failure != "" {
				failures = append(failures, fmt.Sprintf("- %s：%s", name, item.Failure))
			} else {
				failures = append(failures, fmt.Sprintf("- %s：未提供失败原因", name))
			}
		default:
			if item.Status == StatusPassed {
				continue
			}
			reason := item.Error
			if reason == "" {
				reason = fmt.Sprintf("未进入终态（当前状态 %s）", item.Status)
			}
			incompleteLines = append(incompleteLines, fmt.Sprintf("- %s：%s", name, reason))
		}
	}

	if len(failures) > 0 {
		out.WriteString("\n失败信息：\n")
		for _, line := range failures {
			out.WriteString("\n" + line + "\n")
		}
	}
	if len(incompleteLines) > 0 {
		out.WriteString("\n未取得结果：\n")
		for _, line := range incompleteLines {
			out.WriteString("\n" + line + "\n")
		}
	}
	return out.String()
}

// Subject 生成邮件主题。
func Subject(verdict string, when time.Time) string {
	return fmt.Sprintf("[CWT] 主流程巡检 %s - %s", verdict, when.In(cst).Format("2006-01-02"))
}

// Mailer 负责把正文投递出去。
type Mailer struct {
	// Bin 是 wecom-cli 的路径。
	Bin string
	// To 是收件邮箱。
	To []string
	// DryRun 为 true 时只落盘不真发，用于本地演练。
	DryRun bool
}

// Send 发送邮件并返回 CLI 的原始输出。只有 CLI 返回成功回执才算发出。
// SendText 把正文写成临时文件后发送，发完立即删除。
// wecom-cli 只接受 file_path，所以必须落一个临时文件；
// 放在系统临时目录而不是批次目录，避免为一次发信留下长期残留。
func (m Mailer) SendText(ctx context.Context, subject, body string) (string, error) {
	if m.DryRun {
		return "已跳过发送（演练模式）", nil
	}
	temp, err := os.CreateTemp("", "cwt-mail-*.md")
	if err != nil {
		return "", fmt.Errorf("创建临时正文失败: %w", err)
	}
	path := temp.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := temp.WriteString(body); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("写入临时正文失败: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("关闭临时正文失败: %w", err)
	}
	return m.Send(ctx, subject, path)
}

func (m Mailer) Send(ctx context.Context, subject, bodyPath string) (string, error) {
	if m.DryRun {
		return "已跳过发送（演练模式）", nil
	}
	request, err := json.Marshal(map[string]any{
		"to":           map[string]any{"emails": m.To},
		"subject":      subject,
		"file_path":    bodyPath,
		"content_type": "markdown",
	})
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, m.Bin, "mail", "send", "--json", string(request))
	output, err := command.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		// 把 CLI 的原始输出带进错误里：只报 "exit status 1" 无法排查是权限、
		// 参数还是网络问题。
		if text != "" {
			return text, fmt.Errorf("发信失败（%v）: %s", err, text)
		}
		return text, fmt.Errorf("发信失败: %w", err)
	}
	return text, nil
}

