import { useState, useEffect, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken, redirectToLogin, addressesApi, ordersApi } from '../services/api'
import { ArrowLeft, MapPin, Clock, Calendar, Plus, CheckCircle } from 'lucide-react'
import dayjs from 'dayjs'
import { dialog, env, session, storage, eventBus } from '../platform'
import regions from '../data/regions.json'

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

function getItemPricing(item) {
  const pricing = parsePricing(item.pricing)
  const dailyRent = pricing[0]?.daily_rent || item.base_daily_rate || 0
  const deposit = pricing[0]?.deposit || 0
  const rentQty = item.rent_qty || 1
  const rent = item.calculated_rent !== undefined ? item.calculated_rent : dailyRent * rentQty
  return { dailyRent, deposit, rent, shippingFee: pricing[0]?.shipping_fee || 0 }
}

function SingleCheckout({ id, navigate }) {
  const [instrument, setInstrument] = useState(null)
  const [pricingV2, setPricingV2] = useState(null)
  const [addresses, setAddresses] = useState([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [user, setUser] = useState(null)
  const [selectedAddressId, setSelectedAddressId] = useState('')
  const [useNewAddress, setUseNewAddress] = useState(false)
  const [newAddress, setNewAddress] = useState({ recipient_name: '', phone: '', province: '', city: '', district: '', detail: '', postal_code: '' })
  const [saveAddress, setSaveAddress] = useState(true)

  useEffect(() => {
    const token = getToken()
    if (!token) {
      session.setItem('post_auth_redirect', `/checkout/${id}`)
      redirectToLogin()
      return
    }

    const loadData = async () => {
      setLoading(true)
      try {
        const [instRes, addrRes] = await Promise.all([
          apiFetch(`${env.apiBaseUrl}/public/instruments/${id}`),
          addressesApi.list(),
        ])
        const instResult = await instRes.json()
        if (instResult.code === 20000) {
          setInstrument(instResult.data)
        }

        let addrList = []
        if (Array.isArray(addrRes)) {
          addrList = addrRes
        } else if (addrRes?.code === 20000) {
          addrList = addrRes.data?.list || []
        }
        const seen = new Set()
        addrList = addrList.filter(addr => {
          const key = JSON.stringify({ n: addr.recipient_name, p: addr.phone, d: addr.detail })
          if (seen.has(key)) return false
          seen.add(key)
          return true
        })
        setAddresses(addrList)
        const defaultAddr = addrList.find(a => a.is_default)
        if (defaultAddr) {
          setSelectedAddressId(defaultAddr.id)
        } else if (addrList.length === 0) {
          setUseNewAddress(true)
        }

        const pv2Res = await apiFetch(`${env.apiBaseUrl}/public/instruments/${id}/pricing-v2`)
        const pv2Result = await pv2Res.json()
        if (pv2Result.code === 20000) {
          setPricingV2(pv2Result.data)
        }
      } catch (err) {
        console.error('Failed to load checkout data:', err)
      }
      setLoading(false)
    }
    loadData()
  }, [id])

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${env.apiBaseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          setUser(result.data)
          setNewAddress(prev => ({
            ...prev,
            recipient_name: prev.recipient_name || result.data.name || '',
            phone: prev.phone || result.data.phone || '',
          }))
        }
      } catch {}
    }
    fetchUser()
  }, [])

  const computeTieredRent = (daysCount) => {
    if (!pricingV2?.tiers?.length) {
      return (pricingV2?.base_daily_rate || 0) * daysCount
    }
    let remaining = daysCount
    let total = 0
    let prevMax = 0
    for (const tier of pricingV2.tiers) {
      const tierDays = tier.days_max > 0 ? tier.days_max - prevMax : remaining
      const segDays = Math.min(tierDays, remaining)
      total += segDays * tier.daily_rate
      remaining -= segDays
      prevMax = tier.days_max
      if (remaining <= 0) break
    }
    return total
  }

  const totalRent = computeTieredRent(days)
  const deposit = pricingV2?.deposit || 0
  const shippingFee = pricingV2?.shipping_fee || 0
  const totalAmount = totalRent + deposit + shippingFee
  const startDate = new Date().toISOString().slice(0, 10)
  const returnDate = new Date(Date.now() + days * 86400000).toISOString().slice(0, 10)

  const handleDaysChange = (value) => {
    setDays(Math.max(1, Math.min(730, parseInt(value) || 30)))
  }

  const handleSubmit = async () => {
    if (!useNewAddress && !selectedAddressId) {
      dialog.alert('请选择收货地址')
      return
    }
    if (useNewAddress && !newAddress.recipient_name) {
      dialog.alert('请填写收货人')
      return
    }

    setSubmitting(true)
    try {
      let deliveryAddress = null

      if (useNewAddress) {
        if (saveAddress) {
          try {
            await addressesApi.create(newAddress)
          } catch (e) { console.error('save address failed', e) }
        }
        deliveryAddress = `${newAddress.recipient_name} ${newAddress.phone} ${newAddress.province}${newAddress.city}${newAddress.district} ${newAddress.detail}${newAddress.postal_code ? ' ' + newAddress.postal_code : ''}`
      } else {
        const addr = addresses.find(a => a.id === selectedAddressId)
        if (addr) {
          deliveryAddress = `${addr.recipient_name} ${addr.phone} ${addr.province}${addr.city}${addr.district} ${addr.detail}${addr.postal_code ? ' ' + addr.postal_code : ''}`
        }
      }

      const body = {
        instrument_id: id,
        start_date: startDate,
        end_date: returnDate,
      }
      if (deliveryAddress) body.delivery_address = deliveryAddress

      const resp = await ordersApi.create(body)
      if (resp.code === 20000 || resp.code === 20100) {
        navigate(`/order/${resp.data.order_id}`)
      } else {
        dialog.alert('下单失败: ' + (resp.message || '未知错误'))
      }
    } catch (err) {
      dialog.alert('下单失败: ' + (err?.message || '网络错误'))
    }
    setSubmitting(false)
  }

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  if (loading) return <View className="min-h-screen bg-gray-50 flex items-center justify-center"><Text className="text-gray-500">加载中...</Text></View>
  if (!instrument) return <View className="min-h-screen bg-gray-50 flex items-center justify-center"><Text className="text-gray-500">乐器不存在</Text></View>

  return (
    <View className="min-h-screen bg-gray-50 pb-28">
      <View className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">确认订单</Text>
      </View>

      <View className="p-4 space-y-4">
        <View className="bg-white rounded-xl p-4">
          <Text className="text-sm font-medium text-gray-900 mb-2">租赁乐器</Text>
          <View className="flex gap-3">
            <Image
              src={parseImages(instrument.images)?.[0] || ''}
              alt=""
              className="w-16 h-16 object-cover rounded-lg bg-gray-100"
              onError={(e) => { e.target.style.display = 'none' }}
            />
            <View>
              <Text className="font-medium text-sm">SN: {instrument.sn || id?.slice(0, 8)}</Text>
              <Text className="text-xs text-gray-500">{instrument.category_name}{instrument.level_name ? ` · ${instrument.level_name}` : ''}</Text>
            </View>
          </View>
        </View>

        <View className="bg-white rounded-xl p-4">
          <Text className="text-sm font-medium text-gray-900 mb-3 flex items-center gap-2">
            <Calendar size={16} className="text-brand-primary" />
            租期选择
          </Text>
          <View className="flex items-center gap-3">
            <Button
              onClick={() => handleDaysChange(days - 1)}
              className="w-10 h-10 border rounded-lg text-lg font-medium text-gray-500"
            >−</Button>
            <input
              type="number"
              min={1}
              max={730}
              value={days}
              onChange={e => handleDaysChange(e.target.value)}
              className="flex-1 text-center text-xl font-bold border rounded-lg py-2"
            />
            <Button
              onClick={() => handleDaysChange(days + 1)}
              className="w-10 h-10 border rounded-lg text-lg font-medium text-gray-500"
            >+</Button>
            <Text className="text-sm text-gray-500">天</Text>
          </View>
          <View className="mt-2 text-xs text-gray-400 flex items-center gap-1">
            <Clock size={12} />
            预计归还: {returnDate}
            {pricingV2?.tiers?.length > 0 && <Text className="ml-1">· 阶梯计价</Text>}
          </View>
        </View>

        <View className="bg-white rounded-xl p-4">
          <Text className="text-sm font-medium text-gray-900 mb-3">费用明细</Text>
          <View className="space-y-2 text-sm">
            <View className="flex justify-between">
              <Text className="text-gray-500">租金 ({days}天)</Text>
              <Text className="font-medium">¥{totalRent.toFixed(0)}</Text>
            </View>
            {pricingV2?.tiers?.length > 0 && (
              <View className="text-xs text-gray-400 pl-2 pb-1 border-b border-dashed">
                {pricingV2.tiers.map((t, i) => {
                  const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                  const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
                  return <Text key={i} className="mr-3">{range}: ¥{t.daily_rate}/天</Text>
                })}
              </View>
            )}
            <View className="flex justify-between">
              <Text className="text-gray-500">押金</Text>
              <Text className="font-medium">¥{deposit}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">物流费</Text>
              <Text className="font-medium">¥{shippingFee}</Text>
            </View>
            <View className="border-t pt-2 flex justify-between font-bold text-base">
              <Text className="text-gray-900">合计</Text>
              <Text className="text-brand-primary">¥{totalAmount.toFixed(0)}</Text>
            </View>
          </View>
        </View>

        <View className="bg-white rounded-xl p-4">
          <Text className="text-sm font-medium text-gray-900 mb-3 flex items-center gap-2">
            <MapPin size={16} className="text-brand-primary" />
            收货地址
          </Text>

          {addresses.length > 0 && !useNewAddress && (
            <View className="space-y-2 mb-3">
              {addresses.map(addr => (
                <label
                  key={addr.id}
                  className={`flex items-start gap-3 p-3 border rounded-lg cursor-pointer ${
                    selectedAddressId === addr.id ? 'border-brand-primary bg-blue-50' : 'border-gray-200'
                  }`}
                >
                  <input
                    type="radio"
                    name="address"
                    checked={selectedAddressId === addr.id}
                    onChange={() => { setSelectedAddressId(addr.id); setUseNewAddress(false) }}
                    className="mt-1"
                  />
                  <View className="flex-1 text-sm">
                    <Text className="font-medium">{addr.recipient_name} · {addr.phone}</Text>
                    <Text className="text-xs text-gray-400">{addr.province}{addr.city}{addr.district} {addr.detail}</Text>
                    {addr.is_default && <Text className="text-xs text-brand-primary">默认</Text>}
                  </View>
                </label>
              ))}
            </View>
          )}

          {(addresses.length === 0 || useNewAddress) && (
            <View className="space-y-3">
              <View className="grid grid-cols-2 gap-2">
                <View>
                  <label className={labelClass}>收货人</label>
                  <input className={inputClass} value={newAddress.recipient_name} onChange={e => setNewAddress(prev => ({ ...prev, recipient_name: e.target.value }))} placeholder="姓名" />
                </View>
                <View>
                  <label className={labelClass}>电话</label>
                  <input className={inputClass} value={newAddress.phone} onChange={e => setNewAddress(prev => ({ ...prev, phone: e.target.value }))} placeholder="手机号" />
                </View>
              </View>
              <View className="grid grid-cols-3 gap-2">
                <select className={inputClass} value={newAddress.province} onChange={e => setNewAddress(prev => ({ ...prev, province: e.target.value, city: '', district: '' }))}>
                  <option value="">省</option>
                  {regions.map((r, i) => <option key={i} value={r.name}>{r.name}</option>)}
                </select>
                <select className={inputClass} value={newAddress.city} onChange={e => setNewAddress(prev => ({ ...prev, city: e.target.value, district: '' }))}>
                  <option value="">市</option>
                  {(() => {
                    const prov = regions.find(r => r.name === newAddress.province)
                    return prov ? prov.children.map((c, i) => <option key={i} value={c.name}>{c.name}</option>) : null
                  })()}
                </select>
                <select className={inputClass} value={newAddress.district} onChange={e => setNewAddress(prev => ({ ...prev, district: e.target.value }))}>
                  <option value="">区</option>
                  {(() => {
                    const prov = regions.find(r => r.name === newAddress.province)
                    if (!prov) return null
                    const city = prov.children.find(c => c.name === newAddress.city)
                    return city ? city.children.map((d, i) => <option key={i} value={d.name}>{d.name}</option>) : null
                  })()}
                </select>
              </View>
              <View>
                <input className={inputClass} value={newAddress.detail} onChange={e => setNewAddress(prev => ({ ...prev, detail: e.target.value }))} placeholder="详细地址" />
              </View>
              <View>
                <input className={inputClass} value={newAddress.postal_code} onChange={e => setNewAddress(prev => ({ ...prev, postal_code: e.target.value }))} placeholder="邮编" />
              </View>
              <label className="flex items-center gap-2 text-sm text-gray-500 cursor-pointer">
                <input type="checkbox" checked={saveAddress} onChange={e => setSaveAddress(e.target.checked)} />
                设置为我的收货地址
              </label>
            </View>
          )}

          {addresses.length > 0 && !useNewAddress && (
            <Button
              onClick={() => setUseNewAddress(true)}
              className="mt-3 text-sm text-brand-primary flex items-center gap-1"
            >
              <Plus size={14} /> 使用新地址
            </Button>
          )}
          {useNewAddress && addresses.length > 0 && (
            <Button
              onClick={() => setUseNewAddress(false)}
              className="mt-3 text-sm text-gray-400"
            >
              选择已有地址
            </Button>
          )}
        </View>

        <View className="bg-blue-50 border border-blue-200 rounded-xl p-4 text-sm text-blue-700">
          <Text className="font-medium mb-1">租赁须知</Text>
          <ul className="text-xs space-y-1 text-blue-600">
            <li>· 提交即生成订单，需在10分钟内完成支付</li>
            <li>· 超时未支付订单将自动取消</li>
            <li>· 发货前可取消订单免手续费</li>
            <li>· 押金在归还验收后原路退还</li>
          </ul>
        </View>
      </View>

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <View className="flex items-center justify-between mb-2">
          <Text className="text-sm text-gray-500">应付总额</Text>
          <Text className="text-xl font-bold text-brand-primary">¥{totalAmount.toFixed(0)}</Text>
        </View>
        <Button
          onClick={handleSubmit}
          disabled={submitting}
          className="w-full py-3 bg-brand-primary text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {submitting ? '提交中...' : '提交订单'}
        </Button>
      </View>
    </View>
  )
}

function BatchCheckout({ navigate }) {
  const [submitting, setSubmitting] = useState(false)
  const [cartItems, setCartItems] = useState([])
  const [addresses, setAddresses] = useState([])
  const [selectedAddressId, setSelectedAddressId] = useState('')
  const [useNewAddress, setUseNewAddress] = useState(false)
  const [newAddress, setNewAddress] = useState({ recipient_name: '', phone: '', province: '', city: '', district: '', detail: '', postal_code: '' })
  const [saveAddress, setSaveAddress] = useState(true)
  const [previewImages, setPreviewImages] = useState([])
  const [previewIndex, setPreviewIndex] = useState(-1)
  const [user, setUser] = useState(null)

  useEffect(() => {
    const token = getToken()
    if (!token) {
      session.setItem('post_auth_redirect', '/checkout')
      redirectToLogin()
      return
    }
    const loadData = async () => {
      const data = storage.getJSON('cart', { items: [] }) || { items: [] }
      setCartItems(data.items)
      try {
        const addrRes = await addressesApi.list()
        let addrList = []
        if (Array.isArray(addrRes)) {
          addrList = addrRes
        } else if (addrRes?.code === 20000) {
          addrList = addrRes.data?.list || []
        }
        setAddresses(addrList)
        const defaultAddr = addrList.find(a => a.is_default)
        if (defaultAddr) setSelectedAddressId(defaultAddr.id)
        else if (addrList.length === 0) setUseNewAddress(true)
      } catch (e) {
        setUseNewAddress(true)
      }
    }
    loadData()
  }, [])

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${env.apiBaseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          setUser(result.data)
          setNewAddress(prev => ({
            ...prev,
            recipient_name: prev.recipient_name || result.data.name || '',
            phone: prev.phone || result.data.phone || '',
          }))
        }
      } catch {}
    }
    fetchUser()
  }, [])

  const groups = useMemo(() => {
    const map = {}
    cartItems.forEach(item => {
      const key = `${item.tenant_id || 'unknown'}-${item.site_id || 'unknown'}`
      if (!map[key]) {
        const itemShipping = parsePricing(item.pricing)[0]?.shipping_fee || 0
        map[key] = {
          tenant_id: item.tenant_id,
          tenant_name: item.tenant_name || '',
          site_name: item.site_name || '',
          site_address: item.site_address || '',
          site_phone: item.site_phone || '',
          shippingFee: itemShipping,
          items: [],
        }
      }
      map[key].items.push(item)
    })
    return Object.values(map)
  }, [cartItems])

  const grandTotal = useMemo(() => {
    let total = 0
    for (const group of groups) {
      let groupRent = 0
      let groupDeposit = 0
      for (const item of group.items) {
        const p = getItemPricing(item)
        groupRent += p.rent
        groupDeposit += p.deposit
      }
      total += groupRent + groupDeposit + (group.shippingFee || 0)
    }
    return total
  }, [groups])

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  const handleSubmit = async () => {
    if (cartItems.length === 0) return
    if (!useNewAddress && !selectedAddressId) {
      dialog.alert('请选择收货地址')
      return
    }
    if (useNewAddress && !newAddress.recipient_name) {
      dialog.alert('请填写收货人')
      return
    }
    setSubmitting(true)
    try {
      let deliveryAddress = null

      if (useNewAddress) {
        if (saveAddress) {
          try {
            await addressesApi.create(newAddress)
          } catch (e) { console.error('save address failed', e) }
        }
        deliveryAddress = `${newAddress.recipient_name} ${newAddress.phone} ${newAddress.province}${newAddress.city}${newAddress.district} ${newAddress.detail}${newAddress.postal_code ? ' ' + newAddress.postal_code : ''}`
      } else {
        const addr = addresses.find(a => a.id === selectedAddressId)
        if (addr) {
          deliveryAddress = `${addr.recipient_name} ${addr.phone} ${addr.province}${addr.city}${addr.district} ${addr.detail}${addr.postal_code ? ' ' + addr.postal_code : ''}`
        }
      }

      const items = cartItems.map(item => ({
        instrument_id: item.instrument_id,
        start_date: dayjs().format('YYYY-MM-DD'),
        end_date: dayjs().add(item.rent_qty || 30, 'day').format('YYYY-MM-DD'),
      }))
      const body = { items }
        if (deliveryAddress) body.delivery_address = deliveryAddress
        const orderResp = await ordersApi.batchCreate(body)
        if (orderResp.code === 20000) {
          const orderId = orderResp.data?.order_id
          if (orderId) {
            await apiFetch(`${baseUrl}/orders/${orderId}/pay`, { method: 'POST' })
          }
          storage.removeItem('cart')
          eventBus.emit('cartUpdated')
          navigate('/success')
        } else {
          dialog.alert('下单失败: ' + (orderResp.message || '未知错误'))
        }
    } catch (err) {
      dialog.alert('下单失败: ' + (err?.message || '网络错误'))
    }
    setSubmitting(false)
  }

  if (cartItems.length === 0) {
    return (
      <View className="min-h-screen bg-zinc-50 flex items-center justify-center">
        <Text className="text-zinc-400">购物车为空</Text>
      </View>
    )
  }

  return (
    <View className="container h-screen w-screen bg-zinc-50 overflow-hidden flex flex-col relative antialiased">
      <View className="w-full pt-3 pb-2 px-4 flex justify-between items-center bg-white border-b border-zinc-100 flex-shrink-0">
        <Text className="text-xl font-bold text-black" onClick={() => navigate(-1)}>❮</Text>
        <Text className="text-lg font-black text-black">确认支付</Text>
        <View className="w-6"></View>
      </View>

      <ScrollView className="w-full flex-1 pb-28" scrollY showScrollbar={false}>
        <View className="p-4 m-4 bg-white rounded-2xl shadow-sm border border-zinc-100 space-y-6 flex flex-col items-center">
          <View className="text-center space-y-1">
            <Text className="text-xs text-zinc-400 font-bold tracking-widest block uppercase">TOTAL PAYABLE</Text>
            <Text className="text-[#C21838] text-4xl font-black tracking-tight block">
              ¥{grandTotal.toFixed(0)}
            </Text>
          </View>

          <View className="w-full border-t border-dashed border-zinc-200 pt-4 space-y-3">
            {groups.map((group) => {
              let groupRent = 0
              let groupDeposit = 0
              group.items.forEach(item => {
                const p = getItemPricing(item)
                groupRent += p.rent
                groupDeposit += p.deposit
              })
              const groupSubtotal = groupRent + groupDeposit + (group.shippingFee || 0)
              return (
                <View key={group.tenant_id || 'unknown'} className="bg-zinc-50/40 rounded-xl p-3">
                  <View className="flex items-center justify-between mb-2">
                    <View className="flex items-center space-x-1">
                      <Text>🏢</Text>
                      <Text className="text-sm font-bold text-zinc-700">{group.tenant_name}</Text>
                      <Text className="text-zinc-300 mx-0.5">|</Text>
                      <Text>📍</Text>
                      <Text className="text-sm text-zinc-600">{group.site_name}</Text>
                    </View>
                    <Text className="text-xs text-zinc-400">{group.items.length}件</Text>
                  </View>
                  {group.items.map((item) => {
                    const p = getItemPricing(item)
                    const images = parseImages(item.images)
                    const imgSrc = images[0] || item.cover || ''
                    return (
                      <View key={item.instrument_id || item.id} className="flex items-center py-1.5 border-b border-zinc-100 last:border-b-0">
                        {imgSrc && (
                          <Image src={imgSrc} className="w-8 h-8 rounded object-cover bg-zinc-100 mr-2 flex-shrink-0" />
                        )}
                        <View className="flex-1 min-w-0">
                          <Text className="text-xs font-bold text-zinc-700 truncate">{item.sn || item.name}</Text>
                          <Text className="text-[10px] text-zinc-400">{item.category_name || ''}</Text>
                        </View>
                        <Text className="text-[10px] text-zinc-500 flex-shrink-0 ml-2">
                          {item.rent_qty || 1}天 · ¥{p.rent}
                        </Text>
                      </View>
                    )
                  })}
                  <View className="flex justify-between items-center mt-1 pt-1 border-t border-zinc-200/60">
                    <Text className="text-[10px] text-zinc-400">
                      押金 ¥{groupDeposit} + 运费 ¥{group.shippingFee || 0}
                    </Text>
                    <Text className="text-sm font-bold text-zinc-800">小计 ¥{groupSubtotal}</Text>
                  </View>
                </View>
              )
            })}
          </View>

          <View className="w-full bg-zinc-50 p-3 rounded-xl text-[11px] text-zinc-400 leading-normal">
            🔒 暖心提示：资产固定押金将在乐器归还、网点网管质检合格后，按原支付渠道原路退回至您的微信零钱。
          </View>

          <View className="w-full border-t border-dashed border-zinc-200 pt-4">
            <Text className="text-xs font-bold text-zinc-500 mb-3">📍 收货地址</Text>

            {addresses.length > 0 && !useNewAddress && (
              <View className="space-y-2 mb-3">
                {addresses.map(addr => (
                  <label
                    key={addr.id}
                    className={`flex items-start gap-3 p-3 border rounded-lg cursor-pointer ${
                      selectedAddressId === addr.id ? 'border-brand-primary bg-blue-50' : 'border-zinc-200'
                    }`}
                  >
                    <input
                      type="radio"
                      name="batch-address"
                      checked={selectedAddressId === addr.id}
                      onChange={() => { setSelectedAddressId(addr.id); setUseNewAddress(false) }}
                      className="mt-1"
                    />
                    <View className="flex-1 text-xs">
                      <Text className="font-medium text-zinc-800">{addr.recipient_name} · {addr.phone}</Text>
                      <Text className="text-zinc-400">{addr.province}{addr.city}{addr.district} {addr.detail}</Text>
                      {addr.is_default && <Text className="text-xs text-brand-primary">默认</Text>}
                    </View>
                  </label>
                ))}
              </View>
            )}

            {(addresses.length === 0 || useNewAddress) && (
              <View className="space-y-3">
                <View className="grid grid-cols-2 gap-2">
                  <View>
                    <label className={labelClass}>收货人</label>
                    <input className={inputClass} value={newAddress.recipient_name} onChange={e => setNewAddress(prev => ({ ...prev, recipient_name: e.target.value }))} placeholder="姓名" />
                  </View>
                  <View>
                    <label className={labelClass}>电话</label>
                    <input className={inputClass} value={newAddress.phone} onChange={e => setNewAddress(prev => ({ ...prev, phone: e.target.value }))} placeholder="手机号" />
                  </View>
                </View>
                <View className="grid grid-cols-3 gap-2">
                  <input className={inputClass} value={newAddress.province} onChange={e => setNewAddress(prev => ({ ...prev, province: e.target.value }))} placeholder="省" />
                  <input className={inputClass} value={newAddress.city} onChange={e => setNewAddress(prev => ({ ...prev, city: e.target.value }))} placeholder="市" />
                  <input className={inputClass} value={newAddress.district} onChange={e => setNewAddress(prev => ({ ...prev, district: e.target.value }))} placeholder="区" />
                </View>
                <input className={inputClass} value={newAddress.detail} onChange={e => setNewAddress(prev => ({ ...prev, detail: e.target.value }))} placeholder="详细地址" />
                <input className={inputClass} value={newAddress.postal_code} onChange={e => setNewAddress(prev => ({ ...prev, postal_code: e.target.value }))} placeholder="邮编" />
                <label className="flex items-center gap-2 text-xs text-zinc-500 cursor-pointer">
                  <input type="checkbox" checked={saveAddress} onChange={e => setSaveAddress(e.target.checked)} />
                  设置为我的收货地址
                </label>
              </View>
            )}

            {addresses.length > 0 && !useNewAddress && (
              <Text className="mt-3 text-xs text-brand-primary" onClick={() => setUseNewAddress(true)}>+ 使用新地址</Text>
            )}
            {useNewAddress && addresses.length > 0 && (
              <Text className="mt-3 text-xs text-zinc-400" onClick={() => setUseNewAddress(false)}>选择已有地址</Text>
            )}
          </View>
        </View>

        <View className="mx-4 p-4 bg-white rounded-2xl shadow-sm flex items-center justify-between border border-zinc-100">
          <View className="flex items-center space-x-3">
            <Text className="text-2xl">🟢</Text>
            <View>
              <Text className="block text-base font-black text-black">微信支付</Text>
              <Text className="block text-[11px] text-zinc-400">亿万用户的安全选择</Text>
            </View>
          </View>
          <Text className="text-sm font-black text-orange-500">✓</Text>
        </View>
      </ScrollView>

      <View className="absolute bottom-0 left-0 right-0 bg-white p-4 pb-6 border-t border-zinc-100 z-50 flex flex-col items-center">
        <Button
          className="w-full m-0 bg-[#B98E5F] active:bg-[#A87D50] text-white font-extrabold text-base h-12 rounded-full shadow-md flex items-center justify-center tracking-wider"
          onClick={handleSubmit}
          disabled={submitting}
        >
          {submitting ? '提交中...' : `确认支付 ¥${grandTotal.toFixed(0)}`}
        </Button>
      </View>
    </View>
  )
}

export default function Checkout() {
  const { id } = useParams()
  const navigate = useNavigate()
  if (id) {
    return <SingleCheckout id={id} navigate={navigate} />
  }
  return <BatchCheckout navigate={navigate} />
}
