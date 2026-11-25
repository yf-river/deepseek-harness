package main

import (
	"path/filepath"
	"testing"
	"time"

	"chainweaver-studio/robot"
)

// 新建库应能建表、写批次、按时间倒序读出。
func TestStoreSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "robot.db")
	s, err := openStore(path)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = s.close() }()

	base := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		entry := historyEntry{
			Dir:       "cwt-robot-" + string(rune('a'+index)),
			Trigger:   "schedule",
			Verdict:   "PASS",
			StartedAt: base.Add(time.Duration(index) * time.Hour).Format(time.RFC3339),
			EndedAt:   base.Add(time.Duration(index)*time.Hour + time.Minute).Format(time.RFC3339),
			Suites:    []historySuite{{ID: "ida", Status: "passed"}},
		}
		if err := s.saveBatch(entry); err != nil {
			t.Fatalf("写批次失败: %v", err)
		}
	}

	items, total, err := s.loadPage(1, 10, false)
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("应为 3 条，实际 total=%d len=%d", total, len(items))
	}
	// 按开始时间倒序，最新的是第 3 条。
	if items[0].Dir != "cwt-robot-c" {
		t.Fatalf("第一条应是最新批次，实际 %q", items[0].Dir)
	}
	if len(items[0].Suites) != 1 || items[0].Suites[0].ID != "ida" {
		t.Fatalf("套件明细应完整保留: %+v", items[0].Suites)
	}
}

// 同一批次重复写入应按覆盖处理，不产生重复行。
func TestStoreSaveIsIdempotent(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = s.close() }()

	entry := historyEntry{Dir: "cwt-robot-x", Verdict: "PARTIAL", StartedAt: "2026-11-25T08:00:00+08:00"}
	if err := s.saveBatch(entry); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	entry.Verdict = "PASS"
	if err := s.saveBatch(entry); err != nil {
		t.Fatalf("二次写入失败: %v", err)
	}

	items, total, err := s.loadPage(1, 10, false)
	if err != nil {
		t.Fatalf("读历史失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("重复写入不应新增行，实际 %d", total)
	}
	if items[0].Verdict != "PASS" {
		t.Fatalf("应覆盖为最新值，实际 %q", items[0].Verdict)
	}
}

// 分页要正确：总数不受页大小影响，越界页返回空。
func TestStorePagination(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = s.close() }()

	for index := 0; index < 25; index++ {
		started := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Hour)
		_ = s.saveBatch(historyEntry{
			Dir:       "cwt-robot-" + string(rune('a'+index%26)) + string(rune('0'+index/26)),
			StartedAt: started.Format(time.RFC3339),
		})
	}
	items, total, err := s.loadPage(1, 10, false)
	if err != nil || total != 25 || len(items) != 10 {
		t.Fatalf("第 1 页应为 10/25，实际 %d/%d err=%v", len(items), total, err)
	}
	items, _, _ = s.loadPage(3, 10, false)
	if len(items) != 5 {
		t.Fatalf("第 3 页应为 5 条，实际 %d", len(items))
	}
	items, _, _ = s.loadPage(99, 10, false)
	if len(items) != 0 {
		t.Fatalf("越界页应为空，实际 %d", len(items))
	}
}

// 机器人状态要能存取，重启后据此恢复。
func TestStoreRobotStateRoundTrip(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = s.close() }()

	if _, ok := s.loadRobotState(); ok {
		t.Fatal("空库不应有状态")
	}
	want := robotState{
		Status:  robotIdle,
		Verdict: "PASS",
		Results: []robot.RunSummary{{ID: "ida", Status: robot.StatusPassed, Validated: 551}},
	}
	if err := s.saveRobotState(want); err != nil {
		t.Fatalf("保存状态失败: %v", err)
	}
	got, ok := s.loadRobotState()
	if !ok {
		t.Fatal("应能读回状态")
	}
	if got.Verdict != "PASS" || len(got.Results) != 1 || got.Results[0].Validated != 551 {
		t.Fatalf("状态内容不对: %+v", got)
	}
}

// 补跑判断依赖「某天是否已有批次」。
func TestStoreHasBatchOn(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = s.close() }()

	loc := time.FixedZone("CST", 8*3600)
	day := time.Date(2026, 11, 25, 12, 0, 0, 0, loc)
	if has, _ := s.hasBatchOn(day, loc); has {
		t.Fatal("空库不应判定为已有批次")
	}
	_ = s.saveBatch(historyEntry{Dir: "cwt-robot-x", StartedAt: time.Date(2026, 11, 25, 8, 0, 0, 0, loc).Format(time.RFC3339)})
	if has, _ := s.hasBatchOn(day, loc); !has {
		t.Fatal("当天已有批次时应判定为 true")
	}
	if has, _ := s.hasBatchOn(day.AddDate(0, 0, 1), loc); has {
		t.Fatal("次日不应判定为已有批次")
	}
}

// 清理要按时间删除，保留期内的记录不受影响。
func TestStorePruneBefore(t *testing.T) {
	s, err := openStore(filepath.Join(t.TempDir(), "robot.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = s.close() }()

	_ = s.saveBatch(historyEntry{Dir: "old", StartedAt: "2026-01-01T00:00:00+08:00"})
	_ = s.saveBatch(historyEntry{Dir: "new", StartedAt: "2026-11-25T00:00:00+08:00"})
	removed, err := s.pruneBefore(time.Date(2026, 6, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if removed != 1 {
		t.Fatalf("应删除 1 条，实际 %d", removed)
	}
	items, total, _ := s.loadPage(1, 10, false)
	if total != 1 || items[0].Dir != "new" {
		t.Fatalf("应只保留 new，实际 %d 条", total)
	}
}
