// 门户前端：站点清单与健康状态都由服务端接口提供，本脚本只负责渲染与跳转。
// 抽成独立文件而不是写在 HTML 里，是为了让 CSP 保持 script-src 'self'，不放开 inline。

const ICONS = {
  home: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <path d="M3 9.2 10 3l7 6.2V16a1 1 0 0 1-1 1h-3.6v-4.6H7.6V17H4a1 1 0 0 1-1-1V9.2Z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
  </svg>`,
  robot: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <rect x="3.5" y="6.5" width="13" height="9.5" rx="2.5" stroke="currentColor" stroke-width="1.5"/>
    <path d="M10 3v3.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
    <circle cx="10" cy="2.6" r="1.1" fill="currentColor"/>
    <circle cx="7.6" cy="10.8" r="1.15" fill="currentColor"/>
    <circle cx="12.4" cy="10.8" r="1.15" fill="currentColor"/>
    <path d="M8 13.6h4" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/>
  </svg>`,
  flow: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <rect x="2" y="3" width="6" height="5" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <rect x="12" y="12" width="6" height="5" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <path d="M8 5.5h3.5A2.5 2.5 0 0 1 14 8v4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
  </svg>`,
  test: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <path d="M7 2h6M8 2v6.2L4.4 15A1.3 1.3 0 0 0 5.5 17h9a1.3 1.3 0 0 0 1.1-2L12 8.2V2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M6.2 12.5h7.6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
  </svg>`,
  dashboard: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <rect x="2" y="10" width="4" height="8" rx="1" fill="currentColor" opacity=".5"/>
    <rect x="8" y="6" width="4" height="12" rx="1" fill="currentColor" opacity=".75"/>
    <rect x="14" y="2" width="4" height="16" rx="1" fill="currentColor"/>
  </svg>`,
  default: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <circle cx="10" cy="10" r="7" stroke="currentColor" stroke-width="1.5"/>
    <path d="M10 6v4l3 2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
  </svg>`,
}

const navTabs = document.getElementById('nav-tabs')
const siteGrid = document.getElementById('site-grid')
const loading = document.getElementById('loading')

let sites = []
// 站内视图：首页与机器人详情。机器人不在顶部签里，从首页卡片进入。
const HOME_TAB = { key: '__home__', label: '首页', icon: 'home' }
const ROBOT_VIEW = 'view-robot'
const VIEWS = { [HOME_TAB.key]: 'view-home', env: 'view-env', robot: ROBOT_VIEW }

function iconFor(name) {
  return ICONS[name] || ICONS.default
}

// 用 textContent 而非 innerHTML 写入站点文本，配置内容不会被当成 HTML 解析。
function el(tag, className, text) {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

// go 统一在一个地方打开站点：用新标签页打开，门户本身保留在原来那页，
// 这样从站点退回来还能继续选别的入口。noopener 防止目标页拿到 window.opener。
function go(site) {
  if (!site || !site.url) return false
  window.open(site.url, '_blank', 'noopener')
  return false
}

function navIconSpan(site) {
  const span = el('span', 'nav-tab-icon')
  span.innerHTML = iconFor(site.icon)
  return span
}

// selectTab 把某一个签置为当前态，并切换到对应视图。站内视图才有 view 元素。
function selectTab(key) {
  navTabs.querySelectorAll('.nav-tab').forEach((node) => {
    const active = node.dataset.key === key
    node.classList.toggle('active', active)
    node.setAttribute('aria-selected', active ? 'true' : 'false')
  })
  Object.entries(VIEWS).forEach(([viewKey, id]) => {
    const node = document.getElementById(id)
    if (node) node.hidden = viewKey !== key
  })

  // 把当前视图写进 hash：刷新停在同一页，前进/后退也能回到对应视图。
  const hash = key === HOME_TAB.key ? '' : '#/' + key
  const target = hash || location.pathname
  if (location.hash !== hash && location.pathname + location.hash !== target) {
    history.pushState(null, '', target)
  }
}


// ── 路由 ──
// 视图状态放在 hash 里：刷新、收藏、前进后退都停在同一个页面。
function currentRoute() {
  const raw = location.hash.replace(/^#\/?/, '')
  if (raw === 'env') return { view: 'env', env: true }
  if (raw === 'robot') return { view: 'robot', robot: true }
  return { view: HOME_TAB.key, robot: false }
}

// applyRoute 按当前 URL 同步选中态与视图，不写回 hash（避免循环）。
function applyRoute() {
  const route = currentRoute()
  navTabs.querySelectorAll('.nav-tab').forEach((node) => {
    const active = node.dataset.key === route.view
    node.classList.toggle('active', active)
    node.setAttribute('aria-selected', active ? 'true' : 'false')
  })
  Object.entries(VIEWS).forEach(([viewKey, id]) => {
    const node = document.getElementById(id)
    if (node) node.hidden = viewKey !== route.view
  })
  if (route.robot) {
    loadRobot()
    loadHistory()
  }
  if (route.env) loadEnv()
}

window.addEventListener('hashchange', applyRoute)

function renderTabs() {
  navTabs.replaceChildren()

  const home = el('button', 'nav-tab')
  home.type = 'button'
  home.setAttribute('role', 'tab')
  home.dataset.key = HOME_TAB.key
  home.title = '首页'
  home.appendChild(navIconSpan(HOME_TAB))
  home.appendChild(el('span', 'nav-tab-label', HOME_TAB.label))
  // 已经在首页时点首页签不跳转，只把选中态落回首页。
  home.addEventListener('click', () => selectTab(HOME_TAB.key))
  navTabs.appendChild(home)

  sites.forEach((site) => {
    // 用 <a> 而不是 button：新标签页打开是链接的原生行为，不受浏览器弹窗拦截影响，
    // 中键和右键"在新标签打开"也自动可用。
    const tab = el('a', 'nav-tab')
    tab.href = site.url
    tab.target = '_blank'
    tab.rel = 'noopener'
    tab.dataset.key = site.key
    tab.title = `${site.title} · ${site.url}`
    tab.appendChild(navIconSpan(site))
    tab.appendChild(el('span', 'nav-tab-label', site.label))
    navTabs.appendChild(tab)
  })

  // 这里只负责生成签，视图由 applyRoute 按 URL 决定，不在渲染时写路由。
}

function statusBadge(health) {
  if (!health) {
    const badge = el('span', 'status status-unknown', '检测中')
    return badge
  }
  if (health.reachable) {
    const label = health.latencyMs > 0 ? `可访问 · ${health.latencyMs}ms` : '可访问'
    const badge = el('span', 'status status-up', label)
    badge.title = `HTTP ${health.status} · ${health.latencyMs}ms`
    return badge
  }
  const badge = el('span', 'status status-down', '不可达')
  badge.title = health.error || `HTTP ${health.status}`
  return badge
}

function renderCards(healthByKey) {
  siteGrid.replaceChildren()
  sites.forEach((site) => {
    const card = el('a', 'site-card')
    card.href = site.url
    card.target = '_blank'
    card.rel = 'noopener'
    card.dataset.key = site.key
    card.title = `进入 ${site.title}`

    const head = el('div', 'site-card-head')
    const iconBox = el('div', 'site-card-icon')
    iconBox.innerHTML = iconFor(site.icon)
    head.appendChild(iconBox)

    const heading = el('div', 'site-card-heading')
    heading.appendChild(el('h3', '', site.title))
    heading.appendChild(el('p', '', site.subtitle || site.label))
    head.appendChild(heading)
    head.appendChild(statusBadge(healthByKey[site.key]))
    card.appendChild(head)

    card.appendChild(el('p', 'site-card-desc', site.description || ''))

    if (site.tags && site.tags.length) {
      const tags = el('ul', 'site-card-tags')
      site.tags.forEach((tag) => tags.appendChild(el('li', '', tag)))
      card.appendChild(tags)
    }

    const foot = el('div', 'site-card-foot')
    foot.appendChild(el('span', 'site-url', site.url))
    const goLabel = el('span', 'site-go', '进入 →')
    foot.appendChild(goLabel)
    card.appendChild(foot)

    siteGrid.appendChild(card)
  })
}

// 健康探测放在渲染之后异步补齐，先出内容再出状态，避免首页被一次探测拖慢。
async function loadHealth() {
  try {
    const response = await fetch('/api/sites', { cache: 'no-store' })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const payload = await response.json()
    const healthByKey = {}
    ;(payload.sites || []).forEach((item) => {
      healthByKey[item.key] = item
    })
    renderCards(healthByKey)
  } catch (error) {
    const down = {}
    sites.forEach((site) => {
      down[site.key] = { reachable: false, status: 0, latencyMs: 0, error: String(error) }
    })
    renderCards(down)
  }
}


// ─────────────── ChainWeaver Env 环境管理 ───────────────
// 分页状态：两个列表各自独立，每页固定 100 条。
// 必须在 loadEnv 首次调用之前初始化，否则会命中 let 的暂时性死区。
const PAGE_SIZE = 100
let envOpsPage = 1
let historyPage = 1
// 环境池轮询：cwe 读一次要 1.4s（走 ssh 解析 514KB state.json），
// 环境池变化也不快，所以 60 秒一次；页面不可见时暂停，避免后台空转。
const ENV_POLL_MS = 60000
let envPollTimer = null
let envLastUpdated = null

const envEl = {
  entry: document.getElementById('env-entry'),
  entrySub: document.getElementById('env-entry-sub'),
  entryStatus: document.getElementById('env-entry-status'),
  entryMeta: document.getElementById('env-entry-meta'),
  updated: document.getElementById('env-updated'),
  total: document.getElementById('env-total'),
  idle: document.getElementById('env-idle'),
  leased: document.getElementById('env-leased'),
  unhealthy: document.getElementById('env-unhealthy'),
  busy: document.getElementById('env-busy'),
  list: document.getElementById('env-list'),
  empty: document.getElementById('env-empty'),
  error: document.getElementById('env-error'),
  refresh: document.getElementById('env-refresh'),
  opsList: document.getElementById('env-ops-list'),
  opsCount: document.getElementById('env-ops-count'),
  opsEmpty: document.getElementById('env-ops-empty'),
  back: document.getElementById('env-back'),
}

// renderEnvPool 渲染环境池统计与逐环境卡片。
function renderEnvPool(pool) {
  if (pool.error) {
    envEl.error.hidden = false
    envEl.error.textContent = pool.error
    envEl.list.replaceChildren()
    envEl.empty.hidden = true
    return
  }
  envEl.error.hidden = true
  envEl.total.textContent = String(pool.total || 0)
  envEl.idle.textContent = String(pool.idle || 0)
  envEl.leased.textContent = String(pool.leased || 0)
  envEl.busy.textContent = String(pool.busy || 0)
  envEl.unhealthy.textContent = String(pool.unhealthy || 0)

  const items = pool.environments || []
  envEl.empty.hidden = items.length > 0
  envEl.list.replaceChildren()

  items.forEach((env) => {
    const row = el('div', 'env-row')

    const head = el('div', 'env-row-head')
    head.appendChild(el('span', 'env-name', env.name))
    // 状态用带圆点的小徽标，独立占位在名称右侧；名称 flex 撑开，两者之间自然留白。
    let state = 'ready'
    let text = '就绪'
    if (env.status === 'restoring') {
      state = 'run'
      text = '释放中'
    } else if (env.lease) {
      state = 'leased'
      text = '已租用'
    } else if (!env.ready) {
      state = 'error'
      text = '异常'
    }
    head.appendChild(envPill(state, text))
    row.appendChild(head)

    // 第一行元信息：版本、数据库、基线
    const meta = el('div', 'env-row-meta')
    if (env.version) meta.appendChild(el('span', 'env-bit', env.version))
    if (env.database) meta.appendChild(el('span', 'env-bit', env.database))
    if (env.branch) meta.appendChild(el('span', 'env-bit', '基线 ' + env.branch))
    // 模块：显示「模块名 · 命名空间」而不是只给数量，数量本身说明不了任何事
    Object.entries(env.moduleNamespaces || {}).forEach(([name, ns]) => {
      meta.appendChild(el('span', 'env-bit is-key', `${name} · ${ns}`))
    })
    row.appendChild(meta)

    // 第二行：命名空间列表。一个环境通常跨 base/public/ida/front 多个 namespace，
    // 列全了才能判断某个服务落在哪。
    if (env.namespaces && env.namespaces.length) {
      const nsRow = el('div', 'env-row-ns')
      nsRow.appendChild(el('span', 'env-ns-label', '命名空间'))
      env.namespaces.forEach((ns) => nsRow.appendChild(el('span', 'env-ns', ns)))
      row.appendChild(nsRow)
    }

    // 第三行：服务器地址与前端链接
    const addr = el('div', 'env-row-addr')
    addr.appendChild(el('span', 'env-addr-label', '服务器'))
    addr.appendChild(el('span', 'env-addr', hostOf(env)))
    if (env.uiUrl) {
      addr.appendChild(el('span', 'env-addr-label', '前端'))
      // 链接文字直接给完整地址，不需要再加一个重复的端口
      const link = el('a', 'env-link', env.uiUrl)
      link.href = env.uiUrl
      link.target = '_blank'
      link.rel = 'noopener'
      link.title = `打开前端（端口 ${env.uiPort}）`
      addr.appendChild(link)
    }
    if (env.ports && env.ports.database) {
      addr.appendChild(el('span', 'env-addr-label', '数据库'))
      addr.appendChild(el('span', 'env-addr', `:${env.ports.database}`))
    }
    row.appendChild(addr)

    if (env.lease) {
      const lease = el('div', 'env-lease')
      lease.appendChild(el('span', '', '租约 ' + shortID(env.lease.id)))
      if (env.lease.owner) lease.appendChild(el('span', '', '占用 ' + env.lease.owner))
      if (env.lease.expiresAt) lease.appendChild(el('span', '', '到期 ' + beijingTime(env.lease.expiresAt)))
      row.appendChild(lease)
    }

    envEl.list.appendChild(row)
  })

  // 首页卡片同步显示概况，不必进详情就能看出池子占用。
  if (envEl.entryStatus) {
    envEl.entryStatus.className = 'status ' + (pool.leased > 0 ? 'status-down' : 'status-up')
    envEl.entryStatus.textContent = `${pool.leased || 0}/${pool.total || 0} 占用`
  }
  if (envEl.entrySub) {
    envEl.entrySub.textContent = pool.unhealthy > 0
      ? `${pool.unhealthy} 个环境异常`
      : `${pool.idle || 0} 个空闲 · ${pool.leased || 0} 个已租用`
  }
  renderOperations(pool)
}

const OP_KIND_TEXT = { deploy: '部署', free: '释放', destroy: '销毁' }
const OP_STATUS_TEXT = { queued: '排队中', running: '执行中', succeeded: '成功', failed: '失败' }

// renderOperations 渲染 cwe 的部署/释放任务记录，最新的在前。
function renderOperations(pool) {
  if (!envEl.opsList) return
  const items = pool.recentOperations || []
  envEl.opsEmpty.hidden = items.length > 0
  renderPager('env-ops', pool.operationsTotal || 0, pool.operationsPage || 1)
  envEl.opsList.replaceChildren()

  items.forEach((op) => {
    const row = el('div', 'env-op')

    const head = el('div', 'env-op-head')
    head.appendChild(el('span', 'env-op-kind', OP_KIND_TEXT[op.kind] || op.kind || '-'))
    head.appendChild(el('span', 'env-op-env', op.environment || '-'))
    head.appendChild(opPill(op.status))
    head.appendChild(el('span', 'env-op-time', beijingTime(op.started_at || op.created_at) || '-'))
    row.appendChild(head)

    // cwe 的 message 只是当前阶段的通用描述（这批全是「正在恢复环境基线」），
    // 对已结束的记录没有信息量，所以只在执行中显示。
    if (op.status === 'running' || op.status === 'queued') {
      const detail = op.message || (op.step ? `第 ${op.step} 阶段` : '')
      if (detail) row.appendChild(el('p', 'env-op-msg', detail))
    }
    // 失败要给出真正的原因：cwe 把它放在 error.message，
    // 处理建议在 error.next_action，而不是那句通用的 message。
    if (op.status === 'failed' && op.error) {
      row.appendChild(el('p', 'env-op-msg is-error', op.error.message || '执行失败'))
      if (op.error.nextAction) {
        row.appendChild(el('p', 'env-op-msg is-hint', op.error.nextAction))
      }
    }
    envEl.opsList.appendChild(row)
  })
}


// renderPager 统一渲染分页控件。prefix 对应 env-ops / history 两组 id。
// 只有一页时隐藏控件，不占位。
function renderPager(prefix, total, page) {
  const box = document.getElementById(prefix + '-pager')
  if (!box) return
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  box.hidden = total === 0
  const info = document.getElementById(prefix + '-count')
  if (info) info.textContent = `${total} 条`
  const label = document.getElementById(prefix + '-page')
  if (label) label.textContent = `${page} / ${pages}`
  const prev = document.getElementById(prefix + '-prev')
  const next = document.getElementById(prefix + '-next')
  if (prev) prev.disabled = page <= 1
  if (next) next.disabled = page >= pages
  // 只有一页时整块不显示，避免一个无意义的 1/1
  box.hidden = pages <= 1
}

// bindPager 把页码变化交给各页自己的加载函数。
function bindPager(prefix, onChange) {
  const prev = document.getElementById(prefix + '-prev')
  const next = document.getElementById(prefix + '-next')
  if (prev) prev.addEventListener('click', () => onChange(-1))
  if (next) next.addEventListener('click', () => onChange(1))
}


// envPill / opPill 是环境与操作记录的状态徽标：一个圆点 + 文字，
// 比原来的纯色块更轻，也能和名称、时间拉出稳定的间距。
function dotPill(tone, text, title) {
  const pill = el('span', `pill is-${tone}`)
  pill.appendChild(el('span', 'pill-dot'))
  pill.appendChild(el('span', 'pill-text', text))
  if (title) pill.title = title
  return pill
}

function envPill(state, text) {
  // ready 绿、leased 琥珀（被占但不是故障）、run 蓝（处理中）、error 红
  const tone = state === 'ready' ? 'ok'
    : state === 'leased' ? 'busy'
    : state === 'run' ? 'run' : 'bad'
  return dotPill(tone, text)
}

function opPill(status) {
  const tone = status === 'succeeded' ? 'ok'
    : status === 'failed' ? 'bad'
    : status === 'running' ? 'run' : 'idle'
  return dotPill(tone, OP_STATUS_TEXT[status] || status || '-')
}


// renderEnvUpdated 把上次更新时间写到卡片与详情页的刷新按钮旁。
// 相对时间比绝对时间更有用：一眼能判断数据有多旧。
function relativeTime(date) {
  const seconds = Math.max(0, Math.round((Date.now() - date.getTime()) / 1000))
  if (seconds < 10) return '刚刚'
  if (seconds < 60) return `${seconds} 秒前`
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.round(minutes / 60)
  return `${hours} 小时前`
}

function renderEnvUpdated() {
  if (!envLastUpdated) return
  const text = relativeTime(envLastUpdated)
  if (envEl.entryMeta) envEl.entryMeta.textContent = `更新于 ${text}`
  if (envEl.updated) envEl.updated.textContent = `更新于 ${text}`
}

// 相对时间会过时，每 30 秒刷新一次文案（纯本地计算，不发请求）。
function startRelativeClock() {
  setInterval(renderEnvUpdated, 30000)
}

// startEnvPoll / stopEnvPoll 控制轮询。页面隐藏时停，回来时立刻补一次再继续，
// 这样切回标签页看到的是最新数据，而不是最多等满 60 秒。
function startEnvPoll() {
  if (envPollTimer) return
  envPollTimer = setInterval(() => { loadEnv() }, ENV_POLL_MS)
}

function stopEnvPoll() {
  if (!envPollTimer) return
  clearInterval(envPollTimer)
  envPollTimer = null
}

document.addEventListener('visibilitychange', () => {
  if (document.hidden) {
    stopEnvPoll()
  } else {
    loadEnv()
    startEnvPoll()
  }
})

// hostOf 取出环境所在宿主机，用于展示"服务器"一栏。
function hostOf(env) {
  if (env.uiUrl) {
    const match = /^https?:\/\/([^:/]+)/.exec(env.uiUrl)
    if (match) return match[1]
  }
  return '-'
}

function shortID(value) {
  return value && value.length > 8 ? value.slice(0, 8) : value || ''
}

async function loadEnv() {
  if (!envEl.list) return
  try {
    const response = await fetch(`/api/env?page=${envOpsPage}&page_size=${PAGE_SIZE}`, { cache: 'no-store' })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    renderEnvPool(await response.json())
    envLastUpdated = new Date()
    renderEnvUpdated()
  } catch (error) {
    envEl.error.hidden = false
    envEl.error.textContent = `读取环境池失败：${error.message || error}`
  }
}

if (envEl.entry) {
  envEl.entry.addEventListener('click', () => selectTab('env'))
  envEl.refresh.addEventListener('click', loadEnv)
  bindPager('env-ops', (delta) => {
    envOpsPage = Math.max(1, envOpsPage + delta)
    loadEnv()
  })
  envEl.back.addEventListener('click', () => {
    history.pushState(null, '', location.pathname)
    applyRoute()
  })
  // 首页卡片要显示池子占用，启动时就拉一次，之后按 60 秒轮询。
  loadEnv()
  startEnvPoll()
  startRelativeClock()
}

// ─────────────────────────── 机器人页 ───────────────────────────

const robotEl = {
  state: document.getElementById('robot-state'),
  summary: document.getElementById('robot-summary'),
  suites: document.getElementById('robot-suites'),
  schedule: document.getElementById('robot-schedule'),
  hour: document.getElementById('robot-hour'),
  minute: document.getElementById('robot-minute'),
  next: document.getElementById('robot-next'),
  mailto: document.getElementById('robot-mailto'),
  mailChips: document.getElementById('robot-mailto-chips'),
  enabled: document.getElementById('robot-enabled'),
  sendmail: document.getElementById('robot-sendmail'),
  save: document.getElementById('robot-save'),
  run: document.getElementById('robot-run'),
  entry: document.getElementById('robot-entry'),
  entryDesc: document.getElementById('robot-entry-desc'),
  entryStatus: document.getElementById('robot-entry-status'),
  entrySub: document.getElementById('robot-entry-sub'),
  entryMeta: document.getElementById('robot-entry-meta'),
  back: document.getElementById('robot-back'),
}

let robotAvailable = false
let robotPollTimer = null
// 收件人列表。空输入框 + 胶囊展示，提交时直接取这个数组。
let mailRecipients = []

// beijingTime 把后端给的 RFC3339 显示成北京时间，卡片里只到分钟。
function beijingTime(raw) {
  if (!raw) return ''
  const parsed = new Date(raw)
  if (Number.isNaN(parsed.getTime())) return ''
  const parts = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai', hour12: false,
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).formatToParts(parsed)
  const pick = (type) => (parts.find((p) => p.type === type) || {}).value || ''
  return `${pick('month')}-${pick('day')} ${pick('hour')}:${pick('minute')}`
}

// suiteCheckboxes 渲染主流程多选。用复选而不是下拉，三个主流程一眼看得全。
// suiteCheckboxes 渲染主流程勾选块。用 button + aria-pressed 而不是原生 checkbox：
// 原生方块在胶囊里视觉冲突明显，自绘勾选态更贴近整页语言。
function suiteCheckboxes(available, selected) {
  robotEl.suites.replaceChildren()
  available.forEach((suite) => {
    const chip = el('button', 'suite-chip')
    chip.type = 'button'
    chip.dataset.suite = suite.id
    chip.setAttribute('aria-pressed', selected.includes(suite.id) ? 'true' : 'false')
    chip.appendChild(el('span', 'suite-chip-mark'))
    chip.appendChild(el('span', 'suite-chip-name', suite.name))
    chip.addEventListener('click', () => {
      const on = chip.getAttribute('aria-pressed') === 'true'
      chip.setAttribute('aria-pressed', on ? 'false' : 'true')
    })
    robotEl.suites.appendChild(chip)
  })
}

function renderRobotState(payload) {
  const state = payload.state || {}
  const settings = payload.settings || {}

  const running = state.status === 'running'
  robotEl.state.textContent = running ? '执行中' : '空闲'
  robotEl.state.className = 'robot-state' + (running ? ' is-running' : '')

  if (running) {
    robotEl.summary.textContent = state.message || '正在执行…'
  } else if (state.verdict) {
    robotEl.summary.textContent = `上次结果：${state.verdict}${state.endedAt ? ' · ' + beijingTime(state.endedAt) : ''}`
  } else {
    robotEl.summary.textContent = '每天把选中的主流程依次跑一遍，结果发到指定邮箱。'
  }

  // 设置只在非执行中回填，避免轮询覆盖用户正在改的内容。
  if (!running && payload.availableSuites) {
    suiteCheckboxes(payload.availableSuites, settings.suites || [])
    setScheduleInput(settings.schedule || '')
    mailRecipients = (settings.mailTo || []).slice()
    renderMailChips()
    robotEl.enabled.checked = !!settings.enabled
    robotEl.sendmail.checked = !!settings.sendMail
  }

  robotEl.next.textContent = payload.nextRunAt ? '下次 ' + beijingTime(payload.nextRunAt) : ''
  robotEl.run.disabled = running
  robotEl.run.textContent = running ? '执行中…' : '立即执行一次'

  // 首页卡片同步显示同一份状态，用户不进详情也能看到上次结果。
  if (robotEl.entryStatus) {
    // 与站点卡片共用 .status 徽标：绿=正常、红=失败、灰=未执行。
    const tone = running || state.verdict === 'PASS'
      ? 'status status-up'
      : state.verdict ? 'status status-down' : 'status status-unknown'
    robotEl.entryStatus.className = tone
    robotEl.entryStatus.textContent = running ? '执行中' : state.verdict || '未执行'
    robotEl.entryStatus.title = state.endedAt ? `上次 ${beijingTime(state.endedAt)}` : ''
  }
  if (robotEl.entrySub) {
    const suites = (settings.suites || []).map((id) => id.toUpperCase()).join(' · ')
    const when = settings.enabled ? `每天 ${settings.schedule || '—'}` : '定时未启用'
    robotEl.entrySub.textContent = suites ? `${suites} · ${when}` : '每天定时执行'
  }
  if (robotEl.entryDesc) {
    robotEl.entryDesc.textContent = running
      ? (state.message || '正在执行…')
      : '每天把选中的主流程依次跑一遍，结果发到指定邮箱。'
  }
  if (robotEl.entryMeta) {
    if (running) {
      robotEl.entryMeta.textContent = '正在执行'
    } else if (state.verdict) {
      const tail = state.endedAt ? ` · ${beijingTime(state.endedAt)}` : ''
      robotEl.entryMeta.textContent = `上次结果 ${state.verdict}${tail}`
    } else {
      robotEl.entryMeta.textContent = '尚未执行'
    }
  }

  // 执行中持续轮询，结束后自动停。
  if (running && !robotPollTimer) {
    robotPollTimer = setInterval(loadRobot, 5000)
  } else if (!running && robotPollTimer) {
    clearInterval(robotPollTimer)
    robotPollTimer = null
    // 批次刚结束时刷新一次执行记录，不用手动再点。
    loadHistory()
  }
}




// toast 承接保存/触发这类短反馈，卡片里不再留一块常驻的结果区。
let toastTimer = null
function toast(message, isError) {
  let node = document.getElementById('app-toast')
  if (!node) {
    node = el('div', 'app-toast')
    node.id = 'app-toast'
    node.setAttribute('role', 'status')
    document.body.appendChild(node)
  }
  node.textContent = message
  node.className = 'app-toast' + (isError ? ' is-error' : '') + ' is-visible'
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { node.className = 'app-toast' }, 2600)
}

// ── 时间输入 ──
// 两个输入框拼成 HH:MM。输入过程中自动补齐与夹取范围，失焦时归一化。
function pad2(value) {
  return String(value).padStart(2, '0')
}

function clampPart(raw, max) {
  const digits = String(raw).replace(/\D/g, '').slice(0, 2)
  if (!digits) return ''
  const number = Math.min(Number(digits), max)
  return pad2(number)
}

function setScheduleInput(value) {
  const match = /^(\d{1,2}):(\d{1,2})$/.exec(String(value || '').trim())
  if (!match) {
    robotEl.hour.value = '08'
    robotEl.minute.value = '00'
    return
  }
  robotEl.hour.value = pad2(Math.min(Number(match[1]), 23))
  robotEl.minute.value = pad2(Math.min(Number(match[2]), 59))
}

function readScheduleInput() {
  const hour = clampPart(robotEl.hour.value, 23) || '00'
  const minute = clampPart(robotEl.minute.value, 59) || '00'
  return `${hour}:${minute}`
}

// 输入即校验：只留数字、最多两位、超范围夹到边界；离开时补零。
function bindTimeInput(input, max) {
  input.addEventListener('input', () => {
    const cleaned = String(input.value).replace(/\D/g, '').slice(0, 2)
    input.value = cleaned
  })
  input.addEventListener('blur', () => {
    input.value = clampPart(input.value, max) || '00'
  })
  input.addEventListener('keydown', (event) => {
    if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
    event.preventDefault()
    const current = Number(clampPart(input.value, max) || 0)
    const next = event.key === 'ArrowUp' ? current + 1 : current - 1
    input.value = pad2(Math.max(0, Math.min(next, max)))
  })
}

// ── 收件人胶囊 ──
// 邮箱以胶囊列出，输入框负责新增：回车 / 逗号 / 分号提交，退格在空输入时删掉最后一个。
const EMAIL_RE = /^[^@\s]+@[^@\s]+\.[^@\s]+$/

function renderMailChips() {
  robotEl.mailChips.replaceChildren()
  mailRecipients.forEach((address, index) => {
    const chip = el('span', 'mail-chip')
    chip.appendChild(el('span', 'mail-chip-text', address))
    const remove = el('button', 'mail-chip-remove', '×')
    remove.type = 'button'
    remove.setAttribute('aria-label', `移除 ${address}`)
    remove.addEventListener('click', () => {
      mailRecipients.splice(index, 1)
      renderMailChips()
    })
    chip.appendChild(remove)
    robotEl.mailChips.appendChild(chip)
  })
  // 已有收件人时输入框收窄，和胶囊同排；没有时占满一行。
  robotEl.mailto.placeholder = mailRecipients.length ? '继续添加…' : '输入邮箱后回车，可加多个'
}

// commitMailInput 把输入框里的内容（可能一次粘多个，用逗号/空格分隔）加进列表。
function commitMailInput() {
  const raw = robotEl.mailto.value
  if (!raw.trim()) return 0
  const parts = raw.split(/[,;\s]+/).map((t) => t.trim()).filter(Boolean)
  let added = 0
  parts.forEach((address) => {
    if (!EMAIL_RE.test(address)) return
    if (mailRecipients.some((existing) => existing.toLowerCase() === address.toLowerCase())) return
    mailRecipients.push(address)
    added++
  })
  renderMailChips()
  robotEl.mailto.value = ''
  return added
}


// ── 执行记录 ──
const histEl = {
  list: document.getElementById('history-list'),
  count: document.getElementById('history-count'),
  empty: document.getElementById('history-empty'),
}

let historyOpen = null


const VERDICT_CLASS = { PASS: 'is-pass', FAILED: 'is-fail', PARTIAL: 'is-partial' }

function verdictClass(verdict) {
  return VERDICT_CLASS[verdict] || 'is-none'
}

// verdictTone 把批次结论映射到圆点徽标的色调。
function verdictTone(verdict) {
  if (verdict === 'PASS') return 'ok'
  if (verdict === 'FAILED') return 'bad'
  if (verdict === 'PARTIAL') return 'busy'
  return 'idle'
}

const STATUS_TEXT = { passed: '通过', failed: '失败', error: '未完成', running: '执行中', queued: '排队中' }

// renderHistory 渲染批次列表。一次只展开一条，避免长页面来回滚。
function renderHistory(items) {
  histEl.list.replaceChildren()
  histEl.empty.hidden = items.length > 0

  items.forEach((item) => {
    const expanded = historyOpen === item.dir
    const row = el('button', 'history-row')
    row.type = 'button'
    row.setAttribute('aria-expanded', expanded ? 'true' : 'false')

    row.appendChild(el('time', '', beijingTime(item.endedAt || item.startedAt) || '—'))

    // 与环境页同一套圆点徽标，避免两种风格并存。
    const verdict = dotPill(verdictTone(item.verdict), item.verdict || '未完成')
    row.appendChild(verdict)

    const suites = el('span', 'history-suites')
    ;(item.suites || []).forEach((suite) => {
      const one = el('span', `history-suite is-${suite.status}`)
      one.appendChild(el('b', '', suite.id.toUpperCase()))
      one.appendChild(document.createTextNode(` ${STATUS_TEXT[suite.status] || suite.status}`))
      if (suite.durationText) one.appendChild(document.createTextNode(` ${suite.durationText}`))
      suites.appendChild(one)
    })
    row.appendChild(suites)
    row.appendChild(el('span', 'history-mail', item.mailStatus || ''))

    row.addEventListener('click', () => {
      historyOpen = expanded ? null : item.dir
      loadHistory()
    })
    histEl.list.appendChild(row)

    if (expanded) {
      const detail = el('div', 'history-detail')
      const dl = document.createElement('dl')
      const add = (label, value) => {
        dl.appendChild(el('dt', '', label))
        dl.appendChild(el('dd', '', value || '—'))
      }
      add('批次', item.dir)
      add('触发', item.trigger === 'schedule' ? '定时' : '手动')
      add('邮件', item.mailStatus || '—')
      ;(item.suites || []).forEach((suite) => {
        const name = suite.id.toUpperCase()
        const detailText = suite.error || suite.failure || ''
        add(name, `${STATUS_TEXT[suite.status] || suite.status}${suite.durationText ? ' · ' + suite.durationText : ''}${suite.validated ? ' · ' + suite.validated + ' 项验证' : ''}`)
        if (detailText) add('', detailText)
      })
      detail.appendChild(dl)
      // 直达 CWT 报告的链接放在明细里，列表行本身保持干净。
      const links = (item.suites || []).filter((x) => x.detailUrl)
      if (links.length) {
        const p = el('p', 'history-detail-links')
        links.forEach((suite) => {
          const a = el('a', '', `${suite.id.toUpperCase()} 报告 →`)
          a.href = suite.detailUrl
          a.target = '_blank'
          a.rel = 'noopener'
          p.appendChild(a)
        })
        detail.appendChild(p)
      }
      histEl.list.appendChild(detail)
    }
  })
}

async function loadHistory() {
  if (!histEl.list) return
  try {
    const response = await fetch(`/api/robot/history?page=${historyPage}&page_size=${PAGE_SIZE}`, { cache: 'no-store' })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const payload = await response.json()
    renderHistory(payload.history || [])
    renderPager('history', payload.total || 0, payload.page || 1)
  } catch (error) {
    histEl.empty.hidden = false
    histEl.empty.textContent = `执行记录加载失败：${error.message || error}`
  }
}

async function loadRobot() {
  try {
    const response = await fetch('/api/robot', { cache: 'no-store' })
    if (response.status === 404) {
      robotAvailable = false
      robotEl.summary.textContent = '机器人未配置。'
      return
    }
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    robotAvailable = true
    renderRobotState(await response.json())
  } catch (error) {
    robotEl.summary.textContent = `读取机器人状态失败：${error}`
  }
}

// collectSettings 从卡片读出设置。空行与空邮箱一律丢掉。
function collectSettings() {
  const suites = Array.from(robotEl.suites.querySelectorAll('.suite-chip[aria-pressed="true"]'))
    .map((chip) => chip.dataset.suite)
  const mailTo = mailRecipients.slice()
  return {
    suites,
    schedule: readScheduleInput(),
    mailTo,
    sendMail: robotEl.sendmail.checked,
    enabled: robotEl.enabled.checked,
  }
}

async function saveRobot() {
  // 用户可能敲完邮箱直接点保存，先把输入框内容收进来。
  commitMailInput()
  robotEl.save.disabled = true
  try {
    const response = await fetch('/api/robot/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(collectSettings()),
    })
    const text = await response.text()
    if (!response.ok) throw new Error(text || `HTTP ${response.status}`)
    await loadRobot()
    // 刷新会重写结果区，所以保存提示放在刷新之后写，确保用户看得见。
    toast('设置已保存')
  } catch (error) {
    toast(`保存失败：${error.message || error}`, true)
  } finally {
    robotEl.save.disabled = false
  }
}

async function runRobot() {
  robotEl.run.disabled = true
  try {
    const response = await fetch('/api/robot/run', { method: 'POST' })
    const payload = await response.json().catch(() => ({}))
    if (response.status === 409) {
      toast('已有批次在执行', true)
    } else if (!response.ok) {
      throw new Error(payload.message || `HTTP ${response.status}`)
    }
    await loadRobot()
  } catch (error) {
    toast(`触发失败：${error.message || error}`, true)
  }
}

if (robotEl.save) {
  robotEl.save.addEventListener('click', saveRobot)
  robotEl.run.addEventListener('click', runRobot)
  bindPager('history', (delta) => {
    historyPage = Math.max(1, historyPage + delta)
    loadHistory()
  })

  robotEl.entry.addEventListener('click', () => {
    selectTab('robot')
    loadHistory()
  })
  robotEl.back.addEventListener('click', () => {
    history.pushState(null, '', location.pathname)
    applyRoute()
  })

  // 回车或逗号提交；输入框为空时退格删掉最后一个胶囊（与常见标签输入一致）。
  robotEl.mailto.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' || event.key === ',' || event.key === ';') {
      event.preventDefault()
      commitMailInput()
      return
    }
    if (event.key === 'Backspace' && !robotEl.mailto.value && mailRecipients.length) {
      mailRecipients.pop()
      renderMailChips()
    }
  })
  // 粘贴多个邮箱时直接解析，不用逐个回车。
  bindTimeInput(robotEl.hour, 23)
  bindTimeInput(robotEl.minute, 59)
  robotEl.mailto.addEventListener('blur', () => { commitMailInput() })
  robotEl.mailto.addEventListener('paste', () => {
    setTimeout(commitMailInput, 0)
  })
}

async function boot() {
  try {
    const response = await fetch('/api/config', { cache: 'no-store' })
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const config = await response.json()
    sites = config.sites || []

    const portal = config.portal || {}
    if (portal.title) {
      document.getElementById('portal-title').textContent = portal.title
      document.title = portal.title
    }

    renderTabs()
    renderCards({})
    // 按 URL 恢复视图，保证刷新后停在同一页。
    applyRoute()
    loading.hidden = true
    loadHealth()
    // 首页的机器人卡片要显示状态，因此启动时就拉一次，不等用户点进详情。
    loadRobot()
  } catch (error) {
    loading.hidden = false
    loading.className = 'loading error'
    loading.textContent = `站点清单加载失败：${error}`
  }
}

// 数字键快速跳转：1 是首页，2 起对应站点签；输入框内不拦截。
document.addEventListener('keydown', (event) => {
  if (event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target
  if (target && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)) return
  const index = Number(event.key)
  if (!Number.isInteger(index) || index < 1 || index > sites.length + 1) return
  event.preventDefault()
  if (index === 1) {
    selectTab(HOME_TAB.key)
    return
  }
  go(sites[index - 2])
})

boot()
