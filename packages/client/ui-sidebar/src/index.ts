/** 侧边栏的宿主配置。 */
import type {} from '@deepseek-ai/dsh-settings'
import type { Context, Volatile } from '@deepseek-ai/cordis'
import z from '@deepseek-ai/schemastery'

/** 侧边栏品牌展示。 */
export interface Config {
  /** 是否显示品牌名称与图标，包括收起侧栏时的图标。 */
  showBrand: Volatile<boolean>
}

/** 侧边栏展示配置。 */
export const Config = z.object({
  showBrand: z.boolean().default(true).volatile(),
})

/** 向浏览器配置同步通道注册品牌展示配置。
 * @param ctx - 宿主插件上下文。
 */
export function apply(ctx: Context): void {
  ctx.inject(['settings'], child => { child.effect(() => child.settings.configure({ auto: false }, ctx.fiber)) })
}
