import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..')

// Phase 1 critical pages: confirm order / payment / order detail / cart / profile / register.
// Both H5 implementations and pages-weapp copies are rendered.
export const criticalRoutes = [
  {
    name: 'H5 /checkout',
    pattern: '/checkout',
    entry: '/checkout?id=inst-1',
    markers: ['费用明细', '押金档位'],
    load: () => import('../../src/pages/checkout'),
  },
  {
    name: 'H5 /checkout (zero deposit)',
    pattern: '/checkout',
    entry: '/checkout?id=inst-zero',
    markers: ['费用明细', '零押金'],
    load: () => import('../../src/pages/checkout'),
  },
  {
    name: 'H5 /payment',
    pattern: '/payment',
    entry: '/payment?type=rent&id=order-1',
    marker: '支付确认',
    load: () => import('../../src/pages/Payment'),
  },
  {
    name: 'H5 /order/:id',
    pattern: '/order/:id',
    entry: '/order/order-1',
    marker: '费用信息',
    load: () => import('../../src/pages/OrderDetail'),
  },
  {
    name: 'H5 /cart',
    pattern: '/cart',
    entry: '/cart',
    marker: '购物车',
    load: () => import('../../src/pages/Cart'),
  },
  {
    name: 'H5 /profile',
    pattern: '/profile',
    entry: '/profile',
    marker: '退出登录',
    load: () => import('../../src/pages/Profile'),
  },
  {
    name: 'H5 /register',
    pattern: '/register',
    entry: '/register',
    marker: '注册账号',
    load: () => import('../../src/pages/Register'),
  },
  {
    name: 'weapp checkout',
    pattern: '/weapp-checkout',
    entry: '/weapp-checkout?id=inst-1',
    markers: ['费用明细', '差额押金（减免）'],
    load: () => import('../../src/pages-weapp/checkout'),
  },
  {
    name: 'weapp checkout (zero deposit)',
    pattern: '/weapp-checkout',
    entry: '/weapp-checkout?id=inst-zero',
    markers: ['费用明细', '零押金'],
    load: () => import('../../src/pages-weapp/checkout'),
  },
  {
    name: 'weapp payment',
    pattern: '/weapp-payment',
    entry: '/weapp-payment?type=rent&id=order-1',
    marker: '支付确认',
    load: () => import('../../src/pages-weapp/payment'),
  },
  {
    name: 'weapp order-detail',
    pattern: '/weapp-order-detail',
    entry: '/weapp-order-detail?id=order-1',
    marker: '费用明细',
    load: () => import('../../src/pages-weapp/order-detail'),
  },
  {
    name: 'weapp cart',
    pattern: '/weapp-cart',
    entry: '/weapp-cart',
    marker: '购物车',
    load: () => import('../../src/pages-weapp/cart'),
  },
  {
    name: 'weapp profile',
    pattern: '/weapp-profile',
    entry: '/weapp-profile',
    marker: '退出登录',
    load: () => import('../../src/pages-weapp/profile'),
  },
  {
    name: 'weapp profile-complete',
    pattern: '/weapp-profile-complete',
    entry: '/weapp-profile-complete',
    marker: '注册账号',
    load: () => import('../../src/pages-weapp/profile-complete'),
  },
]

// Discovery helpers keep the route/page list in sync with App-H5.jsx and app.config.ts.
// Phase 2 can turn these into the full render matrix; Phase 1 runs criticalRoutes.
export function discoverH5Routes() {
  const src = fs.readFileSync(path.join(ROOT, 'src/App-H5.jsx'), 'utf8')
  return [...src.matchAll(/<Route\s+path="([^"]+)"\s+element=\{<[^>]*?>?\s*<([A-Za-z0-9]+)/g)]
    .map((m) => ({ path: m[1], component: m[2] }))
}

export function discoverWeappPages() {
  const src = fs.readFileSync(path.join(ROOT, 'src/app.config.ts'), 'utf8')
  const match = src.match(/const weappPages = \[([\s\S]*?)\]/)
  if (!match) return []
  return [...match[1].matchAll(/'([^']+)'/g)].map((m) => m[1])
}
