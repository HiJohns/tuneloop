import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, User, MapPin, Calendar, Clock, Package, Truck, RotateCcw, CheckCircle, XCircle, AlertTriangle } from 'lucide-react'

const STATUS_LABELS = {
  reserved: '未支付',
  paid: '待发货',
  pending_shipment: '待发货',
  in_transit: '运输中',
  shipped: '已发货',
  in_lease: '租赁中',
  returning: '归还中',
  returned: '已归还',
  completed: '已完成',
  cancelled: '已取消',
  expired: '超期',
  transferred: '已过户',
}

const STATUS_COLORS = {
  reserved: 'bg-blue-100 text-blue-700',
  paid: 'bg-orange-100 text-orange-700',
  pending_shipment: 'bg-orange-100 text-orange-700',
  in_transit: 'bg-cyan-100 text-cyan-700',
  shipped: 'bg-green-100 text-green-700',
  in_lease: 'bg-indigo-100 text-indigo-700',
  returning: 'bg-yellow-100 text-yellow-700',
  returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600',
  cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700',
  transferred: 'bg-purple-100 text-purple-700',
}

export default function StaffOrderDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchOrder = async () => {
      setLoading(true)
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${id}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrder(result.data)
          const o = result.data
          if (o.instrument_id) {
            try {
              const iresp = await apiFetch(`${baseUrl}/public/instruments/${o.instrument_id}`)
              const iresult = await iresp.json()
              if (iresult.code === 20000) {
                setInstrument(iresult.data)
              }
            } catch {}
          }
        }
      } catch (err) {
        console.error('Failed to fetch order:', err)
      }
      setLoading(false)
    }
    fetchOrder()
  }, [id])

  if (loading) {
    return (
      <View className="min-h-screen bg-brand-bg pb-20">
        <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
          <Text className="text-lg font-bold">订单详情</Text>
        </View>
        <View className="text-center text-gray-500 py-12">加载中...</View>
      </View>
    )
  }

  if (!order) {
    return (
      <View className="min-h-screen bg-brand-bg pb-20">
        <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
          <Text className="text-lg font-bold">订单详情</Text>
        </View>
        <View className="text-center text-gray-400 py-12">
          <Package size={48} className="mx-auto mb-3 opacity-50" />
          <Text>订单未找到</Text>
        </View>
      </View>
    )
  }

  const status = order.status || ''
  const statusLabel = STATUS_LABELS[status] || status
  const statusColor = STATUS_COLORS[status] || 'bg-gray-100'

  const startDate = formatDisplayDate(order.start_date)
  const endDate = formatDisplayDate(order.end_date)
  const leaseTerm = order.lease_term || 0
  const rentalDays = leaseTerm * 30

  const showShipButton = status === 'paid' || status === 'pending_shipment'
  const showTransitButton = status === 'in_transit'
  const showReceiveButton = status === 'returning'

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <View onClick={() => navigate(-1)}>
          <ArrowLeft size={20} className="text-black" />
        </View>
        <Text className="text-lg font-black text-black">订单详情</Text>
      </View>

      {/* Order ID Banner */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-xs text-zinc-400 font-medium mb-1">订单编号</Text>
        <Text className="text-xs font-black text-black tracking-wide truncate">{id}</Text>
      </View>

      {/* Status */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <View className="flex items-center justify-between min-w-0">
          <Text className="text-sm font-bold text-zinc-500">当前状态</Text>
          <Text className={`text-xs px-3 py-1 rounded-full font-black flex-shrink-0 ${statusColor}`}>
            {statusLabel}
          </Text>
        </View>
      </View>

      {/* Instrument Info */}
      {instrument && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm pl-7 pr-0 py-4 cursor-pointer" onClick={() => navigate(`/instrument/${instrument.id}`)}>
          <Text className="text-base font-black text-black mb-3">乐器信息</Text>
          <View className="flex gap-3 pr-4">
            {instrument.thumbnail ? (
              <Image src={instrument.thumbnail} alt="" className="w-16 h-16 object-cover rounded-lg bg-zinc-100 flex-shrink-0" />
            ) : (instrument.images && (() => {
              try {
                const imgs = typeof instrument.images === 'string' ? JSON.parse(instrument.images) : instrument.images
                if (imgs[0]) return <Image src={imgs[0]} alt="" className="w-16 h-16 object-cover rounded-lg bg-zinc-100 flex-shrink-0" />
              } catch {}
              return <View className="w-16 h-16 bg-zinc-100 rounded-lg flex items-center justify-center"><Text className="text-xs text-zinc-400">暂无图片</Text></View>
            })())}
            <View className="flex-1 min-w-0">
              <Text className="block text-sm font-black text-black">SN: {instrument.sn || '-'}</Text>
              <Text className="block text-xs font-bold text-zinc-500">{instrument.category_name || ''}</Text>
              {instrument.level_name && <Text className="block text-xs font-bold text-zinc-500">级别: {instrument.level_name}</Text>}
              {instrument.tenant_name && <Text className="block text-xs text-zinc-400 font-medium mt-1">{instrument.tenant_name}</Text>}
              {instrument.site_name && <Text className="block text-xs text-zinc-400 font-medium">网点: {instrument.site_name}</Text>}
            </View>
          </View>
        </View>
      )}

      {/* Customer Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">客户信息</Text>
        <View className="space-y-3">
          <View className="flex items-start gap-3">
            <User size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">下单人</Text>
              <Text className="text-sm font-black text-black truncate">{order.user_name || order.user_email || order.user_phone || ''}</Text>
            </View>
          </View>
          {order.delivery_address && (
            <View className="flex items-start gap-3">
              <MapPin size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">收货地址</Text>
                <Text className="text-sm font-medium text-black">{formatDeliveryAddress(order.delivery_address)}</Text>
              </View>
            </View>
          )}
        </View>
      </View>

      {/* Lease Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">租期信息</Text>
        <View className="space-y-3">
          {startDate && (
          <View className="flex items-start gap-3">
            <Calendar size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">租期起点</Text>
              <Text className="text-sm font-black text-black">{startDate}</Text>
            </View>
          </View>
          )}
          {leaseTerm !== undefined && (
          <View className="flex items-start gap-3">
            <Clock size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">预计租期</Text>
              <Text className="text-sm font-black text-black">{rentalDays} 天（{leaseTerm} 个月）</Text>
            </View>
          </View>
          )}
          {endDate && (
          <View className="flex items-start gap-3">
            <Calendar size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">预计到期</Text>
              <Text className="text-sm font-black text-black">{endDate}</Text>
            </View>
          </View>
          )}
        </View>
      </View>

      {/* Logistics Info */}
      {(order.tracking_number || order.courier_company) && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3">物流信息</Text>
          <View className="space-y-3">
              {order.courier_company && (
              <View className="flex items-start gap-3">
                <Truck size={18} className="text-zinc-400 mt-0.5" />
                <View className="flex items-start flex-1 min-w-0">
                  <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">物流公司</Text>
                  <Text className="text-sm font-black text-black">{order.courier_company}</Text>
                </View>
              </View>
            )}
            {order.tracking_number && (
              <View className="flex items-start gap-3">
                <Package size={18} className="text-zinc-400 mt-0.5" />
                <View className="flex items-start flex-1 min-w-0">
                  <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">物流单号</Text>
                  <Text className="text-sm font-mono font-black text-black">{order.tracking_number}</Text>
                </View>
              </View>
            )}
          </View>
        </View>
      )}

      {/* Action Buttons */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <View className="space-y-3 max-w-[480px] mx-auto">
          {showShipButton && (
            <View
              onClick={() => navigate(`/staff/shipping?order=${id}`)}
              className="w-full py-3 bg-black text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80"
            >
              <Truck size={20} />
              <Text>发货</Text>
            </View>
          )}
          {showTransitButton && (
            <View
              onClick={() => navigate(`/staff/shipping?order=${id}`)}
              className="w-full py-3 bg-cyan-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80"
            >
              <Truck size={20} />
              <Text>接收并转发</Text>
            </View>
          )}
          {showReceiveButton && (
            <View
              onClick={() => navigate(`/staff/receiving?order_id=${id}`)}
              className="w-full py-3 bg-[#C21838] text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80"
            >
              <RotateCcw size={20} />
              <Text>收货</Text>
            </View>
          )}
          {(status === 'reserved' || status === 'cancelled' || status === 'shipped' ||
            status === 'in_lease' || status === 'expired' || status === 'returned' ||
            status === 'completed' || status === 'transferred') && (
            <View className="w-full py-3 rounded-2xl font-black text-zinc-500 flex items-center justify-center gap-2">
              {status === 'reserved' ? (
                <><Clock size={16} /> 待发货</>
              ) : status === 'shipped' ? (
                <><CheckCircle size={16} /> 乐器已发货，等待用户签收</>
              ) : status === 'in_lease' ? (
                <><CheckCircle size={16} /> 租赁中</>
              ) : status === 'expired' ? (
                <><AlertTriangle size={16} className="text-red-400" /> 租约已超期</>
              ) : status === 'returned' || status === 'completed' ? (
                <><CheckCircle size={16} /> 该订单已完成</>
              ) : status === 'cancelled' ? (
                <><XCircle size={16} /> 该订单已取消</>
              ) : status === 'transferred' ? (
                <><CheckCircle size={16} /> 已过户</>
              ) : (
                <><Clock size={16} /> 当前状态无操作</>
              )}
            </View>
          )}
        </View>
      </View>
    </View>
  )
}
