import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api, instrumentsApi } from '../services/api'
import { ArrowLeft, MapPin } from 'lucide-react'

export default function Checkout() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [addresses, setAddresses] = useState([])
  const [depositRules, setDepositRules] = useState([])
  const [loading, setLoading] = useState(true)
  
  const [rentMonths, setRentMonths] = useState(3)
  const [selectedAddress, setSelectedAddress] = useState(null)

  useEffect(() => {
    const fetchCheckoutData = async () => {
      try {
        setLoading(true)
        const instrumentData = await instrumentsApi.get(id)
        setInstrument(instrumentData)
        
        const addrRes = await api.get('/user/addresses')
        setAddresses(addrRes || [])
        if (addrRes && addrRes.length > 0) {
          setSelectedAddress(addrRes[0])
        }
        
        const rulesRes = await api.get('/config/deposit-rules')
        setDepositRules(rulesRes || [])
        
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch checkout data:', error)
        setLoading(false)
      }
    }
    
    fetchCheckoutData()
  }, [id])

  if (loading) {
    return <div className="p-4">加载中...</div>
  }

  if (!instrument) {
    return <div className="p-4">乐器不存在</div>
  }

  const rentOptions = [3, 6, 12]
  const defaultLevel = instrument.levels[0]
  
  const calculatePayment = (months) => {
    const monthlyRent = defaultLevel?.monthlyRent || 0
    const deposit = defaultLevel?.deposit || 0
    const baseRent = monthlyRent * (months || 3)
    const discount = (months || 3) >= 12 ? 0.95 : 1
    const finalRent = Math.round(baseRent * discount)
    return {
      rent: finalRent,
      deposit: deposit,
      total: finalRent + deposit
    }
  }

  const payment = calculatePayment(rentMonths)

  return (
    <div className="min-h-screen bg-gray-50 pb-20">
      {/* Header */}
      <div className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">确认订单</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Instrument Info */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">租赁乐器</h2>
          <div className="flex gap-3">
            <img 
              src={instrument.image} 
              alt={instrument.name}
              className="w-20 h-20 object-cover rounded"
            />
            <div>
              <p className="font-medium">{instrument.name}</p>
              <p className="text-orange-500 text-sm">¥{(defaultLevel?.monthlyRent || 0)}/月</p>
            </div>
          </div>
        </div>

        {/* Rent Period Selector */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">租期选择</h2>
          <div className="flex gap-2">
            {rentOptions.map(months => (
              <button
                key={months}
                onClick={() => setRentMonths(months)}
                className={`flex-1 py-2.5 rounded-lg text-sm font-medium border-2 transition-colors ${
                  rentMonths === months
                    ? 'border-orange-500 bg-orange-50 text-orange-600'
                    : 'border-gray-200 text-gray-600'
                }`}
              >
                {months}个月
                {months >= 12 && <span className="block text-xs">享95折</span>}
              </button>
            ))}
          </div>
        </div>

        {/* Price Calculator */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">费用计算</h2>
          <div className="space-y-2">
            <div className="flex justify-between text-sm">
              <span className="text-gray-500">租金 ({rentMonths}个月)</span>
              <span>¥{payment.rent}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-gray-500">押金</span>
              <span>¥{payment.deposit}</span>
            </div>
            <div className="flex justify-between font-bold text-lg pt-2 border-t">
              <span>首期预付金额</span>
              <span className="text-orange-500">¥{payment.total}</span>
            </div>
          </div>
        </div>

        {/* Address Selector */}
        <div className="bg-white rounded-lg p-4">
          <h2 className="font-medium text-gray-800 mb-3">配送地址</h2>
          <select
            value={selectedAddress.id}
            onChange={(e) => setSelectedAddress(addresses.find(a => a.id === parseInt(e.target.value)))}
            className="w-full p-3 border rounded-lg mb-2"
          >
            {addresses.map(addr => (
              <option key={addr.id} value={addr.id}>
                {addr.name} - {addr.detail}
              </option>
            ))}
          </select>
          <div className="flex items-center gap-2 text-gray-500 text-sm">
            <MapPin size={16} />
            <span>{selectedAddress.detail}</span>
          </div>
        </div>

        {/* Rental Agreement */}
        <div className="bg-orange-50 border border-orange-200 rounded-lg p-4">
          <h3 className="font-bold text-orange-800 mb-2">📋 租用协议</h3>
          <div className="text-sm text-orange-700 space-y-1">
            <p className="font-medium">押金扣除规则:</p>
            <ul className="list-disc pl-4">
              {depositRules.map((rule, index) => (
                <li key={index}>{rule.condition}: {rule.penalty}</li>
              ))}
            </ul>
            <p className="mt-2 text-xs text-gray-500">
              正常使用磨损不计入赔偿
            </p>
          </div>
        </div>
      </div>

      {/* Bottom Action */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <button 
          onClick={() => navigate('/success')}
          className="w-full bg-orange-500 text-white py-3 rounded-lg font-medium"
        >
          确认支付 ¥{payment.total}
        </button>
      </div>
    </div>
  )
}