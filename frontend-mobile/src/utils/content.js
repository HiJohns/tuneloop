// Content rendering helpers shared by H5 and weapp (single source).
// Extracted from ContentPage.jsx (#1830) so the membership handbook can
// reuse the same URL normalization for rich-text rendering.

// Normalize relative src/href URLs in rich-text HTML to absolute ones.
// WeChat mini-program <rich-text> does not resolve relative image URLs —
// they must be full https URLs. `origin` is derived from the API base
// (env.apiBaseUrl minus the /api suffix) so assets resolve to the current
// environment (wx / prewx).
export const normalizeContentUrls = (html, origin) => {
  if (!html || !origin) return html
  return html.replace(/(src|href)=(["'])\/(?!\/)/g, `$1=$2${origin}/`)
}

// Inject inline max-width on every <img> in rich-text HTML.
// WeChat native <rich-text> renders its internal nodes in a WebView that is
// isolated from page/body CSS, so the global app.css `img{max-width:100%}`
// (which only takes effect on H5) cannot constrain weapp images. Inline style
// is the only reliable way to make oversized images fit the screen width.
export const injectResponsiveImageStyle = (html) => {
  if (!html) return html
  return html.replace(/<img(?![^>]*\bstyle=)/gi, '<img style="max-width:100%;height:auto;"')
}

// Split quill-editor standalone image blocks (<p ...><img ...></p>) out of
// rich-text HTML so each image can be rendered as a Taro <Image> component.
// This is required because weapp <rich-text> cannot attach tap handlers to
// its internal nodes, making it impossible to preview the reduced images.
// Returns an ordered array: [{type:'html',content}, {type:'img',src}, ...].
// Non-image segments are preserved verbatim; when no image exists a single
// html segment is returned.
export const splitRichTextImages = (html) => {
  if (!html) return [{ type: 'html', content: html || '' }]
  const hasImage = /<img[\s>]/i.test(html)
  if (!hasImage) return [{ type: 'html', content: html }]

  const segments = []
  const imgBlockRe = /<p[^>]*>[^<]*<img[^>]*src="([^"]+)"[^>]*>[^<]*<\/p>/gi
  let lastIndex = 0
  let match
  while ((match = imgBlockRe.exec(html)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ type: 'html', content: html.slice(lastIndex, match.index) })
    }
    segments.push({ type: 'img', src: match[1] })
    lastIndex = imgBlockRe.lastIndex
  }
  if (lastIndex < html.length) {
    segments.push({ type: 'html', content: html.slice(lastIndex) })
  }
  return segments.length > 0 ? segments : [{ type: 'html', content: html }]
}

// #2049 富文本 → 纯文本摘要（列表卡片用）：去标签 + 解实体 + 折叠空白。
export const htmlToPlainText = (html) => {
  if (!html) return ''
  return String(html)
    .replace(/<[^>]*>/g, ' ')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&amp;/gi, '&')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/\s+/g, ' ')
    .trim()
}
