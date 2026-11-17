/**
 * #2387：跨上下文文本复制。
 *
 * 背景：`navigator.clipboard` 仅在安全上下文（https / localhost）可用；内网以
 * `http://<内网IP>:<port>` 访问时该 API 不存在，直接调用必然失败。
 *
 * 策略（渐进增强）：
 *   1. 优先 Clipboard API；
 *   2. 不可用/失败 → 隐藏 textarea + `document.execCommand('copy')` 降级（http 内网可用）；
 *   3. 两条路径都失败 → 返回 false，并选中调用方指定的回退文本节点（便于 Ctrl+C）。
 *
 * @param {string} text 要复制的文本；空串直接返回 false（不抛错）。
 * @param {{ fallbackEl?: HTMLElement | null }} [options] 复制失败时用于选中文本的节点。
 * @returns {Promise<boolean>} 是否复制成功。
 */
export async function copyText(text, options = {}) {
  if (typeof text !== 'string' || text === '') return false

  if (typeof navigator !== 'undefined' && navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 无用户手势/权限拒绝等 → 走降级路径
    }
  }

  const copied = legacyCopy(text)
  if (!copied) selectFallback(options.fallbackEl)
  return copied
}

/**
 * 降级复制：隐藏 textarea + execCommand('copy')。
 * 返回 execCommand 的结果（true = 浏览器已执行复制）。
 */
function legacyCopy(text) {
  if (typeof document === 'undefined' || !document.body) return false

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.setAttribute('readonly', '')
  textarea.style.position = 'fixed'
  textarea.style.top = '-9999px'
  textarea.style.left = '-9999px'
  textarea.style.opacity = '0'

  const selection = document.getSelection()
  const previousRange = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null

  document.body.appendChild(textarea)
  textarea.focus()
  textarea.select()
  try {
    textarea.setSelectionRange(0, textarea.value.length)
  } catch {
    // 部分旧浏览器不支持 setSelectionRange，忽略
  }

  let copied = false
  try {
    copied = document.execCommand('copy')
  } catch {
    copied = false
  }

  document.body.removeChild(textarea)
  if (previousRange && selection) {
    selection.removeAllRanges()
    selection.addRange(previousRange)
  }
  return copied
}

/** 复制失败时选中回退节点内容，便于用户 Ctrl+C。 */
function selectFallback(element) {
  if (!element || typeof document === 'undefined') return
  try {
    const range = document.createRange()
    range.selectNodeContents(element)
    const selection = document.getSelection()
    if (!selection) return
    selection.removeAllRanges()
    selection.addRange(range)
  } catch {
    // 选中失败不影响错误提示
  }
}
