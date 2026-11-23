package main

import (
	"testing"
	"time"

	"chainweaver-studio/robot"
)

func TestParseSchedule(t *testing.T) {
	cases := []struct {
		raw           string
		hour, minute  int
		wantErr       bool
	}{
		{"08:00", 8, 0, false},
		{"23:59", 23, 59, false},
		{"0:05", 0, 5, false},
		{"24:00", 0, 0, true},
		{"08:60", 0, 0, true},
		{"abc", 0, 0, true},
		{"", 0, 0, true},
	}
	for _, item := range cases {
		hour, minute, err := parseSchedule(item.raw)
		if item.wantErr {
			if err == nil {
				t.Fatalf("parseSchedule(%q) 应报错", item.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseSchedule(%q) 意外报错: %v", item.raw, err)
		}
		if hour != item.hour || minute != item.minute {
			t.Fatalf("parseSchedule(%q) = %d:%d，期望 %d:%d", item.raw, hour, minute, item.hour, item.minute)
		}
	}
}

func TestNextOccurrence(t *testing.T) {
	location := time.FixedZone("CST", 8*3600)
	// 08:00 之前，下一次就是当天 08:00。
	now := time.Date(2026, 11, 23, 7, 30, 0, 0, location)
	next := nextOccurrence(now, 8, 0)
	if next.Day() != 23 || next.Hour() != 8 {
		t.Fatalf("应为当天 08:00，实际 %v", next)
	}
	// 已过 08:00，下一次是次日。
	now = time.Date(2026, 11, 23, 9, 0, 0, 0, location)
	next = nextOccurrence(now, 8, 0)
	if next.Day() != 24 || next.Hour() != 8 {
		t.Fatalf("应为次日 08:00，实际 %v", next)
	}
	// 恰好等于触发时刻，应排到次日，避免同一分钟重复触发。
	now = time.Date(2026, 11, 23, 8, 0, 0, 0, location)
	next = nextOccurrence(now, 8, 0)
	if next.Day() != 24 {
		t.Fatalf("恰在触发时刻时应排次日，实际 %v", next)
	}
}

func TestSettingsValidate(t *testing.T) {
	base := func() robotSettings {
		return robotSettings{
			Suites:   []string{"ida"},
			Schedule: "08:00",
			MailTo:   []string{"a@b.com"},
			SendMail: true,
			Enabled:  true,
		}
	}

	// 正常值应通过，且去重。
	ok := base()
	ok.Suites = []string{"ida", "mira", "ida"}
	ok.MailTo = []string{"a@b.com", " c@d.com "}
	if err := ok.validate(); err != nil {
		t.Fatalf("合法设置报错: %v", err)
	}
	if len(ok.Suites) != 2 {
		t.Fatalf("suites 应去重为 2，实际 %v", ok.Suites)
	}
	if ok.MailTo[1] != "c@d.com" {
		t.Fatalf("邮箱应去空格，实际 %q", ok.MailTo[1])
	}

	cases := []struct {
		name   string
		mutate func(*robotSettings)
	}{
		{"未知主流程", func(s *robotSettings) { s.Suites = []string{"tdh"} }},
		{"没有主流程", func(s *robotSettings) { s.Suites = nil }},
		{"时间非法", func(s *robotSettings) { s.Schedule = "25:00" }},
		{"发信但无收件人", func(s *robotSettings) { s.MailTo = nil }},
		{"收件人不是邮箱", func(s *robotSettings) { s.MailTo = []string{"yunfeihu"} }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			settings := base()
			item.mutate(&settings)
			if err := settings.validate(); err == nil {
				t.Fatalf("%s 应报错", item.name)
			}
		})
	}

	// 不发信时不校验收件人。
	noMail := base()
	noMail.SendMail = false
	noMail.MailTo = nil
	if err := noMail.validate(); err != nil {
		t.Fatalf("不发信时不应要求收件人: %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	config := &RobotConfig{ArtifactDir: t.TempDir()}
	settings := robotSettings{
		Suites:   []string{"ida", "dpm"},
		Schedule: "09:30",
		MailTo:   []string{"a@b.com"},
		SendMail: true,
		Enabled:  true,
	}
	if err := saveSettings(config, settings); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	loaded, err := loadSettings(config)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if loaded.Schedule != "09:30" || len(loaded.Suites) != 2 || !loaded.Enabled {
		t.Fatalf("读回的内容不对: %+v", loaded)
	}
}

// 没有 settings.json 时返回配置默认值，不能报错。
func TestLoadSettingsDefaults(t *testing.T) {
	config := &RobotConfig{
		ArtifactDir: t.TempDir(),
		Schedule:    "08:00",
	}
	settings, err := loadSettings(config)
	if err != nil {
		t.Fatalf("无设置文件不应报错: %v", err)
	}
	if settings.Schedule != "08:00" {
		t.Fatalf("应回退到配置默认值，实际 %q", settings.Schedule)
	}
}

func TestAsSuites(t *testing.T) {
	settings := robotSettings{Suites: []string{"ida", "mira"}}
	suites := settings.asSuites()
	if len(suites) != 2 || suites[1].ID != "mira" || suites[1].Name != "Mira" {
		t.Fatalf("转换结果不对: %+v", suites)
	}
}

// 单流程与整批的超时必须递进，否则 context 先取消会让结论变成含糊的「已取消」。
func TestRunnerTimeoutsAreLayered(t *testing.T) {
	if suiteTimeout <= robot.NewClient("http://example").RunTimeout {
		t.Fatalf("suiteTimeout=%s 必须大于 client 的 RunTimeout", suiteTimeout)
	}
	if batchTimeout <= suiteTimeout {
		t.Fatalf("batchTimeout=%s 必须大于 suiteTimeout=%s", batchTimeout, suiteTimeout)
	}
}
