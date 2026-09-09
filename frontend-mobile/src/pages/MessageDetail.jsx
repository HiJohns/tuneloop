import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { notificationApi, appealsApi } from '../services/api'
import { dialog, env } from '../platform'
import { formatBeijingDateTime } from '../utils/format'
import { ArrowLeft, Bell } from 'lucide-react'
import { View, Text, Button, Textarea } from '@tarojs/components'

const typeConfig = {
  damage: { bg: 'bg-red-100', text: 'text-red-600', label: '定损通知' },
  appeal: { bg: 'bg-orange-100', text: 'text-orange-600', label: '申诉通知' },
  refund: { bg: 'bg-green-100', text: 'text-green-600', label: '退款通知' },
  payment: { bg: 'bg-blue-100', text: 'text-blue-600', label: '支付通知' },
  order: { bg: 'bg-gray-100', text: 'text-gray-600', label: '系统通知' },
  invoice: { bg: 'bg-purple-100', text: 'text-purple-600', label: '发票通知' },
}

// 结算明细行（payment_shortfall 通知等结构化展示）。#1838 补漏：41c1ea92
// 引入 <Row> 用法但未定义/导入 → ReferenceError: Row is not defined。
function Row({ label, value, color, bold }) {
  return (
    <View className="flex justify-between items-center py-1">
      <Text className="text-xs text-zinc-500">{label}</Text>
      <Text className={`text-xs ${bold ? 'font-bold' : ''}`} style={{ color: color || '#18181b' }}>{value}</Text>
    </View>
  )
}

export default function MessageDetail() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const id = searchParams.get('id') || ''
  const [notification, setNotification] = useState(null)
  const [ref, setRef] = useState(null)
  const [loading, setLoading] = useState(true)
  const [appealModalVisible, setAppealModalVisible] = useState(false)
  const [appealReason, setAppealReason] = useState('')
  const [accepting, setAccepting] = useState(false)

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
  // #1858: damage 预览统一取 ref.damage（通知详情后端挂载，与订单详情同源），
  // 兼容旧响应回退 order.damage——ref.order 原始行不再含有 refund/shortfall。
  const damagePreview = ref?.damage || order?.damage || null
  // #1854：补缴额用后端净缺口（damageData.shortfall），非 damage−refund（refund clamp 后恒 0）
  const shortfall = Number(damagePreview?.shortfall ?? 0)
  const refund = Number(damagePreview?.refund ?? 0)
  const actualRent = Number(damagePreview?.actual_rent_amount ?? 0)

  const goBack = () => {
    if (env.isMiniProgram) {
      Taro.redirectTo({ url: '/pages-weapp/messages/index' })
    } else {
      navigate('/messages', { replace: true })
    }
  }

  const handleAccept = async () => {
    const ok = await dialog.confirm(
      shortfall > 0
        ? `定损金额 ¥${(damageAmount / 100).toFixed(2)}，实际租期租金 ¥${(actualRent / 100).toFixed(2)}，需补缴 ¥${(shortfall / 100).toFixed(2)}`
        : refund > 0
          ? `定损金额 ¥${(damageAmount / 100).toFixed(2)}，应退 ¥${(refund / 100).toFixed(2)}，将退还差额 ¥${(Math.max(0, refund - damageAmount) / 100).toFixed(2)}`
          : `定损金额 ¥${(damageAmount / 100).toFixed(2)}，无额外补缴或退还`
    )
    if (!ok) return
    setAccepting(true)
    try {
      const result = await appealsApi.agree(damageReport.id)
      // #1858 幂等/冲突：非 20000（如 40900 定损已处理）→ 提示后返回，不产生实质动作
      if (!result || result.code !== 20000) {
        dialog.toast(result?.message || '定损已处理，请勿重复操作')
        goBack()
        return
      }
      // #1858: 按服务端真实结果分流——order_status=deposit_refunding 即退款方向，
      // 绝不再把退款单导向付款页（84ab8ebd 实证：应退 ¥0.34 却被引导付 ¥0.01）。
      if (result.data?.order_status === 'deposit_refunding') {
        dialog.toast('已接受定损，退款将在 3-5 个工作日内原路退回')
        goBack()
        return
      }
      // 需补缴：跳付款（H5/weapp 入口与 OrderDetail「去支付」一致）
      const orderId = actionData.order_id || order?.id || ''
      if (env.isMiniProgram) {
        Taro.redirectTo({ url: `/pages-weapp/payment/index?type=damage&id=${orderId}` })
      } else {
        navigate(`/payment?type=damage&id=${orderId}`)
      }
    } catch (err) {
      dialog.toast('操作失败: ' + (err.message || '未知错误'))
      setAccepting(false)
    }
  }

  const handleReject = () => {
    setAppealReason('')
    setAppealModalVisible(true)
  }

  const submitAppeal = async () => {
    if (!appealReason.trim()) {
      dialog.toast('请输入申诉原因')
      return
    }
    try {
      const result = await appealsApi.submit({
        damage_report_id: damageReport.id,
        appeal_reason: appealReason,
      })
      // #1858 幂等/冲突：非 20000（如 40900 定损已处理）→ 提示后关闭弹窗，不提交
      if (!result || result.code !== 20000) {
        dialog.toast(result?.message || '提交失败，请重试')
        setAppealModalVisible(false)
        return
      }
      dialog.toast('申诉已提交，等待处理')
      setAppealModalVisible(false)
      goBack()
    } catch (err) {
      dialog.toast('提交失败: ' + (err.message || '未知错误'))
    }
  }

  const handlePayment = () => {
    if (env.isMiniProgram) {
      Taro.redirectTo({ url: `/pages-weapp/payment/index?type=damage&id=${actionData.order_id || order?.id || ''}` })
      return
    }
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

  const handleRepairAction = () => {
    // weapp repair module not built yet (#1559) — guide to H5 for now
    if (env.isMiniProgram) {
      dialog.toast('请在网页端查看报修详情')
      return
    }
    navigate(`/repair-request?id=${ref?.repair_request_id || notification?.ref_id || ''}`)
  }

  const handleViewInvoice = async (fileUrl) => {
    if (!fileUrl) return
    if (env.isMiniProgram) {
      try {
        const res = await Taro.downloadFile({ url: fileUrl })
        Taro.openDocument({ filePath: res.tempFilePath, showMenu: true })
      } catch (e) {
        dialog.toast('打开失败')
      }
    } else {
      window.open(fileUrl)
    }
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
          <Button onClick={() => env.isMiniProgram ? Taro.navigateBack() : navigate(-1)} className="mt-4 text-brand-primary">返回</Button>
        </View>
      </View>
    )
  }

  const type = typeConfig[notification.type] || typeConfig.order

  return (
    <View style={{ backgroundColor: "#FDFBF7" }} className="min-h-screen pb-20">
      {/* #1706: 手写顶条仅 H5（无原生导航栏）；weapp 用原生导航栏 */}
      {!env.isMiniProgram && (
      <View style={{ backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }} className="px-4 pt-4 pb-3 flex items-center gap-2">
        <ArrowLeft size={20} className="text-black cursor-pointer" onClick={() => env.isMiniProgram ? Taro.navigateBack() : navigate(-1)} />
        <Text className="text-lg font-black text-black">消息详情</Text>
      </View>
      )}

      <View className="p-4">
        <View className="bg-white rounded-xl p-4 shadow-sm">
          <View className="flex items-center gap-2 mb-3">
            <Text className={`text-xs px-2 py-0.5 rounded ${type.bg} ${type.text}`}>
              {type.label}
            </Text>
          </View>

          <Text className="text-base font-bold mb-2">{notification.title}</Text>
          {/* 日期行块级化：weapp/H5 的 Text 均 inline，纵向 margin 不生效 →
              标题与日期同行；北京时区 + 中文格式（设备时区无关，#1857 同约定） */}
          <Text style={{ display: 'block' }} className="text-gray-400 text-xs mb-4">
            {formatBeijingDateTime(notification.created_at)}
          </Text>

          <Text className="text-gray-700 text-sm leading-relaxed mb-6">{notification.content}</Text>

          {damageReport && (
            <View className="border-t pt-4 mb-4">
              <Text className="text-sm font-medium text-gray-500 mb-2">定损信息</Text>
              <View className="bg-gray-50 rounded-lg p-3 text-sm" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                <View className="flex justify-between">
                  <Text className="text-gray-500">定损金额</Text>
                  <Text className="font-medium">
                    ¥{((damageReport.damage_amount || 0) / 100).toFixed(2)}
                  </Text>
                </View>
                {damageReport.damage_description && (
                  <View className="flex justify-between">
                    <Text className="text-gray-500">说明</Text>
                    <Text className="text-right">{damageReport.damage_description}</Text>
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
              <View className="bg-gray-50 rounded-lg p-3 text-sm" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {/* #1859: 免押金单显示「免押金」语义（deposit=0 但 deposit_waived 时展示金额易误读） */}
                <View className="flex justify-between">
                  <Text className="text-gray-500">押金</Text>
                  <Text className="font-medium">{order.deposit_waived ? '免押金' : `¥${((order.deposit || 0) / 100).toFixed(2)}`}</Text>
                </View>
                {/* #1859: 结算方向行（定损上下文）。ref.damage 由 #1858 后端挂载
                    （与订单详情 damage 对象同源）；未部署时优雅降级不渲染。 */}
                {damageReport && (() => {
                  const damagePreview = ref?.damage || order?.damage || null
                  if (!damagePreview) return null
                  const shortfall = Number(damagePreview.shortfall || 0)
                  const refund = Number(damagePreview.refund || 0)
                  if (shortfall > 0) {
                    return (
                      <View className="flex justify-between">
                        <Text className="text-gray-500">结算方向</Text>
                        <Text className="font-medium" style={{ color: '#dc2626' }}>需补缴 ¥{(shortfall / 100).toFixed(2)}</Text>
                      </View>
                    )
                  }
                  if (refund > 0) {
                    return (
                      <View className="flex justify-between">
                        <Text className="text-gray-500">结算方向</Text>
                        <Text className="font-medium" style={{ color: '#16a34a' }}>应退 ¥{(refund / 100).toFixed(2)}</Text>
                      </View>
                    )
                  }
                  return null
                })()}
              </View>
            </View>
          )}

          {/* Action buttons */}
          {notification.action_type === 'damage_accept_reject' && damageReport?.status === 'pending' && (
            <View className="flex gap-3 mt-6">
              <Button
                onClick={handleAccept}
                disabled={accepting}
                style={accepting ? { opacity: 0.6 } : undefined}
                className="flex-1 py-2.5 bg-green-500 text-white rounded-lg text-sm font-medium"
              >
                {accepting ? '处理中...' : '接受'}
              </Button>
              <Button
                onClick={handleReject}
                disabled={accepting}
                style={accepting ? { opacity: 0.6 } : undefined}
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
              支付 ¥{(Math.max(0, damageAmount - deposit) / 100).toFixed(2)}
            </Button>
          )}

          {notification.ref_type === 'appeal' && notification.action_type === 'info' && (
            <Button
              onClick={() => {
                if (env.isMiniProgram) {
                  Taro.redirectTo({ url: `/pages-weapp/payment/index?type=appeal&id=${notification.ref_id}` })
                } else {
                  navigate(`/payment?type=appeal&id=${notification.ref_id}`)
                }
              }}
              className="w-full mt-6 py-2.5 bg-blue-500 text-white rounded-lg text-sm font-medium"
            >
              查看申诉结果
            </Button>
          )}

          {notification.action_type === 'repair_request' && (
            <Button
              onClick={handleRepairAction}
              className="w-full mt-6 py-2.5 bg-brand-primary text-white rounded-lg text-sm font-medium"
            >
              查看详情 / 确认新报价
            </Button>
          )}

          {notification.type === 'invoice' && notification.action_type === 'invoice_reply' && actionData.invoice_file && (
            <Button
              onClick={() => handleViewInvoice(actionData.invoice_file)}
              className="w-full mt-6 py-2.5 bg-brand-primary text-white rounded-lg text-sm font-medium"
            >
              查看/下载发票
            </Button>
          )}

          {notification.type === 'invoice' && notification.action_type === 'invoice_reply' && (
            <Button
              onClick={() => {
                if (env.isMiniProgram) {
                  Taro.redirectTo({ url: '/pages-weapp/invoice/index' })
                } else {
                  navigate('/invoices')
                }
              }}
              className="w-full mt-3 py-2.5 bg-white border border-brand-primary text-brand-primary rounded-lg text-sm font-medium"
            >
              查看申请详情
            </Button>
          )}

          {notification.action_type === 'order' && (
            <>
            <Button
              onClick={() => {
                const orderId = actionData.order_id || notification.ref_id || ''
                if (env.isMiniProgram) {
                  Taro.redirectTo({ url: `/pages-weapp/order-detail/index?id=${orderId}` })
                } else {
                  navigate(`/order/${orderId}`)
                }
              }}
              className="w-full mt-6 py-2.5 bg-brand-primary text-white rounded-lg text-sm font-medium"
            >
              查看订单详情
            </Button>
            {actionData.membership === true && (
              <Button
                onClick={() => {
                  if (env.isMiniProgram) {
                    Taro.redirectTo({ url: '/pages-weapp/membership/index' })
                  } else {
                    navigate('/membership')
                  }
                }}
                className="w-full mt-3 py-2.5 bg-brand-primary text-white rounded-lg text-sm font-medium"
              >
                前往会员中心查看赠点
              </Button>
            )}
            </>
          )}

          {notification.action_type === 'payment_shortfall' && (
            <>
            <View className="mt-4 rounded-xl bg-zinc-50 p-3">
              <View className="text-xs text-zinc-400 font-bold mb-2">结算明细（服务端计算）</View>
              {actionData.breakdown?.rent !== undefined && (
                <Row label="租金" value={`¥${(Number(actionData.breakdown.rent) / 100).toFixed(2)}`} />
              )}
              {Number(actionData.breakdown?.shipping_fee) > 0 && (
                <Row label="物流费" value={`¥${(Number(actionData.breakdown.shipping_fee) / 100).toFixed(2)}`} />
              )}
              {Number(actionData.breakdown?.overdue_fee) > 0 && (
                <Row label="逾期费" value={`¥${(Number(actionData.breakdown.overdue_fee) / 100).toFixed(2)}`} />
              )}
              {Number(actionData.breakdown?.damage_amount) > 0 && (
                <Row label="损坏赔偿" value={`¥${(Number(actionData.breakdown.damage_amount) / 100).toFixed(2)}`} />
              )}
              {actionData.breakdown?.paid_total !== undefined && (
                <Row label="已付总额" value={`¥${(Number(actionData.breakdown.paid_total) / 100).toFixed(2)}`} />
              )}
              <View className="border-t border-zinc-200 mt-2 pt-2">
                <Row label="需补缴" value={`¥${(Number(actionData.shortfall_amount) / 100).toFixed(2)}`} color="#dc2626" bold />
              </View>
            </View>
            <Button
              onClick={() => {
                const orderId = actionData.order_id || notification.ref_id || ''
                if (env.isMiniProgram) {
                  Taro.redirectTo({ url: `/pages-weapp/payment/index?type=payment_shortfall&id=${orderId}` })
                } else {
                  navigate(`/payment?type=payment_shortfall&id=${orderId}`)
                }
              }}
              className="w-full mt-6 py-2.5 bg-brand-primary text-white rounded-lg text-sm font-medium"
            >
              去补缴
            </Button>
            </>
          )}
        </View>
      </View>

      {appealModalVisible && (
        <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.45)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <View style={{ width: '82%', backgroundColor: '#fff', borderRadius: 16, padding: 20 }}>
            <Text className="text-base font-bold mb-3">申诉</Text>
            <Textarea
              className="w-full border rounded-lg p-3 text-sm"
              value={appealReason}
              onChange={e => setAppealReason(e.detail?.value ?? e.target?.value ?? '')}
              placeholder="请输入申诉原因..."
            />
            <View style={{ display: 'flex', gap: 12, marginTop: 16 }}>
              <Button onClick={() => setAppealModalVisible(false)} style={{ flex: 1, backgroundColor: '#f4f4f5', color: '#71717a', borderRadius: 10 }}>
                取消
              </Button>
              <Button onClick={submitAppeal} style={{ flex: 1, backgroundColor: '#002140', color: '#fff', borderRadius: 10 }}>
                提交
              </Button>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
