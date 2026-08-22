import { useState, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { apiFetch , resolveErrorMessage } from '../services/api'
import { env, dialog } from '../platform'
import { formatDisplayDate } from '../utils/format'

const baseUrl = env.apiBaseUrl

function clampPoints(raw, max) {
  const v = parseInt(raw, 10)
  if (Number.isNaN(v)) return 0
  return Math.min(Math.max(0, v), Math.floor(max))
}

export default function Payment() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const pType = searchParams.get('type') || ''
  const pId = searchParams.get('id') || ''

  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [giftUsed, setGiftUsed] = useState(0)
  // #1719 优惠码通用化（P3 分语义）：OREZ 全免 / ENO 1%（千分比 10‰）
  const [couponCode, setCouponCode] = useState('')
  const [appliedCoupon, setAppliedCoupon] = useState(null)
  const [couponAmount, setCouponAmount] = useState(0)

  useEffect(() => {
    if (!pType) return
    const fetchData = async () => {
      if (pType === 'appeal') {
        // Appeal resolution receipt: load settled refund (#1576).
        try {
          const resp = await apiFetch(`${baseUrl}/user/settlements/${pId}/calculate`)
          const result = await resp.json()
          if (result.code === 20000) {
            setData({ type: 'appeal', title: '申诉结果确认', amount: result.data?.cash_refundable || 0, details: result.data, wallet: null })
          }
        } catch {}
        setLoading(false)
        return
      }
      try {
        const resp = await apiFetch(`${baseUrl}/pay/calculate`, {
          method: 'POST',
          body: JSON.stringify({ type: pType, id: pId }),
        })
        const result = await resp.json()
        if (result.code === 20000) setData(result.data)
      } catch {}
      setLoading(false)
    }
    fetchData()
  }, [pType, pId])

  if (loading) {
    return <div className="h-screen flex items-center justify-center bg-[#FDFBF7]">
      <span className="text-zinc-400">加载中...</span>
    </div>
  }
  if (!data) {
    return <div className="h-screen flex items-center justify-center bg-[#FDFBF7]">
      <span className="text-zinc-400">支付数据不存在</span>
    </div>
  }

  const wallet = data.wallet || {}
  const maxGift = Math.min(wallet.promo_points || 0, wallet.max_gift_amount || 0)

  const isRefund = ['refund', 'deposit-refund'].includes(pType)

  const displayAmount = appliedCoupon ? couponAmount : data.amount
  const cashAmount = isRefund
    ? data.amount
    : Math.max(0, displayAmount - giftUsed)

  return (
    <div className="min-h-screen bg-[#FDFBF7] pb-[100px]">
      <div className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 py-3 flex items-center">
        <span className="text-xl font-bold text-black cursor-pointer" onClick={() => navigate(-1)}>❮</span>
        <span className="text-lg font-bold flex-1 text-center">
          {pType === 'appeal' ? '申诉结果确认' : pType === 'payment_shortfall' ? '补缴确认' : isRefund ? '退款确认' : '支付确认'}
        </span>
        <span className="w-6" />
      </div>

      <div className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
        <div className="text-sm font-bold text-black mb-3">{data.title}</div>

        {data.details && (
          <>
            {renderDetailsBlock(data.details, data.type)}
            {data.type === 'rent' && data.details.pricing_breakdown && (
              <div className="border-t border-zinc-100 mt-2 pt-2">
                <Row label="合计" value={`¥${(Number(data.amount) / 100).toFixed(2)}`} bold />
              </div>
            )}
          </>
        )}

        {isRefund && (
          <div className="mt-2">
            {data.details?.cancel_refund && (
              <>
                <div className="text-xs text-zinc-400 font-bold mt-2 mb-1">原支付明细</div>
                {data.details.total_paid !== undefined && (
                  <Row label="原支付总额" value={`¥${(Number(data.details.total_paid) / 100).toFixed(2)}`} />
                )}
                {data.details.cash_paid !== undefined && Number(data.details.cash_paid) > 0 && (
                  <Row label="  现金" value={`¥${(Number(data.details.cash_paid) / 100).toFixed(2)}`} />
                )}
                {data.details.prepaid_used !== undefined && Number(data.details.prepaid_used) > 0 && (
                  <Row label="  预付点" value={`¥${(Number(data.details.prepaid_used) / 100).toFixed(2)}`} />
                )}
                {data.details.gift_used !== undefined && Number(data.details.gift_used) > 0 && (
                  <Row label="  赠点" value={`¥${(Number(data.details.gift_used) / 100).toFixed(2)}`} />
                )}
                <div className="text-xs text-zinc-400 font-bold mt-3 mb-1">退款明细（原路退回）</div>
              </>
            )}
            {data.details?.cash_refundable !== undefined && Number(data.details.cash_refundable) > 0 && (
              <Row label="退现金（微信原路）" value={`¥${(Number(data.details.cash_refundable) / 100).toFixed(2)}`} />
            )}
            {data.details?.prepaid_refunded !== undefined && Number(data.details.prepaid_refunded) > 0 && (
              <Row label="退回预付点" value={`+¥${(Number(data.details.prepaid_refunded) / 100).toFixed(2)}`} color="#16a34a" />
            )}
            {data.details?.gift_refunded !== undefined && Number(data.details.gift_refunded) > 0 && (
              <Row label="退回赠点" value={`+¥${(Number(data.details.gift_refunded) / 100).toFixed(2)}`} color="#16a34a" />
            )}
            {data.details?.gift_cap !== undefined && Number(data.details.gift_cap) > 0 && (
              <Row label="赠点抵扣（按当前级别比例）" value={`${Number(data.details.gift_cap).toFixed(0)} 点`} color="#16a34a" />
            )}
            <Row label="退款金额" value={`¥${(Number(data.amount) / 100).toFixed(2)}`} bold />
            <div className="text-xs text-zinc-400 mt-2">退款完成后将按实付现金发放返点赠点，可在会员中心查看。</div>
          </div>
        )}
      </div>

      {!isRefund && pType !== 'appeal' && data.amount > 0 && maxGift > 0 && (
        <div className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <div className="text-sm font-black text-black mb-3">点数使用</div>

          {maxGift > 0 && (
          <div className="mb-1">
            <Row label="赠点余额" value={`¥${(Number(maxGift) / 100).toFixed(2)}`} />
            <div className="flex items-center mt-1">
              <span className="text-xs text-zinc-500 w-[72px]">使用</span>
              <div className="flex-1 flex items-center gap-2">
                <input type="number" min={0} max={Math.min(maxGift, data.amount)} step={1}
                  value={giftUsed}
                  onChange={e => setGiftUsed(clampPoints(e.target.value, Math.min(maxGift, data.amount)))}
                  className="flex-1 border border-zinc-200 rounded-lg px-2 py-1 text-right"
                />
              </div>
              <span className="text-xs text-zinc-500 ml-1">点</span>
            </div>
            <div className="text-[11px] text-zinc-400 text-right mb-2">
              赠点使用不得超过租金的 {Math.round((wallet.max_gift_ratio || 0.3) * 100)}%
            </div>
          </div>
          )}

          <div className="border-t border-zinc-200 pt-2">
            <Row label="现金差额" value={`¥${(Number(cashAmount) / 100).toFixed(2)}`} bold />
          </div>
        </div>
      )}

      {!isRefund && pType !== 'appeal' && (
        <div className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <div className="text-sm font-black text-black mb-3">优惠码</div>
          <div style={{ display: 'flex', gap: 8 }}>
            <input
              type="text"
              value={couponCode}
              onChange={e => setCouponCode(e.target.value)}
              placeholder="输入优惠码（选填）"
              className="flex-1 border border-zinc-200 rounded-lg px-3 py-2 text-sm focus:outline-none"
            />
            <button
              style={{ backgroundColor: appliedCoupon ? '#f4f4f5' : '#915F38', color: appliedCoupon ? '#71717a' : '#fff', borderRadius: 10, padding: '0 16px', border: 'none', fontWeight: 600, fontSize: 13 }}
              onClick={applyCoupon}
            >
              {appliedCoupon ? '已应用' : '应用'}
            </button>
          </div>
          {appliedCoupon && (
            <div className="text-xs text-green-600 mt-2">{appliedCoupon.hint}</div>
          )}
          {appliedCoupon && (
            <Row label="优惠后金额" value={`¥${(Number(couponAmount) / 100).toFixed(2)}`} bold />
          )}
        </div>
      )}

      {isRefund && (
        <div className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <div className="text-sm font-bold text-black mb-2">退款说明</div>
          <div className="text-xs text-zinc-500">
            金额将在提交后原路退回至您的微信支付账户，预计 1-7 个工作日到账。
          </div>
        </div>
      )}

      <div className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4">
        {isRefund ? (
          data.details?.cancel_refund ? (
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                style={{ flex: 1, paddingTop: 14, paddingBottom: 14, backgroundColor: '#B98E5F', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16, border: 'none', outline: 'none' }}
                onClick={handleRefund}
              >
                确认退款 ¥{(Number(cashAmount) / 100).toFixed(2)}
              </button>
            </div>
          ) : (
          <button
            style={{ width: '100%', paddingTop: 14, paddingBottom: 14, backgroundColor: '#B98E5F', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16, border: 'none', outline: 'none' }}
            onClick={handleRefund}
          >
            确认退款 ¥{(Number(cashAmount) / 100).toFixed(2)}
          </button>
          )
        ) : pType === 'appeal' ? (
          <button
            style={{ width: '100%', paddingTop: 14, paddingBottom: 14, backgroundColor: '#16a34a', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16, border: 'none', outline: 'none' }}
            onClick={() => navigate(`/order/${pId}`)}
          >
            确认，查看订单详情
          </button>
        ) : (
          <button
            style={{ width: '100%', paddingTop: 14, paddingBottom: 14, color: '#fff', fontWeight: 900, fontSize: 16, borderRadius: 16, border: 'none', outline: 'none', backgroundColor: cashAmount > 0 ? '#B98E5F' : '#16a34a' }}
            onClick={() => doPay(cashAmount)}
          >
            {pType === 'damage' && cashAmount <= 0
              ? '无需支付 ¥0'
              : cashAmount > 0
                ? `微信支付 ¥${(Number(cashAmount) / 100).toFixed(2)}`
                : '确认支付 ¥0（使用点数）'}
          </button>
        )}
      </div>
    </div>
  )

  function applyCoupon() {
    const code = (couponCode || '').trim().toUpperCase()
    if (!code) { dialog.alert('请输入优惠码'); return }
    if (code === 'OREZ') {
      setAppliedCoupon({ code, hint: '已应用，全额免除' })
      setCouponAmount(0)
    } else if (code === 'ENO') {
      // #1728 P3：金额为分，ENO 千分比 10‰ = 1% → 分运算
      const base = data?.amount || 0
      const discounted = Math.round(base * 10 / 1000)
      setAppliedCoupon({ code, hint: '已应用，优惠后金额 ¥' + (discounted / 100).toFixed(2) })
      setCouponAmount(discounted)
    } else {
      dialog.alert('优惠码无效（仅支持 OREZ / ENO）')
    }
  }

  async function doPay(cashAmount) {
    if (cashAmount <= 0) {
      if (appliedCoupon) {
        // OREZ 全免：调 prepay 走后端 waive 记账
        try {
          const resp = await apiFetch(`${baseUrl}/pay/prepay`, {
            method: 'POST',
            body: JSON.stringify({
              order_id: pId,
              order_type: pType,
              amount: data.amount,
              gift_used: 0,
              coupon_code: appliedCoupon.code,
            }),
          })
          const result = await resp.json()
          if (result.code === 20000) { dialog.alert('支付成功'); navigate(`/success?order_id=${pId}`, { replace: true }) }
          else dialog.alert('支付失败: ' + resolveErrorMessage(result))
        } catch (err) { dialog.alert('支付失败: ' + err.message) }
        return
      }
      dialog.alert('支付成功')
      navigate(`/success?order_id=${pId}`, { replace: true })
      return
    }

    try {
      const resp = await apiFetch(`${baseUrl}/pay/prepay`, {
        method: 'POST',
        body: JSON.stringify({
          order_id: pId,
          order_type: pType,
          amount: appliedCoupon ? data.amount : cashAmount,
          gift_used: appliedCoupon ? 0 : giftUsed,
          ...(appliedCoupon ? { coupon_code: appliedCoupon.code } : {}),
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('支付失败: 暂不支持H5支付')
      } else {
        dialog.alert('支付失败: ' + resolveErrorMessage(result))
      }
    } catch (err) {
      dialog.alert('支付失败: ' + err.message)
    }
  }

  function handleRefund() {
    dialog.alert('退款申请已提交')
    navigate(-1)
  }

}

function Row({ label, value, color, bold, valueSize }) {
  return (
    <div className="flex justify-between py-1">
      <span className="text-[13px] text-zinc-500">{label}</span>
      <span className="text-[13px]" style={{ fontWeight: bold ? 700 : 500, color: color || '#000', fontSize: valueSize }}>
        {value}
      </span>
    </div>
  )
}

function renderDetailsBlock(details, type) {
  if (type === 'rent' && details.pricing_breakdown) {
    let pb
    try { pb = typeof details.pricing_breakdown === 'string' ? JSON.parse(details.pricing_breakdown) : details.pricing_breakdown } catch { pb = null }
    if (pb && pb.tier_segments) {
      return (
        <div>
          <span className="text-[13px] font-semibold text-zinc-600 mb-1">阶梯定价</span>
          {pb.tier_segments.map((seg, i) => (
            <div key={i} className="pl-4 pr-5">
              <Row label={`第${seg.tier}阶 ${seg.days}天`}
                value={`¥${(Number(seg.days * seg.rate) / 100).toFixed(2)}`} valueSize="11px" />
              {seg.discount < 1.0 && (
                <Row label="  折扣" value={`-¥${(Number(seg.days * seg.rate - seg.subtotal) / 100).toFixed(2)}`} color="#16a34a" valueSize="11px" />
              )}
            </div>
          ))}
          <Row label="租金小计" value={`¥${(Number(pb.total_amount || 0) / 100).toFixed(2)}`} bold />
          {details.deposit > 0 && <Row label="押金" value={`¥${(Number(details.deposit) / 100).toFixed(2)}`} />}
          {details.shipping_fee > 0 && <Row label="物流费" value={`¥${(Number(details.shipping_fee) / 100).toFixed(2)}`} />}
        </div>
      )
    }
  }
  if (type === 'repair' || type === 'requote') {
    const oldQ = details.old_quote
    return (
      <div>
        {type === 'requote' && oldQ && (
          <div className="opacity-50 mb-1">
            <Row label="原报价（材料费）" value={`¥${(Number(oldQ.material_fee || 0) / 100).toFixed(2)}`} />
            <Row label="原报价（服务费）" value={`¥${(Number(oldQ.service_fee || 0) / 100).toFixed(2)}`} />
            <Row label="原报价（物流费）" value={`¥${(Number(oldQ.logistics_fee || 0) / 100).toFixed(2)}`} />
            <Row label="原报价合计" value={`¥${(Number(oldQ.total || 0) / 100).toFixed(2)}`} bold />
          </div>
        )}
        <Row label="材料费" value={`¥${(Number(details.material_fee || 0) / 100).toFixed(2)}`} />
        <Row label="服务费" value={`¥${(Number(details.service_fee || 0) / 100).toFixed(2)}`} />
        <Row label="物流费" value={`¥${(Number(details.logistics_fee || 0) / 100).toFixed(2)}`} />
        {type === 'requote' && oldQ && (
          <Row label="需补付" value={`+¥${(Math.max(0, Number(details.total || 0)) / 100).toFixed(2)}`} bold color="#dc2626" />
        )}
      </div>
    )
  }
  if (type === 'damage') {
    const pb = details.paid_breakdown || {}
    return (
      <div>
        <div className="opacity-50">
          <Row label="租金小计" value={`¥${(Number(pb.rent_subtotal || 0) / 100).toFixed(2)}`} />
          <Row label="押金" value={`¥${(Number(pb.deposit || 0) / 100).toFixed(2)}`} />
          <Row label="物流费" value={`¥${(Number(pb.shipping_fee || 0) / 100).toFixed(2)}`} />
          <Row label="已付合计" value={`¥${(Number(pb.paid_total || 0) / 100).toFixed(2)}`} bold />
        </div>
        <div className="border-t border-zinc-100 pt-2 mt-1">
          <Row label="损失评估" value={`¥${(Number(details.damage_amount || 0) / 100).toFixed(2)}`} />
          <Row label="押金抵扣" value={`-¥${(Number(details.deposit_deduction || 0) / 100).toFixed(2)}`} />
          <Row label="需补付" value={`¥${(Number(details.pay_amount || 0) / 100).toFixed(2)}`} bold color="#dc2626" />
        </div>
      </div>
    )
  }
  if (type === 'appeal') {
    // Appeal resolution receipt (#1576).
    return (
      <div>
        <Row label="实际租期" value={`${Number(details.actual_rent_days || 0)} 天`} />
        <Row label="实际租金" value={`¥${(Number(details.actual_rent_amount || 0) / 100).toFixed(2)}`} />
        {Number(details.damage_deducted || 0) > 0 && (
          <Row label="损坏扣款" value={`-¥${(Number(details.damage_deducted) / 100).toFixed(2)}`} color="#dc2626" />
        )}
        {Number(details.overdue_fee || 0) > 0 && (
          <Row label="逾期费用" value={`-¥${(Number(details.overdue_fee) / 100).toFixed(2)}`} color="#dc2626" />
        )}
        <div className="border-t border-zinc-100 pt-2 mt-1">
          <Row label="退款金额" value={`¥${(Number(details.cash_refundable || 0) / 100).toFixed(2)}`} bold color="#3b82f6" />
        </div>
      </div>
    )
  }
  if (type === 'payment_shortfall') {
    // #1748 L-04C 流程 3: 补缴支付确认页明细（全部来自服务端 calculate）。
    return (
      <div>
        <Row label="实际租金" value={`¥${(Number(details.rent || 0) / 100).toFixed(2)}`} />
        {Number(details.shipping_fee || 0) > 0 && (
          <Row label="物流费" value={`¥${(Number(details.shipping_fee) / 100).toFixed(2)}`} />
        )}
        {Number(details.overdue_fee || 0) > 0 && (
          <Row label="逾期费" value={`¥${(Number(details.overdue_fee) / 100).toFixed(2)}`} />
        )}
        {Number(details.damage_amount || 0) > 0 && (
          <Row label="损坏赔偿" value={`¥${(Number(details.damage_amount) / 100).toFixed(2)}`} />
        )}
        <Row label="已付总额" value={`¥${(Number(details.paid_total || 0) / 100).toFixed(2)}`} />
        <div className="border-t border-zinc-100 pt-2 mt-1">
          <Row label="需补缴" value={`¥${(Number(details.shortfall_amount || 0) / 100).toFixed(2)}`} bold color="#dc2626" />
        </div>
      </div>
    )
  }
  return null
}
