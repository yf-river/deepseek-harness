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

func TestEnvFreeRequestValidate(t *testing.T) {
	if err := (envFreeRequest{}).validate(); err == nil {
		t.Fatal("空租约应报错")
	}
	if err := (envFreeRequest{Lease: "lease-1"}).validate(); err != nil {
		t.Fatalf("合法租约不应报错: %v", err)
	}
}

// 操作记录要能解析，且最新的在前。
func TestEnvPoolOperationsNewestFirst(t *testing.T) {
	pool := summarize(t, `{
	  "environments": [], "leases": [],
	  "operations": [
	    {"kind":"free","status":"succeeded","environment":"a","step":5,"created_at":"2026-11-23T10:00:00+08:00"},
	    {"kind":"deploy","status":"running","environment":"b","step":2,"created_at":"2026-11-23T11:00:00+08:00"}
	  ]
	}`)
	if len(pool.RecentOperations) != 2 {
		t.Fatalf("应有 2 条操作，实际 %d", len(pool.RecentOperations))
	}
	// state.json 是追加写入，越靠后越新，所以倒序后第一条应是 b。
	if pool.RecentOperations[0].Environment != "b" {
		t.Fatalf("最新操作应是 b，实际 %q", pool.RecentOperations[0].Environment)
	}
}

// step 是数字，不是字符串——这里锁住这个契约，防止再按 string 解析。
func TestEnvOperationStepIsNumber(t *testing.T) {
	pool := summarize(t, `{
	  "environments": [], "leases": [],
	  "operations": [{"kind":"deploy","status":"running","environment":"a","step":3}]
	}`)
	if pool.Error != "" {
		t.Fatalf("数字 step 不应解析失败: %s", pool.Error)
	}
	if len(pool.RecentOperations) != 1 || pool.RecentOperations[0].Step != 3 {
		t.Fatalf("step 解析不对: %+v", pool.RecentOperations)
	}
}

// 操作记录要截断，不能把全部历史都返回给页面。
func TestEnvPoolOperationsCapped(t *testing.T) {
	body := `{"environments":[],"leases":[],"operations":[`
	for index := 0; index < 40; index++ {
		if index > 0 {
			body += ","
		}
		body += `{"kind":"free","status":"succeeded","environment":"e"}`
	}
	body += `]}`
	pool := summarize(t, body)
	if len(pool.RecentOperations) != 20 {
		t.Fatalf("应截断为 20 条，实际 %d", len(pool.RecentOperations))
	}
}

func TestEnvDeployRequestValidate(t *testing.T) {
	if err := (&envDeployRequest{}).validate(); err == nil {
		t.Fatal("环境名与数据库都为空时应报错")
	}
	if err := (&envDeployRequest{Database: "mysql"}).validate(); err != nil {
		t.Fatalf("只给数据库应通过: %v", err)
	}
	if err := (&envDeployRequest{Environment: "cwe-v52-mysql-dpm"}).validate(); err != nil {
		t.Fatalf("只给环境名应通过: %v", err)
	}
}

// 空字段必须省略，否则「不筛选」会被表达成空字符串改变语义。
func TestEnvDeployPayloadOmitsEmpty(t *testing.T) {
	payload := envDeployRequest{Database: "mysql"}.toCWEPayload()
	if _, ok := payload["version"]; ok {
		t.Fatal("空 version 不应出现在请求体里")
	}
	if payload["database"] != "mysql" {
		t.Fatalf("database 应保留: %v", payload)
	}
}
