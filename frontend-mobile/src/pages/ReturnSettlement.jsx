import { useState, useEffect } from 'react'
import { useParams } from 'react-router-dom'
import { View, Text, Radio, Label } from '@tarojs/components'
import { api } from '../services/api'
import { env, navigation } from '../platform'

export default function ReturnSettlement() {
  const { orderId } = useParams()
  const [loading, setLoading] = useState(true)
  const [settlement, setSettlement] = useState(null)
  const [existing, setExisting] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [refundMethod, setRefundMethod] = useState('prepaid')
  const [confirmed, setConfirmed] = useState(false)

  const fetchSettlement = async () => {
    try {
      const existingResp = await api.get(`/user/settlements/${orderId}`)
      if (existingResp?.code === 20000 && existingResp?.data?.id) {
        setExisting(existingResp.data)
        setLoading(false)
        return
      }
    } catch {}
    try {
      const resp = await api.get(`/user/settlements/${orderId}/calculate`)
      if (resp?.code === 20000) {
        setSettlement(resp.data)
      }
    } catch {}
    setLoading(false)
  }

  useEffect(() => {
    fetchSettlement()
  }, [orderId])

  const handleConfirm = async () => {
    setSubmitting(true)
    try {
      const resp = await api.post(`/user/settlements/${orderId}`, { refund_method: refundMethod })
      if (resp?.code === 20000) {
        setConfirmed(true)
      }
    } catch {}
    setSubmitting(false)
  }

  const s = existing || settlement
  const num = (v) => (v != null ? Number(v).toFixed(2) : '0.00')

  if (loading) {
    return (
      <View className="min-h-screen flex items-center justify-center" style={{ backgroundColor: '#FDFBF7' }}>
        <Text className="text-zinc-400">加载中...</Text>
      </View>
    )
  }

  if (confirmed || existing?.refund_status === 'pending') {
    return (
      <View className="min-h-screen flex flex-col items-center justify-center px-6" style={{ backgroundColor: '#f0fdf4' }}>
        <View><Text className="text-5xl mb-4">✅</Text></View>
        <View><Text className="text-xl font-bold text-gray-800 mb-2">结算完成</Text></View>
        <View className="mb-6 text-center">
          <Text className="text-gray-500 text-sm">
            {existing?.cash_refundable > 0
              ? `可提现金额 ¥${num(existing.cash_refundable)}，已退回预付点 ¥${num(existing.prepaid_refunded)}`
              : existing?.prepaid_refunded > 0
                ? `已退回预付点 ¥${num(existing.prepaid_refunded)}`
                : '本次无需退款'}
          </Text>
        </View>
        <View
          className="bg-blue-500 text-white px-8 py-3 rounded-2xl font-black"
          onClick={() => navigation.redirect('/my-leases')}
        >
          <Text className="text-white">返回租期列表</Text>
        </View>
      </View>
    )
  }

  return (
    <View className="min-h-screen pb-24" style={{ backgroundColor: '#FDFBF7' }}>
      {/* Title bar — H5 only, weapp uses native nav (#1511) */}
      {!env.isMiniProgram && (
        <View className="px-4 pt-4 pb-3" style={{ background: 'linear-gradient(to bottom, #FDF4E7, #fff)' }}>
          <Text className="text-lg font-black text-black block">归还结算</Text>
          <Text className="text-zinc-400 text-sm block">以下为本次租期的费用结算明细</Text>
        </View>
      )}

      <View className="px-4 space-y-3 mt-3">
        {/* Rent Calculation */}
        <View className="bg-white rounded-2xl p-4 shadow-sm">
          <View><Text className="text-sm font-black text-black">租金计算</Text></View>
          <View className="space-y-2 text-sm mt-3">
            <View className="flex justify-between">
              <Text className="text-zinc-400">实际租期</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">{s?.actual_rent_days || 0} 天</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-zinc-400">日租金</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(s?.final_daily_rent)}</Text>
            </View>
            {/* Tier segments */}
            {s?.tier_segments?.length > 0 && (
              <View className="pt-2 border-t border-zinc-100 space-y-1">
                {s.tier_segments.map((t, i) => (
                  <View key={i} className="flex justify-between text-xs">
                    <Text className="text-zinc-400">第 {t.tier} 段 · {t.days} 天</Text>
                    <Text className="text-zinc-500">¥{num(t.rate)} × {t.days}天 = ¥{num(t.subtotal)}</Text>
                  </View>
                ))}
              </View>
            )}
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
          </View>
        </View>

        {/* Points Adjustment */}
        {(s?.gift_points_refunded > 0) && (
          <View className="bg-white rounded-2xl p-4 shadow-sm">
            <View><Text className="text-sm font-black text-black">赠点调整</Text></View>
            <View className="space-y-2 text-sm mt-3">
              <View className="flex justify-between">
                <Text className="text-zinc-400">已用赠点</Text>
                <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(s?.gift_points_used)}</Text>
              </View>
              <View className="flex justify-between">
                <Text className="text-zinc-400">可用额度</Text>
                <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(s?.gift_cap)}</Text>
              </View>
              <View className="flex justify-between text-green-600">
                <Text className="font-medium">退回赠点</Text>
                <Text className="font-bold">+¥{num(s?.gift_points_refunded)}</Text>
              </View>
            </View>
          </View>
        )}

        {/* Overdue Charges */}
        {s?.overdue_charges_total > 0 && (
          <View className="bg-white rounded-2xl p-4 shadow-sm">
            <View><Text className="text-sm font-black text-black">逾期费用</Text></View>
            <View className="flex justify-between text-sm text-red-500 mt-3">
              <Text className="font-medium">{s?.overdue_days ? `逾期 ${s.overdue_days} 天扣款` : '逾期扣款'}</Text>
              <Text className="font-bold">¥{num(s?.overdue_charges_total)}</Text>
            </View>
          </View>
        )}

        {/* Damage Deduction */}
        {s?.deposit_deducted_damage > 0 && (
          <View className="bg-white rounded-2xl p-4 shadow-sm">
            <View><Text className="text-sm font-black text-black">定损扣款</Text></View>
            <View className="flex justify-between text-sm text-red-500 mt-3">
              <Text className="font-medium">定损赔偿（押金扣除）</Text>
              <Text className="font-bold">¥{num(s?.deposit_deducted_damage)}</Text>
            </View>
          </View>
        )}

        {/* Stage-1 notice: awaiting inspection */}
        {!existing && !confirmed && (
          <View className="bg-amber-50 rounded-2xl p-4">
            <View><Text className="text-sm font-black text-amber-700">等待网点验收</Text></View>
            <View className="mt-1">
              <Text className="text-xs text-amber-600 leading-relaxed">
                以上为费用预估明细，最终退款将在网点验收定损后确认（含超期费与定损扣款）。
              </Text>
            </View>
          </View>
        )}

        {/* Refund */}
        <View className="bg-white rounded-2xl p-4 shadow-sm">
          <View><Text className="text-sm font-black text-black">退款明细</Text></View>
          <View className="space-y-2 text-sm mt-3">
            <View className="flex justify-between">
              <Text className="text-zinc-400">原实付现金</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(s?.cash_paid)}</Text>
            </View>
            <View className="flex justify-between">
              <Text className="text-zinc-400">原使用预付点</Text>
              <Text className="font-black text-black flex-shrink-0 whitespace-nowrap">¥{num(s?.prepaid_points_used)}</Text>
            </View>
            <View className="flex justify-between border-t pt-2">
              <Text className="text-zinc-900 font-bold">应退总额</Text>
              <Text className="font-bold text-green-600">¥{num(s?.total_refund)}</Text>
            </View>
            <View className="flex justify-between text-blue-600">
              <Text className="font-medium">可提现</Text>
              <Text className="font-bold">¥{num(s?.cash_refundable)}</Text>
            </View>
            <View className="flex justify-between text-blue-600">
              <Text className="font-medium">退回预付点</Text>
              <Text className="font-bold">+¥{num(s?.prepaid_refunded)}</Text>
            </View>
          </View>
        </View>

        {/* Refund Method */}
        {!existing && (
          <View className="bg-white rounded-2xl p-4 shadow-sm">
            <View><Text className="text-sm font-black text-black">退款方式</Text></View>
            <View className="space-y-2 mt-3">
              <Label className="flex items-center gap-3 p-3 border rounded-xl cursor-pointer active:bg-gray-50">
                <Radio
                  value="prepaid"
                  checked={refundMethod === 'prepaid'}
                  onClick={() => setRefundMethod('prepaid')}
                  style={{ color: '#3b82f6' }}
                />
                <View>
                  <Text className="text-sm font-black text-black block">存为预付点</Text>
                  <Text className="text-xs text-zinc-400 block">即时到账，下次租琴可用</Text>
                </View>
              </Label>
              {s?.cash_refundable > 0 && (
                <Label className="flex items-center gap-3 p-3 border rounded-xl cursor-pointer active:bg-gray-50">
                  <Radio
                    value="cash_withdrawal"
                    checked={refundMethod === 'cash_withdrawal'}
                    onClick={() => setRefundMethod('cash_withdrawal')}
                    style={{ color: '#3b82f6' }}
                  />
                  <View>
                    <Text className="text-sm font-black text-black block">提现</Text>
                    <Text className="text-xs text-zinc-400 block">最多可提现 ¥{num(s?.cash_refundable)}，3-5 个工作日到账</Text>
                  </View>
                </Label>
              )}
            </View>
          </View>
        )}
      </View>

      {!existing && (
        <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4">
          <View
            className="w-full bg-blue-500 text-white py-4 rounded-2xl text-lg font-black flex items-center justify-center"
            style={{ opacity: submitting ? 0.5 : 1 }}
            onClick={submitting ? undefined : handleConfirm}
          >
            <Text className="text-white">{submitting ? '提交中...' : '确认结算'}</Text>
          </View>
        </View>
      )}
    </View>
  )
}
