import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { api } from '../services/api'
import { env } from '../platform'

export default function ReturnSettlement() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const [orderId, setOrderId] = useState('')
  const [loading, setLoading] = useState(true)
  const [settlement, setSettlement] = useState(null)
  const [orderDeposit, setOrderDeposit] = useState(null)
  const [merchantName, setMerchantName] = useState('')

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
        // #1764: 商户名动态（GetOrder 带 merchant_name，fallback 云租吧）
        setMerchantName(orderResp.data?.merchant_name || '云租吧')
      }
    } catch {}
    setLoading(false)
  }

  useEffect(() => {
    // 跨端统一 query 约定（#1674）：跳转一律 ?order_id=
    const id = searchParams.get('order_id') || ''
    setOrderId(id)
    if (id) fetchSettlement(id)
    else setLoading(false)
  }, [])

  const num = (v) => (v != null ? (Number(v) / 100).toFixed(2) : '0.00')
  const s = settlement || {}

  // #1764: fee_items 逐项方向化——服务端输出 {item, direction, amount}（分），
  // 前端只读禁止自算；amount=0 隐藏；refund=待退（绿）/pay=待补缴（红）。
  const feeItemBaseLabels = {
    rent: '租金',
    deposit: '押金',
    shipping_fee: '物流费',
    overdue_fee: '逾期费',
    damage: '损坏赔偿',
  }
  const feeItems = (s?.fee_items || [])
    .filter((it) => it && Number(it.amount) !== 0)
    .map((it) => {
      const base = feeItemBaseLabels[it.item] || it.item
      return {
        ...it,
        label: it.direction === 'refund' ? `待退${base}` : `待补缴${base}`,
      }
    })

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
          感谢您选择{merchantName}，您的订单已在归还途中。
          网点收到乐器并完成验收后，将为您核对结算，请留意平台通知。
          期待与您再次相遇，祝您生活愉快~
        </Text>
      </View>

      {/* Rent estimate notice */}
      <View className="mx-4 mt-6 bg-white rounded-2xl p-4 shadow-sm">
        <View><Text className="text-sm font-black text-black">费用更新（预估）</Text></View>
        <View className="space-y-2 text-sm mt-3">
          {feeItems.length > 0 ? (
            feeItems.map((it) => (
              <View key={it.item} className={`flex justify-between ${it.direction === 'refund' ? 'text-green-600' : 'text-red-500'}`}>
                <Text className="font-medium">{it.label}</Text>
                <Text className="font-bold">{it.direction === 'refund' ? `+¥${num(it.amount)}` : `¥${num(Math.abs(it.amount))}`}</Text>
              </View>
            ))
          ) : (
            <View className="flex justify-between">
              <Text className="text-zinc-400">暂无差额</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥0.00</Text>
            </View>
          )}
          {s?.actual_rent_days > 0 && (
            <View className="flex justify-between border-t pt-2">
              <Text className="text-zinc-400">实际租期</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">{s?.actual_rent_days || 0} 天</Text>
            </View>
          )}
          <View className="flex justify-between">
            <Text className="text-zinc-900 font-bold">实际租金</Text>
            <Text className="font-bold text-blue-600">¥{num(s?.actual_rent_amount)}</Text>
          </View>
          {orderDeposit != null && (
            <View className="flex justify-between">
              <Text className="text-zinc-400">押金</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(orderDeposit)}</Text>
            </View>
          )}
        </View>
        <View className="mt-3 bg-amber-50 rounded-xl p-3">
          <Text className="text-xs text-amber-600 leading-relaxed block">
            以上为费用更新（预估）。由于乐器尚未完成验收定损，此处不显示退款金额；
            最终结算与退款（含押金、超期费与定损扣款）以网点验收定损结果为准。
          </Text>
        </View>
      </View>

      {/* Back button — pure navigation (#1764/#1765): 未定损态禁止任何
          后台操作（结算/退款），按钮仅返回订单详情。 */}
      <View className="px-4 mt-8 pb-10">
        <View
          className="w-full bg-blue-500 text-white py-4 rounded-2xl text-lg font-black flex items-center justify-center"
          onClick={() => {
            // 返回订单详情页（#1702）：归还物流页已在提交时 redirectTo 替换
            // 掉，页面栈为 [order-detail, return-settlement]——navigateBack
            // 直接回订单详情；深链/异常入口（倒数第 2 页非 return-confirm）
            // 时 fallback redirectTo 订单详情。
            if (env.isMiniProgram) {
              const pages = Taro.getCurrentPages()
              const prev = pages.length >= 2 ? pages[pages.length - 2] : null
              if (prev && String(prev.route || '').includes('return-confirm')) {
                Taro.navigateBack()
              } else {
                Taro.redirectTo({ url: `/pages-weapp/order-detail/index?id=${orderId}` })
              }
            } else {
              navigate(`/order/${orderId}`, { replace: true })
            }
          }}
        >
          <Text className="text-white">返回</Text>
        </View>
      </View>
    </View>
  )
}
