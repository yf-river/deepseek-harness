// 机器人设置的可编辑部分与持久化。
//
// 为什么单独存一份 settings.json 而不是回写 sites.yaml：sites.yaml 是人工维护的配置，
// 里面有成段的注释和站点清单，程序回写会把它变成机器格式。所以 sites.yaml 提供出厂默认值，
// 页面上改过的内容落到 artifact_dir 下的 settings.json，启动时覆盖默认值。

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chainweaver-studio/robot"
)

// robotSettings 是卡片里可设置的字段。严格限定为这几项，不堆无意义的开关。
type robotSettings struct {
	// Suites 是要执行的主流程 id 列表，按顺序串行跑。
	Suites []string `json:"suites"`
	// Schedule 是每日执行时间 HH:MM。
	Schedule string `json:"schedule"`
	// MailTo 是收件人邮箱。
	MailTo []string `json:"mailTo"`
	// SendMail 为 false 时执行但不发信。
	SendMail bool `json:"sendMail"`
	// Enabled 为 false 时停掉每日定时，手动执行仍可用。
	Enabled bool `json:"enabled"`
}

// knownSuites 是 CWT 登记的主流程全集，按执行顺序排列。
// 顺序即执行顺序：IDA 最轻，Mira 与 DPM 更重，放后面。
var knownSuites = []robot.Suite{
	{ID: "ida", Name: "IDA"},
	{ID: "mira", Name: "Mira"},
	{ID: "dpm", Name: "DPM"},
}

// defaultSettings 从配置文件里的 robot 段取默认值。
func defaultSettings(config *RobotConfig) robotSettings {
	settings := robotSettings{
		Schedule: config.Schedule,
		MailTo:   config.MailTo,
		SendMail: config.SendMail,
		Enabled:  config.Enabled,
	}
	for _, suite := range config.Suites {
		settings.Suites = append(settings.Suites, suite.ID)
	}
	if len(settings.Suites) == 0 {
		for _, suite := range knownSuites {
			settings.Suites = append(settings.Suites, suite.ID)
		}
	}
	return settings
}

// validate 校验并归一化设置。非法输入返回错误，调用方不得落盘。
func (s *robotSettings) validate() error {
	seen := make(map[string]bool, len(s.Suites))
	valid := make(map[string]bool, len(knownSuites))
	for _, suite := range knownSuites {
		valid[suite.ID] = true
	}
	trimmed := make([]string, 0, len(s.Suites))
	for _, id := range s.Suites {
		id = strings.TrimSpace(id)
		if !valid[id] {
			return fmt.Errorf("未知主流程 %q", id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		trimmed = append(trimmed, id)
	}
	if len(trimmed) == 0 {
		return fmt.Errorf("至少要选一个主流程")
	}
	s.Suites = trimmed

	if _, _, err := parseSchedule(s.Schedule); err != nil {
		return err
	}
	if s.SendMail {
		addresses := make([]string, 0, len(s.MailTo))
		for _, address := range s.MailTo {
			address = strings.TrimSpace(address)
			if address == "" {
				continue
			}
			if !strings.Contains(address, "@") {
				return fmt.Errorf("收件人 %q 不是完整邮箱", address)
			}
			addresses = append(addresses, address)
		}
		if len(addresses) == 0 {
			return fmt.Errorf("开启发信时必须填收件人")
		}
		s.MailTo = addresses
	}
	return nil
}

// asSuites 把 id 列表还原成带名称的 Suite，供执行器使用。
func (s robotSettings) asSuites() []robot.Suite {
	names := make(map[string]string, len(knownSuites))
	for _, suite := range knownSuites {
		names[suite.ID] = suite.Name
	}
	out := make([]robot.Suite, 0, len(s.Suites))
	for _, id := range s.Suites {
		out = append(out, robot.Suite{ID: id, Name: names[id]})
	}
	return out
}

// settingsPath 返回设置文件的落盘路径。
func settingsPath(config *RobotConfig) string {
	return filepath.Join(config.ArtifactDir, "settings.json")
}

// loadSettings 读取设置：默认值打底，再用 settings.json 覆盖。
// 文件损坏时报错但不阻塞启动——调用方回退到默认值。
func loadSettings(config *RobotConfig) (robotSettings, error) {
	settings := defaultSettings(config)
	data, err := os.ReadFile(settingsPath(config))
	if err != nil {
		if os.IsNotExist(err) {
			if err := settings.validate(); err != nil {
				// 默认值本身不合法时给一个能跑的最小集合，避免服务起不来。
				settings.Suites = []string{"ida", "mira", "dpm"}
				settings.Schedule = "08:00"
			}
			return settings, nil
		}
		return settings, err
	}
	var stored robotSettings
	if err := json.Unmarshal(data, &stored); err != nil {
		return defaultSettings(config), fmt.Errorf("settings.json 解析失败: %w", err)
	}
	if err := stored.validate(); err != nil {
		return defaultSettings(config), fmt.Errorf("settings.json 内容非法: %w", err)
	}
	return stored, nil
}

// save 把设置落盘。
func saveSettings(config *RobotConfig, settings robotSettings) error {
	if err := os.MkdirAll(config.ArtifactDir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath(config), payload, 0o600)
}

// applySettings 把设置写进运行器，并唤醒调度器按新时间重排。
func (r *robotRunner) applySettings(settings robotSettings) {
	r.stateMu.Lock()
	r.config.Suites = settings.asSuites()
	r.config.Schedule = settings.Schedule
	r.config.MailTo = settings.MailTo
	r.config.SendMail = settings.SendMail
	r.config.Enabled = settings.Enabled
	r.stateMu.Unlock()

	// 通知调度循环重新计算下一次触发时间。
	select {
	case r.scheduleChanged <- struct{}{}:
	default:
	}
}

// currentSettings 返回运行器当前生效的设置。
func (r *robotRunner) currentSettings() robotSettings {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	settings := robotSettings{
		Schedule: r.config.Schedule,
		MailTo:   r.config.MailTo,
		SendMail: r.config.SendMail,
		Enabled:  r.config.Enabled,
	}
	for _, suite := range r.config.Suites {
		settings.Suites = append(settings.Suites, suite.ID)
	}
	return settings
}

// nextRunAt 返回下一次定时触发的北京时间，供卡片展示。不启用时返回空串。
func (r *robotRunner) nextRunAt() string {
	settings := r.currentSettings()
	if !settings.Enabled {
		return ""
	}
	hour, minute, err := parseSchedule(settings.Schedule)
	if err != nil {
		return ""
	}
	return nextOccurrence(time.Now().In(r.location), hour, minute).Format(time.RFC3339)
}
