import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { instrumentsApi, ordersApi } from '../services/api'
import { ArrowLeft, Shield, Clock, AlertCircle, MapPin, Bell, CheckCircle, X } from 'lucide-react'
import { Switch, Segmented, Tag, Modal, Button } from 'antd'

const TERM_OPTIONS = [
  { label: '3个月', value: 3, discount: 1.0 },
  { label: '6个月', value: 6, discount: 0.98 },
  { label: '12个月', value: 12, discount: 0.95 },
]

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

export default function Detail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  
  const [selectedLevel, setSelectedLevel] = useState('专业级')
  const [selectedTerm, setSelectedTerm] = useState(12)
  const [noDeposit, setNoDeposit] = useState(false)
  const [showComparison, setShowComparison] = useState(false)
  const [userCreditScore] = useState(750)
  const [canUseDepositFree, setCanUseDepositFree] = useState(false)
  
  const [calculatedRent, setCalculatedRent] = useState(0)
  const [calculatedDeposit, setCalculatedDeposit] = useState(0)
  const [depositWaived, setDepositWaived] = useState(0)
  const [totalAmount, setTotalAmount] = useState(0)

  useEffect(() => {
    const fetchInstrument = async () => {
      try {
        setLoading(true)
        const data = await instrumentsApi.get(id)
        setInstrument(data)
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

  const currentLevel = instrument?.levels?.find(l => {
    return l.name === selectedLevel
  })

  const calculatePrice = useCallback(async () => {
    if (!currentLevel || !instrument) return

    const termOption = TERM_OPTIONS.find(t => t.value === selectedTerm)
    const discount = termOption?.discount || 1.0
    
    let rent = currentLevel.monthlyRent * discount
    const baseDeposit = currentLevel.deposit
    
    let deposit = noDeposit && canUseDepositFree ? 0 : baseDeposit
    let waived = noDeposit && canUseDepositFree ? baseDeposit : 0

    setCalculatedRent(rent)
    setCalculatedDeposit(deposit)
    setDepositWaived(waived)
    setTotalAmount(rent + deposit)
  }, [currentLevel, instrument, selectedTerm, noDeposit, canUseDepositFree])

  useEffect(() => {
    calculatePrice()
  }, [calculatePrice])

  const handleDepositToggle = () => {
    if (!canUseDepositFree) {
      return
    }
    setNoDeposit(!noDeposit)
  }

  const handleCreateOrder = async () => {
    const levelMap = { '入门级': 'entry', '专业级': 'professional', '大师级': 'master' }
    
    try {
      const result = await ordersApi.preview({
        instrument_id: instrument.id,
        level: levelMap[selectedLevel],
        lease_term: selectedTerm,
        deposit_mode: noDeposit ? 'free' : 'standard',
      })
      
      navigate(`/checkout/${instrument.id}`, { 
        state: { 
          pricing: result,
          level: selectedLevel,
          term: selectedTerm,
          instrument 
        } 
      })
    } catch (error) {
      console.error('Preview failed:', error)
      navigate(`/checkout/${instrument.id}`, {
        state: {
          pricing: {
            first_month_rent: calculatedRent,
            deposit: calculatedDeposit,
            total_amount: totalAmount,
            discount_info: selectedTerm === 12 ? '95折优惠' : '',
          },
          level: selectedLevel,
          term: selectedTerm,
          instrument
        }
      })
    }
  }

  if (loading) {
    return <div className="p-4">加载中...</div>
  }

  if (!instrument) {
    return <div className="p-4">乐器不存在</div>
  }

  return (
    <div className="min-h-screen bg-gray-50 pb-24">
      <div className="relative">
         <img 
           src={instrument.images?.[0] || PLACEHOLDER_IMAGE} 
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

      <div className="bg-white p-4">
        <h1 className="text-xl font-bold text-gray-800">{instrument.name}</h1>
        <p className="text-gray-500 mt-1">{instrument.description}</p>
        
        <div className="mt-4">
          <div className="flex justify-between items-center">
            <span className="text-gray-700 font-medium">选择级别</span>
            <button 
              onClick={() => setShowComparison(true)}
              className="text-brand-primary text-sm flex items-center gap-1"
            >
              📊 查看服务权益对比
            </button>
          </div>
          <Segmented
            options={[
              { 
                label: (
                  <div className="text-center py-2 px-3">
                    <div className="font-medium">入门级</div>
                    <div className="text-xs text-gray-500">
                      ¥{instrument?.levels?.[0]?.monthlyRent || 0}/月
                    </div>
                  </div>
                ), 
                value: '入门级' 
              },
              { 
                label: (
                  <div className="text-center py-2 px-3">
                    <div className="font-medium">专业级</div>
                    <div className="text-xs text-gray-500">
                      ¥{instrument?.levels?.[1]?.monthlyRent || 0}/月
                    </div>
                  </div>
                ), 
                value: '专业级' 
              },
              { 
                label: (
                  <div className="text-center py-2 px-3">
                    <div className="font-medium">大师级</div>
                    <div className="text-xs text-gray-500">
                      ¥{instrument?.levels?.[2]?.monthlyRent || 0}/月
                    </div>
                  </div>
                ), 
                value: '大师级' 
              }
            ]}
            value={selectedLevel}
            onChange={setSelectedLevel}
            className="w-full"
          />
        </div>

        <div className="mt-4">
          <span className="text-gray-700 font-medium">选择租期</span>
          <div className="flex gap-2 mt-2">
            {TERM_OPTIONS.map(term => (
              <button
                key={term.value}
                onClick={() => setSelectedTerm(term.value)}
                className={`flex-1 py-2 px-3 rounded-lg border text-center transition-all ${
                  selectedTerm === term.value 
                    ? 'border-brand-primary bg-brand-primary/10 text-brand-primary' 
                    : 'border-gray-200 text-gray-600'
                }`}
              >
                <div className="font-medium">{term.label}</div>
                {term.discount < 1 && (
                  <div className="text-xs text-brand-primary mt-0.5">
                    {Math.round(term.discount * 100)}折
                  </div>
                )}
              </button>
            ))}
          </div>
        </div>

        <div className="mt-4 p-3 bg-gray-50 rounded-lg">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-gray-700">信用免押金</span>
              <span className="text-xs text-gray-500 ml-2">(信用分: {userCreditScore})</span>
            </div>
            <Switch 
              size="small" 
              checked={noDeposit} 
              onChange={handleDepositToggle}
              disabled={!canUseDepositFree}
            />
          </div>
          {!canUseDepositFree && (
            <p className="text-xs text-orange-500 mt-1">信用分不足650，无法使用免押金</p>
          )}
          {noDeposit && depositWaived > 0 && (
            <p className="text-xs text-green-600 mt-1">
              已免除押金 ¥{depositWaived}
            </p>
          )}
        </div>

        <div className="mt-4 p-3 bg-orange-50 rounded-lg border border-orange-100">
          <p className="font-medium text-sm text-orange-800 mb-2">费用明细</p>
          <div className="space-y-1">
            <div className="flex justify-between text-sm">
              <span className="text-gray-600">首月租金 {selectedTerm === 12 ? '(95折)' : selectedTerm === 6 ? '(98折)' : ''}</span>
              <span className="font-medium">¥{calculatedRent.toFixed(0)}</span>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-gray-600">
                押金 {noDeposit && '(已免除)'}
              </span>
              <span className={`font-medium ${noDeposit ? 'line-through text-gray-400' : ''}`}>
                ¥{(calculatedDeposit + depositWaived).toFixed(0)}
              </span>
            </div>
            {depositWaived > 0 && (
              <div className="flex justify-between text-sm text-green-600">
                <span>押金减免</span>
                <span>-¥{depositWaived.toFixed(0)}</span>
              </div>
            )}
            <div className="border-t border-orange-200 mt-2 pt-2 flex justify-between font-bold">
              <span className="text-orange-900">合计</span>
              <span className="text-orange-600">¥{totalAmount.toFixed(0)}</span>
            </div>
          </div>
        </div>

        <div className="mt-4 p-3 bg-green-50 rounded-lg">
          <p className="font-medium text-sm text-green-800 mb-2">服务内容</p>
          <div className="flex flex-wrap gap-2">
            {currentLevel?.maintenance.map((item, idx) => (
              <Tag key={idx} color="green" icon={<CheckCircle size={12} />}>{item}</Tag>
            ))}
          </div>
        </div>

        <div className="mt-3 p-3 bg-purple-50 rounded-lg">
          <p className="font-medium text-sm text-purple-800 flex items-center gap-1">
            <span>🎁</span>
            <span className="font-bold">租购转化</span>
          </p>
          <p className="text-purple-600 text-sm mt-0.5 font-bold">
            租满12个月可直接获得所有权
          </p>
        </div>

        <div className="mt-6 space-y-3">
          <div className="flex items-start gap-3 p-3 bg-gray-50 rounded-lg">
            <div className="flex-1">
              <p className="font-medium text-sm text-gray-800">资产信息</p>
              <p className="text-gray-500 text-sm mt-1">SN: {instrument.sn}</p>
              <p className="text-gray-500 text-sm">归属: {instrument.site}</p>
            </div>
            <div className="flex items-center gap-1 text-brand-primary">
              <MapPin size={16} />
              <span className="text-sm">{instrument.distance} km</span>
            </div>
          </div>

          <div className="flex items-start gap-3 p-3 bg-blue-50 rounded-lg">
            <Shield className="text-blue-500 flex-shrink-0" size={20} />
            <div>
              <p className="font-medium text-sm text-blue-800">押金说明</p>
              <p className="text-blue-600 text-sm mt-0.5">{instrument.depositNote}</p>
            </div>
          </div>
        </div>
      </div>

      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <div className="mb-2 p-3 bg-green-50 rounded-lg border border-green-200">
          <p className="text-center font-bold text-lg text-brand-primary">
            首期预付：¥{totalAmount.toFixed(0)} 
            <span className="text-sm font-normal text-gray-500">
              {noDeposit ? '(押金已免除)' : '(押金可退)'}
            </span>
          </p>
        </div>
        <button 
          onClick={handleCreateOrder}
          className="w-full bg-orange-500 text-white py-3 rounded-lg font-medium"
        >
          立即租赁
        </button>
      </div>

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
