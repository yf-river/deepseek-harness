# ChainWeaver 门户

把 ChainWeaver 的多个站点收敛到一个入口：顶部一排横向签，点哪个进哪个站。
当前接入 **集成测试**（chainweaver-test，18091）与 **CWF Flow**（chainweaver-flow，18092）。

**线上地址**

| 环境 | 地址 | 监听 | systemd unit |
|---|---|---|---|
| 本机（开发验证） | <http://9.134.129.162:18094/> | `9.134.129.162:18094` | `chainweaver-portal-local.service` |
| 40.247（发布） | <http://192.168.40.247:18090/> | `192.168.40.247:18090` | `chainweaver-portal.service` |

改完先在本机 `./deploy/deploy.sh local` 跑通验收，再 `./deploy/deploy.sh 40.247` 推发布机。
两份用的是同一个二进制、同一份 `config/sites.yaml`，差异只在监听地址与 unit 文件。

## 设计取舍

| 决策 | 选择 | 原因 |
|---|---|---|
| 点击签的行为 | 整页跳转到目标站点 | 两个被接入站点都下发 `frame-ancestors 'none'` + `X-Frame-Options: DENY`（`chainweaver-test/web/dashboard.go`、`chainweaver-flow/internal/web/server.go`），iframe 会被浏览器直接拒绝渲染。整页跳转零改动、零风险。 |
| 视觉体系 | 沿用 chainweaver-flow / chainweaver-test | 白底卡片、`#0052d9` 主色、8px 圆角、细边框、轻阴影；两个子站点本身就是这套语言，门户不该另起一套。 |
| 签的交互 | 参照 spec-score 的 `nav-tabs` | 同高度的「图标 + 文字」签、2.5px 底边高亮、hover 变色，点击态用 `border-bottom-color` 而非换底色。 |
| 站点数据来源 | `config/sites.yaml` 是唯一真源 | 加快捷签只改配置重启，不需要重新编译。 |
| 前端资源 | `go:embed` 打进二进制 | 部署机只需一个二进制加一份配置，不依赖前端构建链或 CDN。 |
| 健康状态 | 服务端探测后经 `/api/sites` 返回 | 浏览器直接跨源探测会被 CORS 拦，服务端探测顺便避开这个坑。 |

## 结构

```
chainweaver-portal/
├── main.go                 # 配置加载、健康探测、路由、安全头
├── favicon.go              # 内嵌 SVG 图标
├── main_test.go            # 配置校验、健康探测、路由与安全头测试
├── config/sites.yaml       # 站点清单（唯一真源）
├── web/                    # index.html / styles.css / app.js，编译期打进二进制
└── deploy/                 # deploy.sh + 两份 systemd unit（local / 40.247）
```

## 每日巡检机器人

门户自己带一个巡检机器人：每天把选中的主流程依次跑一遍，汇总结果发到指定邮箱。
它由门户进程调度，不是被接入的子站点，所以不出现在「全部站点」里，而是在顶部
「机器人」签下。

| 能力 | 实现 |
|---|---|
| 执行 | 串行调用 CWT 任务 API（`POST /api/v1/test-runs`），每 15s 轮询到终态 |
| 汇总 | 只读本批次自己的 `run_id`，不从「最新几条记录」猜归属 |
| 投递 | `wecom-cli mail send`，收件人来自设置 |
| 调度 | 进程内定时器，按 `schedule` + `schedule_timezone` 每日触发 |
| 手动 | 卡片上的「立即执行一次」，与定时走同一条执行路径 |

要点：

- **只负责跑完并如实汇总**，不做通过/失败之外的干预。某个主流程失败不中断后续流程，也不重试。
- **同一时刻只允许一个批次**。手动触发遇到在跑返回 409；定时触发遇到在跑跳过当日并记日志——
  宁可漏一天也不并发抢占 CWT 环境。
- **开跑前检查在途任务**。查询失败直接放弃本次，不把「查不到」当成「没有在途」。
- **发信失败是独立结论**，不会写成测试失败，也不会写成已送达。
- **配置回写**：`sites.yaml` 提供出厂默认值，页面改过的内容落到 `artifact_dir/settings.json`，
  重启后加载。这样 `sites.yaml` 始终保持人工可读的格式与注释。

卡片上只放四项设置，不加别的开关：主流程多选、每日时间、收件人、两个勾选（定时 / 发信）。

## 接口

| 路径 | 说明 |
|---|---|
| `/` | 门户首页：顶部横向签 + 站点卡片 |
| `/api/config` | 门户标题与站点清单（静态信息） |
| `/api/sites` | 各站点健康状态，含 `reachable` / `status` / `latencyMs` |
| `/api/robot` | 机器人状态、当前设置、下次触发时间 |
| `/api/robot/settings` | `PUT` 保存设置，校验后落盘并立即生效 |
| `/api/robot/run` | `POST` 手动触发一次；已有批次在跑时返回 409 |
| `/assets/*` | 页面脚本与样式 |
| `/favicon.svg` | 站点图标 |

## 本地开发与验证

```bash
cd /data/ida/ai/chainweaver-portal
go vet ./... && go test ./...        # 单元测试
go build -o /tmp/chainweaver-portal .
/tmp/chainweaver-portal -config config/sites.yaml -listen 127.0.0.1:18090
curl -s http://127.0.0.1:18090/api/sites
```

`-listen` 留空时使用 `config/sites.yaml` 里的 `portal.listen`。

## 新增一个站点

1. 在 `config/sites.yaml` 的 `sites` 下追加一条，`key` / `label` / `url` 必填，`icon` 可选
   `test`、`flow`、`dashboard`，其它值走默认图标。
2. 同步本仓 `config/sites.yaml` 与部署机 `/opt/chainweaver-portal/config/sites.yaml`。
3. `systemctl restart chainweaver-portal`。配置有语法错误或缺字段时进程会启动失败并打出
   具体原因，不会带残缺清单上线。

## 部署

统一走 `deploy/deploy.sh`，它会先 `go vet` + `go test`，再交叉编译、铺文件、重启、冒烟验证；
任何一步失败就退出，不会把坏版本留在目标机上。

```bash
cd /data/ida/ai/chainweaver-portal
./deploy/deploy.sh local      # 本机 9.134.129.162:18094，unit 名带 -local 后缀
./deploy/deploy.sh 40.247     # 远端 192.168.40.247:18090
```

两种目标的实际动作一致：装到 `/opt/chainweaver-portal`（二进制 + `config/sites.yaml`），
unit 落到 `/etc/systemd/system/`，`enable` + `restart`，然后对 `/`、`/api/config`、
`/api/sites`、`/assets/*`、`/favicon.svg` 逐个查 200。

unit 里都是 `Restart=on-failure`；`After=` 只引用 `network-online.target`，不 `Requires`
子站点：门户是入口，子站点没起来时它仍应能打开并把签标成「不可达」，不该跟着起不来。

## 验收记录

- `go vet` 与 `go test` 全通过（配置校验、健康探测、路由、CSP/X-Frame-Options、中文不转义）。
- **本机** `http://9.134.129.162:18094/`：`/`、`/api/config`、`/api/sites`、`/assets/app.js`、
  `/assets/styles.css`、`/favicon.svg` 全 200；`/api/sites` 两个站点均 `reachable:true`；
  Chromium 实测两个签分别进入 40.247:18091 与 40.247:18092，无 console 报错。
- **40.247** `http://192.168.40.247:18090/`：同一套验证，全部 200，未知路径 404。
- 站点卡片跳转在两处环境均实测生效。

### 机器人验收（本机）

- 真实跑通一次完整批次：提交 `ida` → 轮询到终态 → 判定 FAILED → 生成邮件正文与主题 → 演练模式不发信。
  产物在 `/opt/chainweaver-portal/run/robot/cwt-robot-819677770/`，`mail_body.md` 的表格、时间、失败原因
  与 CWT 详情接口逐项对账一致。
- 生成的主题：`[CWT] 主流程巡检 FAILED - 2026-11-23`；正文含「0 条通过，1 条失败」、
  北京时间区间、耗时 `3分12秒`、已通过验证数 `2`、看板直达链接。
- 重启服务后能恢复上次结果（`final-state.json`），不需要重跑才能看到结论。
- 部署不会覆盖 `run/robot/settings.json` 与历史批次目录。
- `go test ./...` 覆盖：判定口径、耗时/时区格式化、正文渲染（含「执行器错误」与「CWT 失败」分别表述）、
  假 CWT 的接单/轮询/失败分支、在途任务查询失败必须报错、设置校验与持久化。
