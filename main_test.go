package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig 在临时目录写一份清单，返回路径。用于覆盖各种合法与非法配置。
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sites.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写临时配置失败: %v", err)
	}
	return path
}

func TestLoadConfigValid(t *testing.T) {
	path := writeConfig(t, "portal:\n  title: Demo\nsites:\n  - key: a\n    label: 甲\n    url: http://127.0.0.1:1/\n")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
	if len(cfg.Sites) != 1 || cfg.Sites[0].Key != "a" {
		t.Fatalf("站点解析结果不对: %+v", cfg.Sites)
	}
}

func TestLoadConfigRejects(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"无站点", "portal:\n  title: Demo\nsites: []\n"},
		{"缺 key", "sites:\n  - label: 甲\n    url: http://x/\n"},
		{"缺 url", "sites:\n  - key: a\n    label: 甲\n"},
		{"缺 label", "sites:\n  - key: a\n    url: http://x/\n"},
		{"key 重复", "sites:\n  - key: a\n    label: 甲\n    url: http://x/\n  - key: a\n    label: 乙\n    url: http://y/\n"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := loadConfig(writeConfig(t, item.body)); err == nil {
				t.Fatalf("%s 应该报错但通过了", item.name)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	if _, err := loadConfig(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("不存在的配置文件应报错")
	}
}

// newTestServer 起一个真实子站点，让健康探测有可探目标。
func newTestServer(t *testing.T, sites []Site) *server {
	t.Helper()
	return &server{config: &Config{Sites: sites}, client: &http.Client{}}
}

func TestServeHealth(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()
	// 关闭后的端口用来模拟不可达站点。
	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	downURL := down.URL
	down.Close()

	instance := newTestServer(t, []Site{
		{Key: "up", Label: "在", URL: up.URL, HealthURL: up.URL},
		{Key: "down", Label: "挂", URL: downURL},
	})
	recorder := httptest.NewRecorder()
	instance.serveHealth(recorder, httptest.NewRequest(http.MethodGet, "/api/sites", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码应为 200，实际 %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"key":"up"`) || !strings.Contains(body, `"reachable":true`) {
		t.Fatalf("可达站点结果不对: %s", body)
	}
	if !strings.Contains(body, `"key":"down"`) || !strings.Contains(body, `"reachable":false`) {
		t.Fatalf("不可达站点结果不对: %s", body)
	}
}

func TestRoutes(t *testing.T) {
	instance := newTestServer(t, []Site{{Key: "a", Label: "甲", URL: "http://127.0.0.1:1/"}})
	mux := newMux(instance)

	checks := []struct {
		path        string
		wantStatus  int
		wantSnippet string
	}{
		{"/", http.StatusOK, "ChainWeaver"},
		{"/api/config", http.StatusOK, `"sites"`},
		{"/assets/app.js", http.StatusOK, "nav-tabs"},
		{"/assets/styles.css", http.StatusOK, "nav-tab"},
		{"/favicon.svg", http.StatusOK, "<svg"},
		{"/nope", http.StatusNotFound, ""},
	}
	for _, check := range checks {
		t.Run(check.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, check.path, nil))
			if recorder.Code != check.wantStatus {
				t.Fatalf("%s 状态码应为 %d，实际 %d", check.path, check.wantStatus, recorder.Code)
			}
			if check.wantSnippet != "" && !strings.Contains(recorder.Body.String(), check.wantSnippet) {
				t.Fatalf("%s 响应缺少 %q", check.path, check.wantSnippet)
			}
		})
	}
}

// 首页必须带禁止被嵌入的响应头，避免门户自己被第三方套壳。
func TestIndexSecurityHeaders(t *testing.T) {
	instance := newTestServer(t, []Site{{Key: "a", Label: "甲", URL: "http://127.0.0.1:1/"}})
	recorder := httptest.NewRecorder()
	newMux(instance).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options 应为 DENY，实际 %q", got)
	}
	if header := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(header, "frame-ancestors 'none'") {
		t.Fatalf("CSP 缺少 frame-ancestors: %q", header)
	}
}

// 站点文本来自配置，前端用 textContent 渲染；这里确认接口按原样返回中文，没有被转义成
// HTML 实体，否则页面上会显示成 &#x...。
func TestConfigKeepsUnicode(t *testing.T) {
	instance := newTestServer(t, []Site{{Key: "a", Label: "集成测试", URL: "http://127.0.0.1:1/"}})
	recorder := httptest.NewRecorder()
	instance.serveConfig(recorder, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if !strings.Contains(recorder.Body.String(), "集成测试") {
		t.Fatalf("中文标签被转义了: %s", recorder.Body.String())
	}
}
