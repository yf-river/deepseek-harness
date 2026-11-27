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
  // 工作台图标：一块仪表盘 + 下方承托，表达「多个入口聚合在这里」。
  // 原来的房子（home）容易和「首页」混淆，且工作台并不是首页。
  workbench: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <rect x="2.8" y="3" width="14.4" height="10.4" rx="2" stroke="currentColor" stroke-width="1.5"/>
    <path d="M6.4 8.2v2M10 6.4v3.8M13.6 9.2v1" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
    <path d="M3.4 16.4h13.2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
  </svg>`,
  tools: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <rect x="2.6" y="3.4" width="6" height="6" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <rect x="11.4" y="3.4" width="6" height="6" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <rect x="2.6" y="12.6" width="6" height="4" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <rect x="11.4" y="12.6" width="6" height="4" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
  </svg>`,
  default: `<svg viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
    <circle cx="10" cy="10" r="7" stroke="currentColor" stroke-width="1.5"/>
    <path d="M10 6v4l3 2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
  </svg>`,
}

const navTabs = document.getElementById('nav-tabs')

// 站点内嵌区的元素引用
const siteEl = {
  frame: document.getElementById('site-frame'),
  title: document.getElementById('site-title'),
  open: document.getElementById('site-open'),
  back: document.getElementById('site-back'),
}

if (siteEl.back) {
  siteEl.back.addEventListener('click', () => {
    history.pushState(null, '', '#/workbench')
    applyRoute()
  })
}
// 最近一次拉到的数据。卡片可能在数据回来之后才重建，
// 那时用它回填状态条，不必再打一次请求。
let lastEnvPool = null
let lastRobotPayload = null
// 表单是否已回填过。首次必须填，之后执行中不再覆盖用户输入。
let robotFormFilled = false

const siteGrid = document.getElementById('site-grid')
const loading = document.getElementById('loading')

let sites = []
// 站内视图：首页与机器人详情。机器人不在顶部签里，从首页卡片进入。
// 门户的起始页。它不再是顶部的「首页」签，而是排在最前的「研发工作台」。
const HOME_TAB = { key: '__home__', label: '研发工作台', icon: 'workbench' }

// 入口出现在哪里由配置决定；没写就当两边都显示。
// 顶部菜单和卡片是两种入口，同一个站点通常只在一处出现，避免重复。
const showInNav = (site) => site.showInNav !== false
const showInGrid = (site) => site.showInGrid !== false
const ROBOT_VIEW = 'view-robot'
const VIEWS = { [HOME_TAB.key]: 'view-home', env: 'view-env', robot: ROBOT_VIEW, site: 'view-site' }

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
  // 站点 key 形如 site/test，对应 VIEWS 的 site 视图。
  const viewKey = key.startsWith('site/') ? 'site' : key
  Object.entries(VIEWS).forEach(([name, id]) => {
    const node = document.getElementById(id)
    if (node) node.hidden = name !== viewKey
  })
  // 内嵌站点需要在视图切出来之后再设置 iframe，否则 iframe 还处于 hidden。
  if (viewKey === 'site') showSite(key.slice('site/'.length))

  // 把当前视图写进 hash：刷新停在同一页，前进/后退也能回到对应视图。
  // 工作台也要写 #/workbench——之前写成空 hash，刷新后会被当成
  // 「刚从外部进来」，从而跳到第一个站点，回不到工作台。
  const hash = key === HOME_TAB.key ? '#/workbench' : '#/' + key
  if (location.hash !== hash) history.pushState(null, '', hash)
}


// ── 路由 ──
// 视图状态放在 hash 里：刷新、收藏、前进后退都停在同一个页面。
function currentRoute() {
  const raw = location.hash.replace(/^#\/?/, '')
  // 环境详情与机器人都是从工作台点进去的下级页，让工作台保持选中。
  if (raw === 'env') return { view: 'env', key: HOME_TAB.key, env: true }
  if (raw === 'robot') return { view: 'robot', key: HOME_TAB.key, robot: true }
  // 站点内嵌：/#/site/test 表示在看 ChainWeaver Test
  const siteMatch = /^site\/(.+)$/.exec(raw)
  // key 是「签的标识」（site/quality），view 是视图名（site）：
  // 前者用于点亮顶部签，后者用于切换内容区，两者不能混用。
  if (siteMatch) {
    const key = siteMatch[1]
    // 没有独立签的站点（Test / Flow）是工作台的下级页，
    // 让工作台保持选中，否则顶部一个签都不亮，看不出自己在哪。
    const site = sites.find((one) => one.key === key)
    const navKey = site && showInNav(site) ? 'site/' + key : HOME_TAB.key
    return { view: 'site', key: navKey, site: key }
  }
  // 没有 hash 表示刚进门户：落在第一个站点（研效数据看板），
  // 而不是工作台——工作台现在是第三个菜单。
  const first = sites[0]
  if (raw === '' && first) return { view: 'site', key: 'site/' + first.key, site: first.key }
  if (raw === '' || raw === 'workbench') return { view: HOME_TAB.key, key: HOME_TAB.key }
  return { view: HOME_TAB.key, key: HOME_TAB.key }
}

// applyRoute 按当前 URL 同步选中态与视图，不写回 hash（避免循环）。
function applyRoute() {
  const route = currentRoute()
  navTabs.querySelectorAll('.nav-tab').forEach((node) => {
    const active = node.dataset.key === route.key
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
  if (route.site) showSite(route.site)
}

window.addEventListener('hashchange', applyRoute)

function renderTabs() {
  navTabs.replaceChildren()

  // 工作台是站内视图，但它不排第一个：站点签在前，工作台插在第二个站点之后。
  // 这样打开门户先落在第一个站点（研效看板），工作台作为常用入口排第三。
  const workbench = el('button', 'nav-tab')
  workbench.type = 'button'
  workbench.setAttribute('role', 'tab')
  workbench.dataset.key = HOME_TAB.key
  workbench.title = HOME_TAB.label
  workbench.appendChild(navIconSpan(HOME_TAB))
  workbench.appendChild(el('span', 'nav-tab-label', HOME_TAB.label))
  workbench.addEventListener('click', () => selectTab(HOME_TAB.key))

  const navSites = sites.filter(showInNav)
  navSites.forEach((site, index) => {
    // 站点改成站内内嵌：点击在内容区用 iframe 打开，顶部签栏保留。
    const tab = el('button', 'nav-tab')
    tab.type = 'button'
    tab.dataset.key = 'site/' + site.key
    tab.title = `${site.title} · ${site.url}`
    tab.appendChild(navIconSpan(site))
    tab.appendChild(el('span', 'nav-tab-label', site.label))
    tab.addEventListener('click', () => selectTab('site/' + site.key))
    navTabs.appendChild(tab)
    // 第二个站点之后插入工作台，让它成为第三个菜单。
    if (index === 1) navTabs.appendChild(workbench)
  })
  // 站点不足两个时也不能丢掉工作台。
  if (navSites.length < 2) navTabs.appendChild(workbench)

  // 这里只负责生成签，视图由 applyRoute 按 URL 决定，不在渲染时写路由。
}


// showSite 把指定站点加载进 iframe。顶部签栏保留在外面，内容区换成站点。
// 站点自身的 CSP 已放开 frame-ancestors，所以能正常嵌套。
function showSite(key) {
  const site = sites.find((one) => one.key === key)
  if (!site) return
  if (siteEl.title) siteEl.title.textContent = site.title || site.label
  if (siteEl.frame) {
    // 同一个地址不重复设置，避免重渲染把用户正在操作的页面刷掉。
    if (siteEl.frame.dataset.url !== site.url) {
      siteEl.frame.dataset.url = site.url
      siteEl.frame.src = site.url
    }
  }
  if (siteEl.open) siteEl.open.href = site.url
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
  // 探测由门户服务器发起。若站点在外网段（服务器连不上、浏览器能连上），
  // 报「不可达」是误判——iframe 由浏览器加载，不受服务器网络限制。
  // 这里区分两种情况，避免把"服务器看不到"当成"打不开"。
  const badge = el('span', 'status status-unknown', '外部站点')
  badge.title = `门户服务器无法探测该地址，页面由浏览器直接加载：${health.error || 'HTTP ' + health.status}`
  return badge
}

// 站点按 group 分到三块里。没写 group 的归到「开发」，保证新站点不会消失。
const GROUP_IDS = { dev: 'grid-dev', test: 'grid-test', ops: 'grid-ops' }

function renderCards(healthByKey) {
  Object.values(GROUP_IDS).forEach((id) => {
    const node = document.getElementById(id)
    if (node) node.replaceChildren()
  })

  // 只给 showInGrid 的站点出卡片：顶部菜单已有的站点不在工作台里重复。
  sites.filter(showInGrid).forEach((site) => {
    const group = GROUP_IDS[site.group] ? site.group : 'dev'
    const holder = document.getElementById(GROUP_IDS[group])
    if (!holder) return
    holder.appendChild(buildCard(site, healthByKey[site.key]))
  })

  // Env 与机器人不是外部站点，不由 sites 驱动，单独生成到运维/测试组。
  const opsHolder = document.getElementById(GROUP_IDS.ops)
  if (opsHolder) opsHolder.appendChild(buildEnvCard())
  const testHolder = document.getElementById(GROUP_IDS.test)
  if (testHolder) testHolder.appendChild(buildRobotCard())

  // 卡片是刚生成的，之前 loadEnv 的结果没地方写。这里把缓存回填一次，
  // 否则状态条会一直停在初始的「总计 - · 空闲 -」。
  if (lastEnvPool) renderEnvPool(lastEnvPool)
  if (lastRobotPayload) renderRobotEntry(lastRobotPayload)

  // 某个分组一个卡片都没有时整块隐藏，不留空标题。
  Object.entries(GROUP_IDS).forEach(([group, id]) => {
    const node = document.getElementById(id)
    const section = node && node.closest('.wb-group')
    if (section) section.hidden = !node.children.length
  })
}

// buildEnvCard 生成环境管理卡片。状态条里的数字由 renderEnvPool 回填。
function buildEnvCard() {
  const card = el('button', 'wb-card')
  card.type = 'button'
  card.id = 'env-entry'
  card.dataset.kind = 'view'
  card.title = '查看 ChainWeaver Env 环境'

  const top = el('div', 'wb-card-top')
  const iconBox = el('div', 'wb-card-icon')
  iconBox.innerHTML = iconFor('dashboard')
  top.appendChild(iconBox)
  const title = el('div', 'wb-card-title')
  title.appendChild(el('h3', '', 'ChainWeaver Env'))
  title.appendChild(el('span', '', '环境编排与租约'))
  top.appendChild(title)
  const status = el('span', 'pill is-idle', '检测中')
  status.id = 'env-entry-status'
  top.appendChild(status)
  card.appendChild(top)

  card.appendChild(el('p', 'wb-card-desc', '查看环境池占用、健康状态与租约情况。'))

  const bar = el('div', 'wb-card-bar')
  const kv = el('span', 'wb-kv')
  kv.id = 'env-entry-metric'
  kv.textContent = '总计 - · 空闲 -'
  bar.appendChild(kv)
  bar.appendChild(el('span', 'wb-spacer'))
  bar.appendChild(el('span', 'wb-go', '查看 →'))
  card.appendChild(bar)

  card.addEventListener('click', () => selectTab('env'))
  return card
}

// buildRobotCard 生成每日巡检卡片。状态条由 renderRobotEntry 回填下次执行时间。
function buildRobotCard() {
  const card = el('button', 'wb-card')
  card.type = 'button'
  card.id = 'robot-entry'
  card.dataset.kind = 'view'
  card.title = '查看机器人的设置与运行记录'

  const top = el('div', 'wb-card-top')
  const iconBox = el('div', 'wb-card-icon')
  iconBox.innerHTML = iconFor('robot')
  top.appendChild(iconBox)
  const title = el('div', 'wb-card-title')
  title.appendChild(el('h3', '', 'E2E 每日巡检'))
  const sub = el('span', '', 'IDA · MIRA · DPM · 每天 08:00')
  sub.id = 'robot-entry-sub'
  title.appendChild(sub)
  top.appendChild(title)
  const status = el('span', 'pill is-idle', '未执行')
  status.id = 'robot-entry-status'
  top.appendChild(status)
  card.appendChild(top)

  card.appendChild(el('p', 'wb-card-desc', '每天把所有主流程依次跑一遍，结果发到指定邮箱。'))

  const bar = el('div', 'wb-card-bar')
  const kv = el('span', 'wb-kv')
  kv.id = 'robot-entry-metric'
  kv.textContent = '尚未执行'
  bar.appendChild(kv)
  bar.appendChild(el('span', 'wb-spacer'))
  bar.appendChild(el('span', 'wb-go', '查看 →'))
  card.appendChild(bar)

  card.addEventListener('click', () => selectTab('robot'))
  return card
}

// buildCard 生成一张工作台卡片：标题 + 状态 + 描述 + 底部状态条。
function buildCard(site, health) {
  const card = el('button', 'wb-card')
  card.type = 'button'
  card.dataset.key = site.key
  card.title = `进入 ${site.title}`

  const top = el('div', 'wb-card-top')
  const iconBox = el('div', 'wb-card-icon')
  iconBox.innerHTML = iconFor(site.icon)
  top.appendChild(iconBox)

  const title = el('div', 'wb-card-title')
  title.appendChild(el('h3', '', site.title))
  title.appendChild(el('span', '', site.subtitle || site.label))
  top.appendChild(title)
  top.appendChild(statusBadge(health))
  card.appendChild(top)

  card.appendChild(el('p', 'wb-card-desc', site.description || ''))

  // 底部状态条：把「现在什么情况」压到一行，扫读比看描述快。
  const bar = el('div', 'wb-card-bar')
  const kv = el('span', 'wb-kv')
  kv.appendChild(el('b', '', site.metric || '进入查看'))
  bar.appendChild(kv)
  bar.appendChild(el('span', 'wb-spacer'))
  bar.appendChild(el('span', 'wb-go', '进入 →'))
  card.appendChild(bar)

  card.addEventListener('click', () => selectTab('site/' + site.key))
  return card
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
  // 卡片是渲染时动态生成的，这里用 getter，避免模块加载时取到 null。
  get entry() { return document.getElementById('env-entry') },
  get entryStatus() { return document.getElementById('env-entry-status') },
  get entryMetric() { return document.getElementById('env-entry-metric') },
  get updated() { return document.getElementById('env-updated') },
  get host() { return document.getElementById('env-host') },
  get total() { return document.getElementById('env-total') },
  get idle() { return document.getElementById('env-idle') },
  get leased() { return document.getElementById('env-leased') },
  get unhealthy() { return document.getElementById('env-unhealthy') },
  get busy() { return document.getElementById('env-busy') },
  get list() { return document.getElementById('env-list') },
  get empty() { return document.getElementById('env-empty') },
  get error() { return document.getElementById('env-error') },
  get refresh() { return document.getElementById('env-refresh') },
  get opsList() { return document.getElementById('env-ops-list') },
  get opsCount() { return document.getElementById('env-ops-count') },
  get opsEmpty() { return document.getElementById('env-ops-empty') },
  get back() { return document.getElementById('env-back') },
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

    // 第三行：只有可点的前端入口与数据库端口。
    // 服务器地址四个环境都一样，已提到页面顶部统一说明，不再逐行重复。
    const addr = el('div', 'env-row-addr')
    if (env.uiUrl) {
      const link = el('a', 'env-link', env.uiUrl)
      link.href = env.uiUrl
      link.target = '_blank'
      link.rel = 'noopener'
      link.title = `打开前端（端口 ${env.uiPort}）`
      addr.appendChild(link)
    }
    if (env.ports && env.ports.database) {
      addr.appendChild(el('span', 'env-addr-sep', '数据库'))
      addr.appendChild(el('span', 'env-addr', String(env.ports.database)))
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
    const bad = pool.unhealthy > 0
    envEl.entryStatus.className = 'pill ' + (bad ? 'is-bad' : 'is-ok')
    envEl.entryStatus.textContent = bad
      ? `${pool.unhealthy} 异常`
      : `${pool.idle || 0} 空闲`
  }
  if (envEl.entryMetric) {
    envEl.entryMetric.textContent = `总计 ${pool.total || 0} · 空闲 ${pool.idle || 0} · 已租用 ${pool.leased || 0}`
  }
  // 宿主机只有一个，写在页面顶部；各环境行不再重复。
  if (envEl.host) {
    const first = (pool.environments || []).find((one) => one.uiUrl)
    envEl.host.textContent = first ? hostOf(first) : '-'
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
  if (seconds < 60) return `${seconds}秒前`
  const minutes = Math.round(seconds / 60)
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.round(minutes / 60)
  return `${hours}小时前`
}

// 只更新详情页的「更新于」。工作台卡片的状态条展示的是池子占用数，
// 不再重复时间信息——那里放数字比放时间更有用。
function renderEnvUpdated() {
  if (!envLastUpdated) return
  const text = relativeTime(envLastUpdated)
  if (envEl.updated) envEl.updated.textContent = `更新于${text}`
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
    lastEnvPool = await response.json()
    renderEnvPool(lastEnvPool)
    envLastUpdated = new Date()
    renderEnvUpdated()
  } catch (error) {
    envEl.error.hidden = false
    envEl.error.textContent = `读取环境池失败：${error.message || error}`
  }
}

// 卡片的点击已在 buildEnvCard 里绑定，这里只处理详情页与轮询启动。
// 详情页的控件按需绑定（工作台视图里这些元素不存在）。
if (envEl.refresh) envEl.refresh.addEventListener('click', loadEnv)
if (envEl.back) {
  envEl.back.addEventListener('click', () => {
    history.pushState(null, '', '#/workbench')
    applyRoute()
  })
}
bindPager('env-ops', (delta) => {
  envOpsPage = Math.max(1, envOpsPage + delta)
  loadEnv()
})
// 工作台的环境卡片要显示池子占用，启动时就拉一次，之后按 60 秒轮询。
// 这段不能挂在详情页元素存在与否上，否则工作台里会一直显示「-」。
{
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
  // 同上：卡片动态生成，用 getter 延迟取。
  get entry() { return document.getElementById('robot-entry') },
  get entryStatus() { return document.getElementById('robot-entry-status') },
  get entrySub() { return document.getElementById('robot-entry-sub') },
  get entryMetric() { return document.getElementById('robot-entry-metric') },
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
    // 用真正对勾形状的 SVG，不用旋转边框拼——后者在 14px 下容易看成斜杠
    const mark = el('span', 'suite-chip-mark')
    mark.innerHTML = `<svg viewBox="0 0 12 12" fill="none" aria-hidden="true">
      <path d="M2.6 6.2 4.9 8.5 9.4 4" stroke="currentColor" stroke-width="1.8"
            stroke-linecap="round" stroke-linejoin="round"/>
    </svg>`
    chip.appendChild(mark)
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
    robotEl.summary.textContent = '每天把所有主流程依次跑一遍，结果发到指定邮箱。'
  }

  // 首次加载无条件回填：此时用户还没开始编辑，不填就是一片空白。
  // 之后执行中的轮询才跳过，避免覆盖用户正在改的内容。
  if ((!robotFormFilled || !running) && payload.availableSuites) {
    robotFormFilled = true
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

  // 工作台卡片同步显示同一份状态；卡片可能在数据之后才生成，
  // 由 renderRobotEntry 统一处理，重建时也能回填。
  lastRobotPayload = payload
  renderRobotEntry(payload)

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

// ── 时间选择 ──
// 按钮显示当前时间，点开才出面板：左边小时、右边分钟，各自一列按钮。
// 不用输入框（省得校验 99 / 字母），也不用常驻滚轮（省得页面上挂一堆半截数字）。
const TIME_HOURS = Array.from({ length: 24 }, (_, i) => i)
const TIME_MINUTES = [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]

let scheduleHour = 8
let scheduleMinute = 0

// 列表定位：把选中的那一项滚到列的垂直中间。
// 不能用 offsetTop——它相对 offsetParent，不是相对这个滚动容器。
function scrollIntoColumn(list, active) {
  if (!list || !active) return
  const listBox = list.getBoundingClientRect()
  const itemBox = active.getBoundingClientRect()
  const delta = (itemBox.top - listBox.top) - (list.clientHeight - itemBox.height) / 2
  list.scrollTop += delta
}

function markColumn(list, value, scroll = true) {
  list.querySelectorAll('.time-opt').forEach((opt) => {
    const on = Number(opt.dataset.value) === value
    opt.classList.toggle('is-active', on)
    opt.setAttribute('aria-selected', on ? 'true' : 'false')
  })
  // 面板隐藏时 clientHeight 是 0，居中算法会退化成「滚到底」，
  // 所以只有可见时才滚；等面板打开后再补一次定位。
  if (scroll && list.clientHeight > 0) {
    scrollIntoColumn(list, list.querySelector('.time-opt.is-active'))
  }
}

function buildColumns() {
  const hourList = robotEl.hour
  const minuteList = robotEl.minute
  if (!hourList || !minuteList) return

  if (!hourList.dataset.built) {
    hourList.replaceChildren()
    TIME_HOURS.forEach((h) => {
      const opt = el('button', 'time-opt', pad2(h))
      opt.type = 'button'
      opt.dataset.value = String(h)
      opt.setAttribute('role', 'option')
      opt.addEventListener('click', () => {
        scheduleHour = h
        renderSchedule()
      })
      hourList.appendChild(opt)
    })
    hourList.dataset.built = '1'
  }

  if (!minuteList.dataset.built) {
    minuteList.replaceChildren()
    TIME_MINUTES.forEach((m) => {
      const opt = el('button', 'time-opt', pad2(m))
      opt.type = 'button'
      opt.dataset.value = String(m)
      opt.setAttribute('role', 'option')
      opt.addEventListener('click', () => {
        scheduleMinute = m
        renderSchedule()
      })
      minuteList.appendChild(opt)
    })
    minuteList.dataset.built = '1'
  }
}

// 面板里所有显示时间的地方统一刷新，避免预览和后端值不一致。
function renderSchedule() {
  const text = `${pad2(scheduleHour)}:${pad2(scheduleMinute)}`
  const trigger = document.getElementById('robot-time-text')
  const big = document.getElementById('robot-time-big')
  if (trigger) trigger.textContent = text
  if (big) big.textContent = text
  if (robotEl.hour) markColumn(robotEl.hour, scheduleHour, false)
  if (robotEl.minute) markColumn(robotEl.minute, scheduleMinute, false)
}

function setScheduleInput(value) {
  const match = /^(\d{1,2}):(\d{1,2})$/.exec(String(value || '').trim())
  if (match) {
    scheduleHour = Math.min(Number(match[1]), 23)
    // 只保留 5 分钟一档：外部传来 08:07 时吸附到最近的档位，避免选中项看不见。
    const rawMinute = Math.min(Number(match[2]), 59)
    scheduleMinute = TIME_MINUTES.reduce(
      (best, m) => (Math.abs(m - rawMinute) < Math.abs(best - rawMinute) ? m : best),
      TIME_MINUTES[0],
    )
  }
  buildColumns()
  renderSchedule()
}

function readScheduleInput() {
  return `${pad2(scheduleHour)}:${pad2(scheduleMinute)}`
}

// 打开 / 关闭面板；点外面或按 Esc 关闭。
function openTimePop(open) {
  const pop = document.getElementById('robot-time-pop')
  const trigger = document.getElementById('robot-time-trigger')
  if (!pop || !trigger) return
  pop.hidden = !open
  trigger.setAttribute('aria-expanded', open ? 'true' : 'false')
  if (open) {
    renderSchedule()
    // 展开后再滚，否则容器还没高度，scrollIntoColumn 算不出来。
    requestAnimationFrame(() => {
      if (robotEl.hour) markColumn(robotEl.hour, scheduleHour)
      if (robotEl.minute) markColumn(robotEl.minute, scheduleMinute)
    })
  }
}

function bindTimePop() {
  const trigger = document.getElementById('robot-time-trigger')
  const done = document.getElementById('robot-time-done')
  if (!trigger) return

  trigger.addEventListener('click', () => {
    const pop = document.getElementById('robot-time-pop')
    openTimePop(pop.hidden)
  })
  if (done) done.addEventListener('click', () => openTimePop(false))

  document.querySelectorAll('.time-quick').forEach((btn) => {
    btn.addEventListener('click', () => {
      setScheduleInput(btn.dataset.at)
      openTimePop(false)
    })
  })

  document.addEventListener('click', (event) => {
    const field = document.getElementById('robot-schedule')
    if (!field || field.contains(event.target)) return
    openTimePop(false)
  })
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') openTimePop(false)
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

// renderRobotEntry 把机器人状态写到工作台卡片上。
// 卡片是动态生成的，重建后调用一次即可回填，不必重新请求。
function renderRobotEntry(payload) {
  const state = payload.state || {}
  const settings = payload.settings || {}
  const running = state.status === 'running'
  const status = robotEl.entryStatus
  if (status) {
    const verdict = state.verdict
    const tone = running ? 'is-run' : verdict === 'PASS' ? 'is-ok' : verdict ? 'is-bad' : 'is-idle'
    status.className = 'pill ' + tone
    status.textContent = running ? '执行中' : verdict || '未执行'
    status.title = state.endedAt ? `上次 ${beijingTime(state.endedAt)}` : ''
  }
  const sub = robotEl.entrySub
  if (sub) {
    const suites = (settings.suites || []).map((id) => id.toUpperCase()).join(' · ')
    const when = settings.enabled ? `每天 ${settings.schedule || '—'}` : '定时未启用'
    sub.textContent = suites ? `${suites} · ${when}` : '每天定时执行'
  }
  const metric = robotEl.entryMetric
  if (metric) {
    if (running) metric.textContent = '正在执行'
    else if (payload.nextRunAt) metric.textContent = `下次 ${beijingTime(payload.nextRunAt)}`
    else if (state.verdict) metric.textContent = `上次 ${state.verdict}`
    else metric.textContent = '尚未执行'
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

  robotEl.back.addEventListener('click', () => {
    history.pushState(null, '', '#/workbench')
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
  bindTimePop()
  setScheduleInput('08:00')
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
