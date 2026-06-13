import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Modal, message } from 'antd'
import { notificationApi, appealsApi } from '../services/api'
import { ArrowLeft, Bell } from 'lucide-react'
import { View, Text, Button, Textarea } from '@tarojs/components'

const typeConfig = {
  damage: { bg: 'bg-red-100', text: 'text-red-600', label: '定损通知' },
  appeal: { bg: 'bg-orange-100', text: 'text-orange-600', label: '申诉通知' },
  refund: { bg: 'bg-green-100', text: 'text-green-600', label: '退款通知' },
  payment: { bg: 'bg-blue-100', text: 'text-blue-600', label: '支付通知' },
  order: { bg: 'bg-gray-100', text: 'text-gray-600', label: '系统通知' },
}

export default function MessageDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [notification, setNotification] = useState(null)
  const [ref, setRef] = useState(null)
  const [loading, setLoading] = useState(true)
  const [appealModalVisible, setAppealModalVisible] = useState(false)
  const [appealReason, setAppealReason] = useState('')

  useEffect(() => {
    const fetchDetail = async () => {
      try {
        const resp = await notificationApi.detail(id)
        const data = resp?.data
        if (data) {
          setNotification(data.notification)
          setRef(data.ref || null)

          if (data.notification.status === 'unread') {
            await notificationApi.markRead(id)
          }
        }
      } catch (err) {
        console.error('Failed to fetch notification detail:', err)
      }
      setLoading(false)
    }
    fetchDetail()
  }, [id])

  const parseActionData = () => {
    if (!notification?.action_data) return {}
    if (typeof notification.action_data === 'object') return notification.action_data
    try { return JSON.parse(notification.action_data) } catch { return {} }
  }

  const actionData = parseActionData()
  const damageReport = ref?.damage_report
  const order = ref?.order

  const damageAmount = actionData.damage_amount || damageReport?.damage_amount || 0
  const deposit = actionData.deposit || order?.deposit || 0

  const handleAccept = async () => {
    if (damageAmount < deposit) {
      Modal.confirm({
        title: '确认接受定损',
        content: `定损金额 ¥${damageAmount.toFixed(2)}，押金 ¥${deposit.toFixed(2)}，将退还差额 ¥${(deposit - damageAmount).toFixed(2)}`,
        onOk: async () => {
          try {
            await appealsApi.agree(damageReport.id)
            message.success('已接受定损，押金退还流程将开始')
            navigate('/messages', { replace: true })
          } catch (err) {
            message.error('操作失败: ' + (err.message || '未知错误'))
          }
        },
      })
    } else {
      Modal.confirm({
        title: '确认接受定损',
        content: `定损金额 ¥${damageAmount.toFixed(2)}，押金 ¥${deposit.toFixed(2)}，需补缴 ¥${(damageAmount - deposit).toFixed(2)}`,
        onOk: async () => {
          try {
            await appealsApi.agree(damageReport.id)
            navigate('/payment-complete', {
              state: {
                paymentAmount: damageAmount - deposit,
                damageAmount,
                deposit,
                merchantName: ref?.order?.merchant_name || '商户',
                orderId: actionData.order_id || order?.id,
              },
              replace: true,
            })
          } catch (err) {
            message.error('操作失败: ' + (err.message || '未知错误'))
          }
        },
      })
    }
  }

  const handleReject = () => {
    setAppealReason('')
    setAppealModalVisible(true)
  }

  const submitAppeal = async () => {
    if (!appealReason.trim()) {
      message.warning('请输入申诉原因')
      return
    }
    try {
      await appealsApi.submit({
        damage_report_id: damageReport.id,
        appeal_reason: appealReason,
      })
      message.success('申诉已提交，等待处理')
      setAppealModalVisible(false)
      navigate('/messages', { replace: true })
    } catch (err) {
      message.error('提交失败: ' + (err.message || '未知错误'))
    }
  }

  const handlePayment = () => {
    navigate('/payment-complete', {
      state: {
        paymentAmount: damageAmount - deposit,
        damageAmount,
        deposit,
        merchantName: ref?.order?.merchant_name || '商户',
        orderId: actionData.order_id || order?.id,
      },
      replace: true,
    })
  }

  if (loading) {
    return (
      <View className="min-h-screen bg-brand-bg flex items-center justify-center">
        <Text className="text-gray-500">加载中...</Text>
      </View>
    )
  }

  if (!notification) {
    return (
      <View className="min-h-screen bg-brand-bg flex items-center justify-center">
        <View className="text-center">
          <Bell size={48} className="mx-auto text-gray-300 mb-4" />
          <Text className="text-gray-500">消息不存在</Text>
          <Button onClick={() => navigate(-1)} className="mt-4 text-brand-primary">返回</Button>
        </View>
      </View>
    )
  }

  const type = typeConfig[notification.type] || typeConfig.order

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </Button>
        <Text className="text-lg font-bold">消息详情</Text>
      </View>

      <View className="p-4">
        <View className="bg-white rounded-xl p-4 shadow-sm">
          <View className="flex items-center gap-2 mb-3">
            <Text className={`text-xs px-2 py-0.5 rounded ${type.bg} ${type.text}`}>
              {type.label}
            </Text>
          </View>

          <Text className="text-base font-bold mb-2">{notification.title}</Text>
          <Text className="text-gray-400 text-xs mb-4">
            {new Date(notification.created_at).toLocaleString()}
          </Text>

          <Text className="text-gray-700 text-sm leading-relaxed mb-6">{notification.content}</Text>

          {damageReport && (
            <View className="border-t pt-4 mb-4">
              <Text className="text-sm font-medium text-gray-500 mb-2">定损信息</Text>
              <View className="bg-gray-50 rounded-lg p-3 space-y-1 text-sm">
                <View className="flex justify-between">
                  <Text className="text-gray-500">定损金额</Text>
                  <Text className="font-medium">
                    ¥{damageReport.damage_amount?.toFixed(2) || '0.00'}
                  </Text>
                </View>
                {damageReport.damage_description && (
                  <View className="flex justify-between">
                    <Text className="text-gray-500">说明</Text>
                    <Text className="text-right max-w-[60%]">{damageReport.damage_description}</Text>
                  </View>
                )}
                <View className="flex justify-between">
                  <Text className="text-gray-500">定损状态</Text>
                  <Text>{damageReport.status}</Text>
                </View>
              </View>
            </View>
          )}

          {order && (
            <View className="border-t pt-4">
              <Text className="text-sm font-medium text-gray-500 mb-2">订单信息</Text>
              <View className="bg-gray-50 rounded-lg p-3 space-y-1 text-sm">
                <View className="flex justify-between">
                  <Text className="text-gray-500">押金</Text>
                  <Text className="font-medium">¥{order.deposit?.toFixed(2) || '0.00'}</Text>
                </View>
                <View className="flex justify-between">
                  <Text className="text-gray-500">月租</Text>
                  <Text>¥{order.monthly_rent?.toFixed(2) || '0.00'}</Text>
                </View>
              </View>
            </View>
          )}

          {/* Action buttons */}
          {notification.action_type === 'damage_accept_reject' && damageReport?.status === 'pending' && (
            <View className="flex gap-3 mt-6">
              <Button
                onClick={handleAccept}
                className="flex-1 py-2.5 bg-green-500 text-white rounded-lg text-sm font-medium"
              >
                接受
              </Button>
              <Button
                onClick={handleReject}
                className="flex-1 py-2.5 bg-red-500 text-white rounded-lg text-sm font-medium"
              >
                拒绝
              </Button>
            </View>
          )}

          {notification.action_type === 'payment' && (
            <Button
              onClick={handlePayment}
              className="w-full mt-6 py-2.5 bg-brand-primary text-white rounded-lg text-sm font-medium"
            >
              支付 ¥{Math.max(0, damageAmount - deposit).toFixed(2)}
            </Button>
          )}
        </View>
      </View>

      <Modal
        title="申诉"
        open={appealModalVisible}
        onCancel={() => setAppealModalVisible(false)}
        onOk={submitAppeal}
        okText="提交"
        cancelText="取消"
      >
        <Textarea
          className="w-full border rounded-lg p-3 text-sm min-h-[120px]"
          value={appealReason}
          onChange={e => setAppealReason(e.target.value)}
          placeholder="请输入申诉原因..."
        />
      </Modal>
    </View>
  )
}
