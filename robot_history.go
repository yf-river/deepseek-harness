// 执行历史：把每次批次的 final-state.json 汇总成列表。
//
// 为什么读目录而不是另建数据库：每个批次本来就把 final-state.json 落在自己的目录里，
// 目录本身就是一份不可变的历史记录。再存一份索引会出现两份真源不一致的问题。

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"chainweaver-studio/robot"
)

// historyEntry 是列表里的一行。
type historyEntry struct {
	// Dir 是批次目录名，同时充当唯一键。
	Dir       string             `json:"dir"`
	Trigger   string             `json:"trigger"`
	Verdict   string             `json:"verdict"`
	StartedAt string             `json:"startedAt"`
	EndedAt   string             `json:"endedAt"`
	Subject   string             `json:"subject"`
	MailState string             `json:"mailStatus"`
	// PassCount / FailCount 让列表不必展开就能看出好坏比例。
	PassCount int `json:"passCount"`
	FailCount int `json:"failCount"`
	// Suites 是逐套件的简要结果，用于列表里的一行摘要。
	Suites []historySuite `json:"suites"`
	// Detail 是完整结果，只有展开某条时才需要。
	Detail []robot.RunSummary `json:"detail,omitempty"`
	// LastError 记录执行器自身的异常（如未开始、在途占用）。
	LastError string `json:"lastError"`
}

type historySuite struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	DurationText string `json:"durationText"`
	Validated    int    `json:"validationCount"`
	DetailURL    string `json:"detailUrl"`
	Error        string `json:"error"`
	Failure      string `json:"failure"`
}

// loadHistory 从产物目录读取最近 limit 条批次，按时间倒序。
// 单个批次文件损坏时跳过该条，不影响其余历史。
func (r *robotRunner) loadHistory(limit int) []historyEntry {
	entries, err := os.ReadDir(r.config.ArtifactDir)
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "cwt-robot-") {
			continue
		}
		dirs = append(dirs, entry.Name())
	}
	// 目录名带 UTC 时间戳前缀，字典序即时间序；倒序取最近若干条。
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	if limit > 0 && len(dirs) > limit {
		dirs = dirs[:limit]
	}

	history := make([]historyEntry, 0, len(dirs))
	for _, dir := range dirs {
		path := filepath.Join(r.config.ArtifactDir, dir, "final-state.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var state robotState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}
		// 老批次缺 durationText 时补出来，保证列表里耗时列不为空。
		for index := range state.Results {
			if state.Results[index].DurationText == "" && state.Results[index].Duration != "" {
				state.Results[index].DurationText = robot.FormatDuration(state.Results[index].Duration)
			}
		}
		item := historyEntry{
			Dir:       dir,
			Trigger:   state.Trigger,
			Verdict:   state.Verdict,
			StartedAt: state.StartedAt,
			EndedAt:   state.EndedAt,
			Subject:   state.Subject,
			MailState: state.MailStatus,
			Detail:    state.Results,
			LastError: state.LastError,
		}
		for _, result := range state.Results {
			if result.OK() {
				item.PassCount++
			} else {
				item.FailCount++
			}
			item.Suites = append(item.Suites, historySuite{
				ID:           result.ID,
				Status:       result.Status,
				DurationText: result.DurationText,
				Validated:    result.Validated,
				DetailURL:    result.DetailURL,
				Error:        result.Error,
				Failure:      result.Failure,
			})
		}
		history = append(history, item)
	}
	return history
}
