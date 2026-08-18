// Cross-end navigation mapping (issue-1673).
// H5 uses react-router paths; weapp must use the full /pages-weapp/... page
// paths — Taro's react-router shim converts navigate('/xxx') into
// Taro.navigateTo('/pages/xxx/index'), which does NOT exist in the weapp
// build (all pages live under pages-weapp/), so navigation silently fails.
// Single source of truth: add new cross-end routes here (see AGENTS.md).
const ROUTE_MAP = [
  { match: '/',            type: 'switchTab',  url: '/pages-weapp/home/index' },
  { match: '/profile',     type: 'switchTab',  url: '/pages-weapp/profile/index' },
  { match: '/my-leases',   type: 'switchTab',  url: '/pages-weapp/my-leases/index' },
  { match: '/my-repairs',  type: 'navigateTo', url: '/pages-weapp/my-repairs/index' },
  { match: '/messages',    type: 'navigateTo', url: '/pages-weapp/messages/index' },
  { match: '/message-detail', type: 'navigateTo', url: '/pages-weapp/message-detail/index' },
  { match: '/profile/edit', type: 'navigateTo', url: '/pages-weapp/profile/edit/index' },
  { match: '/membership',  type: 'navigateTo', url: '/pages-weapp/membership/index' },
  { match: '/checkout',    type: 'navigateTo', url: '/pages-weapp/checkout/index' },
  { match: '/cart',        type: 'navigateTo', url: '/pages-weapp/cart/index' },
  { match: '/payment',     type: 'navigateTo', url: '/pages-weapp/payment/index' },
  { match: '/order/:id',   type: 'navigateTo', url: (p) => `/pages-weapp/order-detail/index?id=${p.id}` },
  { match: '/staff/orders', type: 'navigateTo', url: '/pages-weapp/staff-orders/index' },
  { match: '/staff/orders/:id', type: 'navigateTo', url: (p) => `/pages-weapp/order-detail/index?id=${p.id}` },
  { match: '/staff/shipping', type: 'navigateTo', url: '/pages-weapp/shipping-interface/index' },
  { match: '/staff/receiving', type: 'navigateTo', url: '/pages-weapp/receiving-interface/index' },
  { match: '/staff/receive', type: 'navigateTo', url: '/pages-weapp/staff-receive-confirm/index' },
  { match: '/instrument/:id', type: 'navigateTo', url: (p) => `/pages-weapp/detail/index?id=${p.id}` },
  { match: '/return-settlement', type: 'navigateTo', url: '/pages-weapp/return-settlement/index' },
  { match: '/receive-confirm', type: 'navigateTo', url: '/pages-weapp/receive-confirm/index' },
  { match: '/return-confirm', type: 'navigateTo', url: '/pages-weapp/return-confirm/index' },
  { match: '/content',     type: 'navigateTo', url: '/pages-weapp/content/index' },
  { match: '/create-repair', type: 'navigateTo', url: '/pages-weapp/create-repair/index' },
  { match: '/repair',      type: 'navigateTo', url: '/pages-weapp/repair/index' },
  { match: '/repair-request', type: 'navigateTo', url: '/pages-weapp/repair-request/index' },
  { match: '/repair-quote', type: 'navigateTo', url: '/pages-weapp/repair-quote/index' },
  { match: '/payment-complete', type: 'navigateTo', url: '/pages-weapp/payment-complete/index' },
  { match: '/repair-payment-complete', type: 'navigateTo', url: '/pages-weapp/repair-payment-complete/index' },
  { match: '/receiving-repair-scan', type: 'navigateTo', url: '/pages-weapp/receiving-repair-scan/index' },
  { match: '/repair-scan', type: 'navigateTo', url: '/pages-weapp/repair-scan/index' },
]

// Translate an H5 react-router path (optionally with query string) into a
// weapp navigation descriptor { type, url }, or null when the route has no
// weapp target. Query string is preserved and appended to the target url.
export function toWeappRoute(h5Path) {
  if (typeof h5Path !== 'string') return null
  const [path, query] = h5Path.split('?')
  const segments = path.split('/').filter(Boolean)
  for (const rule of ROUTE_MAP) {
    const ruleSegs = rule.match.split('/').filter(Boolean)
    if (ruleSegs.length !== segments.length) continue
    const params = {}
    let matched = true
    for (let i = 0; i < ruleSegs.length; i++) {
      const rs = ruleSegs[i]
      if (rs.startsWith(':')) {
        params[rs.slice(1)] = decodeURIComponent(segments[i])
      } else if (rs !== segments[i]) {
        matched = false
        break
      }
    }
    if (!matched) continue
    const base = typeof rule.url === 'function' ? rule.url(params) : rule.url
    return { type: rule.type, url: query ? `${base}?${query}` : base }
  }
  return null
}