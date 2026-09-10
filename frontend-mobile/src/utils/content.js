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
