// 机器人的调度与运行状态。手动触发和每日定时走完全同一条执行路径，
// 区别只是谁触发的；两者共用一把互斥锁，保证同一时刻只有一个批次在跑。

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"chainweaver-studio/robot"
)

// suiteTimeout 是单个主流程的等待上限，必须大于 client 的 RunTimeout（90 分钟），
// 让 client 先给出明确的「等待超时」结论，而不是让 context 先取消成含糊的「已取消」。
const suiteTimeout = 100 * time.Minute

// batchTimeout 是整批的上限。三个套件最坏情况各接近上限，给 6 小时足够，
// 作用只是防止批次永久挂住占着互斥锁。
const batchTimeout = 6 * time.Hour

// 机器人状态机的几个取值，供前端展示。
const (
	robotIdle    = "idle"
	robotRunning = "running"
)

// robotState 是一次运行的快照，也是 /api/robot 的返回体。
type robotState struct {
	Status string `json:"status"`
	// Trigger 说明这次是谁触发的：manual 或 schedule。
	Trigger   string `json:"trigger"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
	// Verdict 是 PASS / FAILED / PARTIAL，跑完后才有值。
	Verdict string `json:"verdict"`
	// Subject 与 MailStatus 反映发信结果，发信失败不影响批次结果。
	Subject    string             `json:"subject"`
	MailStatus string             `json:"mailStatus"`
	MailError  string             `json:"mailError"`
	Results    []robot.RunSummary `json:"results"`
	// Message 是给人看的一句话，比如「正在执行 Mira 主流程」或错误说明。
	Message string `json:"message"`
	// LastError 记录执行器自身的异常。
	LastError string `json:"lastError"`
}

// robotRunner 持有配置与当前状态。
type robotRunner struct {
	config *RobotConfig
	client *robot.Client
	// runMu 保证同一时刻只有一个批次在跑。TryLock 失败直接拒绝，不排队：
	// 让用户知道「已经在跑了」，比默默排队更清楚。
	runMu   sync.Mutex
	stateMu sync.Mutex
	state   robotState
	// location 是定时触发用的时区。
	location *time.Location
	// scheduleChanged 在设置变更时唤醒调度循环重算触发时间。
	scheduleChanged chan struct{}
}

// newRobotRunner 构造运行器。机器人未配置时返回 nil，调用方据此决定是否挂载路由。
func newRobotRunner(config *RobotConfig) (*robotRunner, error) {
	if config == nil {
		return nil, nil
	}
	if config.CWTURL == "" {
		return nil, fmt.Errorf("robot.cwt_url 不能为空")
	}
	location := time.Local
	if config.Timezone != "" {
		parsed, err := time.LoadLocation(config.Timezone)
		if err != nil {
			return nil, fmt.Errorf("robot.schedule_timezone 无效: %w", err)
		}
		location = parsed
	}
	if config.ArtifactDir == "" {
		config.ArtifactDir = "run/robot"
	}
	if config.MailBin == "" {
		config.MailBin = "wecom-cli"
	}
	return &robotRunner{
		config:   config,
		client:   robot.NewClient(config.CWTURL),
		location:        location,
		scheduleChanged: make(chan struct{}, 1),
		state:           robotState{Status: robotIdle, Message: "尚未执行过"},
	}, nil
}

// snapshot 返回当前状态的副本。
func (r *robotRunner) snapshot() robotState {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.state
}

// update 在锁内改状态，避免并发读写。
func (r *robotRunner) update(mutate func(*robotState)) robotState {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	mutate(&r.state)
	return r.state
}

// Trigger 手动触发一次。已有批次在跑时返回 false，不排队。
func (r *robotRunner) Trigger() (bool, robotState) {
	if !r.runMu.TryLock() {
		return false, r.snapshot()
	}
	current := r.update(func(s *robotState) {
		*s = robotState{
			Status:    robotRunning,
			Trigger:   "manual",
			StartedAt: time.Now().In(cstZone()).Format(time.RFC3339),
			Message:   "正在提交主流程…",
		}
	})
	// 异步跑，接口立刻返回，前端轮询 /api/robot 看进度。
	go func() {
		defer r.runMu.Unlock()
		r.execute("manual")
	}()
	return true, current
}

// StartSchedule 启动调度循环。循环常驻：设置里 enabled 为 false 时只是不触发，
// 这样页面上随时开关都不用重启进程。
func (r *robotRunner) StartSchedule() {
	log.Printf("机器人调度已启动（enabled=%v, schedule=%q, tz=%s）",
		r.config.Enabled, r.config.Schedule, r.location)
	go func() {
		for {
			settings := r.currentSettings()
			if !settings.Enabled {
				// 未启用时静默等待设置变更，不空转。
				select {
				case <-r.scheduleChanged:
				case <-time.After(30 * time.Second):
				}
				continue
			}
			hour, minute, err := parseSchedule(settings.Schedule)
			if err != nil {
				log.Printf("机器人 schedule 无效，等待修改: %v", err)
				<-r.scheduleChanged
				continue
			}

			next := nextOccurrence(time.Now().In(r.location), hour, minute)
			timer := time.NewTimer(time.Until(next))
			select {
			case <-timer.C:
				// 到点触发。上一批还在跑就跳过本日，宁可漏一天也不并发抢占 CWT 环境。
				if !r.runMu.TryLock() {
					log.Printf("跳过本日定时巡检：上一批仍在执行")
					continue
				}
				r.update(func(s *robotState) {
					*s = robotState{
						Status:    robotRunning,
						Trigger:   "schedule",
						StartedAt: time.Now().In(cstZone()).Format(time.RFC3339),
						Message:   "正在提交主流程…",
					}
				})
				r.execute("schedule")
				r.runMu.Unlock()
			case <-r.scheduleChanged:
				timer.Stop()
				log.Printf("机器人设置已变更，重算下一次触发时间")
			}
		}
	}()
}

// execute 是一次批次的完整流程：跑完所有 suite → 汇总 → 发信 → 落盘。
// 任何一步失败都写入状态，不 panic、不阻塞后续批次。
func (r *robotRunner) execute(trigger string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("机器人执行异常: %v", recovered)
			r.update(func(s *robotState) {
				s.Status = robotIdle
				s.LastError = fmt.Sprintf("执行异常: %v", recovered)
				s.Message = "执行异常"
			})
		}
	}()

	startedAt := time.Now()
	batch, err := robot.NewBatch(r.config.ArtifactDir)
	if err != nil {
		r.update(func(s *robotState) {
			s.Status = robotIdle
			s.LastError = "建立批次目录失败: " + err.Error()
			s.Message = "执行失败"
		})
		return
	}

	// 开跑前确认没有在途任务，避免和别人的运行抢环境。查不到就当作有在途：
	// 「查不到」不能当成「没有」。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	active, err := r.client.ActiveRuns(ctx)
	cancel()
	if err != nil {
		r.update(func(s *robotState) {
			s.Status = robotIdle
			s.LastError = "检查在途任务失败: " + err.Error()
			s.Message = "未能开始：无法确认在途任务"
		})
		return
	}
	if active > 0 {
		r.update(func(s *robotState) {
			s.Status = robotIdle
			s.LastError = fmt.Sprintf("当前有 %d 个在途任务", active)
			s.Message = "未能开始：CWT 上有在途任务"
		})
		return
	}

	names := make(map[string]string, len(r.config.Suites))
	for _, suite := range r.config.Suites {
		if suite.Name == "" {
			names[suite.ID] = suite.ID
			continue
		}
		names[suite.ID] = suite.Name
	}

	// 整批一个总 deadline，防止某个套件把互斥锁永久占住。
	batchCtx, batchCancel := context.WithTimeout(context.Background(), batchTimeout)
	defer batchCancel()

	results := make([]robot.RunSummary, 0, len(r.config.Suites))
	for _, suite := range r.config.Suites {
		if batchCtx.Err() != nil {
			results = append(results, robot.RunSummary{
				ID:     suite.ID,
				Suites: []string{suite.ID},
				Status: robot.StatusError,
				Error:  "跳过：整批已超过总时限",
			})
			continue
		}
		r.update(func(s *robotState) {
			s.Message = fmt.Sprintf("正在执行 %s…", names[suite.ID])
			s.Results = results
		})
		// 单个 suite 的 context 要比 client 的 RunTimeout 更长，否则 context 先到期，
		// 错误会变成含糊的 "已取消"，看不出到底是超时还是别的原因。
		// 从 batchCtx 派生：整批超时能真正打断当前套件，而不是等它自己跑完。
		suiteCtx, suiteCancel := context.WithTimeout(batchCtx, suiteTimeout)
		outcome := r.client.Run(suiteCtx, batch, []robot.Suite{suite})
		suiteCancel()
		results = append(results, outcome...)
		r.update(func(s *robotState) { s.Results = results })
	}

	endedAt := time.Now()
	verdict := robot.Verdict(results)
	subject := robot.Subject(verdict, endedAt)
	body := robot.RenderBody(results, names, startedAt, endedAt)
	if err := robot.WriteArtifacts(batch.Dir, subject, body, results); err != nil {
		log.Printf("写入批次产物失败: %v", err)
	}

	mailer := robot.Mailer{Bin: r.config.MailBin, To: r.config.MailTo, DryRun: !r.config.SendMail}
	bodyPath := filepath.Join(batch.Dir, "mail_body.md")
	mailStatus := "已发送"
	var mailErr string
	if _, err := mailer.Send(context.Background(), subject, bodyPath); err != nil {
		// 发信失败是独立结论，不能写成测试失败，也不能写成已送达。
		mailStatus = "发送失败"
		mailErr = err.Error()
		log.Printf("机器人发信失败: %v", err)
	} else if !r.config.SendMail {
		mailStatus = "演练未发送"
	}

	final := r.update(func(s *robotState) {
		s.Status = robotIdle
		s.Trigger = trigger
		s.EndedAt = endedAt.In(cstZone()).Format(time.RFC3339)
		s.Verdict = verdict
		s.Subject = subject
		s.MailStatus = mailStatus
		s.MailError = mailErr
		s.Results = results
		s.Message = fmt.Sprintf("完成：%s，批次目录 %s", verdict, batch.Dir)
	})
	_ = os.WriteFile(filepath.Join(batch.Dir, "final-state.json"), mustJSON(final), 0o600)
	log.Printf("机器人批次完成：%s，产物 %s", verdict, batch.Dir)
}

// restoreLastState 从最近一个批次目录恢复上次结果，让服务重启后仍能回答
// 「昨晚跑得怎么样」。找不到就保持初始状态，不报错。
func (r *robotRunner) restoreLastState() {
	entries, err := os.ReadDir(r.config.ArtifactDir)
	if err != nil {
		return
	}
	// 批次目录名带时间戳前缀，取字典序最大的一个即最近一次。
	var latest string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "cwt-robot-") {
			continue
		}
		if entry.Name() > latest {
			latest = entry.Name()
		}
	}
	if latest == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(r.config.ArtifactDir, latest, "final-state.json"))
	if err != nil {
		return
	}
	var state robotState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}
	// 老批次的 results.json 可能没有 durationText，用原始 duration 补出来。
	for index := range state.Results {
		if state.Results[index].DurationText == "" && state.Results[index].Duration != "" {
			state.Results[index].DurationText = robot.FormatDuration(state.Results[index].Duration)
		}
	}
	// 恢复出来的状态一律置为 idle：进程重启过，不可能还有批次在跑。
	state.Status = robotIdle
	state.Message = fmt.Sprintf("上次结果：%s，批次目录 %s", state.Verdict, filepath.Join(r.config.ArtifactDir, latest))
	r.update(func(s *robotState) { *s = state })
	log.Printf("已恢复上次巡检结果：%s", state.Verdict)
}

// cstZone 返回北京时区，用于状态里的时间展示。
func cstZone() *time.Location { return time.FixedZone("CST", 8*3600) }

// parseSchedule 解析 HH:MM。
func parseSchedule(raw string) (int, int, error) {
	var hour, minute int
	if _, err := fmt.Sscanf(raw, "%d:%d", &hour, &minute); err != nil {
		return 0, 0, fmt.Errorf("schedule 需为 HH:MM，收到 %q", raw)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("schedule 超出范围: %q", raw)
	}
	return hour, minute, nil
}

// nextOccurrence 返回 now 之后的下一个 hour:minute。
func nextOccurrence(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func mustJSON(payload any) []byte {
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return encoded
}
