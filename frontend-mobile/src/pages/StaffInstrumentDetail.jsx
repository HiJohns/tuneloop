import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { View, Text, Button, ScrollView, Input, Textarea, Image } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, Truck, Wrench, RotateCcw, CheckCircle, User, Archive, Clock } from 'lucide-react'
import { dialog, env, storage } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'

const eventLabels = {
  'pending_shipment → shipped': '寄出乐器',
  'shipped → in_lease': '租赁开始',
  'in_lease → returning': '申请归还',
  'returning → returned': '收到归还',
  'returned → assessed': '定损',
  'assessed → maintenance': '维修中',
  'maintenance → repaired': '完成维修',
  'returned → completed': '订单完成',
  ' → paid': '已支付',
  ' → pending_shipment': '待发货',
  ' → cancelled': '已取消',
}

function formatActivityTime(timeStr) {
  if (!timeStr) return ''
  const d = new Date(timeStr)
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

function parsePricing(pricing) {
  if (!pricing) return []
  if (Array.isArray(pricing)) return pricing
  if (typeof pricing === 'string') {
    try { return JSON.parse(pricing) } catch { return [] }
  }
  return []
}

export default function StaffInstrumentDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [activeOrder, setActiveOrder] = useState(null)
  const [sessions, setSessions] = useState([])

  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchInstrument = async () => {
      try {
        setLoading(true)
        const resp = await apiFetch(`${baseUrl}/instruments/${id}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setInstrument(result.data)
          const inst = result.data
          if (inst.sn && inst.stock_status === 'rented') {
            try {
              const orderResp = await fetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(inst.sn)}`)
              const orderResult = await orderResp.json()
              if (orderResult.code === 20000 && orderResult.data) {
                setActiveOrder(orderResult.data)
              }
            } catch {}
          }
        }
      } catch (err) {
        console.error('Failed to fetch instrument:', err)
      }
      setLoading(false)
    }
    fetchInstrument()
  }, [id])

  useEffect(() => {
    const fetchActivityLog = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/instruments/${id}/activity-log`)
        const result = await resp.json()
        if (result.code === 20000 && result.data?.sessions) {
          setSessions(result.data.sessions)
        }
      } catch (err) {
        console.error('Failed to fetch activity log:', err)
      }
    }
    if (id) fetchActivityLog()
  }, [id])

  const statusColor = {
    available: 'bg-green-100 text-green-700',
    rented: 'bg-indigo-100 text-indigo-700',
    maintenance: 'bg-orange-100 text-orange-700',
    archived: 'bg-gray-100 text-gray-700',
    lost: 'bg-gray-100 text-gray-700',
  }

  const statusLabel = {
    available: '可租',
    rented: '租赁中',
    maintenance: '维修中',
    archived: '已下架',
    lost: '已丢失',
  }

  const handleShip = async () => {
    if (instrument.stock_status !== 'rented') {
      dialog.alert('乐器不在租赁中状态，无法发货')
      return
    }
    navigate(`/staff/shipping?instrument=${instrument.id}`)
  }

  const handleReceive = async () => {
    if (instrument.stock_status !== 'rented') {
      dialog.alert('乐器不在租赁状态')
      return
    }
    if (activeOrder) {
      navigate(`/staff/receiving/${activeOrder.order_id}?instrument=${instrument.id}`)
    } else {
      dialog.alert('未找到关联订单')
    }
  }

  const handleCompleteMaintenance = async () => {
    if (instrument.stock_status !== 'maintenance') {
      dialog.alert('乐器不在维修中状态')
      return
    }
    try {
      setActionLoading(true)
      const maintResp = await apiFetch(`${baseUrl}/instruments/${id}/status`, {
        method: 'PUT',
        body: JSON.stringify({ stock_status: 'available' }),
      })
      const maintResult = await maintResp.json()
      if (maintResult.code === 20000) {
        dialog.alert('维修完成')
        navigate('/staff/instruments')
      } else {
        dialog.alert('操作失败: ' + maintResult.message)
      }
    } catch (err) {
      dialog.alert('操作失败: ' + err.message)
    }
    setActionLoading(false)
  }

  const handleArchive = async () => {
    try {
      setActionLoading(true)
      const resp = await apiFetch(`${baseUrl}/instruments/${id}/status`, {
        method: 'PUT',
        body: JSON.stringify({ stock_status: 'archived' }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('已下架')
        navigate('/staff/instruments')
      } else {
        dialog.alert('操作失败: ' + result.message)
      }
    } catch (err) {
      dialog.alert('操作失败: ' + err.message)
    }
    setActionLoading(false)
  }

  if (loading) {
    return <View className="p-4">加载中...</View>
  }

  if (!instrument) {
    return <View className="p-4">乐器不存在</View>
  }

  const pricing = parsePricing(instrument.pricing)
  const pricingInfo = pricing[0] || {}

  return (
    <View className="min-h-screen bg-brand-bg pb-24">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </Button>
        <Text className="text-lg font-bold">乐器详情</Text>
      </View>

      <View className="p-4 space-y-4">
        {/* Image */}
        <InstrumentInfo instrument={instrument} />

        {/* Basic Info */}
        <View className="bg-white rounded-xl p-4">
          <View className="flex items-center justify-between mb-3">
            <Text className="text-lg font-bold">{instrument.name}</Text>
            <Text className={`px-3 py-1 rounded-full text-sm font-medium ${statusColor[instrument.stock_status] || 'bg-gray-100'}`}>
              {statusLabel[instrument.stock_status] || instrument.stock_status}
            </Text>
          </View>
          <View className="space-y-2 text-sm">
            <View className="flex justify-between">
              <Text className="text-gray-500">SN</Text>
              <Text className="font-mono">{instrument.sn || '-'}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">分类</Text>
              <Text>{instrument.category_name || '-'}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">分级</Text>
              <Text>{instrument.level_name || instrument.level || '-'}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">网点</Text>
              <Text>{instrument.site_name || '-'}</Text>
            </View>
            {instrument.properties && Object.keys(instrument.properties).length > 0 && (
              <View className="pt-2 border-t">
                <Text className="text-gray-500 text-xs block mb-1">动态属性</Text>
                {Object.entries(instrument.properties).map(([key, vals]) => (
                  <View key={key} className="flex justify-between text-xs mt-1">
                    <Text className="text-gray-400">{key}</Text>
                    <Text>{(Array.isArray(vals) ? vals : [vals]).join(', ')}</Text>
                  </View>
                ))}
              </View>
            )}
          </View>
        </View>

        {/* Lease Info - Only for reserved/returning */}
        {activeOrder && (
          <LeaseInfo
            status={activeOrder.status}
            startDate={activeOrder.start_date ? formatDisplayDate(activeOrder.start_date) : '-'}
            endDate={activeOrder.end_date ? formatDisplayDate(activeOrder.end_date) : '-'}
            dailyRate={activeOrder.pricing_breakdown?.final_daily_rent || activeOrder.pricing_breakdown?.base_daily_rent || 0}
            rentDays={activeOrder.pricing_breakdown?.rent_days || 0}
            actualDays={activeOrder.start_date && activeOrder.returned_at ? Math.max(1, Math.round(((new Date(activeOrder.returned_at) - new Date(activeOrder.start_date)) / 86400000))) : activeOrder.start_date && activeOrder.end_date ? Math.max(1, Math.round(((new Date(activeOrder.end_date) - new Date(activeOrder.start_date)) / 86400000))) : 0}
            createdAt={activeOrder.created_at ? formatDisplayDate(activeOrder.created_at) : '-'}
          />
        )}

        {/* Pricing Info */}
        <View className="bg-white rounded-xl p-4">
          <Text className="font-medium mb-3">租赁设置</Text>
          <View className="space-y-2 text-sm">
            <View className="flex justify-between">
              <Text className="text-gray-500">日租金</Text>
              <Text>¥{pricingInfo.daily_rent || instrument.base_daily_rate || 0}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">押金</Text>
              <Text>¥{pricingInfo.deposit || 0}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">物流费</Text>
              <Text>¥{pricingInfo.shipping_fee || 0}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-gray-500">逾期日费</Text>
              <Text>¥{pricingInfo.overdue_daily_fee || pricingInfo.daily_rent || 0}</Text>
            </View>
          </View>
        </View>

        {/* Activity Log Timeline */}
        {sessions.length > 0 && (
          <View className="bg-white rounded-xl p-4">
            <View className="flex items-center gap-2 mb-4">
              <Clock size={18} className="text-gray-500" />
              <Text className="font-medium">操作记录</Text>
            </View>
            {sessions.map((session) => (
              <View key={session.order_id}>
                {session.events.map((event, ei) => {
                  const label = eventLabels[event.event] || event.event
                  return (
                    <View key={ei} className="relative pl-6 pb-4 border-l-2 border-gray-200 last:border-transparent">
                      <View className="absolute left-[-5px] top-1 w-2 h-2 rounded-full bg-brand-primary" />
                      <View className="text-sm">
                        <View className="flex items-center gap-2">
                          <Text className="font-medium">{label}</Text>
                          <Text className="text-gray-400 text-xs">{formatActivityTime(event.time)}</Text>
                        </View>
                        {event.operator && (
                          <Text className="text-gray-400 text-xs">{event.operator}</Text>
                        )}
                        {event.media?.length > 0 && (
                          <View className="flex gap-2 mt-2 flex-wrap">
                            {event.media.filter(m => m.file_type === 'image').map((m, mi) => (
                              <Image
                                key={mi}
                                src={m.url}
                                className="w-16 h-16 rounded object-cover"
                                mode="aspectFill"
                              />
                            ))}
                            {event.media.filter(m => m.file_type === 'video').map((m, mi) => (
                              <View key={mi} className="relative">
                                <Image
                                  src={m.url}
                                  className="w-16 h-16 rounded object-cover"
                                  mode="aspectFill"
                                />
                                <View className="absolute inset-0 flex items-center justify-center bg-black/30 rounded">
                                  <Text className="text-white text-xs">▶</Text>
                                </View>
                              </View>
                            ))}
                          </View>
                        )}
                      </View>
                    </View>
                  )
                })}
              </View>
            ))}
          </View>
        )}

        {/* Booker Info Card - Only show for reserved status */}
        {instrument.stock_status === 'rented' && (instrument.booker_name || instrument.booker_phone) && (
          <View className="mb-4 p-4 bg-yellow-50 border border-yellow-200 rounded-lg">
            <View className="flex items-center gap-2 mb-3">
              <User size={18} className="text-yellow-600" />
              <Text className="font-medium text-yellow-800">预约人信息</Text>
            </View>
            {instrument.booker_name && (
              <View className="mb-2 text-sm">
                <Text className="text-gray-500">姓名：</Text>
                <Text className="text-gray-800">{instrument.booker_name}</Text>
              </View>
            )}
            {instrument.booker_phone && (
              <View className="mb-2 text-sm">
                <Text className="text-gray-500">电话：</Text>
                <Text className="text-gray-800">{instrument.booker_phone}</Text>
              </View>
            )}
            {instrument.booker_email && (
              <View className="mb-2 text-sm">
                <Text className="text-gray-500">邮箱：</Text>
                <Text className="text-gray-800">{instrument.booker_email}</Text>
              </View>
            )}
            {instrument.delivery_address && (
              <View className="text-sm">
                <Text className="text-gray-500">收货地址：</Text>
                <Text className="text-gray-800">{formatDeliveryAddress(instrument.delivery_address)}</Text>
              </View>
            )}
          </View>
        )}
      </View>

      {/* Action Buttons */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        {(() => {
          const mapping = storage.getJSON('permission_mapping', {})
          const cusPerm = parseInt(storage.getItem('user_cus_perm') || '0')
          const has = (code) => { const b = mapping[code]; return b !== undefined && (cusPerm & (1 << b)) !== 0 }
          return (
            <View className="grid grid-cols-3 gap-3">
              {instrument.stock_status === 'available' && has('instrument:edit') && (
                <Button onClick={handleArchive} disabled={actionLoading} className="py-3 bg-gray-600 text-white rounded-lg font-medium flex items-center justify-center gap-2">
                  <Archive size={18} />下架
                </Button>
              )}
              {instrument.stock_status === 'rented' && has('order:update') && (
                <Button onClick={handleShip} className="py-3 bg-blue-500 text-white rounded-lg font-medium flex items-center justify-center gap-2">
                  <Truck size={18} />发货
                </Button>
              )}
              {instrument.stock_status === 'rented' && has('inventory:manage') && (
                <Button onClick={handleReceive} disabled={actionLoading || !activeOrder} className="py-3 bg-green-600 text-white rounded-lg font-medium flex items-center justify-center gap-2">
                  <RotateCcw size={18} />接收确认
                </Button>
              )}
              {instrument.stock_status === 'maintenance' && (
                <>
                  {instrument.repair_status === 'repair_pending' && (
                    <Button onClick={() => navigate(`/repair?instrument_id=${id}`)} className="py-3 bg-purple-500 text-white rounded-lg font-medium flex items-center justify-center gap-2">
                      <CheckCircle size={18} />开始维修
                    </Button>
                  )}
                  {instrument.repair_status === 'repair_in_progress' && (
                    <Button onClick={() => navigate(`/repair?instrument_id=${id}`)} className="py-3 bg-purple-500 text-white rounded-lg font-medium flex items-center justify-center gap-2">
                      <CheckCircle size={18} />维修完成
                    </Button>
                  )}
                  {instrument.repair_status === 'repair_completed' && (
                    <Button onClick={() => navigate(`/repair?instrument_id=${id}`)} className="py-3 bg-green-600 text-white rounded-lg font-medium flex items-center justify-center gap-2">
                      <CheckCircle size={18} />验收
                    </Button>
                  )}
                </>
              )}
            </View>
          )
        })()}
      </View>
    </View>
  )
}
