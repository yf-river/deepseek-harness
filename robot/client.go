// Package robot 实现 CWT 每日主流程巡检机器人：串行跑完登记的主流程，汇总结果并发邮件。
//
// 设计取舍：
//   - 只负责跑完并如实汇总，不做通过/失败判定之外的干预。某个主流程失败不中断后续流程，
//     也不重试——CWT 侧的环境问题是它自己的事，机器人只负责把事实送到收件人手里。
//   - 串行提交：CWT 的环境是稀缺资源，并发会互相抢占租约。一次只提交一个 suite。
//   - 每个 suite 先把幂等键落盘再提交，进程重启或网络抖动时用同一个键恢复，不重复申请环境。
//   - 汇总只读本批次自己的 run_id，不从「最新几条记录」猜测归属。
package robot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Suite 是一个待执行的主流程。
type Suite struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// RunSummary 是单个主流程在本批次里的执行结果。
type RunSummary struct {
	Suites []string `yaml:"suites" json:"suites"`

	ID        string `json:"id"`
	Status    string `json:"status"`
	Stage     string `json:"stage"`
	StageDur  string `json:"stageDuration"`
	Duration  string `json:"duration"`
	// DurationText 是给人看的耗时，如「3分12秒」。原始值保留在 Duration。
	DurationText string `json:"durationText"`
	Validated int    `json:"validationCount"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
	Failure   string `json:"failure"`
	Error     string `json:"error"`

	// DetailURL 是 CWT 看板上该次运行的直达链接。
	DetailURL string `json:"detailUrl"`
}

// Terminal 判断这个主流程是否已经有结论（含执行器自身的失败）。
func (r RunSummary) Terminal() bool {
	switch r.Status {
	case StatusPassed, StatusFailed, StatusError:
		return true
	}
	return false
}

// OK 表示该流程真实跑通。
func (r RunSummary) OK() bool { return r.Status == StatusPassed }

// 运行状态。前三个来自 CWT，StatusError 是执行器侧没能拿到结果时自己标的。
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusError   = "error"
)

// Client 是 CWT 任务 API 的最小客户端。
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// PollInterval 是轮询间隔；RunTimeout 是单流程的等待上限，超过即判为执行器侧超时。
	PollInterval time.Duration
	RunTimeout   time.Duration
}

// NewClient 用合理默认值构造客户端。超时给足：Mira 历史上跑过 8 分钟以上。
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		PollInterval: 15 * time.Second,
		RunTimeout:   40 * time.Minute,
	}
}

// accepted 是 POST 的接单回执。
type accepted struct {
	RunID string `json:"run_id"`
}

// runDetail 是 GET 单个运行的精简视图，只取汇总需要的字段。
type runDetail struct {
	RunID           string `json:"run_id"`
	Status          string `json:"status"`
	Stage           string `json:"stage"`
	Version         string `json:"version"`
	CreatedAt       string `json:"created_at"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	TotalDuration   string `json:"total_duration"`
	ValidationCount int    `json:"validation_count"`
	Failure         *struct {
		Stage   string `json:"stage"`
		Action  string `json:"action"`
		Message string `json:"message"`
	} `json:"failure"`
}

// Submit 提交一个 suite。key 是该流程的幂等键，重复提交同键会返回已有任务。
// 返回 run_id 只代表接单，不代表执行结果。
func (c *Client) Submit(ctx context.Context, suite, key string) (string, error) {
	body, err := json.Marshal(map[string]string{"suite": suite})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/api/v1/test-runs", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<16))
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK &&
		response.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("接单失败 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var receipt accepted
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return "", fmt.Errorf("接单回执无法解析: %w", err)
	}
	if receipt.RunID == "" {
		return "", errors.New("接单回执缺少 run_id")
	}
	return receipt.RunID, nil
}

// Detail 查询单次运行。
func (c *Client) Detail(ctx context.Context, runID string) (runDetail, error) {
	var detail runDetail
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api/v1/test-runs/"+runID, nil)
	if err != nil {
		return detail, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return detail, err
	}
	defer func() { _ = response.Body.Close() }()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != http.StatusOK {
		return detail, fmt.Errorf("查询失败 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if err := json.Unmarshal(payload, &detail); err != nil {
		return detail, fmt.Errorf("响应无法解析: %w", err)
	}
	return detail, nil
}

// ActiveRuns 返回当前在途（queued/running）的运行数，用于开跑前的互斥检查。
// 查询失败时返回错误，调用方不得把「查不到」当成「没有在途」。
func (c *Client) ActiveRuns(ctx context.Context) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.BaseURL+"/api/v1/test-runs?limit=100", nil)
	if err != nil {
		return 0, err
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return 0, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("查询在途任务失败 HTTP %d", response.StatusCode)
	}
	var payload struct {
		Runs []struct {
			Status string `json:"status"`
		} `json:"runs"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return 0, err
	}
	active := 0
	for _, run := range payload.Runs {
		if run.Status == StatusQueued || run.Status == StatusRunning {
			active++
		}
	}
	return active, nil
}

// Batch 是一次巡检的产物目录与标识。每个 suite 的中间状态都落在目录里，
// 进程中断后可以据此恢复，不必重新申请环境。
type Batch struct {
	Dir string
	Key string
}

// NewBatch 在 root 下建立批次目录。
func NewBatch(root string) (*Batch, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(root, "cwt-robot-")
	if err != nil {
		return nil, err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return &Batch{Dir: dir, Key: fmt.Sprintf("cwt-robot-%s-%s", stamp, filepath.Base(dir))}, nil
}

// keyFor 给单个 suite 派生幂等键：同一批次内同 suite 始终同键。
func (b *Batch) keyFor(suite string) string { return b.Key + "-" + suite }

// Run 串行执行全部 suite，逐个返回结果。任何单个 suite 的失败都只记录，不中断后续流程。
func (c *Client) Run(ctx context.Context, batch *Batch, suites []Suite) []RunSummary {
	results := make([]RunSummary, 0, len(suites))
	for _, suite := range suites {
		results = append(results, c.runOne(ctx, batch, suite))
	}
	return results
}

// runOne 提交并等到终态。返回的 RunSummary 一定带着可判定的状态：
// CWT 侧失败标记为 failed，执行器侧拿不到结果标记为 error。
func (c *Client) runOne(ctx context.Context, batch *Batch, suite Suite) RunSummary {
	summary := RunSummary{ID: suite.ID}
	summary.Suites = []string{suite.ID}

	runID, err := c.Submit(ctx, suite.ID, batch.keyFor(suite.ID))
	if err != nil {
		summary.Status = StatusError
		summary.Error = "提交失败: " + err.Error()
		return summary
	}
	summary.DetailURL = fmt.Sprintf("%s/#run=%s", "http://192.168.40.247:18091", runID)
	// 立即记录 run_id：即使后续轮询失败，也知道本批次提交了哪个任务。
	_ = os.WriteFile(filepath.Join(batch.Dir, suite.ID+".run-id"), []byte(runID), 0o600)

	deadline := time.Now().Add(c.RunTimeout)
	for {
		detail, err := c.Detail(ctx, runID)
		if err != nil {
			summary.Status = StatusError
			summary.Error = "查询失败: " + err.Error()
			return summary
		}
		applyDetail(&summary, detail)
		if summary.Terminal() {
			return summary
		}
		if time.Now().After(deadline) {
			summary.Status = StatusError
			summary.Error = fmt.Sprintf("等待超时（%s 仍未进入终态）", c.RunTimeout)
			return summary
		}
		select {
		case <-ctx.Done():
			summary.Status = StatusError
			summary.Error = "已取消"
			return summary
		case <-time.After(c.PollInterval):
		}
	}
}

// applyDetail 把 CWT 的运行详情映射进汇总。
func applyDetail(summary *RunSummary, detail runDetail) {
	summary.Status = detail.Status
	summary.Stage = detail.Stage
	summary.Duration = detail.TotalDuration
	summary.DurationText = FormatDuration(detail.TotalDuration)
	summary.Validated = detail.ValidationCount
	summary.Version = detail.Version
	summary.StartedAt = detail.StartedAt
	summary.EndedAt = detail.FinishedAt
	if detail.Failure != nil {
		parts := make([]string, 0, 3)
		if detail.Failure.Stage != "" {
			parts = append(parts, "阶段 "+detail.Failure.Stage)
		}
		if detail.Failure.Action != "" {
			parts = append(parts, "动作 "+detail.Failure.Action)
		}
		if detail.Failure.Message != "" {
			parts = append(parts, detail.Failure.Message)
		}
		summary.Failure = strings.Join(parts, " / ")
	}
}
