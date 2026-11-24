// Command chainweaver-studio 是 ChainWeaver 各站点的统一入口门户。
//
// 设计要点：
//   - 站点清单只来自 config/sites.yaml，代码里不硬编码任何站点；新增站点改配置即可。
//   - 前端页面与样式在编译期用 go:embed 打进二进制，部署机只需一个二进制加一份配置。
//   - 顶部横向签的外观沿用 chainweaver-flow / chainweaver-test 的视觉体系（白底卡片、
//     #0052d9 主色、圆角、细边框），签的交互与布局参照 spec-score 的 nav-tabs。
//   - 点击签采用整页跳转到目标站点：两个被接入站点都下发了
//     frame-ancestors 'none' + X-Frame-Options: DENY，iframe 会被浏览器拒绝渲染。
//   - /api/sites 额外做服务端健康探测，避免浏览器跨源请求被 CORS 拦住，也让首页能显示
//     每个站点的可达状态。
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"chainweaver-studio/robot"
)

// pageFiles 随二进制发布，部署机不需要复制前端目录。
//
//go:embed web
var pageFiles embed.FS

// Portal 是配置里的门户自身信息。
type Portal struct {
	Title    string `yaml:"title" json:"title"`
	Subtitle string `yaml:"subtitle" json:"subtitle"`
	Listen   string `yaml:"listen" json:"listen"`
}

// Site 是一个被接入的子站点。health_url 为空时退回用 url 探测。
type Site struct {
	Key         string   `yaml:"key" json:"key"`
	Label       string   `yaml:"label" json:"label"`
	Title       string   `yaml:"title" json:"title"`
	Subtitle    string   `yaml:"subtitle" json:"subtitle"`
	URL         string   `yaml:"url" json:"url"`
	HealthURL   string   `yaml:"health_url" json:"healthUrl"`
	Description string   `yaml:"description" json:"description"`
	Tags        []string `yaml:"tags" json:"tags"`
	Icon        string   `yaml:"icon" json:"icon"`
}

// RobotConfig 是每日巡检机器人的设置。它由门户自己调度，不是被接入的子站点。
type RobotConfig struct {
	// CWTURL 是 ChainWeaver Test 的任务 API 根地址。
	CWTURL string `yaml:"cwt_url" json:"cwtUrl"`
	// Suites 是每次巡检要跑的主流程，按顺序串行执行。
	Suites []robot.Suite `yaml:"suites" json:"suites"`
	// Schedule 是每日触发时间，24 小时制 HH:MM，按 schedule_timezone 解释。
	Schedule string `yaml:"schedule" json:"schedule"`
	// Timezone 是 schedule 的时区名，空则用本机时区。
	Timezone string `yaml:"schedule_timezone" json:"timezone"`
	// MailTo 是收件人。
	MailTo []string `yaml:"mail_to" json:"mailTo"`
	// MailBin 是 wecom-cli 路径。
	MailBin string `yaml:"mail_bin" json:"mailBin"`
	// SendMail 为 false 时只演练不投递。
	SendMail bool `yaml:"send_mail" json:"sendMail"`
	// ArtifactDir 是批次产物落盘目录。
	ArtifactDir string `yaml:"artifact_dir" json:"artifactDir"`
	// Enabled 为 false 时不开定时器，但手动执行仍可用。
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// CWEConfig 是 ChainWeaver Env 的数据源设置。
type CWEConfig struct {
	// URL 是 cwe 的 HTTP API 根地址，用于提交 free 这类操作。
	URL string `yaml:"url" json:"url"`
	// StatePath 是 cwe 的 state.json 路径（在 cwe 所在机器上）。
	// cwe 没有列出环境的 HTTP 接口，只能读它自己持久化的这份状态。
	StatePath string `yaml:"state_path" json:"statePath"`
	// SSHHost 是 cwe 所在机器的 ssh 别名，用于读取 state_path。
	SSHHost string `yaml:"ssh_host" json:"sshHost"`
	// EnvHost 是环境部署所在的宿主机地址，用于拼前端访问链接。
	// cwe 的 state.json 只记 NodePort，不记主机。
	EnvHost string `yaml:"env_host" json:"envHost"`
	// Enabled 为 false 时不挂载环境相关路由。
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// Config 是 sites.yaml 的完整结构。
type Config struct {
	Portal Portal       `yaml:"portal" json:"portal"`
	Sites  []Site       `yaml:"sites" json:"sites"`
	Robot  *RobotConfig `yaml:"robot" json:"robot"`
	CWE    *CWEConfig   `yaml:"cwe" json:"cwe"`
}

// loadConfig 读取并校验清单。key 必须唯一且非空、url 必须存在，否则启动即失败：
// 门户是入口，宁可起不来也不能带一份残缺清单上线。
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if len(cfg.Sites) == 0 {
		return nil, fmt.Errorf("配置 %s 中没有 sites", path)
	}
	seen := make(map[string]bool, len(cfg.Sites))
	for index, site := range cfg.Sites {
		if strings.TrimSpace(site.Key) == "" {
			return nil, fmt.Errorf("第 %d 个站点缺少 key", index+1)
		}
		if seen[site.Key] {
			return nil, fmt.Errorf("站点 key 重复: %s", site.Key)
		}
		seen[site.Key] = true
		if strings.TrimSpace(site.URL) == "" {
			return nil, fmt.Errorf("站点 %s 缺少 url", site.Key)
		}
		if strings.TrimSpace(site.Label) == "" {
			return nil, fmt.Errorf("站点 %s 缺少 label", site.Key)
		}
	}
	if strings.TrimSpace(cfg.Portal.Title) == "" {
		cfg.Portal.Title = "ChainWeaver"
	}
	return &cfg, nil
}

// server 持有清单与健康探测用的客户端。
type server struct {
	config *Config
	client *http.Client
	// robot 可能为 nil：配置里没有 robot 段时机器人相关路由直接 404。
	robot *robotRunner
	// cwe 可能为 nil：没配 ChainWeaver Env 时不挂载环境路由。
	cwe *CWEConfig
}

// siteHealth 是 /api/sites 返回的单个站点状态。
type siteHealth struct {
	Key       string `json:"key"`
	Reachable bool   `json:"reachable"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

// probe 用短超时探测站点是否可达。探测失败不是错误：首页据此把签标为不可达即可。
func (s *server) probe(site Site) siteHealth {
	target := site.HealthURL
	if strings.TrimSpace(target) == "" {
		target = site.URL
	}
	started := time.Now()
	result := siteHealth{Key: site.Key}
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	response, err := s.client.Do(request)
	result.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = "无法连接"
		return result
	}
	defer func() { _ = response.Body.Close() }()
	// 先读掉少量响应体，让连接能被复用；内容本身不需要。
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	result.Status = response.StatusCode
	result.Reachable = response.StatusCode < 500
	return result
}

// serveHealth 并发探测全部站点，避免站点多时串行等待放大首页加载时间。
func (s *server) serveHealth(writer http.ResponseWriter, _ *http.Request) {
	results := make([]siteHealth, len(s.config.Sites))
	var group sync.WaitGroup
	for index, site := range s.config.Sites {
		group.Add(1)
		go func(index int, site Site) {
			defer group.Done()
			results[index] = s.probe(site)
		}(index, site)
	}
	group.Wait()
	writeJSON(writer, map[string]any{"sites": results})
}

// serveIndex 返回门户首页。页面本身不含站点数据，站点清单由前端读 /api/sites 渲染，
// 这样新增站点不需要重新编译二进制。
func (s *server) serveIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	page, err := pageFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(writer, "门户页面加载失败", http.StatusInternalServerError)
		return
	}
	setPageHeaders(writer)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write(page)
}

// serveConfig 返回门户标题与站点清单（不含健康状态），供前端一次拿到全部静态信息。
func (s *server) serveConfig(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, s.config)
}

func writeJSON(writer http.ResponseWriter, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(payload)
}

// setPageHeaders 给页面与静态资源下发安全头。门户只加载自己的脚本样式，不允许被嵌入。
func setPageHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self'; style-src 'self'; "+
			"img-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
	writer.Header().Set("X-Frame-Options", "DENY")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Referrer-Policy", "no-referrer")
}

// staticAssets 提供 web/ 下的脚本与样式，路径外的文件一律 404。
func staticAssets() http.Handler {
	assets, err := fs.Sub(pageFiles, "web")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setPageHeaders(writer)
		writer.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(writer, request)
	})
}

// serveEnv 返回环境池概况与逐环境状态，数据来自 cwe 的 state.json。
func (s *server) serveEnv(writer http.ResponseWriter, request *http.Request) {
	if s.cwe == nil || !s.cwe.Enabled {
		http.NotFound(writer, request)
		return
	}
	page, pageSize := 1, defaultPageSize
	if raw := request.URL.Query().Get("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			page = parsed
		}
	}
	if raw := request.URL.Query().Get("page_size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			pageSize = parsed
		}
	}
	writeJSON(writer, loadEnvPool(s.cwe.SSHHost, s.cwe.EnvHost, s.cwe.StatePath, page, pageSize))
}

// serveRobot 返回机器人当前状态与设置，供机器人页渲染。
func (s *server) serveRobot(writer http.ResponseWriter, request *http.Request) {
	if s.robot == nil {
		http.NotFound(writer, request)
		return
	}
	writeJSON(writer, map[string]any{
		"state":         s.robot.snapshot(),
		"settings":      s.robot.currentSettings(),
		"availableSuites": knownSuites,
		"nextRunAt":     s.robot.nextRunAt(),
		"cwtUrl":        s.robot.config.CWTURL,
		"timezone":      s.robot.config.Timezone,
	})
}

// serveRobotSettings 读取或保存机器人设置。GET 返回当前值，PUT 校验后落盘并立即生效。
func (s *server) serveRobotSettings(writer http.ResponseWriter, request *http.Request) {
	if s.robot == nil {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodPut {
		writer.Header().Set("Allow", http.MethodPut)
		http.Error(writer, "只接受 PUT", http.StatusMethodNotAllowed)
		return
	}
	var incoming robotSettings
	if err := json.NewDecoder(io.LimitReader(request.Body, 1<<16)).Decode(&incoming); err != nil {
		http.Error(writer, "请求体不是合法 JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := incoming.validate(); err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	if err := saveSettings(s.robot.config, incoming); err != nil {
		http.Error(writer, "保存失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.robot.applySettings(incoming)
	writeJSON(writer, map[string]any{
		"settings":   s.robot.currentSettings(),
		"nextRunAt":  s.robot.nextRunAt(),
		"saved":      true,
	})
}

// serveRobotHistory 返回最近的执行历史，最新的在前。
func (s *server) serveRobotHistory(writer http.ResponseWriter, request *http.Request) {
	if s.robot == nil {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "只接受 GET", http.StatusMethodNotAllowed)
		return
	}
	page, pageSize := 1, defaultPageSize
	if raw := request.URL.Query().Get("page"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			page = parsed
		}
	}
	if raw := request.URL.Query().Get("page_size"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			pageSize = parsed
		}
	}
	items, total := s.robot.loadHistoryPage(page, pageSize)
	normPage, normSize := normalizePage(page, pageSize)
	writeJSON(writer, map[string]any{
		"history":  items,
		"total":    total,
		"page":     normPage,
		"pageSize": normSize,
	})
}

// serveRobotRun 手动触发一次巡检。POST 才接受，触发后立即返回，进度靠轮询 /api/robot。
func (s *server) serveRobotRun(writer http.ResponseWriter, request *http.Request) {
	if s.robot == nil {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "只接受 POST", http.StatusMethodNotAllowed)
		return
	}
	started, state := s.robot.Trigger()
	if !started {
		writer.WriteHeader(http.StatusConflict)
		writeJSON(writer, map[string]any{
			"started": false,
			"state":   state,
			"message": "已有批次在执行",
		})
		return
	}
	writeJSON(writer, map[string]any{"started": true, "state": state})
}

// newMux 组装路由。静态资源挂在 /assets/ 下，避免和将来可能新增的页面路径冲突。
func newMux(s *server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveIndex)
	mux.HandleFunc("/api/config", s.serveConfig)
	mux.HandleFunc("/api/sites", s.serveHealth)
	mux.HandleFunc("/api/robot", s.serveRobot)
	mux.HandleFunc("/api/robot/settings", s.serveRobotSettings)
	mux.HandleFunc("/api/robot/history", s.serveRobotHistory)
	mux.HandleFunc("/api/env", s.serveEnv)
	mux.HandleFunc("/api/robot/run", s.serveRobotRun)
	mux.Handle("/assets/", http.StripPrefix("/assets/", staticAssets()))
	mux.HandleFunc("/favicon.svg", func(writer http.ResponseWriter, request *http.Request) {
		setPageHeaders(writer)
		writer.Header().Set("Content-Type", "image/svg+xml")
		writer.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = writer.Write([]byte(faviconSVG))
	})
	return mux
}

func main() {
	configPath := flag.String("config", "config/sites.yaml", "站点清单路径")
	listen := flag.String("listen", "", "监听地址，留空则使用配置中的 portal.listen")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	if strings.TrimSpace(*listen) != "" {
		cfg.Portal.Listen = *listen
	}
	if strings.TrimSpace(cfg.Portal.Listen) == "" {
		cfg.Portal.Listen = "127.0.0.1:18090"
	}

	bot, err := newRobotRunner(cfg.Robot)
	if err != nil {
		log.Fatalf("机器人配置错误: %v", err)
	}
	if bot != nil {
		// 用持久化设置覆盖配置默认值，再启动调度。
		if stored, err := loadSettings(cfg.Robot); err != nil {
			log.Printf("读取机器人设置失败，使用配置默认值: %v", err)
		} else if err := stored.validate(); err == nil {
			bot.applySettings(stored)
		}
		bot.restoreLastState()
		bot.StartSchedule()
	}

	instance := &server{
		config: cfg,
		client: &http.Client{Timeout: 2500 * time.Millisecond},
		robot:  bot,
		cwe:    cfg.CWE,
	}
	address := cfg.Portal.Listen
	log.Printf("ChainWeaver 门户监听 %s，接入 %d 个站点", address, len(cfg.Sites))
	httpServer := &http.Server{
		Addr:              address,
		Handler:           newMux(instance),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}
