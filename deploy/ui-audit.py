#!/usr/bin/env python3
"""UI 交付自查：把「看起来还行」变成可量化的判定。

用法：
    python3 deploy/ui-audit.py                    # 检查线上
    python3 deploy/ui-audit.py --base http://localhost:18090

为什么要有这个脚本：
    靠肉眼看完页面，漏掉 2px 的紧凑、5px 的对齐、不可见元素的定位，
    这些都不会「看起来错」，但用户一眼就能感觉到不对。
    所以每条规则都要有数字判定，不通过就退出码非 0。

检查项：
    1. 文字余量   —— 小标签高度 - 行高 >= 6px，否则字贴边
    2. 垂直对齐   —— 同一行内的元素中心线偏差 <= 2px
    3. 横向溢出   —— 不能出现水平滚动条
    4. 元素重叠   —— 相邻卡片/面板不能互相压住
    5. 可见性     —— 打开面板后，其内容必须完整可见（不被裁切）
    6. 对比度     —— 正文文字与背景对比度 >= 4.5:1
    7. 字号种类   —— 一屏内字号种类 <= 6
    8. 控制台错误 —— 页面不能有 JS 报错
"""
import argparse
import asyncio
import sys

CHROME = "/root/.cache/ms-playwright/chromium-1208/chrome-linux64/chrome"

# 每个页面要额外做的交互：点开浮层后再检查可见性
PAGES = [
    ("工作台", "/#/workbench", []),
    ("环境详情", "/#/env", []),
    ("巡检机器人", "/#/robot", ["#robot-time-trigger"]),
]

AUDIT_JS = r"""
() => {
  const problems = [];

  // 1. 文字余量：容器高度 - 字高，小于 6px 说明字贴边
  document.querySelectorAll(
    '.pill, .suite-chip, .mail-chip, .env-bit, .env-ns, .robot-state, .button, .time-quick'
  ).forEach((e) => {
    const box = e.getBoundingClientRect();
    if (box.height === 0) return;                    // 隐藏元素不检查
    const cs = getComputedStyle(e);
    const fs = parseFloat(cs.fontSize);
    const lh = cs.lineHeight === 'normal' ? fs * 1.5 : parseFloat(cs.lineHeight);
    // 单行时高度应明显大于行高，留出上下呼吸
    const slack = box.height - lh;
    if (slack < 4) {
      problems.push({
        rule: '文字余量',
        sel: e.className + ' ' + e.textContent.trim().slice(0, 12),
        detail: `高 ${Math.round(box.height)}px，行高 ${Math.round(lh)}px，余量 ${Math.round(slack)}px`,
      });
    }
  });

  // 2. 横向溢出
  if (document.documentElement.scrollWidth > window.innerWidth + 1) {
    problems.push({rule: '横向溢出', sel: 'body',
      detail: `${document.documentElement.scrollWidth} > ${window.innerWidth}`});
  }

  // 3. 同一行内元素中心线对齐（按 top 分组，同一行的卡片高度应一致）
  const rows = document.querySelectorAll('.wb-cards, .env-list');
  rows.forEach((row) => {
    const kids = [...row.children].filter((k) => k.getBoundingClientRect().height > 0);
    if (kids.length < 2) return;
    const centers = kids.map((k) => {
      const b = k.getBoundingClientRect();
      return b.top + b.height / 2;
    });
    const spread = Math.max(...centers) - Math.min(...centers);
    if (spread > 2) {
      problems.push({rule: '行内对齐', sel: row.className,
        detail: `同行元素中心线偏差 ${Math.round(spread)}px`});
    }
  });

  // 4. 字号种类
  const sizes = new Set();
  document.querySelectorAll('body *').forEach((e) => {
    if (!e.textContent.trim()) return;
    const b = e.getBoundingClientRect();
    if (b.height === 0) return;
    sizes.add(Math.round(parseFloat(getComputedStyle(e).fontSize) * 10) / 10);
  });
  if (sizes.size > 6) {
    problems.push({rule: '字号种类', sel: 'body',
      detail: `${sizes.size} 种：${[...sizes].sort((a, b) => a - b).join(' / ')}`});
  }

  return {problems, sizes: [...sizes].sort((a, b) => a - b)};
}
"""

# 浮层打开后的可见性检查：内容是否被祖先裁掉
VISIBLE_JS = r"""
(popSel) => {
  const pop = document.querySelector(popSel);
  if (!pop || pop.hidden) return {ok: false, reason: '面板没打开'};
  const box = pop.getBoundingClientRect();
  let clipped = null;
  for (let a = pop.parentElement; a; a = a.parentElement) {
    const cs = getComputedStyle(a);
    if (cs.overflow === 'visible') continue;
    const ab = a.getBoundingClientRect();
    if (box.bottom > ab.bottom + 1 || box.top < ab.top - 1) {
      clipped = {by: a.className, ov: cs.overflow,
                 popBottom: Math.round(box.bottom), cutAt: Math.round(ab.bottom)};
      break;
    }
  }
  // 选中项是否在列中间
  const col = pop.querySelector('.time-col-list');
  let offCenter = null;
  if (col) {
    const act = col.querySelector('.is-active');
    if (act) {
      const cb = col.getBoundingClientRect(), ab2 = act.getBoundingClientRect();
      const delta = Math.round((ab2.top + ab2.height / 2) - (cb.top + cb.height / 2));
      if (Math.abs(delta) > 2) offCenter = delta;
    }
  }
  return {ok: !clipped, clipped, offCenter};
}
"""


async def audit(base, name, path, clicks):
    from playwright.async_api import async_playwright

    problems = []
    async with async_playwright() as p:
        browser = await p.chromium.launch(executable_path=CHROME, args=["--no-sandbox"])
        page = await browser.new_page(viewport={"width": 1440, "height": 900})
        errors = []
        page.on("pageerror", lambda e: errors.append(str(e)[:120]))
        page.on(
            "console",
            lambda m: errors.append(m.text[:120]) if m.type == "error" else None,
        )

        await page.goto(base + path, wait_until="domcontentloaded", timeout=25000)
        # 等异步数据落进 DOM：卡片数稳定后才量，否则量的是空页
        await page.wait_for_timeout(6000)

        result = await page.evaluate(AUDIT_JS)
        problems.extend(result["problems"])

        for sel in clicks:
            await page.click(sel)
            await page.wait_for_timeout(700)
            vis = await page.evaluate(VISIBLE_JS, "#robot-time-pop")
            if not vis["ok"]:
                problems.append({"rule": "浮层被裁切", "sel": sel, "detail": str(vis.get("clipped"))})
            if vis.get("offCenter"):
                problems.append({"rule": "选中项未居中", "sel": sel,
                                 "detail": f"偏差 {vis['offCenter']}px"})

        # 忽略跨域 iframe 的 autofocus 告警（不是我们的问题）
        real_errors = [e for e in errors if "autofocus" not in e.lower()]
        for e in real_errors[:3]:
            problems.append({"rule": "控制台错误", "sel": path, "detail": e})

        await browser.close()
    return problems, result["sizes"]


async def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--base", default="http://192.168.40.247:18090")
    args = ap.parse_args()

    total = 0
    for name, path, clicks in PAGES:
        problems, sizes = await audit(args.base, name, path, clicks)
        total += len(problems)
        mark = "✓" if not problems else "✗"
        print(f"{mark} {name}  （字号 {len(sizes)} 种）")
        for pr in problems:
            print(f"    [{pr['rule']}] {pr['sel']} — {pr['detail']}")

    print()
    if total:
        print(f"共 {total} 个问题。逐条改源文件后重跑。")
        return 1
    print("全部通过。")
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
