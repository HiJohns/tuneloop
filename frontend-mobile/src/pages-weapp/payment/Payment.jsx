import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Input, Slider } from '@tarojs/components'
import { apiFetch } from '../../services/api'
import { env } from '../../platform'
import { formatDisplayDate } from '../../utils/format'

const baseUrl = env.apiBaseUrl

export default function Payment() {
  const params = Taro.getCurrentInstance().router?.params || {}
  const pType = params.type || ''
  const pId = params.id || ''
  const pAmount = parseFloat(params.amount || '0')

  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [prepaidUsed, setPrepaidUsed] = useState(0)
  const [giftUsed, setGiftUsed] = useState(0)
  const [prepayData, setPrepayData] = useState(null)
  const [isPaying, setIsPaying] = useState(false)

  useEffect(() => {
    if (!pType) return
    const fetchData = async () => {
      if (pType === 'points') {
        setData({ type: 'points', title: '预付点充值', amount: pAmount, details: null, wallet: null })
        setLoading(false)
        return
      }
      if (pType === 'renewal') {
        setData({ type: 'renewal', title: '续期支付', amount: pAmount, details: null, wallet: null })
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
    return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#FDFBF7' }}>
      <Text style={{ color: '#a1a1aa' }}>加载中...</Text>
    </View>
  }
  if (!data) {
    return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#FDFBF7' }}>
      <Text style={{ color: '#a1a1aa' }}>支付数据不存在</Text>
    </View>
  }

  const wallet = data.wallet || {}
  const maxPrepaid = wallet.prepaid_points || 0
  const maxGift = Math.min(wallet.promo_points || 0, wallet.max_gift_amount || 0)

  const isRefund = ['refund', 'deposit-refund'].includes(pType)

  const cashAmount = isRefund
    ? data.amount
    : Math.max(0, data.amount - prepaidUsed - giftUsed)

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', paddingBottom: 100 }}>
      <View style={{ background: 'linear-gradient(to bottom, #FDF4E7, #fff)', padding: '16px', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text style={{ fontSize: 18, fontWeight: '700', flex: 1, textAlign: 'center' }}>
          {isRefund ? '退款确认' : '支付确认'}
        </Text>
      </View>

      <ScrollView style={{ width: '100%' }}>
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>{data.title}</Text>

          {/* Fee details */}
          {data.details && (
            <>
              {renderDetailsBlock(data.details, data.type)}
              {data.type === 'rent' && data.details.pricing_breakdown && (
                <View style={{ borderTop: '1px solid #f4f4f5', marginTop: 8, paddingTop: 8 }}>
                  <Row label="合计" value={`¥${Number(data.amount).toFixed(2)}`} bold />
                </View>
              )}
            </>
          )}

          {/* Refund details */}
          {isRefund && (
            <View style={{ marginTop: 8 }}>
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
            </View>
          )}
        </View>

        {/* Points usage (only for non-refund) */}
        {!isRefund && pType !== 'points' && data.amount > 0 && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>点数使用</Text>

            <View style={{ marginBottom: 12 }}>
              <Row label="预付点余额" value={`¥${Number(maxPrepaid).toFixed(2)}`} />
              <View style={{ display: 'flex', alignItems: 'center', marginTop: 4 }}>
                <Text style={{ fontSize: 13, color: '#71717a', width: 72 }}>使用</Text>
                {maxPrepaid > 0 ? (
                  <View style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 4 }}>
                    <Slider min={0} max={Math.min(maxPrepaid, data.amount)} step={1}
                      value={prepaidUsed} style={{ flex: 1, margin: 0, padding: 0 }}
                      onChange={e => setPrepaidUsed(e.detail.value)}
                    />
                    <Text style={{ fontSize: 13, color: '#52525b', width: 48, textAlign: 'right' }}>{prepaidUsed}</Text>
                  </View>
                ) : (
                  <Input style={{ flex: 1, border: '1px solid #e4e4e7', borderRadius: 8, padding: '4px 8px', fontSize: 13, textAlign: 'right', color: '#d4d4d8', backgroundColor: '#fafafa' }}
                    value="0" disabled />
                )}
                <Text style={{ fontSize: 13, color: '#71717a', marginLeft: 4 }}>点</Text>
              </View>
            </View>

            <View style={{ marginBottom: 4 }}>
              <Row label="赠点余额" value={`¥${Number(maxGift).toFixed(2)}`} />
              <View style={{ display: 'flex', alignItems: 'center', marginTop: 4 }}>
                <Text style={{ fontSize: 13, color: '#71717a', width: 72 }}>使用</Text>
                {maxGift > 0 ? (
                  <View style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 4 }}>
                    <Slider min={0} max={Math.min(maxGift, data.amount)} step={1}
                      value={giftUsed} style={{ flex: 1, margin: 0, padding: 0 }}
                      onChange={e => setGiftUsed(e.detail.value)}
                    />
                    <Text style={{ fontSize: 13, color: '#52525b', width: 48, textAlign: 'right' }}>{giftUsed}</Text>
                  </View>
                ) : (
                  <Input style={{ flex: 1, border: '1px solid #e4e4e7', borderRadius: 8, padding: '4px 8px', fontSize: 13, textAlign: 'right', color: '#d4d4d8', backgroundColor: '#fafafa' }}
                    value="0" disabled />
                )}
                <Text style={{ fontSize: 13, color: '#71717a', marginLeft: 4 }}>点</Text>
              </View>
            </View>
            <Text style={{ fontSize: 11, color: '#a1a1aa', textAlign: 'right', marginBottom: 8 }}>
              赠点上限 = min(赠点余额, floor(应付金额 × {Math.round((wallet.max_gift_ratio || 0.3) * 100)}%)) = {Number(maxGift).toFixed(2)}
            </Text>

            <View style={{ borderTop: '1px solid #e4e4e7', paddingTop: 8 }}>
              <Row label="现金差额" value={`¥${Number(cashAmount).toFixed(2)}`} bold />
            </View>
          </View>
        )}

        {/* Refund summary */}
        {isRefund && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 8 }}>退款说明</Text>
            <Text style={{ fontSize: 13, color: '#71717a' }}>
              金额将在提交后原路退回至您的微信支付账户，预计 1-7 个工作日到账。
            </Text>
          </View>
        )}

        {/* Bottom padding for button */}
        <View style={{ height: 80 }} />
      </ScrollView>

      {/* Pay/Confirm button */}
      <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, backgroundColor: '#fff', borderTop: '1px solid #f4f4f5', padding: 16 }}>
        {isRefund ? (
          <Button style={btnStyle('#000')} onClick={handleRefund}>确认退款 ¥{Number(cashAmount).toFixed(2)}</Button>
        ) : prepayData?.data ? (
          <View style={{ display: 'flex', flexDirection: 'row', gap: 12 }}>
            <View style={{ flex: 1 }}>
              <Button style={btnStyle('#C21838')} onClick={doRealPay}>
                微信支付 ¥{Number(cashAmount).toFixed(2)}
              </Button>
            </View>
            {!prepayData.mock && (
              <View style={{ flex: 1 }}>
                <Button style={{ ...btnStyle('#fef3c7'), color: '#92400e' }} onClick={doSimulatePay}>
                  模拟支付 ¥{Number(cashAmount).toFixed(2)}
                </Button>
              </View>
            )}
          </View>
        ) : (
          <Button style={btnStyle(cashAmount > 0 ? '#C21838' : '#16a34a')} onClick={() => handlePay(cashAmount)} disabled={isPaying}>
            {isPaying ? '处理中...' : `发起支付 ¥${Number(cashAmount).toFixed(2)}`}
          </Button>
        )}
      </View>
    </View>
  )
}

function Button({ children, onClick, style }) {
  return (
    <View
      onClick={onClick}
      style={{
        width: '100%', padding: '14px 0', borderRadius: 16, fontWeight: '700', fontSize: 15,
        textAlign: 'center', color: '#fff', cursor: 'pointer', ...style,
      }}
    >
      {children}
    </View>
  )
}

function Row({ label, value, color, bold, valueSize }) {
  return (
    <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4 }}>
      <Text style={{ fontSize: 13, color: '#71717a' }}>{label}</Text>
      <Text style={{ fontSize: valueSize || 13, fontWeight: bold ? '700' : '500', color: color || '#000' }}>{value}</Text>
    </View>
  )
}

function renderDetailsBlock(details, type) {
  if (type === 'rent' && details.pricing_breakdown) {
    let pb
    try { pb = typeof details.pricing_breakdown === 'string' ? JSON.parse(details.pricing_breakdown) : details.pricing_breakdown } catch { pb = null }
    if (pb && pb.tier_segments) {
      return (
        <View>
          <Text style={{ fontSize: 13, fontWeight: '600', color: '#52525b', marginBottom: 4 }}>阶梯定价</Text>
          {pb.tier_segments.map((seg, i) => (
            <View key={i} style={{ paddingLeft: 16, paddingRight: 36 }}>
              <Row label={`第${seg.tier}阶 ${seg.days}天`} value={`¥${Number(seg.days * seg.rate).toFixed(2)}`} valueSize={11} />
              {seg.discount < 1.0 && (
                <Row label="  折扣" value={`-¥${Number(seg.days * seg.rate - seg.subtotal).toFixed(2)}`} color="#16a34a" valueSize={11} />
              )}
            </View>
          ))}
          <Row label="租金小计" value={`¥${Number(pb.total_amount || 0).toFixed(2)}`} bold />
          {details.deposit > 0 && <Row label="押金" value={`¥${Number(details.deposit).toFixed(2)}`} />}
          {details.shipping_fee > 0 && <Row label="物流费" value={`¥${Number(details.shipping_fee).toFixed(2)}`} />}
        </View>
      )
    }
  }
  if (type === 'repair' || type === 'requote') {
    return (
      <View>
        <Row label="材料费" value={`¥${Number(details.material_fee || 0).toFixed(2)}`} />
        <Row label="服务费" value={`¥${Number(details.service_fee || 0).toFixed(2)}`} />
        <Row label="物流费" value={`¥${Number(details.logistics_fee || 0).toFixed(2)}`} />
      </View>
    )
  }
  if (type === 'damage') {
    const pb = details.paid_breakdown || {}
    return (
      <View>
        <View style={{ opacity: 0.5 }}>
          <Row label="租金小计" value={`¥${Number(pb.rent_subtotal || 0).toFixed(2)}`} />
          <Row label="押金" value={`¥${Number(pb.deposit || 0).toFixed(2)}`} />
          <Row label="物流费" value={`¥${Number(pb.shipping_fee || 0).toFixed(2)}`} />
          <Row label="已付合计" value={`¥${Number(pb.paid_total || 0).toFixed(2)}`} bold />
        </View>
        <View style={{ borderTop: '1px solid #f4f4f5', paddingTop: 8, marginTop: 4 }}>
          <Row label="损失评估" value={`¥${Number(details.damage_amount || 0).toFixed(2)}`} />
          <Row label="押金抵扣" value={`-¥${Number(details.deposit_deduction || 0).toFixed(2)}`} />
          <Row label="需补付" value={`¥${Number(details.pay_amount || 0).toFixed(2)}`} bold color="#dc2626" />
        </View>
      </View>
    )
  }
  return null
}

async function handlePay(cashAmount) {
  const params = Taro.getCurrentInstance().router?.params || {}
  const pType = params.type || ''
  const pId = params.id || ''

  if (cashAmount <= 0) {
    Taro.showToast({ title: '支付成功', icon: 'success' })
    setTimeout(() => Taro.redirectTo({ url: '/pages-weapp/home/index' }), 2000)
    return
  }

  setIsPaying(true)
  try {
    // Get WeChat OpenID for JSAPI payment
    let openid = ''
    try {
      const loginRes = await Taro.login()
      if (loginRes.code) {
        const oidResp = await apiFetch(`${baseUrl}/wechat/openid`, {
          method: 'POST',
          body: JSON.stringify({ code: loginRes.code }),
        })
        const oidData = await oidResp.json()
        if (oidData.code === 20000) openid = oidData.data.openid
      }
    } catch (e) { console.warn('[payment] openid lookup failed', e) }

    const resp = await apiFetch(`${baseUrl}/pay/prepay`, {
      method: 'POST',
      body: JSON.stringify({
        order_id: pId,
        order_type: pType,
        amount: cashAmount,
        open_id: openid,
      }),
    })
    const result = await resp.json()
    if (result.code === 20000) {
      const d = result.data
      if (d.mock) {
        Taro.showToast({ title: '支付成功（测试）', icon: 'success' })
        setTimeout(() => Taro.redirectTo({ url: `/pages-weapp/success/index?order_id=${pId}` }), 2000)
      } else if (d.data?.prepay_id) {
        setPrepayData(d)
      } else {
        Taro.showModal({ title: '支付失败', content: '无法获取支付参数', showCancel: false })
      }
    } else {
      Taro.showModal({ title: '支付失败', content: result.message, showCancel: false })
    }
  } catch (err) {
    Taro.showModal({ title: '支付失败', content: err.message, showCancel: false })
  } finally {
    setIsPaying(false)
  }
}

async function doRealPay() {
  if (!prepayData?.data) return
  Taro.requestPayment({
    appId: prepayData.data.app_id || 'wxcb44a1be70e356ed',
    timeStamp: prepayData.data.time_stamp,
    nonceStr: prepayData.data.nonce_str,
    package: prepayData.data.package,
    signType: prepayData.data.sign_type,
    paySign: prepayData.data.pay_sign,
    success: () => {
      Taro.showToast({ title: '支付成功', icon: 'success' })
      setTimeout(() => Taro.redirectTo({ url: `/pages-weapp/success/index?order_id=${params.id}` }), 2000)
    },
    fail: (err) => Taro.showModal({ title: '支付失败', content: err.errMsg || '请重试', showCancel: false }),
  })
}

async function doSimulatePay() {
  if (!prepayData?.data) return
  const resp = await apiFetch(`${baseUrl}/pay/test-callback`, {
    method: 'POST',
    body: JSON.stringify({ out_trade_no: prepayData.data.out_trade_no }),
  })
  const r = await resp.json()
  if (r.code === 20000) {
    Taro.showToast({ title: '测试支付已提交', icon: 'success' })
    setTimeout(() => Taro.redirectTo({ url: `/pages-weapp/success/index?order_id=${params.id}` }), 2000)
  } else {
    Taro.showModal({ title: '测试支付失败', content: r.message, showCancel: false })
  }
}

async function handleRefund() {
  Taro.showToast({ title: '退款申请已提交', icon: 'success' })
  setTimeout(() => Taro.navigateBack(), 2000)
}

function btnStyle(bgColor) {
  return { width: '100%', padding: '14px 0', borderRadius: 16, fontWeight: '700', fontSize: 15, textAlign: 'center', color: '#fff', backgroundColor: bgColor, cursor: 'pointer' }
}
