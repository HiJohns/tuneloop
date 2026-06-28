import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../services/api'
import { navigation } from '../platform'

export default function ReturnSettlement() {
  const { orderId } = useParams()
  const [loading, setLoading] = useState(true)
  const [settlement, setSettlement] = useState(null)
  const [existing, setExisting] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [refundMethod, setRefundMethod] = useState('prepaid')
  const [confirmed, setConfirmed] = useState(false)

  useEffect(() => {
    fetchSettlement()
  }, [orderId])

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
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-gray-500">加载中...</div>
      </div>
    )
  }

  if (confirmed || existing?.refund_status === 'pending') {
    return (
      <div className="min-h-screen bg-green-50 flex flex-col items-center justify-center px-6">
        <div className="text-5xl mb-4">✅</div>
        <h1 className="text-xl font-bold text-gray-800 mb-2">结算完成</h1>
        <p className="text-gray-500 text-sm text-center mb-6">
          {existing?.cash_refundable > 0
            ? `可提现金额 ¥${num(existing.cash_refundable)}，已退回预付点 ¥${num(existing.prepaid_refunded)}`
            : existing?.prepaid_refunded > 0
              ? `已退回预付点 ¥${num(existing.prepaid_refunded)}`
              : '本次无需退款'}
        </p>
        <button
          className="bg-blue-500 text-white px-8 py-3 rounded-xl font-medium active:bg-blue-600"
          onClick={() => navigation.redirect('/my-leases')}
        >
          返回租期列表
        </button>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50 pb-24">
      <div className="bg-white px-5 pt-12 pb-6">
        <h1 className="text-xl font-bold text-gray-800 mb-1">归还结算</h1>
        <p className="text-gray-400 text-sm">以下为本次租期的费用结算明细</p>
      </div>

      <div className="px-4 space-y-3 mt-3">
        {/* Rent Calculation */}
        <div className="bg-white rounded-2xl p-4 shadow-sm">
          <h2 className="text-sm font-bold text-gray-800 mb-3">租金计算</h2>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">实际租期</span>
              <span className="font-medium">{s?.actual_rent_days || 0} 天</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">日租金</span>
              <span className="font-medium">¥{num(s?.final_daily_rent)}</span>
            </div>
            <div className="flex justify-between border-t pt-2">
              <span className="text-gray-800 font-bold">实际租金</span>
              <span className="font-bold text-blue-600">¥{num(s?.actual_rent_amount)}</span>
            </div>
          </div>
        </div>

        {/* Points Adjustment */}
        {(s?.gift_points_refunded > 0) && (
          <div className="bg-white rounded-2xl p-4 shadow-sm">
            <h2 className="text-sm font-bold text-gray-800 mb-3">赠点调整</h2>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-500">已用赠点</span>
                <span className="font-medium">¥{num(s?.gift_points_used)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">可用额度</span>
                <span className="font-medium">¥{num(s?.gift_cap)}</span>
              </div>
              <div className="flex justify-between text-green-600">
                <span className="font-medium">退回赠点</span>
                <span className="font-bold">+¥{num(s?.gift_points_refunded)}</span>
              </div>
            </div>
          </div>
        )}

        {/* Overdue Charges */}
        {s?.overdue_charges_total > 0 && (
          <div className="bg-white rounded-2xl p-4 shadow-sm">
            <h2 className="text-sm font-bold text-gray-800 mb-3">逾期费用</h2>
            <div className="flex justify-between text-sm text-red-500">
              <span className="font-medium">逾期扣款</span>
              <span className="font-bold">¥{num(s?.overdue_charges_total)}</span>
            </div>
          </div>
        )}

        {/* Refund */}
        <div className="bg-white rounded-2xl p-4 shadow-sm">
          <h2 className="text-sm font-bold text-gray-800 mb-3">退款明细</h2>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">原实付现金</span>
              <span className="font-medium">¥{num(s?.cash_paid)}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">原使用预付点</span>
              <span className="font-medium">¥{num(s?.prepaid_points_used)}</span>
            </div>
            <div className="flex justify-between border-t pt-2">
              <span className="text-gray-800 font-bold">应退总额</span>
              <span className="font-bold text-green-600">¥{num(s?.total_refund)}</span>
            </div>
            <div className="flex justify-between text-blue-600">
              <span className="font-medium">可提现</span>
              <span className="font-bold">¥{num(s?.cash_refundable)}</span>
            </div>
            <div className="flex justify-between text-blue-600">
              <span className="font-medium">退回预付点</span>
              <span className="font-bold">+¥{num(s?.prepaid_refunded)}</span>
            </div>
          </div>
        </div>

        {/* Refund Method */}
        {!existing && (
          <div className="bg-white rounded-2xl p-4 shadow-sm">
            <h2 className="text-sm font-bold text-gray-800 mb-3">退款方式</h2>
            <div className="space-y-2">
              <label className="flex items-center gap-3 p-3 border rounded-xl cursor-pointer active:bg-gray-50">
                <input
                  type="radio"
                  name="refundMethod"
                  value="prepaid"
                  checked={refundMethod === 'prepaid'}
                  onChange={() => setRefundMethod('prepaid')}
                  className="accent-blue-500"
                />
                <div>
                  <div className="text-sm font-medium text-gray-800">存为预付点</div>
                  <div className="text-xs text-gray-400">即时到账，下次租琴可用</div>
                </div>
              </label>
              {s?.cash_refundable > 0 && (
                <label className="flex items-center gap-3 p-3 border rounded-xl cursor-pointer active:bg-gray-50">
                  <input
                    type="radio"
                    name="refundMethod"
                    value="cash_withdrawal"
                    checked={refundMethod === 'cash_withdrawal'}
                    onChange={() => setRefundMethod('cash_withdrawal')}
                    className="accent-blue-500"
                  />
                  <div>
                    <div className="text-sm font-medium text-gray-800">提现</div>
                    <div className="text-xs text-gray-400">最多可提现 ¥{num(s?.cash_refundable)}，3-5 个工作日到账</div>
                  </div>
                </label>
              )}
            </div>
          </div>
        )}
      </div>

      {!existing && (
        <div className="fixed bottom-0 left-0 right-0 bg-white border-t border-gray-100 p-4">
          <button
            className="w-full bg-blue-500 text-white py-4 rounded-xl text-lg font-medium active:bg-blue-600 disabled:opacity-50"
            disabled={submitting}
            onClick={handleConfirm}
          >
            {submitting ? '提交中...' : '确认结算'}
          </button>
        </div>
      )}
    </div>
  )
}
