import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Trash2, Package, MapPin, Edit2, Calendar } from 'lucide-react'
import { getToken, redirectToLogin, api } from '../services/api'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

function parseImages(images) {
  if (!images) return []
  if (Array.isArray(images)) return images
  if (typeof images === 'string') {
    try { return JSON.parse(images) } catch { return [] }
  }
  return []
}

function parsePricing(pricing) {
  if (!pricing) return []
  if (Array.isArray(pricing)) return pricing
  if (typeof pricing === 'string') {
    try { return JSON.parse(pricing) } catch { return [] }
  }
  return []
}

const CYCLE_MULTIPLIER = {
  day: 1,
  week: 6,
  month: 25,
}

function calculateItemAmount(item) {
  const pricing = parsePricing(item.pricing)
  const dailyRent = pricing[0]?.daily_rent || 0
  const deposit = pricing[0]?.deposit || 0
  const shippingFee = pricing[0]?.shipping_fee || 0
  const cycle = item.cycle || 'month'
  const termCount = item.lease_term || 1
  const multiplier = CYCLE_MULTIPLIER[cycle] || 25
  const rent = dailyRent * multiplier * termCount
  return { rent, deposit, shippingFee, total: rent + deposit + shippingFee }
}

function calculateDeadline(item) {
  const today = new Date()
  const cycle = item.cycle || 'month'
  const termCount = item.lease_term || 1
  if (cycle === 'day') {
    today.setDate(today.getDate() + termCount)
  } else if (cycle === 'week') {
    today.setDate(today.getDate() + termCount * 7)
  } else {
    today.setMonth(today.getMonth() + termCount)
  }
  return today.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
}

export default function Cart() {
  const navigate = useNavigate()
  const [cart, setCart] = useState({ items: [] })
  const [grouped, setGrouped] = useState({})
  const [address, setAddress] = useState(localStorage.getItem('user_address') || '')
  const [showAddressModal, setShowAddressModal] = useState(false)
  const [tempAddress, setTempAddress] = useState('')

  useEffect(() => {
    const data = JSON.parse(localStorage.getItem('cart') || '{"items":[]}')
    setCart(data)
    const groups = {}
    for (const item of data.items) {
      const tid = item.tenant_id || 'unknown'
      const siteId = item.site_id || 'unknown'
      const siteName = item.site_name || '未知网点'
      if (!groups[tid]) {
        groups[tid] = { name: item.tenant_name || '商户', sites: {} }
      }
      if (!groups[tid].sites[siteId]) {
        groups[tid].sites[siteId] = { name: siteName, items: [] }
      }
      groups[tid].sites[siteId].items.push(item)
    }
    setGrouped(groups)
    setTempAddress(address)
  }, [])

  useEffect(() => {
    const token = getToken()
    const pending = sessionStorage.getItem('pending_order')
    if (token && pending && cart.items.length === 1) {
      sessionStorage.removeItem('pending_order')
      const item = cart.items[0]
      const amount = calculateItemAmount(item)
      const returnDate = calculateDeadline(item)
      navigate('/success', {
        state: {
          order_id: 'TL' + Date.now(),
          instrument_name: item.name,
          instrument_sn: item.sn,
          category_name: item.category_name,
          site_name: item.site_name,
          site_address: item.site_address,
          tenant_name: item.tenant_name,
          lease_term: `${item.lease_term || 1}${item.cycle === 'day' ? '天' : item.cycle === 'week' ? '周' : '个月'}`,
          return_date: returnDate,
          total_amount: amount.total,
        },
      })
    } else if (token && pending) {
      sessionStorage.removeItem('pending_order')
    }
  }, [cart.items.length])

  const recalculateGroups = (items) => {
    const groups = {}
    for (const item of items) {
      const tid = item.tenant_id || 'unknown'
      const siteId = item.site_id || 'unknown'
      const siteName = item.site_name || '未知网点'
      if (!groups[tid]) {
        groups[tid] = { name: item.tenant_name || '商户', sites: {} }
      }
      if (!groups[tid].sites[siteId]) {
        groups[tid].sites[siteId] = { name: siteName, items: [] }
      }
      groups[tid].sites[siteId].items.push(item)
    }
    return groups
  }

  const removeItem = (instrumentId) => {
    const updated = cart.items.filter(i => i.instrument_id !== instrumentId)
    const newCart = { items: updated }
    localStorage.setItem('cart', JSON.stringify(newCart))
    setCart(newCart)
    setGrouped(recalculateGroups(updated))
    window.dispatchEvent(new Event('cartUpdated'))
  }

  const clearInvalidItems = () => {
    const updated = cart.items.filter(i => i.stock_status === 'available')
    const newCart = { items: updated }
    localStorage.setItem('cart', JSON.stringify(newCart))
    setCart(newCart)
    setGrouped(recalculateGroups(updated))
  }

  const updateAddress = () => {
    setAddress(tempAddress)
    localStorage.setItem('user_address', tempAddress)
    setShowAddressModal(false)
  }

  const calculateTotals = () => {
    let totalRent = 0
    let totalDeposit = 0
    let shippingFeePerSite = {}
    for (const tenantData of Object.values(grouped)) {
      for (const siteData of Object.values(tenantData.sites)) {
        const siteKey = Object.keys(tenantData.sites).find(k => tenantData.sites[k] === siteData)
        shippingFeePerSite[siteKey] = shippingFeePerSite[siteKey] || 0
        for (const item of siteData.items) {
          const amount = calculateItemAmount(item)
          totalRent += amount.rent
          totalDeposit += amount.deposit
          const pricing = parsePricing(item.pricing)
          const shippingFee = pricing[0]?.shipping_fee || 0
          shippingFeePerSite[siteKey] += shippingFee
        }
      }
    }
    const totalShipping = Object.values(shippingFeePerSite).reduce((a, b) => a + b, 0)
    return { totalRent, totalDeposit, totalShipping, grandTotal: totalRent + totalDeposit + totalShipping }
  }

  const totals = calculateTotals()

  const handleOrder = async () => {
    const token = getToken()
    if (!token) {
      sessionStorage.setItem('post_auth_redirect', '/cart')
      sessionStorage.setItem('pending_order', 'true')
      redirectToLogin()
      return
    }
    sessionStorage.removeItem('pending_order')
    if (cart.items.length === 1) {
      const item = cart.items[0]
      const amount = calculateItemAmount(item)
      const returnDate = calculateDeadline(item)

      try {
        const resp = await api.post('/orders', {
          instrument_id: item.instrument_id,
          start_date: new Date().toISOString().split('T')[0],
          end_date: returnDate,
        })

        if (resp.data?.code === 20000 || resp.data?.code === 20100) {
          clearCart()
          navigate('/success', {
            state: {
              order_id: resp.data.data.order_id || 'TL' + Date.now(),
              instrument_name: item.name,
              instrument_sn: item.sn,
              category_name: item.category_name,
              site_name: item.site_name,
              site_address: item.site_address,
              tenant_name: item.tenant_name,
              lease_term: `${item.lease_term || 1}${item.cycle === 'day' ? '天' : item.cycle === 'week' ? '周' : '个月'}`,
              return_date: returnDate,
              total_amount: amount.total,
            },
          })
        } else {
          alert(resp.data?.message || '下单失败')
        }
      } catch (err) {
        console.error('Order failed:', err)
        alert('下单失败: ' + (err.message || '未知错误'))
      }
    } else {
      alert('多乐器下单功能开发中，请分别下单')
    }
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">购物车 ({cart.items.length})</h1>
        {cart.items.length > 0 && (
          <button onClick={clearInvalidItems} className="ml-auto text-sm text-white/70">
            清理失效
          </button>
        )}
      </div>

      <div className="p-4 space-y-4">
        {cart.items.length === 0 ? (
          <div className="bg-white rounded-xl p-8 text-center">
            <Package size={48} className="text-gray-300 mx-auto mb-3" />
            <p className="text-gray-400">购物车为空</p>
            <button
              onClick={() => navigate('/')}
              className="mt-4 px-6 py-2 bg-brand-primary text-white rounded-lg"
            >
              去逛逛
            </button>
          </div>
        ) : (
          <>
            {Object.entries(grouped).map(([tenantId, tenantData]) => (
              <div key={tenantId} className="bg-white rounded-xl p-4">
                <h2 className="font-bold text-lg text-gray-800 mb-3">{tenantData.name}</h2>
                {Object.entries(tenantData.sites).map(([siteId, siteData]) => (
                  <div key={siteId} className="ml-2 border-l-2 border-orange-200 pl-3 mb-4">
                    <p className="text-sm text-orange-600 font-medium mb-2">📍 {siteData.name}</p>
                    {siteData.items.map((item) => {
                      const images = parseImages(item.images)
                      const amount = calculateItemAmount(item)
                      return (
                        <div key={item.instrument_id} className="flex gap-3 py-2 border-b border-gray-100">
                          <img
                            src={images[0] || PLACEHOLDER_IMAGE}
                            alt={item.name}
                            className="w-16 h-16 object-cover rounded-lg bg-gray-100"
                          />
                          <div className="flex-1">
                            <p className="font-medium text-sm text-gray-800">{item.name}</p>
                            <p className="text-xs text-gray-500">{item.brand} {item.model}</p>
                            <div className="flex items-center gap-2 mt-1">
                              {item.cycle === 'month' ? `${item.lease_term || 1}个月` : `${item.lease_term || 1}${item.cycle === 'day' ? '天' : '周'}`}
                              <span className="text-xs text-gray-500">→ {calculateDeadline(item)}</span>
                            </div>
                            <p className="text-orange-600 font-bold text-sm mt-1">
                              ¥{amount.rent.toFixed(0)} + ¥{amount.deposit} 押{(parsePricing(item.pricing)[0]?.shipping_fee || 0) > 0 ? ` + ¥${parsePricing(item.pricing)[0].shipping_fee} 运` : ''}
                            </p>
                          </div>
                          <button onClick={() => removeItem(item.instrument_id)}>
                            <Trash2 size={18} className="text-gray-400" />
                          </button>
                        </div>
                      )
                    })}
                  </div>
                ))}
              </div>
            ))}

            <div className="bg-white rounded-xl p-4">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  <MapPin size={18} className="text-gray-400" />
                  <span className="font-medium">收货地址</span>
                </div>
                <button onClick={() => setShowAddressModal(true)} className="text-brand-primary text-sm flex items-center gap-1">
                  <Edit2 size={14} /> 修改
                </button>
              </div>
              <p className="text-gray-600 text-sm">{address || '请添加收货地址'}</p>
            </div>

            <div className="bg-white rounded-xl p-4">
              <h3 className="font-bold mb-3">费用明细</h3>
              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-gray-600">租金</span>
                  <span className="font-medium">¥{totals.totalRent.toFixed(0)}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">押金</span>
                  <span className="font-medium">¥{totals.totalDeposit}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-600">物流费</span>
                  <span className="font-medium">¥{totals.totalShipping}</span>
                </div>
                <div className="border-t pt-2 flex justify-between font-bold text-lg">
                  <span>合计</span>
                  <span className="text-orange-600">¥{totals.grandTotal.toFixed(0)}</span>
                </div>
              </div>
            </div>
          </>
        )}
      </div>

      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <button
          onClick={handleOrder}
          disabled={cart.items.length === 0}
          className="w-full bg-brand-primary text-white py-3 rounded-lg font-bold disabled:opacity-50"
        >
          支付 ¥{totals.grandTotal.toFixed(0)}
        </button>
      </div>

      {showAddressModal && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center">
          <div className="bg-white rounded-xl p-6 mx-4 w-full max-w-sm">
            <h3 className="font-bold text-lg mb-4">修改收货地址</h3>
            <textarea
              value={tempAddress}
              onChange={(e) => setTempAddress(e.target.value)}
              placeholder="请输入详细收货地址"
              className="w-full border rounded-lg p-3 text-sm mb-4"
              rows={3}
            />
            <div className="flex gap-3">
              <button
                onClick={() => setShowAddressModal(false)}
                className="flex-1 py-2 border rounded-lg"
              >
                取消
              </button>
              <button onClick={updateAddress} className="flex-1 py-2 bg-brand-primary text-white rounded-lg">
                确认
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
