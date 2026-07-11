import { useState, useEffect, useMemo } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image, Button, ScrollView, Input, Picker, Checkbox } from '@tarojs/components'
import { apiFetch, getToken, redirectToLogin, addressesApi, ordersApi, pointsApi } from '../services/api'
import dayjs from 'dayjs'
import { dialog, env, session, storage, eventBus } from '../platform'
import regions from '../data/regions.json'

const IMG_BASE = 'https://wx.cadenzayueqi.com'
const fixImg = (url) => url && !url.startsWith('http') && !url.startsWith('data:') ? IMG_BASE + url : url

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

function SingleCheckout({ id, nav }) {
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
  const [days, setDays] = useState(30)
  const [pointsBalance, setPointsBalance] = useState({ prepaid_points: 0, promo_points: 0 })
  const [prepaidPointsUsed, setPrepaidPointsUsed] = useState(0)
  const [giftPointsUsed, setGiftPointsUsed] = useState(0)
  const [usePoints, setUsePoints] = useState(false)
  const [rentalCalc, setRentalCalc] = useState(null)
  const [rentalCalcLoading, setRentalCalcLoading] = useState(false)
  const [daysInputText, setDaysInputText] = useState('30')

  const provinceNames = regions.map(r => r.name)
  const selectedProv = regions.find(r => r.name === newAddress.province)
  const cityNames = selectedProv ? selectedProv.children.map(c => c.name) : []
  const selectedCity = selectedProv ? selectedProv.children.find(c => c.name === newAddress.city) : null
  const districtNames = selectedCity ? selectedCity.children.map(d => d.name) : []

  const provinceIdx = newAddress.province ? provinceNames.indexOf(newAddress.province) : -1
  const cityIdx = newAddress.city ? cityNames.indexOf(newAddress.city) : -1
  const districtIdx = newAddress.district ? districtNames.indexOf(newAddress.district) : -1

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
        const [userRes, pointsRes] = await Promise.all([
          apiFetch(`${env.apiBaseUrl}/users/me`),
          pointsApi.balance(),
        ])
        const userResult = await userRes.json()
        if (userResult.code === 20000) {
          setUser(userResult.data)
          setNewAddress(prev => ({
            ...prev,
            recipient_name: prev.recipient_name || userResult.data.name || '',
            phone: prev.phone || userResult.data.phone || '',
          }))
        }
        if (pointsRes?.code === 20000) {
          setPointsBalance(pointsRes.data)
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

  useEffect(() => {
    if (!instrument?.id || !days) return
    const fetchCalc = async () => {
      setRentalCalcLoading(true)
      try {
        const res = await apiFetch(`${env.apiBaseUrl}/rental/calculate`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ instrument_id: instrument.id, days }),
        })
        const r = await res.json()
        if (r.code === 20000) setRentalCalc(r.data)
      } catch {}
      setRentalCalcLoading(false)
    }
    fetchCalc()
  }, [instrument?.id, days])

  const totalRent = computeTieredRent(days)
  const deposit = pricingV2?.deposit || parsePricing(instrument?.pricing)[0]?.deposit || 0
  const shippingFee = pricingV2?.shipping_fee || parsePricing(instrument?.pricing)[0]?.shipping_fee || 0
  const totalAmount = totalRent + deposit + shippingFee
  const startDate = new Date().toISOString().slice(0, 10)
  const returnDate = new Date(Date.now() + days * 86400000).toISOString().slice(0, 10)

  const handleDaysChange = (value) => {
    const v = parseInt(value) || 30
    setDays(Math.max(1, Math.min(730, v)))
    setDaysInputText(String(Math.max(1, Math.min(730, v))))
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

    const token = getToken()
    let role = ''
    try {
      if (token) {
        const payload = JSON.parse(atob(token.split('.')[1]))
        role = payload.role || ''
      }
    } catch {}
    if (role === 'GUEST') {
      dialog.alert('请先登录才能下单')
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
      if (usePoints) {
        body.prepaid_points_used = prepaidPointsUsed
        body.gift_points_used = giftPointsUsed
      }

      const resp = await ordersApi.create(body)
      if (resp.code === 20000 || resp.code === 20100) {
        eventBus.emit('cartUpdated')
        Taro.redirectTo({ url: '/pages-weapp/success/index' })
      } else {
        dialog.alert('下单失败: ' + (resp.message || '未知错误'))
      }
    } catch (err) {
      dialog.alert('下单失败: ' + (err?.message || '网络错误'))
    }
    setSubmitting(false)
  }

  if (loading) return <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
  if (!instrument) return <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', display: 'flex', alignItems: 'center', justifyContent: 'center' }}><Text style={{ color: '#a1a1aa' }}>乐器不存在</Text></View>

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', paddingBottom: 112 }}>
      <View style={{ background: 'linear-gradient(to bottom, #FDF4E7, #fff)', paddingLeft: 16, paddingRight: 16, paddingTop: 16, paddingBottom: 12, display: 'flex', alignItems: 'center' }}>
        <Text style={{ fontSize: 20, color: '#000', cursor: 'pointer', marginRight: 8 }} onClick={() => Taro.navigateBack()}>←</Text>
        <Text style={{ fontSize: 18, fontWeight: '900', color: '#000' }}>确认订单</Text>
      </View>

      <View style={{ paddingLeft: 16, paddingRight: 16, paddingTop: 16, paddingBottom: 16 }}>
        <View style={{ backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', padding: 16, marginBottom: 12 }}>
          <Text style={{ fontWeight: '900', color: '#000', marginBottom: 8 }}>租赁乐器</Text>
          <View style={{ display: 'flex' }}>
            <Image
              src={fixImg(instrument.cover_image || parseImages(instrument.images)?.[0] || '')}
              style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#FDF4E7', marginRight: 12, flexShrink: 0 }}
            />
            <View style={{ flex: '1 1 0%', justifyContent: 'center' }}>
              <Text style={{ fontWeight: '900', fontSize: 14, color: '#000', marginBottom: 4 }}>{instrument.name || instrument.sn || id?.slice(0, 8)}</Text>
              <Text style={{ fontSize: 12, color: '#71717a', marginBottom: 2 }}>{instrument.category_name}{instrument.level_name ? ` · ${instrument.level_name}` : ''}</Text>
              <Text style={{ fontSize: 12, color: '#a1a1aa' }}>网点: {instrument.site_name || '-'}</Text>
            </View>
          </View>
        </View>

        <View style={{ backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', padding: 16, marginBottom: 12 }}>
          <Text style={{ fontWeight: '900', color: '#000', marginBottom: 12, display: 'flex', alignItems: 'center' }}>
            <Text style={{ fontSize: 16, color: '#915F38', marginRight: 8 }}>📅</Text>
            租期选择
          </Text>
          <View style={{ display: 'flex', alignItems: 'center' }}>
            <Button
              onClick={() => handleDaysChange(days - 1)}
              style={{ width: 40, height: 40, border: '1px solid #d4d4d8', borderRadius: 8, fontSize: 18, fontWeight: '500', color: '#6b7280', display: 'flex', alignItems: 'center', justifyContent: 'center', lineHeight: '40px', textAlign: 'center', marginRight: 0 }}
            >−</Button>
            <Input
              type="number"
              value={daysInputText}
              onInput={e => handleDaysChange(e.detail.value)}
              style={{ flex: '1 1 0%', textAlign: 'center', fontSize: 20, fontWeight: 'bold', border: '1px solid #d4d4d8', borderRadius: 8, paddingTop: 8, paddingBottom: 8, marginLeft: 12, marginRight: 12 }}
            />
            <Button
              onClick={() => handleDaysChange(days + 1)}
              style={{ width: 40, height: 40, border: '1px solid #d4d4d8', borderRadius: 8, fontSize: 18, fontWeight: '500', color: '#6b7280', display: 'flex', alignItems: 'center', justifyContent: 'center', lineHeight: '40px', textAlign: 'center', marginLeft: 0 }}
            >+</Button>
            <Text style={{ fontSize: 14, color: '#6b7280', marginLeft: 12 }}>天</Text>
          </View>
          <View style={{ marginTop: 8, fontSize: 12, color: '#9ca3af', display: 'flex', alignItems: 'center' }}>
            <Text style={{ fontSize: 12, marginRight: 4 }}>🕐</Text>
            <Text>预计归还: {returnDate}</Text>
            {pricingV2?.tiers?.length > 0 && <Text style={{ marginLeft: 4 }}>· 阶梯计价</Text>}
          </View>
        </View>

        <View style={{ backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', padding: 16, marginBottom: 12 }}>
          <Text style={{ fontWeight: '900', color: '#000', marginBottom: 12 }}>费用明细</Text>
          <View style={{ fontSize: 14 }}>
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <Text style={{ color: '#a1a1aa' }}>租金 ({days}天)</Text>
              <Text style={{ fontWeight: '500', flexShrink: 0, marginLeft: 'auto', whiteSpace: 'nowrap' }}>¥{totalRent.toFixed(0)}</Text>
            </View>
            {pricingV2?.tiers?.length > 0 && (
              <View style={{ fontSize: 12, color: '#a1a1aa', paddingLeft: 8, paddingBottom: 4, borderBottom: '1px dashed #d4d4d8' }}>
                {pricingV2.tiers.map((t, i) => {
                  const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                  const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
                  return <Text key={i} style={{ marginRight: 12 }}>{range}: ¥{Math.round(t.daily_rate)}/天</Text>
                })}
              </View>
            )}
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <Text style={{ color: '#a1a1aa' }}>押金</Text>
              <Text style={{ fontWeight: '500', flexShrink: 0, marginLeft: 'auto', whiteSpace: 'nowrap' }}>¥{deposit}{deposit === 0 ? <Text style={{ fontSize: 10, color: '#a1a1aa', marginLeft: 4 }}>(日租金×倍率)</Text> : null}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <Text style={{ color: '#a1a1aa' }}>物流费</Text>
              <Text style={{ fontWeight: '500', flexShrink: 0, marginLeft: 'auto', whiteSpace: 'nowrap' }}>¥{shippingFee}</Text>
            </View>
            <View style={{ borderTop: '1px solid #d4d4d8', paddingTop: 8, display: 'flex', justifyContent: 'space-between', fontWeight: '700', fontSize: 16, marginBottom: 4 }}>
              <Text style={{ color: '#18181b' }}>合计</Text>
              <Text style={{ color: '#915F38', flexShrink: 0, marginLeft: 'auto', whiteSpace: 'nowrap' }}>¥{totalAmount.toFixed(0)}</Text>
            </View>
            <Text style={{ fontSize: 10, color: '#a1a1aa', textAlign: 'right' }}>租金 ¥{totalRent.toFixed(0)} + 押金 ¥{deposit} + 物流费 ¥{shippingFee}</Text>
          </View>
        </View>

        {/* Points selection */}
        <View style={{ backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', padding: 16, marginBottom: 12 }}>
          <Text style={{ fontWeight: '900', color: '#000', marginBottom: 12 }}>点数使用</Text>
          {pointsBalance.prepaid_points > 0 || pointsBalance.promo_points > 0 ? (
            <>
              <View style={{ display: 'flex', alignItems: 'center', marginBottom: 12 }} onClick={() => { setUsePoints(!usePoints); if (usePoints) { setPrepaidPointsUsed(0); setGiftPointsUsed(0) } }}>
                <View style={{ width: 16, height: 16, borderRadius: 4, border: '1px solid #d4d4d8', backgroundColor: usePoints ? '#915F38' : 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 8 }}>
                  {usePoints && <Text style={{ color: '#fff', fontSize: 10 }}>✓</Text>}
                </View>
                <Text style={{ fontSize: 14 }}>使用点数抵扣</Text>
              </View>
              {usePoints && (
                <View>
                  {pointsBalance.prepaid_points > 0 && (
                    <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                      <Text style={{ fontSize: 14, color: '#6b7280' }}>预付点数 (余额: {pointsBalance.prepaid_points})</Text>
                      <Input
                        type="number"
                        value={String(prepaidPointsUsed)}
                        onInput={e => setPrepaidPointsUsed(Math.max(0, Math.min(pointsBalance.prepaid_points, parseInt(e.detail.value) || 0)))}
                        style={{ width: 96, textAlign: 'right', border: '1px solid #d4d4d8', borderRadius: 4, padding: '4px 8px', fontSize: 14 }}
                      />
                    </View>
                  )}
                  {pointsBalance.promo_points > 0 && (
                    <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <Text style={{ fontSize: 14, color: '#6b7280' }}>赠送点数 (余额: {pointsBalance.promo_points})</Text>
                      <Input
                        type="number"
                        value={String(giftPointsUsed)}
                        onInput={e => setGiftPointsUsed(Math.max(0, Math.min(pointsBalance.promo_points, parseInt(e.detail.value) || 0)))}
                        style={{ width: 96, textAlign: 'right', border: '1px solid #d4d4d8', borderRadius: 4, padding: '4px 8px', fontSize: 14 }}
                      />
                    </View>
                  )}
                </View>
              )}
            </>
          ) : (
            <Text style={{ fontSize: 14, color: '#9ca3af' }}>暂无可用点数</Text>
          )}
        </View>

        <View style={{ backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', padding: 16, marginBottom: 12 }}>
          <Text style={{ fontWeight: '900', color: '#000', marginBottom: 12, display: 'flex', alignItems: 'center' }}>
            <Text style={{ fontSize: 16, color: '#915F38', marginRight: 8 }}>📍</Text>
            收货地址
          </Text>

          {addresses.length > 0 && !useNewAddress && (
            <View style={{ marginBottom: 12 }}>
              {addresses.map(addr => (
                <View
                  key={addr.id}
                  style={{ display: 'flex', padding: 12, border: selectedAddressId === addr.id ? '1px solid #915F38' : '1px solid #e4e4e7', borderRadius: 8, backgroundColor: selectedAddressId === addr.id ? '#eff6ff' : 'transparent', marginBottom: 8 }}
                  onClick={() => { setSelectedAddressId(addr.id); setUseNewAddress(false) }}
                >
                  <View style={{ width: 16, height: 16, borderRadius: 999, border: '1px solid #d4d4d8', display: 'flex', alignItems: 'center', justifyContent: 'center', marginTop: 2, marginRight: 12 }}>
                    {selectedAddressId === addr.id && <View style={{ width: 8, height: 8, borderRadius: 999, backgroundColor: '#915F38' }} />}
                  </View>
                  <View style={{ flex: '1 1 0%', fontSize: 14 }}>
                    <Text style={{ fontWeight: '500', marginBottom: 4 }}>{addr.recipient_name} · {addr.phone}</Text>
                    <Text style={{ fontSize: 12, color: '#9ca3af' }}>{addr.province}{addr.city}{addr.district} {addr.detail}</Text>
                    {addr.is_default && <Text style={{ fontSize: 12, color: '#915F38', marginTop: 2 }}>默认</Text>}
                  </View>
                </View>
              ))}
            </View>
          )}

          {(addresses.length === 0 || useNewAddress) && (
            <View>
              <View style={{ display: 'flex', marginBottom: 8 }}>
                <View style={{ flex: '1 1 0%', marginRight: 8 }}>
                  <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>收货人</Text>
                  <Input value={newAddress.recipient_name} onInput={e => setNewAddress(prev => ({ ...prev, recipient_name: e.detail.value }))} placeholder="姓名"
                    style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%', height: 44, boxSizing: 'border-box' }} />
                </View>
                <View style={{ flex: '1 1 0%' }}>
                  <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>电话</Text>
                  <Input value={newAddress.phone} onInput={e => setNewAddress(prev => ({ ...prev, phone: e.detail.value }))} placeholder="手机号"
                    style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%', height: 44, boxSizing: 'border-box' }} />
                </View>
              </View>
              <View style={{ display: 'flex', marginBottom: 8 }}>
                <View style={{ flex: '1 1 0%', marginRight: 8 }}>
                  <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>省</Text>
                  <Picker mode="selector" range={provinceNames} value={provinceIdx >= 0 ? provinceIdx : 0}
                    onChange={e => setNewAddress(prev => ({ ...prev, province: provinceNames[e.detail.value], city: '', district: '' }))}>
                    <View style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, color: newAddress.province ? '#000' : '#9ca3af' }}>
                      {newAddress.province || '省'}
                    </View>
                  </Picker>
                </View>
                <View style={{ flex: '1 1 0%', marginRight: 8 }}>
                  <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>市</Text>
                  <Picker mode="selector" range={cityNames} value={cityIdx >= 0 ? cityIdx : 0}
                    onChange={e => setNewAddress(prev => ({ ...prev, city: cityNames[e.detail.value], district: '' }))}>
                    <View style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, color: newAddress.city ? '#000' : '#9ca3af' }}>
                      {newAddress.city || '市'}
                    </View>
                  </Picker>
                </View>
                 {districtNames.length > 0 && (
                 <View style={{ flex: '1 1 0%' }}>
                   <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>区</Text>
                   <Picker mode="selector" range={districtNames} value={districtIdx >= 0 ? districtIdx : 0}
                     onChange={e => setNewAddress(prev => ({ ...prev, district: districtNames[e.detail.value] }))}>
                     <View style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, color: newAddress.district ? '#000' : '#9ca3af' }}>
                       {newAddress.district || '区'}
                     </View>
                   </Picker>
                 </View>
                 )}
               </View>
               <View style={{ marginBottom: 8 }}>
                 <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>详细地址</Text>
                 <Input value={newAddress.detail} onInput={e => setNewAddress(prev => ({ ...prev, detail: e.detail.value }))} placeholder="详细地址"
                  style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%' }} />
              </View>
              <View style={{ marginBottom: 8 }}>
                <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>邮编</Text>
                <Input value={newAddress.postal_code} onInput={e => setNewAddress(prev => ({ ...prev, postal_code: e.detail.value }))} placeholder="邮编"
                  maxlength={6} style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%' }} />
              </View>
              <View style={{ display: 'flex', alignItems: 'center', marginBottom: 8 }} onClick={() => setSaveAddress(!saveAddress)}>
                <View style={{ width: 16, height: 16, borderRadius: 4, border: '1px solid #d4d4d8', backgroundColor: saveAddress ? '#915F38' : 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 8 }}>
                  {saveAddress && <Text style={{ color: '#fff', fontSize: 10 }}>✓</Text>}
                </View>
                <Text style={{ fontSize: 14, color: '#6b7280' }}>设置为我的收货地址</Text>
              </View>
            </View>
          )}

          {addresses.length > 0 && !useNewAddress && (
            <Text style={{ marginTop: 12, fontSize: 14, color: '#915F38' }} onClick={() => setUseNewAddress(true)}>+ 使用新地址</Text>
          )}
          {useNewAddress && addresses.length > 0 && (
            <Text style={{ marginTop: 12, fontSize: 14, color: '#9ca3af' }} onClick={() => setUseNewAddress(false)}>选择已有地址</Text>
          )}
        </View>

        <View style={{ backgroundColor: '#fffbeb', border: '1px solid #fef3c7', borderRadius: 16, padding: 16, fontSize: 14, color: '#b45309', marginBottom: 12 }}>
          <Text style={{ fontWeight: '500', marginBottom: 4 }}>租赁须知</Text>
          <View style={{ fontSize: 12, color: '#d97706' }}>
            <Text style={{ marginBottom: 2 }}>· 提交即生成订单，需在10分钟内完成支付</Text>
            <Text style={{ marginBottom: 2 }}>· 超时未支付订单将自动取消</Text>
            <Text style={{ marginBottom: 2 }}>· 发货前可取消订单免手续费</Text>
            <Text>· 押金在归还验收后原路退还</Text>
          </View>
        </View>
      </View>

      <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, backgroundColor: '#fff', borderTop: '1px solid #f4f4f5', padding: 16 }}>
        <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
          <Text style={{ fontSize: 14, color: '#a1a1aa' }}>{usePoints && (prepaidPointsUsed > 0 || giftPointsUsed > 0) ? '实付金额' : '应付总额'}</Text>
          <Text style={{ fontSize: 20, fontWeight: '900', color: '#915F38' }}>¥{(totalAmount - (usePoints ? prepaidPointsUsed + giftPointsUsed : 0)).toFixed(0)}</Text>
        </View>
        {usePoints && (prepaidPointsUsed > 0 || giftPointsUsed > 0) && (
          <Text style={{ fontSize: 12, color: '#a1a1aa', textAlign: 'right', marginBottom: 4 }}>
            点数抵扣 ¥{(prepaidPointsUsed + giftPointsUsed).toFixed(0)}
          </Text>
        )}
        <Button
          onClick={handleSubmit}
          disabled={submitting}
          style={{ width: '100%', paddingTop: 12, paddingBottom: 12, backgroundColor: submitting ? 'rgba(145,95,56,0.5)' : '#915F38', color: '#fff', borderRadius: 12, fontWeight: '900', textAlign: 'center' }}
        >
          {submitting ? '提交中...' : '提交订单'}
        </Button>
      </View>
    </View>
  )
}

function BatchCheckout({ nav }) {
  const [submitting, setSubmitting] = useState(false)
  const [cartItems, setCartItems] = useState([])
  const [addresses, setAddresses] = useState([])
  const [selectedAddressId, setSelectedAddressId] = useState('')
  const [useNewAddress, setUseNewAddress] = useState(false)
  const [newAddress, setNewAddress] = useState({ recipient_name: '', phone: '', province: '', city: '', district: '', detail: '', postal_code: '' })
  const [saveAddress, setSaveAddress] = useState(true)
  const [user, setUser] = useState(null)

  const provinceNames = regions.map(r => r.name)
  const selectedProv = regions.find(r => r.name === newAddress.province)
  const cityNames = selectedProv ? selectedProv.children.map(c => c.name) : []
  const selectedCity = selectedProv ? selectedProv.children.find(c => c.name === newAddress.city) : null
  const districtNames = selectedCity ? selectedCity.children.map(d => d.name) : []

  const provinceIdx = newAddress.province ? provinceNames.indexOf(newAddress.province) : -1
  const cityIdx = newAddress.city ? cityNames.indexOf(newAddress.city) : -1
  const districtIdx = newAddress.district ? districtNames.indexOf(newAddress.district) : -1

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

    const token = getToken()
    let role = ''
    try {
      if (token) {
        const payload = JSON.parse(atob(token.split('.')[1]))
        role = payload.role || ''
      }
    } catch {}
    if (role === 'GUEST') {
      dialog.alert('请先登录才能下单')
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
          await apiFetch(`${env.apiBaseUrl}/orders/${orderId}/pay`, { method: 'POST' })
        }
        storage.removeItem('cart')
        eventBus.emit('cartUpdated')
        Taro.redirectTo({ url: '/pages-weapp/success/index' })
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
      <View style={{ minHeight: '100vh', backgroundColor: '#fafafa', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ color: '#a1a1aa' }}>购物车为空</Text>
      </View>
    )
  }

  return (
    <View style={{ height: '100vh', width: '100vw', backgroundColor: '#fafafa', overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <View style={{ width: '100%', paddingTop: 12, paddingBottom: 8, paddingLeft: 16, paddingRight: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center', backgroundColor: '#fff', borderBottom: '1px solid #f4f4f5', flexShrink: 0 }}>
        <Text style={{ fontSize: 20, fontWeight: '700', color: '#000' }} onClick={() => Taro.navigateBack()}>❮</Text>
        <Text style={{ fontSize: 18, fontWeight: '900', color: '#000' }}>确认支付</Text>
        <View style={{ width: 24 }}></View>
      </View>

      <ScrollView style={{ width: '100%', flex: '1 1 0%', paddingBottom: 112 }} scrollY showScrollbar={false}>
        <View style={{ padding: 16, margin: 16, backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', border: '1px solid #f4f4f5', display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <View style={{ textAlign: 'center' }}>
            <Text style={{ fontSize: 12, color: '#a1a1aa', fontWeight: '700', letterSpacing: '0.1em' }}>TOTAL PAYABLE</Text>
            <Text style={{ color: '#C21838', fontSize: 36, fontWeight: '900', letterSpacing: '-0.025em' }}>
              ¥{grandTotal.toFixed(0)}
            </Text>
          </View>

          <View style={{ width: '100%', borderTop: '1px dashed #e4e4e7', paddingTop: 16, marginTop: 16 }}>
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
                <View key={group.tenant_id || 'unknown'} style={{ backgroundColor: 'rgba(250,250,250,0.4)', borderRadius: 12, padding: 12, marginBottom: 12 }}>
                  <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
                    <View style={{ display: 'flex', alignItems: 'center' }}>
                      <Text style={{ marginRight: 4 }}>🏢</Text>
                      <Text style={{ fontSize: 14, fontWeight: '700', color: '#3f3f46', marginRight: 4 }}>{group.tenant_name}</Text>
                      <Text style={{ color: '#d4d4d8', marginRight: 4 }}>|</Text>
                      <Text style={{ marginRight: 4 }}>📍</Text>
                      <Text style={{ fontSize: 14, color: '#52525b' }}>{group.site_name}</Text>
                    </View>
                    <Text style={{ fontSize: 12, color: '#a1a1aa' }}>{group.items.length}件</Text>
                  </View>
                  {group.items.map((item) => {
                    const p = getItemPricing(item)
                    const images = parseImages(item.images)
                    const imgSrc = fixImg(images[0] || item.cover || '')
                    return (
                      <View key={item.instrument_id || item.id} style={{ display: 'flex', alignItems: 'center', paddingTop: 6, paddingBottom: 6, borderBottom: '1px solid #f4f4f5' }}>
                        {imgSrc && (
                          <Image src={imgSrc} style={{ width: 32, height: 32, borderRadius: 4, backgroundColor: '#f4f4f5', marginRight: 8, flexShrink: 0 }} />
                        )}
                        <View style={{ flex: '1 1 0%', minWidth: 0 }}>
                          <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{item.sn || item.name}</Text>
                          <Text style={{ fontSize: 10, color: '#a1a1aa' }}>{item.category_name || ''}</Text>
                        </View>
                        <Text style={{ fontSize: 10, color: '#6b7280', flexShrink: 0, marginLeft: 8 }}>
                          {item.rent_qty || 1}天 · ¥{p.rent}
                        </Text>
                      </View>
                    )
                  })}
                  <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 4, paddingTop: 4, borderTop: '1px solid rgba(228,228,231,0.6)' }}>
                    <Text style={{ fontSize: 10, color: '#a1a1aa' }}>
                      押金 ¥{groupDeposit} + 运费 ¥{group.shippingFee || 0}
                    </Text>
                    <Text style={{ fontSize: 14, fontWeight: '700', color: '#27272a' }}>小计 ¥{groupSubtotal}</Text>
                  </View>
                </View>
              )
            })}
          </View>

          <View style={{ width: '100%', backgroundColor: '#fafafa', padding: 12, borderRadius: 12, fontSize: 11, color: '#a1a1aa', lineHeight: 20 }}>
            🔒 暖心提示：资产固定押金将在乐器归还、网点网管质检合格后，按原支付渠道原路退回至您的微信零钱。
          </View>

          <View style={{ width: '100%', borderTop: '1px dashed #e4e4e7', paddingTop: 16, marginTop: 16 }}>
            <Text style={{ fontSize: 12, fontWeight: '700', color: '#6b7280', marginBottom: 12 }}>📍 收货地址</Text>

            {addresses.length > 0 && !useNewAddress && (
              <View style={{ marginBottom: 12 }}>
                {addresses.map(addr => (
                  <View
                    key={addr.id}
                    style={{ display: 'flex', padding: 12, border: selectedAddressId === addr.id ? '1px solid #915F38' : '1px solid #e4e4e7', borderRadius: 8, backgroundColor: selectedAddressId === addr.id ? '#eff6ff' : 'transparent', marginBottom: 8 }}
                    onClick={() => { setSelectedAddressId(addr.id); setUseNewAddress(false) }}
                  >
                    <View style={{ width: 16, height: 16, borderRadius: 999, border: '1px solid #d4d4d8', display: 'flex', alignItems: 'center', justifyContent: 'center', marginTop: 2, marginRight: 12 }}>
                      {selectedAddressId === addr.id && <View style={{ width: 8, height: 8, borderRadius: 999, backgroundColor: '#915F38' }} />}
                    </View>
                    <View style={{ flex: '1 1 0%', fontSize: 12 }}>
                      <Text style={{ fontWeight: '500', color: '#27272a', marginBottom: 4 }}>{addr.recipient_name} · {addr.phone}</Text>
                      <Text style={{ color: '#a1a1aa' }}>{addr.province}{addr.city}{addr.district} {addr.detail}</Text>
                      {addr.is_default && <Text style={{ fontSize: 12, color: '#915F38', marginTop: 2 }}>默认</Text>}
                    </View>
                  </View>
                ))}
              </View>
            )}

            {(addresses.length === 0 || useNewAddress) && (
              <View>
                <View style={{ display: 'flex', marginBottom: 8 }}>
                  <View style={{ flex: '1 1 0%', marginRight: 8 }}>
                    <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>收货人</Text>
                  <Input value={newAddress.recipient_name} onInput={e => setNewAddress(prev => ({ ...prev, recipient_name: e.detail.value }))} placeholder="姓名"
                    style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%', height: 44, boxSizing: 'border-box' }} />
                </View>
                <View style={{ flex: '1 1 0%' }}>
                  <Text style={{ fontSize: 14, fontWeight: '500', color: '#374151', marginBottom: 4 }}>电话</Text>
                  <Input value={newAddress.phone} onInput={e => setNewAddress(prev => ({ ...prev, phone: e.detail.value }))} placeholder="手机号"
                    style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%', height: 44, boxSizing: 'border-box' }} />
                </View>
              </View>
              <View style={{ display: 'flex', marginBottom: 8 }}>
                <View style={{ flex: '1 1 0%', marginRight: 8 }}>
                  <Picker mode="selector" range={provinceNames} value={provinceIdx >= 0 ? provinceIdx : 0}
                      onChange={e => setNewAddress(prev => ({ ...prev, province: provinceNames[e.detail.value], city: '', district: '' }))}>
                      <View style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, color: newAddress.province ? '#000' : '#9ca3af' }}>{newAddress.province || '省'}</View>
                    </Picker>
                  </View>
                  <View style={{ flex: '1 1 0%', marginRight: 8 }}>
                    <Picker mode="selector" range={cityNames} value={cityIdx >= 0 ? cityIdx : 0}
                      onChange={e => setNewAddress(prev => ({ ...prev, city: cityNames[e.detail.value], district: '' }))}>
                      <View style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, color: newAddress.city ? '#000' : '#9ca3af' }}>{newAddress.city || '市'}</View>
                    </Picker>
                  </View>
                  {districtNames.length > 0 && (
                  <View style={{ flex: '1 1 0%' }}>
                    <Picker mode="selector" range={districtNames} value={districtIdx >= 0 ? districtIdx : 0}
                      onChange={e => setNewAddress(prev => ({ ...prev, district: districtNames[e.detail.value] }))}>
                      <View style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, color: newAddress.district ? '#000' : '#9ca3af' }}>{newAddress.district || '区'}</View>
                    </Picker>
                  </View>
                  )}
                </View>
                <Input value={newAddress.detail} onInput={e => setNewAddress(prev => ({ ...prev, detail: e.detail.value }))} placeholder="详细地址"
                  style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%', marginBottom: 8 }} />
                <Input value={newAddress.postal_code} onInput={e => setNewAddress(prev => ({ ...prev, postal_code: e.detail.value }))} placeholder="邮编"
                  maxlength={6} style={{ border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14, width: '100%', marginBottom: 8 }} />
                <View style={{ display: 'flex', alignItems: 'center', marginBottom: 8 }} onClick={() => setSaveAddress(!saveAddress)}>
                  <View style={{ width: 16, height: 16, borderRadius: 4, border: '1px solid #d4d4d8', backgroundColor: saveAddress ? '#915F38' : 'transparent', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 8 }}>
                    {saveAddress && <Text style={{ color: '#fff', fontSize: 10 }}>✓</Text>}
                  </View>
                  <Text style={{ fontSize: 12, color: '#6b7280' }}>设置为我的收货地址</Text>
                </View>
              </View>
            )}

            {addresses.length > 0 && !useNewAddress && (
              <Text style={{ marginTop: 12, fontSize: 12, color: '#915F38' }} onClick={() => setUseNewAddress(true)}>+ 使用新地址</Text>
            )}
            {useNewAddress && addresses.length > 0 && (
              <Text style={{ marginTop: 12, fontSize: 12, color: '#9ca3af' }} onClick={() => setUseNewAddress(false)}>选择已有地址</Text>
            )}
          </View>
        </View>

        <View style={{ marginLeft: 16, marginRight: 16, padding: 16, backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', display: 'flex', alignItems: 'center', justifyContent: 'space-between', border: '1px solid #f4f4f5' }}>
          <View style={{ display: 'flex', alignItems: 'center' }}>
            <Text style={{ fontSize: 24, marginRight: 12 }}>🟢</Text>
            <View>
              <Text style={{ fontSize: 16, fontWeight: '900', color: '#000' }}>微信支付</Text>
              <Text style={{ fontSize: 11, color: '#a1a1aa' }}>亿万用户的安全选择</Text>
            </View>
          </View>
          <Text style={{ fontSize: 14, fontWeight: '900', color: '#f97316' }}>✓</Text>
        </View>
      </ScrollView>

      <View style={{ position: 'absolute', bottom: 0, left: 0, right: 0, backgroundColor: '#fff', padding: 16, paddingBottom: 24, borderTop: '1px solid #f4f4f5', zIndex: 50, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        <Button
          onClick={handleSubmit}
          disabled={submitting}
          style={{ width: '100%', margin: 0, backgroundColor: submitting ? 'rgba(185,142,95,0.5)' : '#B98E5F', color: '#fff', fontWeight: '800', fontSize: 16, height: 48, borderRadius: 999, boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)', display: 'flex', alignItems: 'center', justifyContent: 'center', letterSpacing: '0.05em' }}
        >
          {submitting ? '提交中...' : `确认支付 ¥${grandTotal.toFixed(0)}`}
        </Button>
      </View>
    </View>
  )
}

export default function Checkout() {
  const instance = Taro.getCurrentInstance()
  const { id } = instance.router?.params || {}
  const nav = (url) => { Taro.navigateTo({ url }) }
  if (id) {
    return <SingleCheckout id={id} nav={nav} />
  }
  return <BatchCheckout nav={nav} />
}
