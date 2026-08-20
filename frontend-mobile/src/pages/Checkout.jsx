import { useState, useEffect, useMemo } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken, redirectToLogin, addressesApi, ordersApi, getCartKey , resolveErrorMessage } from '../services/api'
import { ArrowLeft, MapPin, Clock, Calendar, Plus, CheckCircle } from 'lucide-react'
import dayjs from 'dayjs'
import { dialog, env, session, storage, eventBus, getInputValue } from '../platform'
import { calculateDays, calculateEndDate } from '../utils/daycalc'
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

function computeTieredRent(pricingV2, days, baseDailyRate) {
  if (!pricingV2?.tiers?.length) {
    return (pricingV2?.base_daily_rate || baseDailyRate || 0) * days
  }
  let remaining = days
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

function getItemPricing(item) {
  const days = item.rent_qty || 30
  const dailyRent = item.daily_rent || 0
  const rent = item.pricing_v2?.tiers?.length
    ? computeTieredRent(item.pricing_v2, days, item.pricing_v2.base_daily_rate || dailyRent)
    : dailyRent * days
  const deposit = item.deposit || 0
  const shippingFee = item.shipping_fee || 0
  return { dailyRent, deposit, rent, shippingFee }
}

function SingleCheckout({ id, navigate }) {
  const [instrument, setInstrument] = useState(null)
  const [pricingV2, setPricingV2] = useState(null)
  const [addresses, setAddresses] = useState([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [user, setUser] = useState(null)
  const [selectedAddressId, setSelectedAddressId] = useState('')
  const [discountCode, setDiscountCode] = useState('')
  const [discountInfo, setDiscountInfo] = useState(null)
  const [discountChecking, setDiscountChecking] = useState(false)
  const [useNewAddress, setUseNewAddress] = useState(false)
  const [newAddress, setNewAddress] = useState({ recipient_name: '', phone: '', province: '', city: '', district: '', detail: '', postal_code: '' })
  const [saveAddress, setSaveAddress] = useState(true)
  const [days, setDays] = useState(30)
  const [rentalCalc, setRentalCalc] = useState(null)
  const [rentalCalcLoading, setRentalCalcLoading] = useState(false)
  const [depositWaived, setDepositWaived] = useState(false)
  const [guarantors, setGuarantors] = useState([])
  const [selectedGuarantorIds, setSelectedGuarantorIds] = useState([])
  const [showAddGuarantor, setShowAddGuarantor] = useState(false)
  const [newGuarantor, setNewGuarantor] = useState({ name: '', phone: '', company: '', title: '', address: '' })
  const [savingGuarantor, setSavingGuarantor] = useState(false)

  useEffect(() => {
    const token = getToken()
    if (!token) {
      session.setItem('post_auth_redirect', `/checkout/${id}`)
      redirectToLogin('checkout')
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

        try {
          const guarResp = await apiFetch(`${env.apiBaseUrl}/user/guarantors`)
          const guarResult = await guarResp.json()
          if (guarResult.code === 20000) {
            setGuarantors(Array.isArray(guarResult.data) ? guarResult.data : guarResult.data?.list || [])
          }
        } catch (e) { console.error('Failed to load guarantors:', e) }
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
        const userRes = await apiFetch(`${env.apiBaseUrl}/users/me`)
        const userResult = await userRes.json()
        if (userResult.code === 20000) {
          setUser(userResult.data)
          setNewAddress(prev => ({
            ...prev,
            recipient_name: prev.recipient_name || userResult.data.name || '',
            phone: prev.phone || userResult.data.phone || '',
          }))
        }
      } catch {}
    }
    fetchUser()
  }, [])

  const totalRent = pricingV2 ? computeTieredRent(pricingV2, days, pricingV2.base_daily_rate || 0) : 0
  const deposit = instrument?.deposit || pricingV2?.deposit || parsePricing(instrument?.pricing)[0]?.deposit || 0
  const effectiveDeposit = depositWaived ? 0 : deposit
  // Shipping fee is not determined at order time (#1570): staff fills the
  // actual fee at dispatch, not charged at checkout.
  const totalAmount = totalRent + effectiveDeposit
  const startDate = new Date().toISOString().slice(0, 10)
  const returnDate = calculateEndDate(new Date(), days).toISOString().slice(0, 10)

  const handleDaysChange = (value) => {
    setDays(Math.max(1, Math.min(730, parseInt(value) || 1)))
  }

  const handleApplyDiscount = async () => {
    const code = discountCode.trim()
    if (!code) return
    setDiscountChecking(true)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/discount-codes/apply`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code }),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data) {
        setDiscountInfo(result.data)
        dialog.alert(`优惠码有效：${result.data.policy_name}（租金${((1 - result.data.rent_discount) * 100).toFixed(0)}%折扣）`)
      } else {
        setDiscountInfo(null)
        dialog.alert(resolveErrorMessage(result, '优惠码无效'))
      }
    } catch (e) {
      setDiscountInfo(null)
      dialog.alert('优惠码校验失败')
    }
    setDiscountChecking(false)
  }

  const handleSaveGuarantor = async () => {
    if (!newGuarantor.name.trim() || !newGuarantor.phone.trim()) {
      dialog.alert('请填写担保人姓名和联系电话')
      return
    }
    if (!newGuarantor.company.trim() || !newGuarantor.title.trim()) {
      dialog.alert('请填写工作单位和职务')
      return
    }
    setSavingGuarantor(true)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/user/guarantors`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newGuarantor),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        const g = result.data
        setGuarantors(prev => [...prev, g])
        setSelectedGuarantorIds(prev => prev.includes(g.id) ? prev : [...prev, g.id])
        setNewGuarantor({ name: '', phone: '', company: '', title: '', address: '' })
        setShowAddGuarantor(false)
        dialog.alert('担保人已保存')
      } else {
        dialog.alert('保存失败: ' + (resolveErrorMessage(result, '未知错误')))
      }
    } catch (err) {
      dialog.alert('保存失败: ' + (err?.message || '网络错误'))
    }
    setSavingGuarantor(false)
  }

  const toggleGuarantor = (id) => {
    setSelectedGuarantorIds(prev => prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id])
  }

  const handleSubmit = async () => {
    if (depositWaived && selectedGuarantorIds.length < 2) {
      dialog.alert('免押金订单需提供至少 2 位担保人，请选择或新增担保人')
      return
    }
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
        rent_days: days,
      }
      if (deliveryAddress) body.delivery_address = deliveryAddress
      if (discountCode.trim()) body.discount_code = discountCode.trim()
      if (depositWaived) {
        body.deposit_waived = true
        body.guarantor_ids = selectedGuarantorIds
      }

      const resp = await ordersApi.create(body)
      if (resp.code === 20000 || resp.code === 20100) {
        const orderId = resp.data?.order_id
        if (orderId) {
          // Keep /checkout in history so payment back button returns here (#1629)
          navigate(`/payment?type=rent&id=${orderId}`)
        } else {
          navigate('/success', { replace: true })
        }
      } else {
        dialog.alert('下单失败: ' + (resolveErrorMessage(resp, '未知错误')))
      }
    } catch (err) {
      dialog.alert('下单失败: ' + (err?.message || '网络错误'))
    }
    setSubmitting(false)
  }

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  if (loading) return <View className="min-h-screen bg-[#FDFBF7] flex items-center justify-center"><Text className="text-zinc-400">加载中...</Text></View>
  if (!instrument) return <View className="min-h-screen bg-[#FDFBF7] flex items-center justify-center"><Text className="text-zinc-400">乐器不存在</Text></View>

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-28">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <ArrowLeft size={20} className="text-black cursor-pointer" onClick={() => navigate(-1)} />
        <Text className="text-lg font-black text-black">确认订单</Text>
      </View>

      <View className="p-4">
        <View className="mb-3">
          <Text className="font-black text-black">租赁乐器</Text>
          <View className="flex gap-3 mt-2">
            <Image src={instrument?.cover_image || instrument?.images?.[0]} mode="aspectFill" className="w-20 h-20 object-cover rounded-lg bg-[#FDF4E7]" />
            <View className="flex-1 justify-center">
              <Text className="font-black text-sm text-black">{instrument?.name || instrument?.sn}</Text>
              <Text className="text-xs text-zinc-500">{instrument?.category_name}</Text>
              {instrument?.site_name && <Text className="text-xs text-zinc-400">网点: {instrument.site_name}</Text>}
            </View>
          </View>
        </View>

        <View className="bg-white rounded-2xl shadow-sm p-4 mb-3">
          <Text className="font-black text-black mb-3 flex items-center gap-2">
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
              min={0}
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

        <View className="bg-white rounded-2xl shadow-sm p-4 mb-3">
          <Text className="font-black text-black mb-3">费用明细</Text>
          <View className="text-sm">
            {/* 费用主体 */}
            <View>
              <View className="flex justify-between items-center mb-2">
                <Text className="text-zinc-400">租金 ({days}天)</Text>
                <Text className="font-medium flex-shrink-0 ml-auto whitespace-nowrap">¥{totalRent.toFixed(2)}</Text>
              </View>
              <View className="flex justify-between items-center">
                <Text className="text-zinc-400">押金</Text>
                {depositWaived ? (
                  <Text className="text-green-600 font-medium flex-shrink-0 ml-auto whitespace-nowrap">¥0（免押金）</Text>
                ) : (
                  <Text className="font-medium flex-shrink-0 ml-auto whitespace-nowrap">¥{(deposit || 0).toFixed(2)}{deposit === 0 ? <Text className="text-[10px] text-zinc-400 ml-1">(日租金×倍率)</Text> : null}</Text>
                )}
              </View>
            </View>

            {/* 阶梯定价说明 */}
            {pricingV2?.tiers?.length > 0 && (
              <View className="bg-zinc-50 rounded-lg px-3 py-2.5 mt-3">
                <View className="flex items-center gap-1 mb-1">
                  <Text className="text-xs text-zinc-500 font-medium">阶梯定价</Text>
                </View>
                <View>
                  {pricingV2.tiers.map((t, i) => {
                    const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                    const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
                    return (
                      <View key={i} className="flex justify-between text-xs mb-1">
                        <Text className="text-zinc-400">{range}</Text>
                        <Text className="text-zinc-600 font-medium">¥{Number(t.daily_rate).toFixed(2)}/天</Text>
                      </View>
                    )
                  })}
                </View>
              </View>
            )}

            {/* 优惠码 */}
            <View className="flex items-center gap-2 mt-3">
              <Input
                value={discountCode}
                onInput={e => setDiscountCode(getInputValue(e))}
                placeholder="优惠码（选填）"
                className="flex-1 border rounded-lg px-3 py-2 text-sm"
                style={{ flex: 1, border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14 }}
              />
              <Button
                size="mini"
                disabled={discountChecking}
                onClick={handleApplyDiscount}
                style={{ backgroundColor: '#915F38', color: '#fff', borderRadius: 8, fontSize: 13 }}
              >
                <Text style={{ color: '#fff', fontSize: 13 }}>{discountChecking ? '校验中...' : '使用'}</Text>
              </Button>
            </View>
            {discountInfo && (
              <View className="flex justify-between text-green-600 text-xs mt-2">
                <Text className="font-medium">优惠({discountInfo.policy_name})</Text>
                <Text className="font-bold">-{(1 - discountInfo.rent_discount) * 100}%</Text>
              </View>
            )}

            {/* 合计 */}
            <View className="border-t pt-3 mt-3 flex justify-between items-center">
              <Text className="font-black text-zinc-900 text-base">合计</Text>
              <Text className="text-brand-primary font-black text-lg flex-shrink-0 ml-auto whitespace-nowrap">¥{totalAmount.toFixed(2)}</Text>
            </View>
            <Text className="text-[10px] text-zinc-400 text-right mt-1">租金 ¥{totalRent.toFixed(2)} + 押金 ¥{(deposit || 0).toFixed(2)}</Text>
            {depositWaived ? (
              <Text className="block text-xs text-red-500 font-medium mt-2">乐器往返物流费需由您承担，寄出时将选用顺丰到付，请注意查收哦谢谢。</Text>
            ) : (
              <Text className="block text-xs text-red-500 font-medium mt-2">乐器往返物流费需由您承担，届时将从押金中扣除，望知悉谢谢。</Text>
            )}
          </View>
        </View>

        {/* 免押金开关 */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mb-3">
          <View className="flex items-center justify-between">
            <View>
              <Text className="font-black text-black">免押金租赁</Text>
              <Text className="text-xs text-zinc-400 mt-0.5">需提供两位担保人的联系方式</Text>
            </View>
            <input
              type="checkbox"
              checked={depositWaived}
              onChange={e => setDepositWaived(e.target.checked)}
              className="w-5 h-5 accent-[#B98E5F]"
            />
          </View>
          {depositWaived && (
            <View className="bg-amber-50 border border-amber-100 rounded-lg px-3 py-2 mt-3">
              <Text className="text-xs text-amber-700 leading-relaxed">
                应提供两位担保人的联系方式。我们的员工将会与他们联系确认，若担保人不符合要求，订单将被取消并退款。
              </Text>
            </View>
          )}
        </View>

        {depositWaived && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mb-3">
            <Text className="font-black text-black mb-1">担保人信息</Text>
            <Text className="text-[10px] text-zinc-400 mb-3">已选 {selectedGuarantorIds.length}/2</Text>
            {guarantors.length > 0 && (
              <View className="mb-3">
                {guarantors.map((g, gi) => (
                  <label
                    key={g.id}
                    className={`flex items-start gap-3 p-3 border rounded-lg cursor-pointer ${gi > 0 ? 'mt-2' : ''} ${
                      selectedGuarantorIds.includes(g.id) ? 'border-brand-primary bg-blue-50' : 'border-gray-200'
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={selectedGuarantorIds.includes(g.id)}
                      onChange={() => toggleGuarantor(g.id)}
                      className="mt-1"
                    />
                    <View className="flex-1 text-sm">
                      <Text className="font-medium">{g.name} · {g.phone}</Text>
                      {(g.company || g.title) && <Text className="block text-xs text-gray-400">{[g.company, g.title].filter(Boolean).join(' / ')}</Text>}
                      {g.address && <Text className="block text-xs text-gray-400">{g.address}</Text>}
                    </View>
                  </label>
                ))}
              </View>
            )}
            {showAddGuarantor ? (
              <View className="bg-gray-50 rounded-lg p-3">
                <View className="grid grid-cols-2 gap-2">
                  <input className={inputClass} value={newGuarantor.name} onChange={e => setNewGuarantor(prev => ({ ...prev, name: e.target.value }))} placeholder="姓名" />
                  <input className={inputClass} value={newGuarantor.phone} onChange={e => setNewGuarantor(prev => ({ ...prev, phone: e.target.value }))} placeholder="联系电话" />
                </View>
                <View className="mt-2">
                  <input className={inputClass} value={newGuarantor.company} onChange={e => setNewGuarantor(prev => ({ ...prev, company: e.target.value }))} placeholder="工作单位" />
                </View>
                <View className="mt-2">
                  <input className={inputClass} value={newGuarantor.title} onChange={e => setNewGuarantor(prev => ({ ...prev, title: e.target.value }))} placeholder="职务" />
                </View>
                <View className="mt-2">
                  <input className={inputClass} value={newGuarantor.address} onChange={e => setNewGuarantor(prev => ({ ...prev, address: e.target.value }))} placeholder="地址（选填）" />
                </View>
                <View className="flex gap-2 mt-3">
                  <Button
                    onClick={handleSaveGuarantor}
                    disabled={savingGuarantor}
                    style={{ flex: 1, backgroundColor: '#B98E5F', color: '#fff', borderRadius: 8, fontSize: 14, border: 'none' }}
                  >
                    <Text style={{ color: '#fff', fontSize: 14 }}>{savingGuarantor ? '保存中...' : '保存'}</Text>
                  </Button>
                  <Button
                    onClick={() => setShowAddGuarantor(false)}
                    style={{ flex: 1, backgroundColor: '#f4f4f5', color: '#71717a', borderRadius: 8, fontSize: 14, border: 'none' }}
                  >
                    <Text style={{ color: '#71717a', fontSize: 14 }}>取消</Text>
                  </Button>
                </View>
              </View>
            ) : (
              <Button
                onClick={() => setShowAddGuarantor(true)}
                className="text-sm text-brand-primary flex items-center gap-1"
              >
                <Plus size={14} /> 新增担保人
              </Button>
            )}
          </View>
        )}

        <View className="bg-white rounded-2xl shadow-sm p-4 mb-3">
          <Text className="font-black text-black mb-3 flex items-center gap-2">
            <MapPin size={16} className="text-brand-primary" />
            收货地址
          </Text>

          {addresses.length > 0 && !useNewAddress && (
            <View className="mb-3">
              {addresses.map((addr, ai) => (
                <label
                  key={addr.id}
                  className={`flex items-start gap-3 p-3 border rounded-lg cursor-pointer ${ai > 0 ? 'mt-2' : ''} ${
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
            <View>
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
              <View className="grid grid-cols-3 gap-2 mt-3">
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
              <View className="mt-3">
                <input className={inputClass} value={newAddress.detail} onChange={e => setNewAddress(prev => ({ ...prev, detail: e.target.value }))} placeholder="详细地址" />
              </View>
              <View className="mt-3">
                <input className={inputClass} value={newAddress.postal_code} onChange={e => setNewAddress(prev => ({ ...prev, postal_code: e.target.value }))} placeholder="邮编" pattern="\d{6}" maxLength={6} inputMode="numeric" title="请输入6位数字邮编" />
              </View>
              <label className="flex items-center gap-2 text-sm text-gray-500 cursor-pointer mt-3">
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

        <View className="bg-amber-50 border border-amber-100 rounded-2xl p-4 text-sm text-amber-700 mb-3">
          <Text className="font-medium mb-1">租赁须知</Text>
          <ul className="text-xs text-amber-600">
            <li>· 提交即生成订单，需在10分钟内完成支付</li>
            <li className="mt-1">· 超时未支付订单将自动取消</li>
            <li className="mt-1">· 发货前可取消订单免手续费</li>
            <li className="mt-1">· 押金在归还验收后原路退还</li>
          </ul>
        </View>
      </View>

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb">
        <View className="flex items-center justify-between mb-2">
          <Text className="text-sm text-zinc-400">应付总额</Text>
          <Text className="text-xl font-black" style={{ color: '#915F38' }}>¥{totalAmount.toFixed(2)}</Text>
        </View>
        <Button
          onClick={handleSubmit}
          disabled={submitting}
          style={{ width: '100%', paddingTop: 12, paddingBottom: 12, backgroundColor: submitting ? 'rgba(185,142,95,0.5)' : '#B98E5F', color: '#fff', borderRadius: 12, border: 'none', outline: 'none', fontWeight: 900, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8 }}
        >
          {submitting ? '处理中...' : '提交订单'}
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
  const [depositWaived, setDepositWaived] = useState(false)
  const [guarantors, setGuarantors] = useState([])
  const [selectedGuarantorIds, setSelectedGuarantorIds] = useState([])
  const [showAddGuarantor, setShowAddGuarantor] = useState(false)
  const [newGuarantor, setNewGuarantor] = useState({ name: '', phone: '', company: '', title: '', address: '' })
  const [savingGuarantor, setSavingGuarantor] = useState(false)

  useEffect(() => {
    const token = getToken()
    if (!token) {
      session.setItem('post_auth_redirect', '/checkout')
      redirectToLogin('checkout')
      return
    }
    const loadData = async () => {
      const data = storage.getJSON('cart_checkout', storage.getJSON(getCartKey(), { items: [] })) || { items: [] }
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
      try {
        const guarResp = await apiFetch(`${env.apiBaseUrl}/user/guarantors`)
        const guarResult = await guarResp.json()
        if (guarResult.code === 20000) {
          setGuarantors(Array.isArray(guarResult.data) ? guarResult.data : guarResult.data?.list || [])
        }
      } catch (e) { console.error('Failed to load guarantors:', e) }
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
        map[key] = {
          tenant_id: item.tenant_id,
          tenant_name: item.tenant_name || '',
          site_name: item.site_name || '',
          site_address: item.site_address || '',
          site_phone: item.site_phone || '',
          shippingFee: 0,
          items: [],
        }
      }
      const itemShipping = item.shipping_fee || 0
      if (itemShipping > map[key].shippingFee) map[key].shippingFee = itemShipping
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
      total += groupRent + (depositWaived ? 0 : groupDeposit)
    }
    return total
  }, [groups, depositWaived])

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  const handleSaveGuarantor = async () => {
    if (!newGuarantor.name.trim() || !newGuarantor.phone.trim()) {
      dialog.alert('请填写担保人姓名和联系电话')
      return
    }
    if (!newGuarantor.company.trim() || !newGuarantor.title.trim()) {
      dialog.alert('请填写工作单位和职务')
      return
    }
    setSavingGuarantor(true)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/user/guarantors`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newGuarantor),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        const g = result.data
        setGuarantors(prev => [...prev, g])
        setSelectedGuarantorIds(prev => prev.includes(g.id) ? prev : [...prev, g.id])
        setNewGuarantor({ name: '', phone: '', company: '', title: '', address: '' })
        setShowAddGuarantor(false)
        dialog.alert('担保人已保存')
      } else {
        dialog.alert('保存失败: ' + (resolveErrorMessage(result, '未知错误')))
      }
    } catch (err) {
      dialog.alert('保存失败: ' + (err?.message || '网络错误'))
    }
    setSavingGuarantor(false)
  }

  const toggleGuarantor = (id) => {
    setSelectedGuarantorIds(prev => prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id])
  }

  const handleSubmit = async () => {
    if (cartItems.length === 0) return
    if (depositWaived && selectedGuarantorIds.length < 2) {
      dialog.alert('免押金订单需提供至少 2 位担保人，请选择或新增担保人')
      return
    }
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
        instrument_id: item.instrument_id || item.id,
        start_date: dayjs().format('YYYY-MM-DD'),
        end_date: dayjs().add((item.rent_qty || 30) - 1, 'day').format('YYYY-MM-DD'),
        rent_days: item.rent_qty || 30,
      }))
      const body = { items }
        if (deliveryAddress) body.delivery_address = deliveryAddress
        if (depositWaived) {
          body.deposit_waived = true
          body.guarantor_ids = selectedGuarantorIds
        }
        const orderResp = await ordersApi.batchCreate(body)
        if (orderResp.code === 20000) {
          const orders = orderResp.data?.orders || []
          if (orders.length > 0) {
            const ids = new Set(cartItems.map(item => item.instrument_id || item.id))
            const cart = storage.getJSON(getCartKey(), { items: [] }) || { items: [] }
            storage.setJSON(getCartKey(), { items: cart.items.filter(item => !ids.has(item.instrument_id || item.id)) })
            storage.removeItem('cart_checkout')
            eventBus.emit('cartUpdated')
            // Keep /checkout in history so payment back button returns here (#1629)
            navigate(`/payment?type=rent&id=${orders[0].order_id}`)
          } else {
            dialog.alert('下单成功，但未生成订单')
          }
        } else {
          dialog.alert('下单失败: ' + (resolveErrorMessage(orderResp, '未知错误')))
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
        <View className="p-4 m-4 bg-white rounded-2xl shadow-sm border border-zinc-100 flex flex-col items-center">
          <View className="text-center">
            <Text className="text-xs text-zinc-400 font-bold tracking-widest block uppercase">TOTAL PAYABLE</Text>
            <Text className="text-[#915F38] text-4xl font-black tracking-tight block">
              ¥{grandTotal.toFixed(2)}
            </Text>
          </View>

          <View className="w-full border-t border-dashed border-zinc-200 pt-4">
            {groups.map((group) => {
              let groupRent = 0
              let groupDeposit = 0
              group.items.forEach(item => {
                const p = getItemPricing(item)
                groupRent += p.rent
                groupDeposit += p.deposit
              })
              const groupSubtotal = groupRent + (depositWaived ? 0 : groupDeposit)
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
                  </View>
                  {group.items.map((item) => {
                    const p = getItemPricing(item)
                    const images = parseImages(item.images)
                        const imgSrc = item.cover_image || images[0] || ''
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
                          {item.rent_qty || 30}天 · ¥{(p.rent || 0).toFixed(2)}
                        </Text>
                      </View>
                    )
                  })}
                  <View className="flex justify-between items-center mt-1 pt-1 border-t border-zinc-200/60">
                    <Text className="text-[10px] text-zinc-400">
                      {depositWaived ? '免押金' : `押金 ¥${(groupDeposit || 0).toFixed(2)}`}
                    </Text>
                    <Text className="text-sm font-bold text-zinc-800">小计 ¥{(groupSubtotal || 0).toFixed(2)}</Text>
                  </View>
                </View>
              )
            })}
          </View>

          <View className="w-full bg-zinc-50 p-3 rounded-xl text-[11px] text-zinc-400 leading-normal">
            🔒 暖心提示：资产固定押金将在乐器归还、网点网管质检合格后，按原支付渠道原路退回至您的微信零钱。
          </View>

          <View className="w-full flex items-center justify-between bg-gray-50 rounded-lg px-3 py-2.5">
            <View className="flex-1 min-w-0">
              <Text className="text-sm font-medium text-gray-700">免押金租赁</Text>
              <Text className="block text-[10px] text-gray-400">需提供两位担保人的联系方式</Text>
            </View>
            <input
              type="checkbox"
              checked={depositWaived}
              onChange={e => setDepositWaived(e.target.checked)}
              className="w-5 h-5 accent-[#B98E5F]"
            />
          </View>
          {depositWaived && (
            <View className="w-full bg-amber-50 border border-amber-100 rounded-lg px-3 py-2">
              <Text className="text-xs text-amber-700 leading-relaxed">
                应提供两位担保人的联系方式。我们的员工将会与他们联系确认，若担保人不符合要求，订单将被取消并退款。
              </Text>
            </View>
          )}
          {depositWaived && (
            <View className="w-full bg-white rounded-xl border border-zinc-100 p-3">
              <Text className="text-xs font-bold text-zinc-500 mb-1">🛡 担保人信息</Text>
              <Text className="text-[10px] text-zinc-400 mb-3">已选 {selectedGuarantorIds.length}/2</Text>
              {guarantors.length > 0 && (
                <View className="mb-3">
                  {guarantors.map((g, gi) => (
                    <label
                      key={g.id}
                      className={`flex items-start gap-3 p-3 border rounded-lg cursor-pointer ${gi > 0 ? 'mt-2' : ''} ${
                        selectedGuarantorIds.includes(g.id) ? 'border-brand-primary bg-blue-50' : 'border-gray-200'
                      }`}
                    >
                      <input
                        type="checkbox"
                        checked={selectedGuarantorIds.includes(g.id)}
                        onChange={() => toggleGuarantor(g.id)}
                        className="mt-1"
                      />
                      <View className="flex-1 text-xs">
                        <Text className="font-medium text-zinc-800">{g.name} · {g.phone}</Text>
                        {(g.company || g.title) && <Text className="text-zinc-400">{[g.company, g.title].filter(Boolean).join(' / ')}</Text>}
                        {g.address && <Text className="text-zinc-400">{g.address}</Text>}
                      </View>
                    </label>
                  ))}
                </View>
              )}
              {showAddGuarantor ? (
                <View className="bg-gray-50 rounded-lg p-3">
                  <View className="grid grid-cols-2 gap-2">
                    <input className={inputClass} value={newGuarantor.name} onChange={e => setNewGuarantor(prev => ({ ...prev, name: e.target.value }))} placeholder="姓名" />
                    <input className={inputClass} value={newGuarantor.phone} onChange={e => setNewGuarantor(prev => ({ ...prev, phone: e.target.value }))} placeholder="联系电话" />
                  </View>
                  <View className="mt-2">
                    <input className={inputClass} value={newGuarantor.company} onChange={e => setNewGuarantor(prev => ({ ...prev, company: e.target.value }))} placeholder="工作单位" />
                  </View>
                  <View className="mt-2">
                    <input className={inputClass} value={newGuarantor.title} onChange={e => setNewGuarantor(prev => ({ ...prev, title: e.target.value }))} placeholder="职务" />
                  </View>
                  <View className="mt-2">
                    <input className={inputClass} value={newGuarantor.address} onChange={e => setNewGuarantor(prev => ({ ...prev, address: e.target.value }))} placeholder="地址（选填）" />
                  </View>
                  <View className="flex gap-2 mt-3">
                    <Button
                      onClick={handleSaveGuarantor}
                      disabled={savingGuarantor}
                      style={{ flex: 1, backgroundColor: '#B98E5F', color: '#fff', borderRadius: 8, fontSize: 14, border: 'none' }}
                    >
                      <Text style={{ color: '#fff', fontSize: 14 }}>{savingGuarantor ? '保存中...' : '保存'}</Text>
                    </Button>
                    <Button
                      onClick={() => setShowAddGuarantor(false)}
                      style={{ flex: 1, backgroundColor: '#f4f4f5', color: '#71717a', borderRadius: 8, fontSize: 14, border: 'none' }}
                    >
                      <Text style={{ color: '#71717a', fontSize: 14 }}>取消</Text>
                    </Button>
                  </View>
                </View>
              ) : (
                <Text className="text-xs text-brand-primary" onClick={() => setShowAddGuarantor(true)}>+ 新增担保人</Text>
              )}
            </View>
          )}

          <View className="w-full border-t border-dashed border-zinc-200 pt-4">
            <Text className="text-xs font-bold text-zinc-500 mb-3">📍 收货地址</Text>

            {addresses.length > 0 && !useNewAddress && (
              <View className="mb-3">
                {addresses.map((addr, ai) => (
                  <label
                    key={addr.id}
                    className={`flex items-start gap-3 p-3 border rounded-lg cursor-pointer ${ai > 0 ? 'mt-2' : ''} ${
                      selectedAddressId === addr.id ? 'border-brand-primary bg-blue-50' : 'border-gray-200'
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
               <View>
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
                 <View className="grid grid-cols-3 gap-2 mt-3">
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
                 <View className="mt-3">
                   <input className={inputClass} value={newAddress.detail} onChange={e => setNewAddress(prev => ({ ...prev, detail: e.target.value }))} placeholder="详细地址" />
                 </View>
                 <View className="mt-3">
                   <input className={inputClass} value={newAddress.postal_code} onChange={e => setNewAddress(prev => ({ ...prev, postal_code: e.target.value }))} placeholder="邮编" pattern="\d{6}" maxLength={6} inputMode="numeric" title="请输入6位数字邮编" />
                 </View>
                 <label className="flex items-center gap-2 text-xs text-zinc-500 cursor-pointer mt-3">
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
          <Text className="text-sm font-black" style={{ color: '#915F38' }}>✓</Text>
        </View>
      </ScrollView>

      <View className="absolute bottom-0 left-0 right-0 bg-white p-4 pb-6 border-t border-zinc-100 z-50 flex flex-col items-center">
         <Button
          style={{ width: '100%', margin: 0, backgroundColor: submitting ? 'rgba(185,142,95,0.5)' : '#B98E5F', color: '#fff', fontWeight: 900, fontSize: 16, height: 48, borderRadius: 999, border: 'none', outline: 'none', display: 'flex', alignItems: 'center', justifyContent: 'center', letterSpacing: '0.05em' }}
          onClick={handleSubmit}
          disabled={submitting}
        >
          {submitting ? '处理中...' : `确认支付 ¥${grandTotal.toFixed(2)}`}
        </Button>
      </View>
    </View>
  )
}

export default function Checkout() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  // 跨端统一 query 约定（#1674）：单乐器直达结算 ?id=
  const id = searchParams.get('id') || ''
  if (id) {
    return <SingleCheckout id={id} navigate={navigate} />
  }
  return <BatchCheckout navigate={navigate} />
}
