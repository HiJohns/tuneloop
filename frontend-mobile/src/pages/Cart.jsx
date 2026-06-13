import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input } from '@tarojs/components'
import { ArrowLeft, Trash2, Package, MapPin, Edit2, Calendar } from 'lucide-react'
import { getToken, redirectToLogin, ordersApi, addressesApi } from '../services/api'
import dayjs from 'dayjs'
import { dialog, env, storage, session } from '../platform'
import { formatDisplayDate } from '../utils/format'

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

function calculateItemAmount(item) {
  if (item.calculated_rent !== undefined) {
    const pricing = parsePricing(item.pricing)
    const deposit = pricing[0]?.deposit || 0
    const shippingFee = pricing[0]?.shipping_fee || 0
    return { rent: item.calculated_rent, deposit: deposit, shippingFee: shippingFee, total: item.calculated_rent + deposit + shippingFee }
  }
  const pricing = parsePricing(item.pricing)
  const dailyRent = pricing[0]?.daily_rent || item.base_daily_rate || 0
  const deposit = pricing[0]?.deposit || 0
  const shippingFee = pricing[0]?.shipping_fee || 0
  const startDate = item.start_date ? dayjs(item.start_date) : dayjs()
  const endDate = item.end_date ? dayjs(item.end_date) : startDate.add(1, 'month')
  const days = endDate.diff(startDate, 'day') || 1
  const rent = dailyRent * days
  return { rent, deposit, shippingFee, total: rent + deposit + shippingFee }
}

function calculateDeadline(item) {
  if (item.end_date) return item.end_date
  return dayjs().add(1, 'month').format('YYYY-MM-DD')
}

export default function Cart() {
  const navigate = useNavigate()
  const [cart, setCart] = useState({ items: [] })
  const [grouped, setGrouped] = useState({})
  const [address, setAddress] = useState(storage.getItem('user_address') || '')
  const [showAddressModal, setShowAddressModal] = useState(false)
  const [tempAddress, setTempAddress] = useState('')

  useEffect(() => {
    const data = storage.getJSON('cart', { items: [] })
    setCart(data)
    const groups = {}
    for (const item of data.items) {
      const tid = item.tenant_id || 'unknown'
      const siteId = item.site_id || 'unknown'
      const siteName = item.site_name || '未知网点'
      if (!groups[tid]) {
        groups[tid] = { name: item.tenant_name || item.tenant_id || '', sites: {} }
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
    const fetchAddresses = async () => {
      try {
        const resp = await addressesApi.list()
        if (Array.isArray(resp)) {
          const defaultAddr = resp.find(a => a.is_default)
          if (defaultAddr) {
            const addrStr = `${defaultAddr.recipient_name} ${defaultAddr.phone} ${defaultAddr.province}${defaultAddr.city}${defaultAddr.district}${defaultAddr.detail}`
            setAddress(addrStr)
            storage.setItem('user_address', addrStr)
          }
        } else if (resp.code === 20000 && resp.data?.list) {
          const defaultAddr = resp.data.list.find(a => a.is_default)
          if (defaultAddr) {
            const addrStr = `${defaultAddr.recipient_name} ${defaultAddr.phone} ${defaultAddr.province}${defaultAddr.city}${defaultAddr.district}${defaultAddr.detail}`
            storage.setItem('user_address', addrStr)
          }
        }
      } catch (err) {
        console.error('Failed to fetch addresses:', err)
      }
    }
    const token = getToken()
    if (token) {
      fetchAddresses()
    }
  }, [])

  useEffect(() => {
    const token = getToken()
    const pending = session.getItem('pending_order')
    if (token && pending && cart.items.length === 1) {
      session.removeItem('pending_order')
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
          lease_term: `${dayjs(returnDate).diff(dayjs(item.start_date || dayjs()), 'day') || 1}天`,
          return_date: returnDate,
          total_amount: amount.total,
        },
      })
    } else if (token && pending) {
      session.removeItem('pending_order')
    }
  }, [cart.items.length])

  const recalculateGroups = (items) => {
    const groups = {}
    for (const item of items) {
      const tid = item.tenant_id || 'unknown'
      const siteId = item.site_id || 'unknown'
      const siteName = item.site_name || '未知网点'
      if (!groups[tid]) {
        groups[tid] = { name: item.tenant_name || item.tenant_id || '', sites: {} }
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
    storage.setJSON('cart', { items: updated })
    setCart(newCart)
    setGrouped(recalculateGroups(updated))
    window.dispatchEvent(new Event('cartUpdated'))
  }

  const clearInvalidItems = () => {
    const updated = cart.items.filter(i => i.stock_status === 'available')
    const newCart = { items: updated }
    storage.setJSON('cart', { items: updated })
    setCart(newCart)
    setGrouped(recalculateGroups(updated))
  }

  const updateAddress = () => {
    setAddress(tempAddress)
    storage.setItem('user_address', tempAddress)
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

  const clearCart = () => {
    storage.setJSON('cart', { items: [] })
    window.dispatchEvent(new Event('cartUpdated'))
  }

  const handleOrder = async () => {
    const token = getToken()
    if (!token) {
      session.setItem('post_auth_redirect', '/cart')
      session.setItem('pending_order', 'true')
      redirectToLogin()
      return
    }
    if (token.includes('.')) {
      try {
        const payload = JSON.parse(atob(token.split('.')[1]))
        if (!payload.tid && !payload.oid) {
          session.setItem('post_auth_redirect', '/cart')
          session.setItem('pending_order', 'true')
          redirectToLogin()
          return
        }
      } catch {}
    }
    session.removeItem('pending_order')
    if (!address.trim()) {
      dialog.alert('请先填写收货地址')
      return
    }
    if (cart.items.length === 1) {
      const item = cart.items[0]
      const amount = calculateItemAmount(item)
      const returnDate = calculateDeadline(item)
      const startDate = item.start_date || dayjs().format('YYYY-MM-DD')

      try {
        const resp = await ordersApi.create({
          instrument_id: item.instrument_id,
          start_date: startDate,
          end_date: returnDate,
        })

        if (resp.code === 20000 || resp.code === 20100) {
          clearCart()
          navigate('/success', {
            state: {
              order_id: resp.data?.order_id || 'TL' + Date.now(),
              instrument_name: item.name,
              instrument_sn: item.sn,
              category_name: item.category_name,
              site_name: item.site_name,
              site_address: item.site_address,
              tenant_name: item.tenant_name,
              lease_term: `${dayjs(returnDate).diff(dayjs(startDate), 'day') || 1}天`,
              return_date: returnDate,
              total_amount: amount.total,
            },
          })
        } else {
          dialog.alert(resp.data?.message || '下单失败')
        }
      } catch (err) {
        console.error('Order failed:', err)
        dialog.alert('下单失败: ' + (err.message || '未知错误'))
      }
    } else {
      const items = cart.items.map(item => ({
        instrument_id: item.instrument_id,
        start_date: item.start_date || dayjs().format('YYYY-MM-DD'),
        end_date: item.end_date || dayjs().add(30, 'day').format('YYYY-MM-DD'),
      }))

      try {
        const resp = await ordersApi.batchCreate({ items })
        if (resp.code === 20000) {
          clearCart()
          navigate('/success', {
            state: { orders: resp.data.orders, total_amount: resp.data.total_amount },
          })
        } else {
          dialog.alert(resp.data?.message || '批量下单失败')
        }
      } catch (err) {
        dialog.alert('批量下单失败: ' + (err.message || '未知错误'))
      }
    }
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-24">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">购物车 ({cart.items.length})</Text>
        {cart.items.length > 0 && (
          <Button onClick={clearInvalidItems} className="ml-auto text-sm text-white/70">
            清理失效
          </Button>
        )}
      </View>

      <View className="p-4 space-y-4">
        {cart.items.length === 0 ? (
          <View className="bg-white rounded-xl p-8 text-center">
            <Package size={48} className="text-gray-300 mx-auto mb-3" />
            <Text className="text-gray-400">购物车为空</Text>
            <Button
              onClick={() => navigate('/')}
              className="mt-4 px-6 py-2 bg-brand-primary text-white rounded-lg"
            >
              去逛逛
            </Button>
          </View>
        ) : (
          <>
            {Object.entries(grouped).map(([tenantId, tenantData]) => (
              <View key={tenantId} className="bg-white rounded-xl p-4">
                <Text className="font-bold text-lg text-gray-800 mb-3">{tenantData.name}</Text>
                {Object.entries(tenantData.sites).map(([siteId, siteData]) => (
                  <View key={siteId} className="ml-2 border-l-2 border-orange-200 pl-3 mb-4">
                    <Text className="text-sm text-orange-600 font-medium mb-2">📍 {siteData.name}</Text>
                    {siteData.items.map((item) => {
                      const images = parseImages(item.images)
                      const amount = calculateItemAmount(item)
                      return (
                        <View key={item.instrument_id} className="flex gap-3 py-2 border-b border-gray-100">
                          <img
                            src={images[0] || PLACEHOLDER_IMAGE}
                            alt={item.name}
                            className="w-16 h-16 object-cover rounded-lg bg-gray-100"
                          />
                          <View className="flex-1">
                            <Text className="font-medium text-sm text-gray-800">{item.name}</Text>
                            <Text className="text-xs text-gray-500">{item.brand} {item.model}</Text>
                            <View className="flex items-center gap-2 mt-1">
                              {formatDisplayDate(item.start_date)} → {formatDisplayDate(item.end_date || calculateDeadline(item))}
                            </View>
                            <Text className="text-orange-600 font-bold text-sm mt-1">
                              ¥{amount.rent.toFixed(0)} + ¥{amount.deposit} 押{(parsePricing(item.pricing)[0]?.shipping_fee || 0) > 0 ? ` + ¥${parsePricing(item.pricing)[0].shipping_fee} 运` : ''}
                            </Text>
                          </View>
                          <Button onClick={() => removeItem(item.instrument_id)}>
                            <Trash2 size={18} className="text-gray-400" />
                          </Button>
                        </View>
                      )
                    })}
                  </View>
                ))}
              </View>
            ))}

            <View className="bg-white rounded-xl p-4">
              <View className="flex items-center justify-between mb-2">
                <View className="flex items-center gap-2">
                  <MapPin size={18} className="text-gray-400" />
                  <Text className="font-medium">收货地址</Text>
                </View>
                <Button onClick={() => setShowAddressModal(true)} className="text-brand-primary text-sm flex items-center gap-1">
                  <Edit2 size={14} /> 修改
                </Button>
              </View>
              <Text className="text-gray-600 text-sm">{address || '请添加收货地址'}</Text>
            </View>

            <View className="bg-white rounded-xl p-4">
              <Text className="font-bold mb-3">费用明细</Text>
              <View className="space-y-2 text-sm">
                <View className="flex justify-between">
                  <Text className="text-gray-600">租金</Text>
                  <Text className="font-medium">¥{totals.totalRent.toFixed(0)}</Text>
                </View>
                <View className="flex justify-between">
                  <Text className="text-gray-600">押金</Text>
                  <Text className="font-medium">¥{totals.totalDeposit}</Text>
                </View>
                <View className="flex justify-between">
                  <Text className="text-gray-600">物流费</Text>
                  <Text className="font-medium">¥{totals.totalShipping}</Text>
                </View>
                <View className="border-t pt-2 flex justify-between font-bold text-lg">
                  <Text>合计</Text>
                  <Text className="text-orange-600">¥{totals.grandTotal.toFixed(0)}</Text>
                </View>
              </View>
            </View>
          </>
        )}
      </View>

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <Button
          onClick={handleOrder}
          disabled={cart.items.length === 0}
          className="w-full bg-brand-primary text-white py-3 rounded-lg font-bold disabled:opacity-50"
        >
          支付 ¥{totals.grandTotal.toFixed(0)}
        </Button>
      </View>

      {showAddressModal && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center">
          <View className="bg-white rounded-xl p-6 mx-4 w-full max-w-sm">
            <Text className="font-bold text-lg mb-4">修改收货地址</Text>
            <textarea
              value={tempAddress}
              onChange={(e) => setTempAddress(e.target.value)}
              placeholder="请输入详细收货地址"
              className="w-full border rounded-lg p-3 text-sm mb-4"
              rows={3}
            />
            <View className="flex gap-3">
              <Button
                onClick={() => setShowAddressModal(false)}
                className="flex-1 py-2 border rounded-lg"
              >
                取消
              </Button>
              <Button onClick={updateAddress} className="flex-1 py-2 bg-brand-primary text-white rounded-lg">
                确认
              </Button>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
