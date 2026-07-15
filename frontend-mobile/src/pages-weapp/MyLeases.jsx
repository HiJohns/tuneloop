import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, ScrollView, Image } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'
import BottomNav from '../components-weapp/BottomNav'

const MAIN_TABS = [
  { key: 'active', label: '进行中' },
  { key: 'completed', label: '已完成' },
]

const SUB_FILTERS = {
  active: [
    { key: '', label: '全部' },
    { key: 'reserved', label: '未支付' },
    { key: 'paid', label: '待发货' },
    { key: 'shipped', label: '已发货' },
    { key: 'in_lease', label: '租赁中' },
    { key: 'expired', label: '已超期' },
    { key: 'returning', label: '归还中' },
  ],
  completed: [
    { key: 'completed', label: '已完成' },
    { key: 'cancelled', label: '已取消' },
  ],
}

const STATUS_LABELS = {
  reserved: '未支付', paid: '待发货', pending_shipment: '待发货',
  shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '超期', transferred: '已过户',
}

const STATUS_COLORS = {
  reserved: { backgroundColor: '#dbeafe', color: '#1d4ed8' },
  paid: { backgroundColor: '#ffedd5', color: '#c2410c' },
  pending_shipment: { backgroundColor: '#ffedd5', color: '#c2410c' },
  shipped: { backgroundColor: '#dcfce7', color: '#15803d' },
  in_lease: { backgroundColor: '#e0e7ff', color: '#4338ca' },
  returning: { backgroundColor: '#fef9c3', color: '#a16207' },
  returned: { backgroundColor: '#f3f4f6', color: '#4b5563' },
  completed: { backgroundColor: '#f3f4f6', color: '#4b5563' },
  cancelled: { backgroundColor: '#fee2e2', color: '#b91c1c' },
  expired: { backgroundColor: '#fee2e2', color: '#b91c1c' },
  transferred: { backgroundColor: '#f3e8ff', color: '#7e22ce' },
}

const getActualRent = (order) => {
  if (!order.pricing_breakdown) return 0
  try {
    const pb = typeof order.pricing_breakdown === 'string'
      ? JSON.parse(order.pricing_breakdown)
      : order.pricing_breakdown
    return pb?.total_amount || pb?.actual_rent_amount || 0
  } catch { return 0 }
}

const isScheduledPeriod = (status) =>
  ['completed', 'returned', 'returning', 'cancelled'].includes(status)

const MAIN_INCLUDE = {
  active: ['reserved', 'paid', 'pending_shipment', 'shipped', 'in_lease', 'expired', 'returning'],
  completed: ['returned', 'completed', 'cancelled', 'transferred'],
}

export default function MyLeases() {
  const nav = (url) => { Taro.navigateTo({ url }) }
  const instance = Taro.getCurrentInstance()
  const routerParams = instance.router?.params || {}
  const initStatus = routerParams.status || ''
  const initTab = initStatus && ['returned', 'completed', 'cancelled'].includes(initStatus) ? 'completed' : 'active'
  const [mainTab, setMainTab] = useState(initTab)
  const [subFilter, setSubFilter] = useState(initStatus)
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)

  const baseUrl = env.apiBaseUrl
  const fixImg = (url) => url && !url.startsWith('http') && !url.startsWith('data:') ? baseUrl.replace(/\/api$/, '') + url : url

  const token = getToken()
  const isStaff = (() => {
    try {
      if (!token) return false
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload?.role && payload.role !== 'USER'
    } catch { return false }
  })()

  useEffect(() => {
    setPage(1)
    setOrders([])
    setHasMore(true)
  }, [baseUrl, mainTab, subFilter])

  useEffect(() => {
    const fetchOrders = async () => {
      if (page === 1) setLoading(true)
      else setLoadingMore(true)
      try {
        const statusKey = subFilter || ''
        let url = `${baseUrl}/orders?page=${page}&page_size=10`
        if (statusKey) url += `&status=${statusKey}`
        const resp = await apiFetch(url)
        const result = await resp.json()
        let list = []
        if (result.code === 20000) {
          list = result.data?.list || []
        }
        if (!subFilter) {
          list = list.filter(o => MAIN_INCLUDE[mainTab]?.includes(o.status))
        }
        setOrders(prev => page === 1 ? list : [...prev, ...list])
        setHasMore((result.data?.total || 0) > (page * 10))
      } catch (err) {
        console.error('Failed to fetch orders:', err)
      }
      setLoading(false)
      setLoadingMore(false)
    }
    fetchOrders()
  }, [page, baseUrl, mainTab, subFilter])

  const handleCancelFromList = (orderId, status) => {
    Taro.showModal({
      title: '取消订单',
      content: '确认取消该订单？取消后不可恢复。',
      success: async (res) => {
        if (!res.confirm) return
        try {
          const resp = await apiFetch(`${baseUrl}/orders/${orderId}/cancel-by-user`, { method: 'POST' })
          const result = await resp.json()
          if (result.code === 20000) {
            if (result.data?.refund_amount > 0) {
              Taro.redirectTo({ url: `/pages-weapp/payment/index?type=refund&id=${orderId}` })
            } else {
              setOrders(prev => prev.map(o => o.id === orderId ? { ...o, status: 'cancelled' } : o))
            }
          } else {
            Taro.showModal({ title: '取消失败', content: result.message, showCancel: false })
          }
        } catch (err) {
          Taro.showModal({ title: '取消失败', content: err.message, showCancel: false })
        }
      },
    })
  }

  return (
    <View style={{ display: 'flex', flexDirection: 'column', height: '100vh', backgroundColor: '#FDFBF7' }}>
      <View style={{ background: 'linear-gradient(to bottom, #FDF4E7, #fff)', paddingLeft: 16, paddingRight: 16, paddingTop: 16, paddingBottom: 16 }}>
        <Text style={{ fontSize: 18, fontWeight: '900', color: '#000' }}>我的租约</Text>
      </View>

      {/* Main Tabs */}
      <View style={{ paddingLeft: 16, paddingRight: 16, paddingTop: 12, paddingBottom: 4, display: 'flex' }}>
        {MAIN_TABS.map(tab => (
          <Button
            key={tab.key}
            onClick={() => { setMainTab(tab.key); setSubFilter('') }}
            style={{
              padding: '8px 20px',
              borderRadius: 999,
              fontSize: 14,
              fontWeight: '900',
              backgroundColor: mainTab === tab.key ? '#000' : '#fff',
              color: mainTab === tab.key ? '#fff' : '#71717a',
              marginRight: 8
            }}
          >
            {tab.label}
          </Button>
        ))}
      </View>

      {/* Sub Filters */}
      <ScrollView scrollX style={{ paddingLeft: 16, paddingRight: 16, paddingTop: 8, paddingBottom: 8 }} enhanced showScrollbar={false}>
        <View style={{ display: 'flex', whiteSpace: 'nowrap' }}>
          {SUB_FILTERS[mainTab].map(f => (
            <Button
              key={f.key}
              onClick={() => setSubFilter(f.key)}
              style={{
                padding: '4px 12px',
                borderRadius: 999,
                fontSize: 12,
                fontWeight: '700',
                backgroundColor: subFilter === f.key ? '#000' : '#fff',
                color: subFilter === f.key ? '#fff' : '#a1a1aa',
                marginRight: 8
              }}
            >
              {f.label}
            </Button>
          ))}
        </View>
      </ScrollView>

      <ScrollView scrollY style={{ flex: '1 1 0%', paddingLeft: 16, paddingRight: 16, minHeight: 0, overflowY: 'auto' }}
        onScrollToLower={() => {
          if (!loadingMore && hasMore) {
            setLoadingMore(true)
            setPage(prev => prev + 1)
          }
        }}
        lowerThreshold={50}
        enableBackToTop
      >
        {loading ? (
          <View style={{ textAlign: 'center', paddingTop: 64, paddingBottom: 64, color: '#a1a1aa', fontWeight: '500' }}>加载中...</View>
        ) : orders.length === 0 ? (
          <View style={{ textAlign: 'center', paddingTop: 64, paddingBottom: 64 }}>
            <Text style={{ fontSize: 48, color: '#e4e4e7', marginBottom: 16, textAlign: 'center' }}>📦</Text>
            <Text style={{ color: '#a1a1aa', fontWeight: '500', textAlign: 'center' }}>暂无租约</Text>
          </View>
        ) : (
          <>
          <View>
              {orders.map(order => {
              const showReturn = order.status === 'in_lease'
              const showPay = order.status === 'reserved'
              const showCancel = ['reserved', 'paid', 'pending_shipment'].includes(order.status)
              const showConfirm = order.status === 'shipped'
              const isTerminal = ['completed', 'returned', 'cancelled'].includes(order.status)

              return (
              <View
                key={order.id}
                style={{ backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', padding: 16, marginBottom: 12 }}
                onClick={() => nav(`/pages-weapp/order-detail/index?id=${order.id}`)}
              >
                <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                  <Text style={{ fontSize: 14, fontWeight: '900', color: '#000', flex: '1 1 0%', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    订单 #{order.id?.slice(0, 8)}
                  </Text>
                  <Text style={{ fontSize: 12, padding: '2px 8px', borderRadius: 999, fontWeight: '700', flexShrink: 0, marginLeft: 8, textAlign: 'center', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', lineHeight: 1.4, ...(STATUS_COLORS[order.status] || { backgroundColor: '#f3f4f6', color: '#4b5563' }) }}>
                    {STATUS_LABELS[order.status] || order.status}
                  </Text>
                </View>
                <View style={{ display: 'flex', flexDirection: 'row' }}>
                  <View style={{ flex: '1 1 0%' }}>
                <View style={{ fontSize: 14 }}>
                  {order.instrument_name && (
                    <View style={{ marginBottom: 4 }}><Text style={{ color: '#a1a1aa', fontWeight: '500' }}>
                      乐器: <Text style={{ color: '#000', fontWeight: '500' }}>{order.instrument_name}</Text>
                      {order.instrument_category && <Text style={{ color: '#d4d4d8', marginLeft: 4 }}>({order.instrument_category})</Text>}
                    </Text></View>
                  )}
                  {order.created_at && (
                    <View style={{ marginBottom: 4 }}><Text style={{ color: '#a1a1aa', fontWeight: '500' }}>
                      下单日: <Text style={{ color: '#000', fontWeight: '500' }}>{formatDisplayDate(order.created_at)}</Text>
                    </Text></View>
                  )}
                  <View style={{ display: 'flex', alignItems: 'center' }}>
                    <Text style={{ color: '#a1a1aa', fontWeight: '500', marginRight: 8 }}>总金额:</Text>
                    <Text style={{ color: '#000', fontWeight: '900' }}>¥{(getActualRent(order) || 0) + (order.deposit || 0) + (order.shipping_fee || 0)}</Text>
                  </View>
                </View>
                  </View>
                  {order.cover_image && <Image src={fixImg(order.cover_image)} style={{ width: 80, height: 80, borderRadius: 8, marginLeft: 12 }} mode="aspectFill" />}
                </View>
                <View style={{ marginTop: 12, display: 'flex' }}>
                  {!isTerminal && (
                    <>
                      {showPay && (
                        <Button
                          onClick={(e) => { e.stopPropagation(); Taro.redirectTo({ url: `/pages-weapp/payment/index?type=rent&id=${order.id}` }) }}
                          style={{ flex: '1 1 0%', paddingTop: 10, paddingBottom: 10, backgroundColor: '#000', color: '#fff', borderRadius: 12, fontWeight: '900', fontSize: 14, marginRight: 8 }}
                        >
                          立即支付
                        </Button>
                      )}
                      {showConfirm && (
                        <Button
                          onClick={(e) => { e.stopPropagation(); nav(`/pages-weapp/order-detail/index?id=${order.id}`) }}
                          style={{ flex: '1 1 0%', paddingTop: 10, paddingBottom: 10, backgroundColor: '#000', color: '#fff', borderRadius: 12, fontWeight: '900', fontSize: 14, marginRight: 8 }}
                        >
                          确认收货
                        </Button>
                      )}
                      {showReturn && (
                        <Button
                          onClick={(e) => {
                            e.stopPropagation()
                            nav(`/pages-weapp/return-confirm/index?order_id=${order.id}&instrument=${order.instrument_id}`)
                          }}
                          style={{ flex: '1 1 0%', paddingTop: 10, paddingBottom: 10, backgroundColor: '#000', color: '#fff', borderRadius: 12, fontWeight: '900', fontSize: 14, marginRight: 8 }}
                        >
                          归还乐器
                        </Button>
                      )}
                      {showCancel && (
                        <Button
                          onClick={(e) => { e.stopPropagation(); handleCancelFromList(order.id, order.status) }}
                          style={{ flex: '1 1 0%', paddingTop: 10, paddingBottom: 10, backgroundColor: '#f4f4f5', color: '#52525b', borderRadius: 12, fontWeight: '900', fontSize: 14, marginRight: 8 }}
                        >
                          取消订单
                        </Button>
                      )}
                      {!showPay && !showConfirm && !showReturn && !showCancel && (
                        <View style={{ width: '100%', paddingTop: 10, paddingBottom: 10, backgroundColor: '#f4f4f5', borderRadius: 12, textAlign: 'center' }}>
                          <Text style={{ color: '#a1a1aa', fontWeight: '900', fontSize: 14, textAlign: 'center' }}>等待处理</Text>
                        </View>
                      )}
                    </>
                  )}
                </View>
              </View>
              )
            })}
          </View>
          {loadingMore && (
            <View style={{ textAlign: 'center', paddingTop: 16, paddingBottom: 16 }}>
              <Text style={{ color: '#a1a1aa', fontSize: 14, textAlign: 'center' }}>加载更多...</Text>
            </View>
          )}
          </>
        )}
      </ScrollView>

      <BottomNav
        active="rent"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => nav('/pages-weapp/home/index') },
          { key: 'rent', icon: '🪕', label: '租赁', onClick: () => nav('/pages-weapp/my-leases/index') },
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => nav('/pages-weapp/my-repairs/index') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => nav('/pages-weapp/profile/index') },
        ]}
      />
    </View>
  )
}
