import { useState, useEffect } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { env, dialog } from '../platform'
import { formatDisplayDate } from '../utils/format'

const baseUrl = env.apiBaseUrl

export default function Payment() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const pType = searchParams.get('type') || ''
  const pId = searchParams.get('id') || ''

  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [prepaidUsed, setPrepaidUsed] = useState(0)
  const [giftUsed, setGiftUsed] = useState(0)

  useEffect(() => {
    if (!pType) return
    const fetchData = async () => {
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
  const maxPrepaid = wallet.prepaid_points || 0
  const maxGift = Math.min(wallet.promo_points || 0, wallet.max_gift_amount || 0)

  const isRefund = ['refund', 'deposit-refund'].includes(pType)

  const cashAmount = isRefund
    ? data.amount
    : Math.max(0, data.amount - prepaidUsed - giftUsed)

  return (
    <div className="min-h-screen bg-[#FDFBF7] pb-[100px]">
      <div className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 py-3 flex items-center">
        <span className="text-xl font-bold text-black cursor-pointer" onClick={() => navigate(-1)}>❮</span>
        <span className="text-lg font-bold flex-1 text-center">
          {isRefund ? '退款确认' : '支付确认'}
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
                <Row label="合计" value={`¥${Number(data.amount).toFixed(2)}`} bold />
              </div>
            )}
          </>
        )}

        {isRefund && (
          <div className="mt-2">
            {data.details?.cash_refundable !== undefined && (
              <Row label="可退现金" value={`¥${Number(data.details.cash_refundable).toFixed(2)}`} />
            )}
            {data.details?.prepaid_refunded !== undefined && Number(data.details.prepaid_refunded) > 0 && (
              <Row label="预付点退回" value={`+¥${Number(data.details.prepaid_refunded).toFixed(2)}`} color="#16a34a" />
            )}
            {data.details?.gift_refunded !== undefined && Number(data.details.gift_refunded) > 0 && (
              <Row label="赠点退回" value={`+¥${Number(data.details.gift_refunded).toFixed(2)}`} color="#16a34a" />
            )}
            <Row label="退款金额" value={`¥${Number(data.amount).toFixed(2)}`} bold />
          </div>
        )}

        {!isRefund && (
          <div className="border-t border-zinc-200 mt-2 pt-2">
            <Row label="应付金额" value={`¥${Number(data.amount).toFixed(2)}`} bold />
          </div>
        )}
      </div>

      {!isRefund && data.amount > 0 && (
        <div className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <div className="text-sm font-bold text-black mb-3">点数使用</div>

          <div className="mb-3">
            <Row label="预付点余额" value={`¥${Number(maxPrepaid).toFixed(2)}`} />
            <div className="flex items-center mt-1">
              <span className="text-xs text-zinc-500 w-[72px]">使用</span>
              {maxPrepaid > 0 ? (
                <div className="flex-1 flex items-center gap-2">
                  <input type="range" min={0} max={Math.min(maxPrepaid, data.amount)} step={1}
                    value={prepaidUsed}
                    onChange={e => setPrepaidUsed(parseInt(e.target.value) || 0)}
                    className="flex-1"
                  />
                  <span className="text-xs text-zinc-600 w-12 text-right">{prepaidUsed}</span>
                </div>
              ) : (
                <input className="flex-1 border border-zinc-200 rounded-lg px-2 py-1 text-xs text-right text-zinc-300 bg-zinc-50"
                  value={0} disabled readOnly />
              )}
              <span className="text-xs text-zinc-500 ml-1">点</span>
            </div>
          </div>

          <div className="mb-1">
            <Row label="赠点余额" value={`¥${Number(maxGift).toFixed(2)}`} />
            <div className="flex items-center mt-1">
              <span className="text-xs text-zinc-500 w-[72px]">使用</span>
              {maxGift > 0 ? (
                <div className="flex-1 flex items-center gap-2">
                  <input type="range" min={0} max={Math.min(maxGift, data.amount)} step={1}
                    value={giftUsed}
                    onChange={e => setGiftUsed(parseInt(e.target.value) || 0)}
                    className="flex-1"
                  />
                  <span className="text-xs text-zinc-600 w-12 text-right">{giftUsed}</span>
                </div>
              ) : (
                <input className="flex-1 border border-zinc-200 rounded-lg px-2 py-1 text-xs text-right text-zinc-300 bg-zinc-50"
                  value={0} disabled readOnly />
              )}
              <span className="text-xs text-zinc-500 ml-1">点</span>
            </div>
          </div>
          <div className="text-[11px] text-zinc-400 text-right mb-2">
            赠点上限 = min(赠点余额, floor(应付金额 × {Math.round((wallet.max_gift_ratio || 0.3) * 100)}%)) = {Number(maxGift).toFixed(2)}
          </div>

          <div className="border-t border-zinc-200 pt-2">
            <Row label="现金差额" value={`¥${Number(cashAmount).toFixed(2)}`} bold />
          </div>
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
          <button
            className="w-full py-3.5 bg-[#B98E5F] text-white font-bold text-base rounded-2xl"
            onClick={handleRefund}
          >
            确认退款 ¥{Number(cashAmount).toFixed(2)}
          </button>
        ) : (
          <button
            className={`w-full py-3.5 text-white font-bold text-base rounded-2xl ${
              cashAmount > 0 ? 'bg-[#B98E5F]' : 'bg-green-600'
            }`}
            onClick={() => doPay(cashAmount)}
          >
            {cashAmount > 0 ? `微信支付 ¥${Number(cashAmount).toFixed(2)}` : '确认支付 ¥0（使用点数）'}
          </button>
        )}
      </div>
    </div>
  )

  async function doPay(cashAmount) {
    if (cashAmount <= 0) {
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
          amount: cashAmount,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        const d = result.data
        if (d.mock) {
          dialog.alert('支付成功（测试）')
          navigate(`/success?order_id=${pId}`, { replace: true })
        } else {
          dialog.alert('支付失败: 暂不支持H5支付')
        }
      } else {
        dialog.alert('支付失败: ' + result.message)
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

function Row({ label, value, color, bold }) {
  return (
    <div className="flex justify-between py-1">
      <span className="text-[13px] text-zinc-500">{label}</span>
      <span className="text-[13px]" style={{ fontWeight: bold ? 700 : 500, color: color || '#000' }}>
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
            <div key={i} className="pl-4">
              <Row label={`第${seg.tier}阶 ${seg.days}天`}
                value={`¥${Number(seg.days * seg.rate).toFixed(2)}`} />
              {seg.discount < 1.0 && (
                <Row label="  折扣" value={`-¥${Number(seg.days * seg.rate - seg.subtotal).toFixed(2)}`} color="#16a34a" />
              )}
            </div>
          ))}
          <Row label="租金小计" value={`¥${Number(pb.total_amount || 0).toFixed(2)}`} bold />
          {details.deposit > 0 && <Row label="押金" value={`¥${Number(details.deposit).toFixed(2)}`} />}
          {details.shipping_fee > 0 && <Row label="物流费" value={`¥${Number(details.shipping_fee).toFixed(2)}`} />}
        </div>
      )
    }
  }
  if (type === 'repair' || type === 'requote') {
    return (
      <div>
        <Row label="材料费" value={`¥${Number(details.material_fee || 0).toFixed(2)}`} />
        <Row label="服务费" value={`¥${Number(details.service_fee || 0).toFixed(2)}`} />
        <Row label="物流费" value={`¥${Number(details.logistics_fee || 0).toFixed(2)}`} />
      </div>
    )
  }
  if (type === 'damage') {
    return (
      <div>
        <Row label="定损金额" value={`¥${Number(details.damage_amount || 0).toFixed(2)}`} />
        <Row label="押金抵扣" value={`-¥${Number(details.deposit || 0).toFixed(2)}`} />
        <Row label="需支付" value={`¥${Number(details.pay_amount || 0).toFixed(2)}`} bold />
      </div>
    )
  }
  return null
}
