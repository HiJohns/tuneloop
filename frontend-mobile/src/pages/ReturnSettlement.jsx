import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { api } from '../services/api'
import { env, navigation, dialog } from '../platform'

export default function ReturnSettlement() {
  const [orderId, setOrderId] = useState('')
  const [loading, setLoading] = useState(true)
  const [settlement, setSettlement] = useState(null)
  const [orderDeposit, setOrderDeposit] = useState(null)
  const [confirming, setConfirming] = useState(false)

  const fetchSettlement = async (orderID) => {
    try {
      const [settleResp, orderResp] = await Promise.all([
        api.get(`/user/settlements/${orderID}/calculate`),
        api.get(`/orders/${orderID}`),
      ])
      if (settleResp?.code === 20000) {
        setSettlement(settleResp.data)
      }
      // 押金在订单上（Settlement 无 deposit 字段）——收据明细完整性
      if (orderResp?.code === 20000) {
        setOrderDeposit(orderResp.data?.deposit ?? null)
      }
    } catch {}
    setLoading(false)
  }

  const handleConfirmRefund = async () => {
    if (!orderId) return
    setConfirming(true)
    try {
      const resp = await api.post(`/user/settlements/${orderId}`, { refund_method: 'cash_withdrawal' })
      if (resp?.code === 20000) {
        // H5 has no Taro runtime — use platform dialog (#1615)
        if (env.isMiniProgram) {
          Taro.showToast({ title: '退款已确认', icon: 'success' })
        } else {
          dialog.toast('退款已确认')
        }
        setTimeout(() => {
          if (env.isMiniProgram) {
            Taro.redirectTo({ url: '/pages-weapp/my-leases/index' })
          } else {
            navigation.redirect('/my-leases')
          }
        }, 800)
      } else {
        if (env.isMiniProgram) {
          Taro.showModal({ title: '确认失败', content: resp?.message || '请重试', showCancel: false })
        } else {
          dialog.alert('确认失败: ' + (resp?.message || '请重试'))
        }
      }
    } catch (err) {
      if (env.isMiniProgram) {
        Taro.showModal({ title: '确认失败', content: err.message || '网络错误', showCancel: false })
      } else {
        dialog.alert('确认失败: ' + (err.message || '网络错误'))
      }
    }
    setConfirming(false)
  }

  useEffect(() => {
    let id = ''
    if (env.isMiniProgram) {
      id = Taro.getCurrentInstance().router?.params?.orderId || ''
    } else {
      const m = window.location.pathname.match(/\/return-settlement\/([^/]+)/)
      id = m ? m[1] : ''
    }
    setOrderId(id)
    if (id) fetchSettlement(id)
    else setLoading(false)
  }, [])

  const num = (v) => (v != null ? Number(v).toFixed(2) : '0.00')
  const s = settlement || {}

  if (loading) {
    return (
      <View className="min-h-screen flex items-center justify-center" style={{ backgroundColor: '#FDFBF7' }}>
        <Text className="text-zinc-400">加载中...</Text>
      </View>
    )
  }

  return (
    <View className="min-h-screen" style={{ backgroundColor: '#FDFBF7' }}>
      <View className="px-6 pt-10 flex flex-col items-center">
        {/* Thanks notice */}
        <View className="text-6xl mb-4">🎉</View>
        <Text className="text-xl font-black text-black text-center block mb-2">归还申请已提交</Text>
        <Text className="text-sm text-zinc-500 text-center leading-relaxed block">
          感谢您选择乐音琴行，您的乐器已在归还途中。
          网点收到乐器并完成验收定损后，将为您结算租金并退还押金，请留意到账通知。
          期待与您再次相遇，祝您演奏愉快！🎵
        </Text>
      </View>

      {/* Rent estimate notice */}
      <View className="mx-4 mt-6 bg-white rounded-2xl p-4 shadow-sm">
        <View><Text className="text-sm font-black text-black">租金预估</Text></View>
        <View className="space-y-2 text-sm mt-3">
          <View className="flex justify-between">
            <Text className="text-zinc-400">实际租期</Text>
            <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">{s?.actual_rent_days || 0} 天</Text>
          </View>
          <View className="flex justify-between border-t pt-2">
            <Text className="text-zinc-900 font-bold">实际租金</Text>
            <Text className="font-bold text-blue-600">¥{num(s?.actual_rent_amount)}</Text>
          </View>
          {orderDeposit != null && (
            <View className="flex justify-between">
              <Text className="text-zinc-400">押金</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(orderDeposit)}</Text>
            </View>
          )}
          {s?.early_return_rebate > 0 && (
            <View className="flex justify-between text-green-600">
              <Text className="font-medium">提前归还退费</Text>
              <Text className="font-bold">-¥{num(s?.early_return_rebate)}</Text>
            </View>
          )}
          {s?.overdue_charges_total > 0 && (
            <View className="flex justify-between text-red-500">
              <Text className="font-medium">逾期费用</Text>
              <Text className="font-bold">¥{num(s?.overdue_charges_total)}</Text>
            </View>
          )}
          {s?.damage_deducted > 0 && (
            <View className="flex justify-between text-red-500">
              <Text className="font-medium">损坏赔偿</Text>
              <Text className="font-bold">-¥{num(s?.damage_deducted)}</Text>
            </View>
          )}
        </View>
        <View className="mt-3 bg-amber-50 rounded-xl p-3">
          <Text className="text-xs text-amber-600 leading-relaxed block">
            以上为租金预估。由于乐器尚未完成验收定损，此处不显示退款金额；
            最终结算与退款（含押金、超期费与定损扣款）以网点验收定损结果为准。
          </Text>
        </View>
      </View>

      {/* Confirm button */}
      <View className="px-4 mt-8 pb-10">
        {s?.damage_deducted > 0 ? (
          <View
            className="w-full bg-blue-500 text-white py-4 rounded-2xl text-lg font-black flex items-center justify-center"
            style={{ opacity: confirming ? 0.5 : 1 }}
            onClick={confirming ? undefined : handleConfirmRefund}
          >
            <Text className="text-white">{confirming ? '处理中...' : '确认退款'}</Text>
          </View>
        ) : (
          <View
            className="w-full bg-blue-500 text-white py-4 rounded-2xl text-lg font-black flex items-center justify-center"
            onClick={() => {
              // 返回订单详情页（结算页由 订单详情→归还确认→结算 进入，
              // navigateBack 只会回归还确认页）
              if (env.isMiniProgram) {
                Taro.redirectTo({ url: `/pages-weapp/order-detail/index?id=${orderId}` })
              } else {
                navigation.redirect(`/order/${orderId}`)
              }
            }}
          >
            <Text className="text-white">知道了，返回订单详情</Text>
          </View>
        )}
      </View>
    </View>
  )
}
