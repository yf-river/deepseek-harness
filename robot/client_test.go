package robot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVerdict(t *testing.T) {
	cases := []struct {
		name    string
		results []RunSummary
		want    string
	}{
		{"全通过", []RunSummary{{Status: StatusPassed}, {Status: StatusPassed}}, "PASS"},
		{"有失败", []RunSummary{{Status: StatusPassed}, {Status: StatusFailed}}, "FAILED"},
		// 执行器侧没拿到结果不等于被测系统失败，应为 PARTIAL
		{"执行器侧未取得结果", []RunSummary{{Status: StatusError}}, "PARTIAL"},
		{"业务失败与未取得结果混合", []RunSummary{{Status: StatusFailed}, {Status: StatusError}}, "PARTIAL"},
		{"有未终态", []RunSummary{{Status: StatusPassed}, {Status: StatusRunning}}, "PARTIAL"},
		{"空批次", nil, "PARTIAL"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := Verdict(item.results); got != item.want {
				t.Fatalf("Verdict = %q，期望 %q", got, item.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[string]string{
		"":                 "-",
		"26.017626538s":    "26秒",
		"3m11.86087531s":   "3分12秒",
		"8m24s":            "8分24秒",
		"2m":               "2分",
		"1h2m3s":           "62分3秒",
	}
	for raw, want := range cases {
		if got := FormatDuration(raw); got != want {
			t.Fatalf("FormatDuration(%q) = %q，期望 %q", raw, got, want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	// UTC 07:09 应显示为北京时间 15:09。
	if got := formatTime("2026-11-23T07:09:11Z"); got != "2026-11-23 15:09" {
		t.Fatalf("formatTime = %q", got)
	}
	if got := formatTime(""); got != "-" {
		t.Fatalf("空时间 = %q", got)
	}
}

func TestRenderBody(t *testing.T) {
	started := time.Date(2026, 11, 23, 7, 16, 0, 0, time.UTC)
	ended := time.Date(2026, 11, 23, 7, 20, 0, 0, time.UTC)
	results := []RunSummary{
		{ID: "ida", Status: StatusPassed, Duration: "3m12s", Validated: 551,
			DetailURL: "http://example/#run=run-1"},
		{ID: "mira", Status: StatusFailed, Duration: "8m24s", Validated: 0,
			Failure: "阶段 部署合约 / 动作 准备并安装合约", DetailURL: "http://example/#run=run-2"},
		{ID: "dpm", Status: StatusError, Error: "查询失败: timeout"},
	}
	body := RenderBody(results, map[string]string{"ida": "IDA", "mira": "Mira", "dpm": "DPM"}, started, ended)

	for _, want := range []string{
		"1 条通过，1 条失败，1 条未取得结果",
		"2026-11-23 15:16—2026-11-23 15:20",
		"| IDA | 通过 | 3分12秒 | 551 |",
		"| Mira | 失败 | 8分24秒 | 0 |",
		"| DPM | 未取得结果 |", // 执行器侧失败不带耗时
		"Mira：阶段 部署合约",
		"DPM：查询失败: timeout",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("正文缺少 %q\n实际正文:\n%s", want, body)
		}
	}
}

// 失败信息里不能把「没查到」写成「测试失败」，两者必须分别表述。
func TestRenderBodyKeepsErrorAndFailureApart(t *testing.T) {
	results := []RunSummary{
		{ID: "ida", Status: StatusError, Error: "查询失败: connection refused"},
		{ID: "mira", Status: StatusFailed, Failure: "阶段 部署合约"},
	}
	body := RenderBody(results, nil, time.Now(), time.Now())
	if !strings.Contains(body, "IDA：查询失败") {
		t.Fatalf("执行器错误未单独表述:\n%s", body)
	}
	if !strings.Contains(body, "Mira：阶段 部署合约") {
		t.Fatalf("CWT 失败未表述:\n%s", body)
	}
}

func TestSubject(t *testing.T) {
	// UTC 20:00 是北京时间次日 04:00，主题日期必须用北京时间。
	when := time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC)
	if got := Subject("PASS", when); got != "[CWT] 主流程巡检 PASS - 2026-11-24" {
		t.Fatalf("Subject = %q", got)
	}
}

// newFakeCWT 起一个假的 CWT：先返回 running，若干次后返回终态。
func newFakeCWT(t *testing.T, finalStatus string, pollsBeforeDone int) *httptest.Server {
	t.Helper()
	polls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/test-runs":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("提交必须带幂等键")
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"run_id":"run-test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/test-runs":
			_, _ = w.Write([]byte(`{"runs":[{"status":"passed"}],"count":1}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/test-runs/"):
			polls++
			status := StatusRunning
			if polls > pollsBeforeDone {
				status = finalStatus
			}
			w.Write([]byte(`{"run_id":"run-test","status":"` + status + `","stage":"执行完成",
				"total_duration":"3m12s","validation_count":551}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestClientRunOneReachesTerminal(t *testing.T) {
	fake := newFakeCWT(t, StatusPassed, 2)
	defer fake.Close()

	client := NewClient(fake.URL)
	client.PollInterval = time.Millisecond
	batch := &Batch{Dir: t.TempDir(), Key: "k"}
	summary := client.runOne(context.Background(), batch, Suite{ID: "ida"})

	if summary.Status != StatusPassed {
		t.Fatalf("状态应为 passed，实际 %q（error=%q）", summary.Status, summary.Error)
	}
	if summary.Validated != 551 || summary.DurationText != "3分12秒" {
		t.Fatalf("汇总字段不对: %+v", summary)
	}
	if !strings.Contains(summary.DetailURL, "run-test") {
		t.Fatalf("详情链接不对: %q", summary.DetailURL)
	}
}

func TestClientRunOneHandlesSubmitFailure(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer fake.Close()

	client := NewClient(fake.URL)
	client.BusyRetries = 0
	summary := client.runOne(context.Background(), &Batch{Dir: t.TempDir(), Key: "k"}, Suite{ID: "ida"})
	if summary.Status != StatusError {
		t.Fatalf("提交失败应为 error，实际 %q", summary.Status)
	}
	if !strings.Contains(summary.Error, "500") {
		t.Fatalf("错误信息应包含状态码: %q", summary.Error)
	}
}

// 503 environment_busy 是环境排队，重试后应当能正常接单，而不是直接判失败。
func TestSubmitRetriesOnEnvironmentBusy(t *testing.T) {
	attempts := 0
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "environment_busy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"run_id":"run-after-retry"}`))
	}))
	defer fake.Close()

	client := NewClient(fake.URL)
	client.BusyRetryInterval = time.Millisecond
	runID, err := client.Submit(context.Background(), "ida", "k")
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if runID != "run-after-retry" {
		t.Fatalf("run_id 不对: %q", runID)
	}
	if attempts != 3 {
		t.Fatalf("应重试到第 3 次成功，实际尝试 %d 次", attempts)
	}
}

// 重试耗尽后要给出能看懂的说明，而不是裸 503。
func TestSubmitReportsBusyExhausted(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "environment_busy", http.StatusServiceUnavailable)
	}))
	defer fake.Close()

	client := NewClient(fake.URL)
	client.BusyRetries = 2
	client.BusyRetryInterval = time.Millisecond
	_, err := client.Submit(context.Background(), "ida", "k")
	if err == nil {
		t.Fatal("重试耗尽应报错")
	}
	if !strings.Contains(err.Error(), "环境占用") {
		t.Fatalf("错误应说明是环境占用: %v", err)
	}
}

func TestClientActiveRuns(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"runs":[{"status":"passed"},{"status":"running"},{"status":"queued"}],"count":3}`))
	}))
	defer fake.Close()

	count, err := NewClient(fake.URL).ActiveRuns(context.Background())
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("在途数应为 2，实际 %d", count)
	}
}

// 查询在途任务失败时必须报错，不能返回 0——「查不到」不等于「没有在途」。
func TestClientActiveRunsReportsError(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer fake.Close()

	if _, err := NewClient(fake.URL).ActiveRuns(context.Background()); err == nil {
		t.Fatal("应返回错误")
	}
}

func TestNewBatchKeyIsUnique(t *testing.T) {
	root := t.TempDir()
	first, err := NewBatch(root)
	if err != nil {
		t.Fatalf("建批次失败: %v", err)
	}
	second, err := NewBatch(root)
	if err != nil {
		t.Fatalf("建批次失败: %v", err)
	}
	if first.Key == second.Key {
		t.Fatalf("两批次的键不应相同: %q", first.Key)
	}
	if first.keyFor("ida") == first.keyFor("mira") {
		t.Fatal("同一批次内不同 suite 的键应不同")
	}
}

// 超时不能设得比实测最长耗时还短：Mira 最长到过 44 分钟，若 RunTimeout 小于它，
// 明明还在正常跑的任务会被判成失败。
func TestRunTimeoutCoversObservedLongest(t *testing.T) {
	client := NewClient("http://example")
	const longestObserved = 45 * time.Minute
	if client.RunTimeout <= longestObserved {
		t.Fatalf("RunTimeout=%s 不足以覆盖实测最长耗时 %s", client.RunTimeout, longestObserved)
	}
}

// 复现 11-25 早上那次：三个套件全部提交超时。
// 这种批次必须判为 PARTIAL，邮件里也不能写成"3 条失败"，
// 否则收件人会以为被测系统坏了，实际只是提交链路没通。
func TestAllSubmitFailuresIsPartialNotFailed(t *testing.T) {
	results := []RunSummary{
		{ID: "ida", Status: StatusError, Error: `提交失败: Post "...": context deadline exceeded`},
		{ID: "mira", Status: StatusError, Error: `提交失败: Post "...": context deadline exceeded`},
		{ID: "dpm", Status: StatusError, Error: `提交失败: Post "...": EOF`},
	}

	if got := Verdict(results); got != "PARTIAL" {
		t.Fatalf("全部未取得结果应为 PARTIAL，实际 %q", got)
	}

	body := RenderBody(results, map[string]string{"ida": "IDA", "mira": "Mira", "dpm": "DPM"},
		time.Now(), time.Now())

	// 不能出现"3 条失败"
	if strings.Contains(body, "3 条失败") {
		t.Fatalf("不应把未取得结果写成失败：\n%s", body)
	}
	if !strings.Contains(body, "3 条未取得结果") {
		t.Fatalf("应写明 3 条未取得结果：\n%s", body)
	}
	// 要有一句说明，避免被误读成功能有问题
	if !strings.Contains(body, "不代表被测功能有问题") {
		t.Fatalf("缺少未取得结果的说明：\n%s", body)
	}
	// 失败段落不应出现
	if strings.Contains(body, "失败信息：") {
		t.Fatalf("没有真实失败时不应出现失败信息段落：\n%s", body)
	}
}
