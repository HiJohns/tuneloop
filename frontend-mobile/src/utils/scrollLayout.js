// #2343: 首页滚动布局的确定性计算（weapp）。
// 纯函数便于单测；页面只负责测量与渲染。

export const NAV_FALLBACK_HEIGHT = 60
export const LOAD_MORE_THRESHOLD = 300

// 底栏实际高度 = 视口高度 − 底栏 top（同一视口坐标系）。
// 输入缺失/非有限/越界时回退 60，绝不返回负值。
export function computeBottomInset({ windowHeight, navTop } = {}) {
  const wh = Number(windowHeight)
  const top = Number(navTop)
  if (!Number.isFinite(wh) || !Number.isFinite(top) || top < 0 || top > wh) {
    return NAV_FALLBACK_HEIGHT
  }
  const inset = Math.round(wh - top)
  return inset > 0 ? inset : NAV_FALLBACK_HEIGHT
}

// #1540 不变量：内容高度必须 > 视口高度，避免短列表重载时原生滚动复位。
// 用 viewport + 1 代替旧的固定 2000px 占位。
export function computeContentMinHeight(viewportHeight) {
  const vh = Number(viewportHeight)
  if (!Number.isFinite(vh) || vh <= 0) return 0
  return Math.ceil(vh) + 1
}

// 触底加载：距底部 <= threshold 时触发；scrollHeight 未知（0）时不触发。
export function shouldLoadMore({
  scrollTop,
  viewportHeight,
  scrollHeight,
  threshold = LOAD_MORE_THRESHOLD,
} = {}) {
  const st = Number(scrollTop)
  const vh = Number(viewportHeight)
  const sh = Number(scrollHeight)
  const th = Number(threshold)
  if (!Number.isFinite(st) || !Number.isFinite(vh) || !Number.isFinite(sh) || sh <= 0) {
    return false
  }
  return st + vh >= sh - (Number.isFinite(th) ? th : LOAD_MORE_THRESHOLD)
}
