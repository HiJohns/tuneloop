import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { api } from '../services/api'
import { env, navigation } from '../platform'

export default function ReturnSettlement() {
  const [orderId, setOrderId] = useState('')
  const [loading, setLoading] = useState(true)
  const [settlement, setSettlement] = useState(null)

  const fetchSettlement = async (orderID) => {
    try {
      const resp = await api.get(`/user/settlements/${orderID}/calculate`)
      if (resp?.code === 20000) {
        setSettlement(resp.data)
      }
    } catch {}
    setLoading(false)
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
        </View>
        <View className="mt-3 bg-amber-50 rounded-xl p-3">
          <Text className="text-xs text-amber-600 leading-relaxed block">
            以上为租金预估，最终结算与退款以网点验收定损结果为准（含超期费与定损扣款）。
          </Text>
        </View>
      </View>

      {/* Confirm button */}
      <View className="px-4 mt-8 pb-10">
        <View
          className="w-full bg-blue-500 text-white py-4 rounded-2xl text-lg font-black flex items-center justify-center"
          onClick={() => {
            if (env.isMiniProgram) {
              const pages = Taro.getCurrentPages()
              if (pages.length > 1) {
                Taro.navigateBack()
              } else {
                Taro.redirectTo({ url: '/pages-weapp/my-leases/index' })
              }
            } else {
              navigation.redirect('/my-leases')
            }
          }}
        >
          <Text className="text-white">知道了，返回订单列表</Text>
        </View>
      </View>
    </View>
  )
}
