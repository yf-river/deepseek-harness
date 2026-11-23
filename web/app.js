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
// 站内视图：首页与机器人。站点签是整页跳走，所以回到门户时必然又落在站内视图上，
// 不需要额外的状态记忆。
const HOME_TAB = { key: '__home__', label: '首页', icon: 'home' }
const ROBOT_TAB = { key: '__robot__', label: '机器人', icon: 'robot' }
const VIEWS = { [HOME_TAB.key]: 'view-home', [ROBOT_TAB.key]: 'view-robot' }

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

// go 统一在一个地方跳转：整页进入目标站点，浏览器地址栏保留目标地址。
function go(site) {
  if (!site || !site.url) return
  window.location.href = site.url
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
  if (key === ROBOT_TAB.key) loadRobot()
}

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

  const robotTab = el('button', 'nav-tab')
  robotTab.type = 'button'
  robotTab.setAttribute('role', 'tab')
  robotTab.dataset.key = ROBOT_TAB.key
  robotTab.title = '每日巡检机器人'
  robotTab.appendChild(navIconSpan(ROBOT_TAB))
  robotTab.appendChild(el('span', 'nav-tab-label', ROBOT_TAB.label))
  robotTab.addEventListener('click', () => selectTab(ROBOT_TAB.key))
  navTabs.appendChild(robotTab)

  sites.forEach((site) => {
    const tab = el('button', 'nav-tab')
    tab.type = 'button'
    tab.setAttribute('role', 'tab')
    tab.dataset.key = site.key
    tab.title = `${site.title} · ${site.url}`
    tab.appendChild(navIconSpan(site))
    tab.appendChild(el('span', 'nav-tab-label', site.label))
    tab.addEventListener('click', () => go(site))
    navTabs.appendChild(tab)
  })

  selectTab(HOME_TAB.key)
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
    const card = el('button', 'site-card')
    card.type = 'button'
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

    card.addEventListener('click', () => go(site))
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


// ─────────────────────────── 机器人页 ───────────────────────────

const robotEl = {
  state: document.getElementById('robot-state'),
  summary: document.getElementById('robot-summary'),
  suites: document.getElementById('robot-suites'),
  schedule: document.getElementById('robot-schedule'),
  next: document.getElementById('robot-next'),
  mailto: document.getElementById('robot-mailto'),
  enabled: document.getElementById('robot-enabled'),
  sendmail: document.getElementById('robot-sendmail'),
  result: document.getElementById('robot-result'),
  save: document.getElementById('robot-save'),
  run: document.getElementById('robot-run'),
}

let robotAvailable = false
let robotPollTimer = null

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
function suiteCheckboxes(available, selected) {
  robotEl.suites.replaceChildren()
  available.forEach((suite) => {
    const label = el('label', 'robot-chip')
    const input = document.createElement('input')
    input.type = 'checkbox'
    input.value = suite.id
    input.checked = selected.includes(suite.id)
    label.appendChild(input)
    label.appendChild(el('span', '', suite.name))
    robotEl.suites.appendChild(label)
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
    robotEl.schedule.value = settings.schedule || ''
    robotEl.mailto.value = (settings.mailTo || []).join(', ')
    robotEl.enabled.checked = !!settings.enabled
    robotEl.sendmail.checked = !!settings.sendMail
  }

  robotEl.next.textContent = payload.nextRunAt ? '下次 ' + beijingTime(payload.nextRunAt) : ''
  robotEl.run.disabled = running
  robotEl.run.textContent = running ? '执行中…' : '立即执行一次'

  // 结果区：跑完才显示；执行中显示进度。
  if (running) {
    const done = (state.results || []).filter((r) => ['passed', 'failed', 'error'].includes(r.status))
    robotEl.result.hidden = false
    robotEl.result.textContent = state.message || '正在执行…'
    robotEl.result.className = 'robot-result'
    if (done.length) {
      robotEl.result.textContent = (state.results || []).map(
        (r) => `${r.id}：${r.status === 'passed' ? '通过' : r.status === 'failed' ? '失败' : '未完成'}`
      ).join(' · ')
    }
  } else if (state.verdict) {
    const rows = (state.results || []).map((r) => {
      const label = r.status === 'passed' ? '通过' : r.status === 'failed' ? '失败' : '未取得结果'
      return `${r.id}：${label}${r.durationText ? ' · ' + r.durationText : ''}`
    })
    robotEl.result.hidden = false
    robotEl.result.className = 'robot-result'
    robotEl.result.textContent = rows.join(' · ')
    if (state.mailStatus) {
      robotEl.result.textContent += ` · 邮件${state.mailStatus}`
    }
    if (state.lastError) {
      robotEl.result.textContent += ` · ${state.lastError}`
    }
  } else if (state.lastError) {
    robotEl.result.hidden = false
    robotEl.result.className = 'robot-result is-error'
    robotEl.result.textContent = state.lastError
  } else {
    robotEl.result.hidden = true
  }

  // 执行中持续轮询，结束后自动停。
  if (running && !robotPollTimer) {
    robotPollTimer = setInterval(loadRobot, 5000)
  } else if (!running && robotPollTimer) {
    clearInterval(robotPollTimer)
    robotPollTimer = null
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
  const suites = Array.from(robotEl.suites.querySelectorAll('input:checked')).map((i) => i.value)
  const mailTo = robotEl.mailto.value.split(',').map((s) => s.trim()).filter(Boolean)
  return {
    suites,
    schedule: robotEl.schedule.value,
    mailTo,
    sendMail: robotEl.sendmail.checked,
    enabled: robotEl.enabled.checked,
  }
}

async function saveRobot() {
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
    robotEl.result.hidden = false
    robotEl.result.className = 'robot-result'
    robotEl.result.textContent = '设置已保存' 
  } catch (error) {
    robotEl.result.hidden = false
    robotEl.result.className = 'robot-result is-error'
    robotEl.result.textContent = `保存失败：${error.message || error}`
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
      robotEl.result.hidden = false
      robotEl.result.className = 'robot-result is-error'
      robotEl.result.textContent = '已有批次在执行'
    } else if (!response.ok) {
      throw new Error(payload.message || `HTTP ${response.status}`)
    }
    await loadRobot()
  } catch (error) {
    robotEl.result.hidden = false
    robotEl.result.className = 'robot-result is-error'
    robotEl.result.textContent = `触发失败：${error.message || error}`
  }
}

if (robotEl.save) {
  robotEl.save.addEventListener('click', saveRobot)
  robotEl.run.addEventListener('click', runRobot)
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
    loading.hidden = true
    loadHealth()
  } catch (error) {
    loading.hidden = false
    loading.className = 'loading error'
    loading.textContent = `站点清单加载失败：${error}`
  }
}

// 数字键快速跳转：1 是首页，2 是机器人，3 起对应站点签；输入框内不拦截。
document.addEventListener('keydown', (event) => {
  if (event.metaKey || event.ctrlKey || event.altKey) return
  const target = event.target
  if (target && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)) return
  const index = Number(event.key)
  if (!Number.isInteger(index) || index < 1 || index > sites.length + 2) return
  event.preventDefault()
  if (index === 1) {
    selectTab(HOME_TAB.key)
    return
  }
  if (index === 2) {
    selectTab(ROBOT_TAB.key)
    return
  }
  go(sites[index - 3])
})

boot()
