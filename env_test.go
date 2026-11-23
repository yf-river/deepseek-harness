package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeState(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写入临时状态失败: %v", err)
	}
	return path
}

// summarize 是测试用的入口：直接解析内容而不走 ssh。
func summarize(t *testing.T, body string) envPool {
	t.Helper()
	path := writeState(t, body)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	return parseEnvPool(raw)
}

func TestEnvPoolCountsStates(t *testing.T) {
	pool := summarize(t, `{
	  "environments": [
	    {"name":"a","status":"idle","health":{"ready":true},"modules":[{"name":"mira"}]},
	    {"name":"b","status":"leased","health":{"ready":true},"modules":[]},
	    {"name":"c","status":"restoring","health":{"ready":true}},
	    {"name":"d","status":"idle","health":{"ready":false}}
	  ],
	  "leases": [{"id":"lease-1","environment":"b","owner":"tester","expires_at":"2026-11-24T00:00:00+08:00"}]
	}`)

	if pool.Total != 4 {
		t.Fatalf("Total 应为 4，实际 %d", pool.Total)
	}
	// c 是 restoring，占着环境，不能算空闲。
	if pool.Idle != 1 {
		t.Fatalf("Idle 应为 1，实际 %d", pool.Idle)
	}
	if pool.Leased != 1 {
		t.Fatalf("Leased 应为 1，实际 %d", pool.Leased)
	}
	if pool.Busy != 1 {
		t.Fatalf("Busy 应为 1，实际 %d", pool.Busy)
	}
	if pool.Unhealthy != 1 {
		t.Fatalf("Unhealthy 应为 1，实际 %d", pool.Unhealthy)
	}
}

func TestEnvPoolAttachesLease(t *testing.T) {
	pool := summarize(t, `{
	  "environments": [{"name":"b","status":"leased","health":{"ready":true}}],
	  "leases": [{"id":"lease-1","environment":"b","owner":"tester","expires_at":"2026-11-24T00:00:00+08:00"}]
	}`)
	if len(pool.Environments) != 1 {
		t.Fatalf("应有 1 条环境")
	}
	if pool.Environments[0].Lease == nil {
		t.Fatal("应挂上租约")
	}
	if pool.Environments[0].Lease.Owner != "tester" {
		t.Fatalf("租约 owner 不对: %+v", pool.Environments[0].Lease)
	}
}

// 没有租约的环境不应伪造租约。
func TestEnvPoolWithoutLease(t *testing.T) {
	pool := summarize(t, `{"environments":[{"name":"a","status":"idle","health":{"ready":true}}],"leases":[]}`)
	if pool.Environments[0].Lease != nil {
		t.Fatal("空闲环境不应有租约")
	}
}

func TestEnvPoolHandlesBadJSON(t *testing.T) {
	pool := summarize(t, `{not json`)
	if pool.Error == "" {
		t.Fatal("非法状态应返回错误而不是空池")
	}
}
