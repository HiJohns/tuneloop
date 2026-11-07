const tiers = [
  { tier: 1, days: 7, days_max: 7, rate: 1000, daily_rate: 1000, subtotal: 7000 },
  { tier: 2, days: 23, days_max: 30, rate: 800, daily_rate: 800, subtotal: 18400 },
  { tier: 3, days: 30, days_max: -1, rate: 600, daily_rate: 600, subtotal: 18000 },
]

const instrument = {
  id: 'inst-1',
  name: '冒烟测试乐器',
  brand: 'Test',
  model: 'T1',
  sn: 'SN-SMOKE-1',
  category_name: '吉他',
  level_name: '专业',
  deposit: 10000,
  daily_rent: 1000,
  rent: 30000,
  pricing: '[]',
  images: [],
  cover_image: '',
  poster: '',
  stock_status: 'available',
  description: '用于页面渲染冒烟',
}

const pricingV2 = {
  base_daily_rate: 1000,
  deposit: 10000,
  shipping_fee: 0,
  tiers: [
    { days_max: 7, daily_rate: 1000 },
    { days_max: 30, daily_rate: 800 },
    { days_max: -1, daily_rate: 600 },
  ],
}

const address = {
  id: 'addr-1',
  recipient_name: '测试收货人',
  phone: '13800000000',
  province: '北京市',
  city: '北京市',
  district: '朝阳区',
  detail: '测试路 1 号',
  postal_code: '100000',
  is_default: true,
}

const user = {
  id: 'user-1',
  iam_sub: 'user-1',
  name: '测试用户',
  nickname: '测试用户',
  phone: '13800000000',
  email: 'smoke@example.test',
  avatar: '',
  role: 'STAFF',
  oid: 'org-1',
  tid: 'tenant-1',
  id_verify_status: 'verified',
  membership_level_id: '',
  membership_level_name: '',
}

const order = {
  id: 'order-1',
  order_no: 'ORDER-SMOKE-1',
  status: 'paid',
  user_id: 'user-1',
  instrument_id: 'inst-1',
  rent_days: 30,
  rent_qty: 30,
  daily_rent: 1000,
  deposit: 10000,
  total_amount: 40000,
  amount: 30000,
  start_date: '2026-11-01',
  end_date: '2026-12-01',
  created_at: '2026-11-01T00:00:00Z',
  payment_records: [],
  status_history: [],
  logs: [],
  settlement: null,
  damage: null,
  address,
  instrument,
  fee_detail: {
    paid_block: {
      contract_rent: { amount: 30000, date: '2026-11-01', tiers },
      deposit: { amount: 10000 },
      subtotal: 40000,
      renewals: [],
    },
    payable_block: {
      actual_rent: { amount: 30000, tiers },
      damage: { amount: 0 },
      refund: { amount: 0 },
      subtotal: 0,
    },
  },
}

const payment = {
  type: 'rent',
  title: '支付确认',
  amount: 40000,
  details: {
    pricing_breakdown: {
      tier_segments: tiers.map(t => ({ tier: t.tier, days: t.days, rate: t.rate, discount: 1, subtotal: t.subtotal })),
      total_amount: 30000,
      deposit: 10000,
      shipping_fee: 0,
    },
    total_paid: 40000,
    cash_paid: 40000,
    gift_used: 0,
    prepaid_used: 0,
    cash_refundable: 0,
    deposit: 10000,
    shipping_fee: 0,
  },
  wallet: { promo_points: 0, max_gift_amount: 0, max_gift_ratio: 0.3 },
}

export const fixtures = {
  tiers,
  instrument,
  pricingV2,
  address,
  addresses: [address],
  guarantors: [{ id: 'g-1', name: '测试担保人', phone: '13900000000', status: 'confirmed' }],
  user,
  order,
  payment,
  counts: { pending_payment: 0, pending_shipment: 0, in_lease: 1, returning: 0, completed: 1, total: 2 },
  siteMember: { sites: [], roles: [] },
  cartItem: {
    instrument_id: 'inst-1',
    name: '冒烟测试乐器',
    brand: 'Test',
    model: 'T1',
    sn: 'SN-SMOKE-1',
    cover_image: '',
    daily_rent: 1000,
    deposit: 10000,
    rent_qty: 30,
    pricing_v2: pricingV2,
    stock_status: 'available',
  },
}
