import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { apiFetch, getToken, redirectToLogin, addressesApi, ordersApi } from '../services/api'
import { ArrowLeft, MapPin, Clock, Calendar, Plus, CheckCircle } from 'lucide-react'
import { dialog, env, session } from '../platform'

export default function Checkout() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [pricingV2, setPricingV2] = useState(null)
  const [addresses, setAddresses] = useState([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)

  const [days, setDays] = useState(30)
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
      let addressId = selectedAddressId
      let deliveryAddress = null

      if (useNewAddress) {
        if (saveAddress) {
          try {
            const createResp = await addressesApi.create(newAddress)
            if (createResp.code === 20000 && createResp.data) {
              addressId = createResp.data.id
              deliveryAddress = `${newAddress.recipient_name} ${newAddress.phone} ${newAddress.province}${newAddress.city}${newAddress.district} ${newAddress.detail}`
            }
          } catch {}
        }
        if (!addressId) {
          deliveryAddress = `${newAddress.recipient_name} ${newAddress.phone} ${newAddress.province}${newAddress.city}${newAddress.district} ${newAddress.detail}${newAddress.postal_code ? ' ' + newAddress.postal_code : ''}`
        }
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
      if (addressId) body.address_id = addressId
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

  if (loading) return <div className="min-h-screen bg-gray-50 flex items-center justify-center"><p className="text-gray-500">加载中...</p></div>
  if (!instrument) return <div className="min-h-screen bg-gray-50 flex items-center justify-center"><p className="text-gray-500">乐器不存在</p></div>

  const selectedAddr = addresses.find(a => a.id === selectedAddressId)

  return (
    <div className="min-h-screen bg-gray-50 pb-28">
      <div className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">确认订单</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Instrument Summary */}
        <div className="bg-white rounded-xl p-4">
          <h2 className="text-sm font-medium text-gray-900 mb-2">租赁乐器</h2>
          <div className="flex gap-3">
            <img
              src={instrument.images?.[0] || ''}
              alt=""
              className="w-16 h-16 object-cover rounded-lg bg-gray-100"
              onError={(e) => { e.target.style.display = 'none' }}
            />
            <div>
              <p className="font-medium text-sm">SN: {instrument.sn || id?.slice(0, 8)}</p>
              <p className="text-xs text-gray-500">{instrument.category_name}{instrument.level_name ? ` · ${instrument.level_name}` : ''}</p>
            </div>
          </div>
        </div>

        {/* Rental Period */}
        <div className="bg-white rounded-xl p-4">
          <h2 className="text-sm font-medium text-gray-900 mb-3 flex items-center gap-2">
            <Calendar size={16} className="text-brand-primary" />
            租期选择
          </h2>
          <div className="flex items-center gap-3">
            <button
              onClick={() => handleDaysChange(days - 1)}
              className="w-10 h-10 border rounded-lg text-lg font-medium text-gray-500"
            >−</button>
            <input
              type="number"
              min={1}
              max={730}
              value={days}
              onChange={e => handleDaysChange(e.target.value)}
              className="flex-1 text-center text-xl font-bold border rounded-lg py-2"
            />
            <button
              onClick={() => handleDaysChange(days + 1)}
              className="w-10 h-10 border rounded-lg text-lg font-medium text-gray-500"
            >+</button>
            <span className="text-sm text-gray-500">天</span>
          </div>
          <div className="mt-2 text-xs text-gray-400 flex items-center gap-1">
            <Clock size={12} />
            预计归还: {returnDate}
            {pricingV2?.tiers?.length > 0 && <span className="ml-1">· 阶梯计价</span>}
          </div>
        </div>

        {/* Cost Breakdown */}
        <div className="bg-white rounded-xl p-4">
          <h2 className="text-sm font-medium text-gray-900 mb-3">费用明细</h2>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">租金 ({days}天)</span>
              <span className="font-medium">¥{totalRent.toFixed(0)}</span>
            </div>
            {pricingV2?.tiers?.length > 0 && (
              <div className="text-xs text-gray-400 pl-2 pb-1 border-b border-dashed">
                {pricingV2.tiers.map((t, i) => {
                  const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                  const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
                  return <span key={i} className="mr-3">{range}: ¥{t.daily_rate}/天</span>
                })}
              </div>
            )}
            <div className="flex justify-between">
              <span className="text-gray-500">押金</span>
              <span className="font-medium">¥{deposit}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">物流费</span>
              <span className="font-medium">¥{shippingFee}</span>
            </div>
            <div className="border-t pt-2 flex justify-between font-bold text-base">
              <span className="text-gray-900">合计</span>
              <span className="text-brand-primary">¥{totalAmount.toFixed(0)}</span>
            </div>
          </div>
        </div>

        {/* Address Selector */}
        <div className="bg-white rounded-xl p-4">
          <h2 className="text-sm font-medium text-gray-900 mb-3 flex items-center gap-2">
            <MapPin size={16} className="text-brand-primary" />
            收货地址
          </h2>

          {addresses.length > 0 && !useNewAddress && (
            <div className="space-y-2 mb-3">
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
                  <div className="flex-1 text-sm">
                    <p className="font-medium">{addr.recipient_name} · {addr.phone}</p>
                    <p className="text-xs text-gray-400">{addr.province}{addr.city}{addr.district} {addr.detail}</p>
                    {addr.is_default && <span className="text-xs text-brand-primary">默认</span>}
                  </div>
                </label>
              ))}
            </div>
          )}

          {(addresses.length === 0 || useNewAddress) && (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className={labelClass}>收货人</label>
                  <input className={inputClass} value={newAddress.recipient_name} onChange={e => setNewAddress(prev => ({ ...prev, recipient_name: e.target.value }))} placeholder="姓名" />
                </div>
                <div>
                  <label className={labelClass}>电话</label>
                  <input className={inputClass} value={newAddress.phone} onChange={e => setNewAddress(prev => ({ ...prev, phone: e.target.value }))} placeholder="手机号" />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                <input className={inputClass} value={newAddress.province} onChange={e => setNewAddress(prev => ({ ...prev, province: e.target.value }))} placeholder="省" />
                <input className={inputClass} value={newAddress.city} onChange={e => setNewAddress(prev => ({ ...prev, city: e.target.value }))} placeholder="市" />
                <input className={inputClass} value={newAddress.district} onChange={e => setNewAddress(prev => ({ ...prev, district: e.target.value }))} placeholder="区" />
              </div>
              <div>
                <input className={inputClass} value={newAddress.detail} onChange={e => setNewAddress(prev => ({ ...prev, detail: e.target.value }))} placeholder="详细地址" />
              </div>
              <div>
                <input className={inputClass} value={newAddress.postal_code} onChange={e => setNewAddress(prev => ({ ...prev, postal_code: e.target.value }))} placeholder="邮编" />
              </div>
              <label className="flex items-center gap-2 text-sm text-gray-500 cursor-pointer">
                <input type="checkbox" checked={saveAddress} onChange={e => setSaveAddress(e.target.checked)} />
                设置为我的收货地址
              </label>
            </div>
          )}

          {addresses.length > 0 && !useNewAddress && (
            <button
              onClick={() => setUseNewAddress(true)}
              className="mt-3 text-sm text-brand-primary flex items-center gap-1"
            >
              <Plus size={14} /> 使用新地址
            </button>
          )}
          {useNewAddress && addresses.length > 0 && (
            <button
              onClick={() => setUseNewAddress(false)}
              className="mt-3 text-sm text-gray-400"
            >
              选择已有地址
            </button>
          )}
        </div>

        {/* Rental Agreement Note */}
        <div className="bg-blue-50 border border-blue-200 rounded-xl p-4 text-sm text-blue-700">
          <p className="font-medium mb-1">租赁须知</p>
          <ul className="text-xs space-y-1 text-blue-600">
            <li>· 提交即生成订单，需在10分钟内完成支付</li>
            <li>· 超时未支付订单将自动取消</li>
            <li>· 发货前可取消订单免手续费</li>
            <li>· 押金在归还验收后原路退还</li>
          </ul>
        </div>
      </div>

      {/* Submit Button */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <div className="flex items-center justify-between mb-2">
          <span className="text-sm text-gray-500">应付总额</span>
          <span className="text-xl font-bold text-brand-primary">¥{totalAmount.toFixed(0)}</span>
        </div>
        <button
          onClick={handleSubmit}
          disabled={submitting}
          className="w-full py-3 bg-brand-primary text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {submitting ? '提交中...' : '提交订单'}
        </button>
      </div>
    </div>
  )
}
