import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, User, MapPin, Calendar, Clock, Package, Truck, RotateCcw, CheckCircle, XCircle, AlertTriangle } from 'lucide-react'

const STATUS_LABELS = {
  reserved: '已预约',
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
    <View className="min-h-screen bg-brand-bg pb-24">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">订单详情</Text>
      </View>

      {/* Order ID Banner */}
      <View className="bg-white px-4 py-4 border-b">
        <Text className="text-xs text-gray-400 mb-1">订单编号</Text>
        <Text className="text-xl font-mono font-bold text-gray-900 tracking-wide">{id}</Text>
      </View>

      {/* Status */}
      <View className="bg-white px-4 py-3 border-b">
        <View className="flex items-center justify-between">
          <Text className="text-sm text-gray-500">当前状态</Text>
          <Text className={`text-sm px-3 py-1 rounded-full font-medium ${statusColor}`}>
            {statusLabel}
          </Text>
        </View>
      </View>

      {/* Instrument Info */}
      {instrument && (
        <View className="mt-3 bg-white px-4 py-4 cursor-pointer" onClick={() => navigate(`/instrument/${instrument.id}`)}>
          <Text className="text-sm font-medium text-gray-900 mb-3">乐器信息</Text>
          <View className="flex gap-3">
            {instrument.images && (() => {
              try {
                const imgs = typeof instrument.images === 'string' ? JSON.parse(instrument.images) : instrument.images
                if (imgs[0]) return <Image src={imgs[0]} alt="" className="w-16 h-16 object-cover rounded bg-gray-100" />
              } catch {}
              return <View className="w-16 h-16 bg-gray-100 rounded flex items-center justify-center text-xs text-gray-400">暂无图片</View>
            })()}
            <View>
              <Text className="text-sm font-mono font-medium">SN: {instrument.sn || '-'}</Text>
              <Text className="text-xs text-gray-500">{instrument.category_name || ''}</Text>
              {instrument.tenant_name && <Text className="text-xs text-gray-400 mt-1">{instrument.tenant_name}</Text>}
              {instrument.site_name && <Text className="text-xs text-gray-400">网点: {instrument.site_name}</Text>}
            </View>
          </View>
        </View>
      )}

      {/* Customer Info */}
      <View className="mt-3 bg-white px-4 py-4">
        <Text className="text-sm font-medium text-gray-900 mb-3">客户信息</Text>
        <View className="space-y-3">
          <View className="flex items-center gap-3">
            <User size={18} className="text-gray-400" />
            <View>
              <Text className="text-xs text-gray-400">下单人</Text>
              <Text className="text-sm font-medium">{order.user_name || '未实名用户'}</Text>
            </View>
          </View>
          {order.delivery_address && (
            <View className="flex items-start gap-3">
              <MapPin size={18} className="text-gray-400 mt-0.5" />
              <View>
                <Text className="text-xs text-gray-400">收货地址</Text>
                <Text className="text-sm font-medium">{formatDeliveryAddress(order.delivery_address)}</Text>
              </View>
            </View>
          )}
        </View>
      </View>

      {/* Lease Info */}
      <View className="mt-3 bg-white px-4 py-4">
        <Text className="text-sm font-medium text-gray-900 mb-3">租期信息</Text>
        <View className="space-y-3">
          <View className="flex items-center gap-3">
            <Calendar size={18} className="text-gray-400" />
            <View>
              <Text className="text-xs text-gray-400">租期起点</Text>
              <Text className="text-sm font-medium">{startDate}</Text>
            </View>
          </View>
          <View className="flex items-center gap-3">
            <Clock size={18} className="text-gray-400" />
            <View>
              <Text className="text-xs text-gray-400">预计租期天数</Text>
              <Text className="text-sm font-medium">{rentalDays} 天（{leaseTerm} 个月）</Text>
            </View>
          </View>
          <View className="flex items-center gap-3">
            <Calendar size={18} className="text-gray-400" />
            <View>
              <Text className="text-xs text-gray-400">预计到期日</Text>
              <Text className="text-sm font-medium">{endDate}</Text>
            </View>
          </View>
        </View>
      </View>

      {/* Logistics Info */}
      {(order.tracking_number || order.courier_company) && (
        <View className="mt-3 bg-white px-4 py-4">
          <Text className="text-sm font-medium text-gray-900 mb-3">物流信息</Text>
          <View className="space-y-3">
            {order.courier_company && (
              <View className="flex items-center gap-3">
                <Truck size={18} className="text-gray-400" />
                <View>
                  <Text className="text-xs text-gray-400">物流公司</Text>
                  <Text className="text-sm font-medium">{order.courier_company}</Text>
                </View>
              </View>
            )}
            {order.tracking_number && (
              <View className="flex items-center gap-3">
                <Package size={18} className="text-gray-400" />
                <View>
                  <Text className="text-xs text-gray-400">物流单号</Text>
                  <Text className="text-sm font-mono">{order.tracking_number}</Text>
                </View>
              </View>
            )}
          </View>
        </View>
      )}

      {/* Action Buttons */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <View className="space-y-3">
          {showShipButton && (
            <Button
              onClick={() => navigate(`/staff/shipping?order_id=${id}`)}
              className="w-full py-3 bg-blue-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <Truck size={20} />
              发货
            </Button>
          )}
          {showTransitButton && (
            <Button
              onClick={() => navigate(`/staff/shipping?order_id=${id}`)}
              className="w-full py-3 bg-cyan-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <Truck size={20} />
              接收并转发
            </Button>
          )}
          {showReceiveButton && (
            <Button
              onClick={() => navigate(`/staff/receiving?order_id=${id}`)}
              className="w-full py-3 bg-green-600 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <RotateCcw size={20} />
              收货
            </Button>
          )}
          {(status === 'reserved' || status === 'cancelled' || status === 'shipped' ||
            status === 'in_lease' || status === 'expired' || status === 'returned' ||
            status === 'completed' || status === 'transferred') && (
            <View className="text-center text-sm text-gray-400 py-2 flex items-center justify-center gap-2">
              {status === 'reserved' ? (
                <><Clock size={16} /> 等待用户支付</>
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
