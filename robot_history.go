// 执行历史的读写。
//
// 历史存在 SQLite（见 robot_store.go）而不是每个批次一个目录：
// 目录方案每次请求都要遍历全部批次并逐个读文件，条目越多越慢，且没有清理机制。
// 现在列表查询走索引，历史量增长不影响响应。

package main

import (
	"log"

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
	// PassCount / FailCount / Incomplete 让列表不必展开就能看出分布。
	// Incomplete 是「未取得结果」的条数，与失败分开统计。
	PassCount  int `json:"passCount"`
	FailCount  int `json:"failCount"`
	Incomplete int `json:"incompleteCount"`
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

// loadHistoryPage 返回第 page 页历史与总条数，数据来自 SQLite。
func (r *robotRunner) loadHistoryPage(page, pageSize int) ([]historyEntry, int) {
	if r.store == nil {
		return nil, 0
	}
	items, total, err := r.store.loadPage(page, pageSize, true)
	if err != nil {
		log.Printf("读取运行记录失败: %v", err)
		return nil, 0
	}
	return items, total
}

// loadHistory 返回最近 limit 条历史，limit<=0 表示不限。
// 仅用于补跑判断这类需要遍历的场景。
func (r *robotRunner) loadHistory(limit int) []historyEntry {
	if r.store == nil {
		return nil
	}
	size := limit
	if size <= 0 {
		size = 10000
	}
	items, _, err := r.store.loadPage(1, size, false)
	if err != nil {
		log.Printf("读取运行记录失败: %v", err)
		return nil
	}
	return items
}
