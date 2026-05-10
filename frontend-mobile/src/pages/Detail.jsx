import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { instrumentsApi, ordersApi, getToken, apiFetch, redirectToLogin } from '../services/api'
import { ArrowLeft, Shield, Clock, AlertCircle, MapPin, Bell, CheckCircle, X, ShoppingCart, Calendar } from 'lucide-react'
import { Switch, Segmented, Tag, Modal, Button, InputNumber } from 'antd'

const CYCLE_OPTIONS = [
  { label: '按天', value: 'day' },
  { label: '按周', value: 'week' },
  { label: '按月', value: 'month' },
]

const CYCLE_MULTIPLIER = {
  day: 1,
  week: 6,
  month: 25,
}

const SERVICE_ITEMS = [
  { name: '基础清洁', entry: '✓', professional: '✓', master: '✓' },
  { name: '免费调音', entry: '1次/年', professional: '2次/年', master: '无限次' },
  { name: '深度维护', entry: '✗', professional: '✓', master: '✓' },
  { name: '免费维修', entry: '✗', professional: '✓', master: '✓' },
  { name: '专家精调', entry: '✗', professional: '✗', master: '✓' },
  { name: '上门保养', entry: '✗', professional: '✗', master: '✓' },
]

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
  if (!pricing) return {}
  if (typeof pricing === 'object') return pricing
  if (typeof pricing === 'string') {
    try { return JSON.parse(pricing) } catch { return {} }
  }
  return {}
}

export default function Detail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  
  const [selectedLevel, setSelectedLevel] = useState('专业级')
  const [cycle, setCycle] = useState('month')
  const [termCount, setTermCount] = useState(1)
  const [noDeposit, setNoDeposit] = useState(false)
  const [showComparison, setShowComparison] = useState(false)
  const [userCreditScore] = useState(750)
  const [canUseDepositFree, setCanUseDepositFree] = useState(false)
  
  const [calculatedRent, setCalculatedRent] = useState(0)
  const [totalAmount, setTotalAmount] = useState(0)
  const [cartToast, setCartToast] = useState(false)
  const cartItemCount = (() => {
    try {
      const cartData = JSON.parse(localStorage.getItem('cart') || '{"items":[]}')
      return cartData.items?.length || 0
    } catch { return 0 }
  })()
  
  // Load saved cycle/termCount from cart when instrument loads
  useEffect(() => {
    if (!instrument?.id) return
    try {
      const cart = JSON.parse(localStorage.getItem('cart') || '{"items":[]}')
      const item = cart.items?.find(i => i.instrument_id === instrument.id)
      if (item) {
        if (item.cycle && item.cycle !== cycle) setCycle(item.cycle)
        if (item.lease_term && item.lease_term !== termCount) setTermCount(item.lease_term)
      }
    } catch {}
  }, [instrument?.id])
  const isInCart = (() => {
    if (!instrument) return false
    try {
      const cartData = JSON.parse(localStorage.getItem('cart') || '{"items":[]}')
      return cartData.items?.some(i => i.instrument_id === instrument.id) || false
    } catch { return false }
  })()

  const handleAddToCart = () => {
    if (!instrument) return
    const existing = JSON.parse(localStorage.getItem('cart') || '{"items":[]}')
    
    // Remove existing item for this instrument if present
    const filtered = existing.items.filter(i => i.instrument_id !== instrument.id)
    
    // Add new item with current lease terms
    filtered.push({
      instrument_id: instrument.id,
      name: instrument.name,
      sn: instrument.sn,
      category_name: instrument.category_name,
      brand: instrument.brand,
      model: instrument.model,
      tenant_id: instrument.tenant_id,
      tenant_name: instrument.tenant_name || '',
      site_id: instrument.site_id,
      site_name: instrument.site_name,
      site_address: instrument.site_address || '',
      images: instrument.images,
      pricing: instrument.pricing,
      cycle: cycle,
      lease_term: termCount,
    })
    
    localStorage.setItem('cart', JSON.stringify({ items: filtered }))
    setCartToast(true)
    window.dispatchEvent(new Event('cartUpdated'))
  }

  const isRentable = instrument?.stock_status === 'available'

  useEffect(() => {
    const fetchInstrument = async () => {
      try {
        setLoading(true)
        const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
        const endpoint = `/public/instruments/${id}`
        const response = await apiFetch(`${baseUrl}${endpoint}`)
        const result = await response.json()
        if (result.code === 20000) {
          const inst = result.data
          inst._parsedPricing = parsePricing(inst.pricing)
          setInstrument(inst)
        }
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch instrument:', error)
        setLoading(false)
      }
    }
    
    fetchInstrument()
  }, [id])

  useEffect(() => {
    const creditScore = userCreditScore
    setCanUseDepositFree(creditScore >= 650)
  }, [userCreditScore])

  // Computed pricing values
  const pricing = parsePricing(instrument?.pricing)
  const dailyRent = pricing[0]?.daily_rent || 0
  const deposit = pricing[0]?.deposit || 0
  const shippingFee = pricing[0]?.shipping_fee || 0
  const overdueDailyFee = pricing[0]?.overdue_daily_fee || dailyRent
  
  const cycleMultiplier = CYCLE_MULTIPLIER[cycle]
  const rentPerCycle = dailyRent * cycleMultiplier
  const totalRent = rentPerCycle * termCount
  
  const calculatePrice = useCallback(() => {
    if (!instrument) return
    setCalculatedRent(totalRent)
    setTotalAmount(totalRent + deposit + shippingFee)
  }, [dailyRent, deposit, shippingFee, instrument, cycle, termCount])

  useEffect(() => {
    calculatePrice()
  }, [calculatePrice])

  const getExpiryDate = () => {
    const today = new Date()
    if (cycle === 'day') {
      today.setDate(today.getDate() + termCount)
    } else if (cycle === 'week') {
      today.setDate(today.getDate() + termCount * 7)
    } else {
      today.setMonth(today.getMonth() + termCount)
    }
    return today.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric' })
  }

  const handleCreateOrder = async () => {
    const token = getToken()
    
    if (!token) {
      sessionStorage.setItem('post_auth_redirect', window.location.pathname)
      redirectToLogin()
      return
    }
    
    const amount = calculatedRent + deposit + (pricing[0]?.shipping_fee || 0)
    const returnDate = getExpiryDate()
    
    navigate('/success', {
      state: {
        order_id: 'TL' + Date.now(),
        instrument_name: instrument?.name,
        instrument_sn: instrument?.sn,
        lease_term: `${termCount}${cycle === 'day' ? '天' : cycle === 'week' ? '周' : '月'}`,
        return_date: returnDate,
        total_amount: amount,
      },
    })
  }

  if (loading) {
    return <div className="p-4">加载中...</div>
  }

  if (!instrument) {
    return <div className="p-4">乐器不存在</div>
  }

  const images = parseImages(instrument.images)

  return (
    <div className="min-h-screen bg-gray-50 pb-24">
      <div className="relative">
         <img 
           src={images[0] || PLACEHOLDER_IMAGE} 
           alt={instrument.name}
           className="w-full h-64 object-contain bg-gray-100"
           onError={(e) => {
             e.target.onerror = null
             e.target.src = PLACEHOLDER_IMAGE
           }}
         />
        <button 
          onClick={() => navigate(-1)}
          className="absolute top-4 left-4 bg-black/30 text-white p-2 rounded-full"
        >
          <ArrowLeft size={20} />
        </button>
      </div>

      <div className="bg-white p-4 pb-24">
        <h1 className="text-xl font-bold text-gray-800">{instrument.name || instrument.category_name}</h1>
        {instrument.sn && <p className="text-sm text-gray-400 mt-1">编号: {instrument.sn}</p>}
        <p className="text-gray-500 mt-1">{instrument.description}</p>

        {/* Site/Merchant Info */}
        {(instrument.tenant_name || instrument.site_name) && (
          <div className="mt-3 p-3 bg-gray-50 rounded-lg">
            {instrument.tenant_name && (
              <p className="text-sm text-gray-600">
                <span className="font-medium">商户:</span> {instrument.tenant_name}
              </p>
            )}
            {instrument.site_name && (
              <p className="text-sm text-gray-600 mt-1">
                <span className="font-medium">网点:</span> {instrument.site_name}
                {instrument.site_address && <span className="text-gray-400"> ({instrument.site_address})</span>}
              </p>
            )}
          </div>
        )}

        <div className="mt-4">
          <span className="text-gray-700 font-medium">租赁周期</span>
          
          {/* Row 1: Cycle selector buttons */}
          <div className="flex gap-2 mt-2">
            {CYCLE_OPTIONS.map(opt => (
              <button
                key={opt.value}
                onClick={() => setCycle(opt.value)}
                className={`flex-1 py-2 rounded-lg border text-center transition-all ${
                  cycle === opt.value
                    ? 'border-brand-primary bg-brand-primary/10 text-brand-primary'
                    : 'border-gray-200 text-gray-600'
                }`}
              >
                {opt.label}
              </button>
            ))}
          </div>
          
          {/* Row 2: Number input + cycle text + rent per cycle */}
          <div className="mt-3 flex items-center gap-2">
            <InputNumber
              min={1}
              max={365}
              value={termCount}
              onChange={(val) => setTermCount(val || 1)}
              className="w-20"
            />
            <span className="text-gray-600">
              {cycle === 'day' ? '天' : cycle === 'week' ? '周' : '月'}
            </span>
            <span className="text-gray-500">×</span>
            <span className="text-brand-primary font-bold">¥{rentPerCycle.toFixed(0)}</span>
            <span className="text-gray-500">=</span>
            <span className="text-brand-primary font-bold">¥{calculatedRent.toFixed(0)}</span>
          </div>
          
          {/* Row 3: Expiry date */}
          <div className="mt-2 text-sm text-gray-500 flex items-center gap-1">
            <Calendar size={14} />
            截止日期: <span className="font-medium">{getExpiryDate()}</span>
          </div>
        </div>

        <div className="mt-4 p-3 bg-blue-50 rounded-lg border border-blue-100">
          <p className="font-medium text-sm text-blue-800 mb-1">费用明细</p>
          <div className="space-y-1 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-600">租金 ({termCount}{cycle === 'day' ? '天' : cycle === 'week' ? '周' : '月'} × ¥{rentPerCycle.toFixed(0)})</span>
              <span className="font-medium">¥{calculatedRent.toFixed(0)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">押金</span>
              <span className="font-medium">¥{deposit}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-600">物流费</span>
              <span className="font-medium">¥{shippingFee}</span>
            </div>
            <div className="border-t border-blue-200 mt-2 pt-2 flex justify-between font-bold">
              <span className="text-blue-900">合计</span>
              <span className="text-blue-600">¥{totalAmount.toFixed(0)}</span>
            </div>
          </div>
        </div>

        <div className="mt-3 p-2 bg-orange-50 rounded-lg text-sm">
          <p className="text-orange-800">
            ⚠️ 逾期后将每日自动扣款，按 ¥{overdueDailyFee}/日 计算
          </p>
          <p className="text-orange-700 mt-1">
            💰 押金将在乐器归还、质检通过后原路退还。如乐器损坏，将在定损后从押金中抵扣
          </p>
        </div>

        <div className="mt-3 p-3 bg-purple-50 rounded-lg mb-24">
          <p className="font-medium text-sm text-purple-800 flex items-center gap-1">
            <span>🎁</span>
            <span className="font-bold">租购转化</span>
          </p>
          <p className="text-purple-600 text-sm mt-0.5 font-bold">
            租满12个月可直接获得所有权
          </p>
        </div>
      </div>

      {/* Floating Cart Icon */}
      {(cartItemCount > 0) && (
        <button
          onClick={() => navigate('/cart')}
          className="fixed bottom-24 right-4 bg-brand-primary text-white p-3 rounded-full shadow-lg z-50"
        >
          <ShoppingCart size={24} />
          <span className="absolute -top-1 -right-1 bg-red-500 text-white text-xs w-5 h-5 rounded-full flex items-center justify-center font-bold">
            {cartItemCount}
          </span>
        </button>
      )}

      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <div className="mb-2 p-3 bg-green-50 rounded-lg border border-green-200">
          <p className="text-center font-bold text-lg text-brand-primary">
            合计：¥{totalAmount.toFixed(0)}
          </p>
        </div>
        <div className="flex gap-2">
          {isRentable && (
            <button 
              onClick={handleAddToCart}
              className="flex-1 py-3 rounded-lg font-medium flex items-center justify-center gap-1 bg-orange-100 text-orange-600"
            >
              <ShoppingCart size={18} />
              加入购物车
            </button>
          )}
          <button 
            onClick={handleCreateOrder}
            className={`py-3 rounded-lg font-medium bg-orange-500 text-white ${isRentable ? 'flex-1' : 'w-full'}`}
          >
            立即租赁
          </button>
        </div>
      </div>

      {cartToast && (
        <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center" onClick={() => setCartToast(false)}>
          <div className="bg-white rounded-xl p-6 mx-8 text-center" onClick={e => e.stopPropagation()}>
            <CheckCircle size={48} className="text-green-500 mx-auto mb-3" />
            <p className="text-lg font-bold mb-1">加入成功</p>
            <p className="text-gray-500 text-sm mb-4">该乐器已添加到购物车</p>
            <div className="flex gap-3">
              <button 
                onClick={() => { setCartToast(false); navigate('/') }}
                className="flex-1 py-3 px-6 border rounded-lg text-gray-600 min-w-[100px]"
              >
                继续浏览
              </button>
              <button 
                onClick={() => { setCartToast(false); navigate('/cart') }}
                className="flex-1 py-3 px-6 bg-brand-primary text-white rounded-lg min-w-[100px]"
              >
                提交订单
              </button>
            </div>
          </div>
        </div>
      )}

      <Modal
        title="📊 服务权益对比"
        open={showComparison}
        onCancel={() => setShowComparison(false)}
        footer={null}
        width={600}
      >
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-gray-100">
                <th className="p-2 text-left font-medium">权益项</th>
                <th className="p-2 text-center font-medium">入门级</th>
                <th className="p-2 text-center font-medium">专业级</th>
                <th className="p-2 text-center font-medium text-purple-600">大师级</th>
              </tr>
            </thead>
            <tbody>
              {SERVICE_ITEMS.map((item, idx) => (
                <tr key={idx} className="border-b">
                  <td className="p-2">{item.name}</td>
                  <td className={`p-2 text-center ${item.entry === '✓' ? 'text-green-600' : 'text-gray-400'}`}>
                    {item.entry}
                  </td>
                  <td className={`p-2 text-center ${item.professional === '✓' ? 'text-green-600' : 'text-gray-400'}`}>
                    {item.professional}
                  </td>
                  <td className={`p-2 text-center font-medium ${item.master === '✓' ? 'text-purple-600' : 'text-gray-400'}`}>
                    {item.master}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="mt-4 flex justify-end">
          <Button onClick={() => setShowComparison(false)}>关闭</Button>
        </div>
      </Modal>
    </div>
  )
}
