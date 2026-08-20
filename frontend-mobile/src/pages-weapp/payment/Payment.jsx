import { useState, useEffect, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Input } from '@tarojs/components'
import { apiFetch, resolveLogin } from '../../services/api'
import { env, session } from '../../platform'
import { formatDisplayDate } from '../../utils/format'

const baseUrl = env.apiBaseUrl

function clampPoints(raw, max) {
  const v = parseInt(raw, 10)
  if (Number.isNaN(v)) return 0
  return Math.min(Math.max(0, v), Math.floor(max))
}

export default function Payment() {
  const params = Taro.getCurrentInstance().router?.params || {}
  const pType = params.type || ''
  const pId = params.id || ''
  const pAmount = parseFloat(params.amount || '0')
  const pSessionId = params.session_id || ''

  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [giftUsed, setGiftUsed] = useState(0)
  const [prepayData, setPrepayData] = useState(null)
  const [isPaying, setIsPaying] = useState(false)
  const [maxPayRatio, setMaxPayRatio] = useState(0.3)
  // 优惠码预览（#1719 通用化）：所有支付页可用，最终金额服务端重算。
  const [couponCode, setCouponCode] = useState('')
  const [appliedCoupon, setAppliedCoupon] = useState(null)
  const [couponAmount, setCouponAmount] = useState(pAmount)
  // #1682: sync ref — weapp controlled Input setState is async; clicking
  // 应用 right after typing reads the stale state. The ref always holds the
  // latest input value at click time.
  const couponInputRef = useRef('')


  useEffect(() => {
    if (!pType) return
    const fetchData = async () => {
      if (pType === 'renewal') {
        setData({ type: 'renewal', title: '续期支付', amount: pAmount, details: null, wallet: null })
        setLoading(false)
        return
      }
      if (pType === 'membership') {
        setData({ type: 'membership', title: '会员入会费', amount: pAmount, details: { items: [{ label: 'VIP 会员注册', amount: pAmount }] }, wallet: null })
        setLoading(false)
        // Fetch max_pay_ratio for the benefits note (#1575).
        try {
          const resp = await apiFetch(`${baseUrl}/user/points`)
          const result = await resp.json()
          if (result.code === 20000) setMaxPayRatio(result.data?.max_pay_ratio ?? 0.3)
        } catch {}
        return
      }
      if (pType === 'appeal') {
        // Appeal resolution receipt: load the settled refund from the
        // appeal outcome (#1576).
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
  const maxGift = Math.min(wallet.promo_points || 0, wallet.max_gift_amount || 0)

  const isRefund = ['refund', 'deposit-refund'].includes(pType)

  // Coupon applies a preview discount (final price is server-authoritative at
  // prepay); OREZ waives the fee entirely (¥0).
  const displayAmount = pType === 'membership' && appliedCoupon ? couponAmount : data.amount

  const cashAmount = isRefund
    ? data.amount
    : Math.max(0, displayAmount - giftUsed)

  const applyCoupon = () => {
    const tryApply = () => {
      const rawInput = couponInputRef.current
      const rawState = couponCode
      const code = (rawInput || rawState || '').trim().toUpperCase()
      // #1682 debug: first-click coupon may not apply — log the exact input
      // state at click time (ref vs state race) and the resulting branch.
      console.warn('[payment/coupon] applyCoupon click', {
        rawInput,
        rawState,
        code,
        pAmount,
        appliedBefore: !!appliedCoupon,
        ts: Date.now(),
      })
      if (!code) { Taro.showToast({ title: '请输入优惠码', icon: 'none' }); return }
      if (code === 'OREZ') {
        setAppliedCoupon({ code, hint: '已应用，会员费全额免除' })
        setCouponAmount(0)
      } else if (code === 'ENO') {
        setAppliedCoupon({ code, hint: '已应用，优惠后金额 ¥' + Number(pAmount * 0.01).toFixed(2) })
        setCouponAmount(Math.round(pAmount * 0.01 * 100) / 100)
      } else {
        Taro.showToast({ title: '优惠码无效（仅支持 OREZ / ENO）', icon: 'none' })
      }
    }
    // Fast typing + immediate tap: the last input event may still be in
    // flight when the tap is handled (ref/state both empty). Retry once
    // after 150ms — the input event lands well within that window.
    if (!couponInputRef.current && !couponCode) {
      console.warn('[payment/coupon] input state empty at click, retrying in 150ms', { ts: Date.now() })
      setTimeout(tryApply, 150)
      return
    }
    tryApply()
  }

  // Two-phase registration (#1663): after the membership fee is paid the
  // account is created by the payment callback. Poll the session until it is
  // completed, then log in via wx-login-select and land on the profile page.
  const finishMembershipFlow = async () => {
    if (!pSessionId) return
    let completed = false
    for (let i = 0; i < 10; i++) {
      try {
        const resp = await apiFetch(`${baseUrl}/auth/registration-sessions/${pSessionId}/status`, { auth: false })
        const r = await resp.json()
        if (r.code === 20000 && r.data?.status === 'completed') { completed = true; break }
      } catch (e) { console.warn('[payment] session status poll failed', e) }
      await new Promise(res => setTimeout(res, 1000))
    }
    session.removeItem('pending_registration_session')
    if (completed) {
      Taro.showToast({ title: '注册成功，正在登录...', icon: 'none', duration: 1500 })
      const ok = await resolveLogin('profile')
      // #1690: 来源 B（立即租赁/购物车支付）注册完成后回跳原页面
      const redirect = session.getItem('post_auth_redirect')
      if (redirect) {
        session.removeItem('post_auth_redirect')
        // H5 短路径 → weapp 完整路径（/checkout?id=x → /pages-weapp/checkout/index?id=x）
        const [path, query] = redirect.split('?')
        const weappUrl = `/pages-weapp${path}/index${query ? '?' + query : ''}`
        Taro.redirectTo({ url: weappUrl })
        return
      }
      if (ok) Taro.switchTab({ url: '/pages-weapp/profile/index' })
      else Taro.switchTab({ url: '/pages-weapp/home/index' })
    } else {
      Taro.showModal({ title: '注册处理中', content: '会员费已支付，账户正在创建，请稍后返回首页刷新。', showCancel: false })
      setTimeout(() => Taro.switchTab({ url: '/pages-weapp/home/index' }), 2000)
    }
  }

  const doPrepay = async (amountOverride) => {
    // openid 由后端解析（#1678）：membership 两阶段流程创建 session 时已
    // 解析并存储微信身份，prepay 无需前端传递 open_id。
    // 优惠码（#1719 通用化，所有支付类型）：有优惠码时提交原金额 + coupon_code，
    // 最终金额由后端服务端重算（OREZ → 0 走 waive 记账，ENO → 1%）；优惠码
    // 场景不叠加赠点抵扣（gift_used=0）。
    const payAmount = appliedCoupon
      ? (data?.amount || 0)
      : (pType === 'membership'
        ? (data?.amount || 0)
        : (amountOverride !== undefined ? amountOverride : cashAmount))
    const body = {
      order_id: pId,
      order_type: pType,
      amount: payAmount,
      open_id: '',
      gift_used: appliedCoupon ? 0 : (pType === 'membership' ? 0 : giftUsed),
    }
    if (pType === 'membership' && pSessionId) {
      body.session_id = pSessionId
    }
    if (appliedCoupon) {
      body.coupon_code = appliedCoupon.code
    }
    // #1682 debug: what exactly reaches the server on prepay
    console.warn('[payment/prepay] body', {
      ...body,
      appliedCoupon,
      couponCode,
      couponAmount,
      displayAmount: appliedCoupon ? couponAmount : data?.amount,
      ts: Date.now(),
    })
    const resp = await apiFetch(`${baseUrl}/pay/prepay`, {
      method: 'POST',
      body: JSON.stringify(body),
      auth: false, // #1681: anonymous — no account exists during registration
    })
    return resp.json()
  }

  const handlePay = async (cashAmount) => {
    const params = Taro.getCurrentInstance().router?.params || {}
    const pType = params.type || ''
    const pId = params.id || ''

    if (cashAmount <= 0) {
      if (appliedCoupon || (pType === 'membership' && pSessionId)) {
        // OREZ full waiver (or membership session waiver): run prepay so
        // the server books the paid record and applies side effects
        // (#1719 通用化：任意支付类型的 waive 优惠码都走此路径).
        setIsPaying(true)
        try {
          const result = await doPrepay()
          if (result.code === 20000) {
            if (pType === 'membership' && pSessionId) {
              Taro.showToast({ title: '会员已激活，赠点已到账', icon: 'success' })
              setTimeout(finishMembershipFlow, 2000)
            } else {
              Taro.showToast({ title: '支付成功', icon: 'success' })
              setTimeout(() => Taro.redirectTo({ url: `/pages-weapp/success/index?order_id=${pId}` }), 2000)
            }
          } else {
            Taro.showModal({ title: '支付失败', content: result.message, showCancel: false })
          }
        } catch (err) {
          Taro.showModal({ title: '支付失败', content: err.message, showCancel: false })
        } finally {
          setIsPaying(false)
        }
        return
      }
      Taro.showToast({ title: pType === 'membership' ? '会员已激活，赠点已到账' : '支付成功', icon: 'success' })
      setTimeout(() => Taro.switchTab({ url: '/pages-weapp/home/index' }), 2000)
      return
    }

    setIsPaying(true)
    try {
      const result = await doPrepay()
      if (result.code === 20000) {
        const d = result.data
        if (d.data?.prepay_id) {
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

  const doRealPay = async () => {
    if (!prepayData?.data) return
    Taro.requestPayment({
      appId: prepayData.data.app_id || 'wxcb44a1be70e356ed',
      timeStamp: prepayData.data.time_stamp,
      nonceStr: prepayData.data.nonce_str,
      package: prepayData.data.package,
      signType: prepayData.data.sign_type,
      paySign: prepayData.data.pay_sign,
      success: () => {
        if (pType === 'membership' && pSessionId) {
          Taro.showToast({ title: '支付成功，注册处理中', icon: 'none', duration: 1500 })
          setTimeout(finishMembershipFlow, 1500)
        } else {
          Taro.showToast({ title: params.type === 'membership' ? '会员已激活，赠点已到账' : '支付成功', icon: 'success' })
          setTimeout(() => Taro.redirectTo({ url: `/pages-weapp/success/index?order_id=${params.id}` }), 2000)
        }
      },
      fail: (err) => Taro.showModal({ title: '支付失败', content: err.errMsg || '请重试', showCancel: false }),
    })
  }

  const handleRefund = async () => {
    Taro.showToast({ title: '退款申请已提交', icon: 'success' })
    setTimeout(() => Taro.navigateBack(), 2000)
  }

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', paddingBottom: 100 }}>
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
              {data.details?.cancel_refund && (
                <>
                  <Text style={{ fontSize: 12, color: '#a1a1aa', fontWeight: '700', marginTop: 8, marginBottom: 4 }}>原支付明细</Text>
                  {data.details.total_paid !== undefined && (
                    <Row label="原支付总额" value={`¥${Number(data.details.total_paid).toFixed(2)}`} />
                  )}
                  {data.details.cash_paid !== undefined && Number(data.details.cash_paid) > 0 && (
                    <Row label="  现金" value={`¥${Number(data.details.cash_paid).toFixed(2)}`} />
                  )}
                  {data.details.prepaid_used !== undefined && Number(data.details.prepaid_used) > 0 && (
                    <Row label="  预付点" value={`¥${Number(data.details.prepaid_used).toFixed(2)}`} />
                  )}
                  {data.details.gift_used !== undefined && Number(data.details.gift_used) > 0 && (
                    <Row label="  赠点" value={`¥${Number(data.details.gift_used).toFixed(2)}`} />
                  )}
                  <Text style={{ fontSize: 12, color: '#a1a1aa', fontWeight: '700', marginTop: 12, marginBottom: 4 }}>退款明细（原路退回）</Text>
                </>
              )}
              {data.details?.cash_refundable !== undefined && Number(data.details.cash_refundable) > 0 && (
                <Row label="退现金（微信原路）" value={`¥${Number(data.details.cash_refundable).toFixed(2)}`} />
              )}
              {data.details?.prepaid_refunded !== undefined && Number(data.details.prepaid_refunded) > 0 && (
                <Row label="退回预付点" value={`+¥${Number(data.details.prepaid_refunded).toFixed(2)}`} color="#16a34a" />
              )}
              {data.details?.gift_refunded !== undefined && Number(data.details.gift_refunded) > 0 && (
                <Row label="退回赠点" value={`+¥${Number(data.details.gift_refunded).toFixed(2)}`} color="#16a34a" />
              )}
              <Row label="退款金额" value={`¥${Number(data.amount).toFixed(2)}`} bold />
            </View>
          )}
        </View>

        {/* Points usage (only for non-refund, non-appeal) */}
        {!isRefund && pType !== 'appeal' && pType !== 'membership' && data.amount > 0 && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
           {maxGift > 0 && (<>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>点数使用</Text>

            {maxGift > 0 && (
            <View style={{ marginBottom: 4 }}>
              <Row label="赠点余额" value={`¥${Number(maxGift).toFixed(2)}`} />
              <View style={{ display: 'flex', alignItems: 'center', marginTop: 4 }}>
                <Text style={{ fontSize: 13, color: '#71717a', width: 72 }}>使用</Text>
                <View style={{ flex: 1, display: 'flex', alignItems: 'center', gap: 4 }}>
                  <Input type="number" value={String(giftUsed || '')}
                    onInput={e => setGiftUsed(clampPoints(e.detail.value, Math.min(maxGift, data.amount)))}
                    style={{ flex: 1, border: '1px solid #e4e4e7', borderRadius: 8, padding: '6px 10px', fontSize: 13, textAlign: 'right' }}
                  />
                </View>
                <Text style={{ fontSize: 13, color: '#71717a', marginLeft: 4 }}>点</Text>
              </View>
            </View>
            )}
            {maxGift > 0 && (
            <Text style={{ fontSize: 11, color: '#a1a1aa', textAlign: 'right', marginBottom: 8 }}>
              赠点使用不得超过租金的 {Math.round((wallet.max_gift_ratio || 0.3) * 100)}%
            </Text>
            )}

            <View style={{ borderTop: '1px solid #e4e4e7', paddingTop: 8 }}>
              <Row label="现金差额" value={`¥${Number(cashAmount).toFixed(2)}`} bold />
            </View>
          </>)}
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

        {/* Membership benefits note (#1575) */}
        {pType === 'membership' && (
          <View style={{ backgroundColor: '#fef9ec', margin: 16, borderRadius: 16, padding: 16 }}>
            <Text style={{ fontSize: 13, fontWeight: '700', color: '#92400e', marginBottom: 8 }}>成为 VIP 会员后，你将获得以下权益：</Text>
            <View style={{ marginBottom: 4 }}>
              <Text style={{ fontSize: 12, color: '#92400e', lineHeight: '18px' }}>· 解锁全部乐器租赁服务</Text>
            </View>
            <View>
              <Text style={{ fontSize: 12, color: '#92400e', lineHeight: '18px' }}>· 获赠 99 赠点，可用于抵扣租金</Text>
              <Text style={{ fontSize: 11, color: '#a16207', lineHeight: '16px', paddingLeft: 12 }}>（每次最多抵扣租金的 {Math.round((maxPayRatio || 0.3) * 100)}%）</Text>
            </View>
          </View>
        )}

        {/* 优惠码（#1719 通用化）：所有付款类支付页显示；退款/申诉结果页除外 */}
        {!isRefund && pType !== 'appeal' && !prepayData?.data && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 8 }}>优惠码</Text>
            <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', gap: 8 }}>
              <Input
                defaultValue={couponCode}
                onInput={e => { couponInputRef.current = e.detail.value; setCouponCode(e.detail.value) }}
                placeholder="输入优惠码（选填）"
                placeholderStyle="color:#a1a1aa;font-size:12px"
                style={{ flex: 1, border: '1px solid #e4e4e7', borderRadius: 10, padding: '8px 12px', fontSize: 13 }}
              />
              <View
                onClick={applyCoupon}
                style={{ backgroundColor: appliedCoupon ? '#f4f4f5' : '#915F38', borderRadius: 10, padding: '8px 16px' }}
              >
                <Text style={{ color: appliedCoupon ? '#71717a' : '#fff', fontSize: 13, fontWeight: '600' }}>
                  {appliedCoupon ? '已应用' : '应用'}
                </Text>
              </View>
            </View>
            {appliedCoupon && (
              <Text style={{ fontSize: 12, color: '#16a34a', marginTop: 8, display: 'block' }}>{appliedCoupon.hint}</Text>
            )}
            {appliedCoupon && (
              <Row label="优惠后金额" value={`¥${Number(couponAmount).toFixed(2)}`} bold />
            )}
          </View>
        )}

        {/* Bottom padding for button */}
        <View style={{ height: 80 }} />
      </ScrollView>

      {/* Pay/Confirm button */}
      <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, backgroundColor: '#fff', borderTop: '1px solid #f4f4f5', padding: 16 }}>
        {isRefund ? (
          <View style={{ display: 'flex', flexDirection: 'row', gap: 12 }}>
            <View style={{ flex: 1 }}>
              <Button style={btnStyle('#B98E5F')} onClick={handleRefund}>确认退款 ¥{Number(cashAmount).toFixed(2)}</Button>
            </View>
          </View>
        ) : pType === 'appeal' ? (
          <Button style={btnStyle('#16a34a')} onClick={() => Taro.redirectTo({ url: `/pages-weapp/order-detail/index?id=${pId}` })}>
            确认，查看订单详情
          </Button>
        ) : prepayData?.data ? (
          <View style={{ display: 'flex', flexDirection: 'row', gap: 12 }}>
            <View style={{ flex: 1 }}>
              <Button style={btnStyle('#B98E5F')} onClick={doRealPay}>
                微信支付 ¥{Number(cashAmount).toFixed(2)}
              </Button>
            </View>
          </View>
        ) : (
          <View style={{ display: 'flex', flexDirection: 'row', gap: 12 }}>
            <View style={{ flex: 1 }}>
              <Button style={btnStyle(cashAmount > 0 ? '#B98E5F' : '#16a34a')} onClick={() => handlePay(cashAmount)} disabled={isPaying}>
                {isPaying ? '处理中...' : `发起支付 ¥${Number(cashAmount).toFixed(2)}`}
              </Button>
            </View>
          </View>
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
  if (type === 'membership') {
    // Standard receipt format (#1575): item rows + total.
    const items = details?.items || []
    return (
      <View>
        {items.map((item, i) => (
          <Row key={i} label={item.label} value={`¥${Number(item.amount || 0).toFixed(2)}`} />
        ))}
        <View style={{ borderTop: '1px solid #f4f4f5', marginTop: 8, paddingTop: 8 }}>
          <Row label="合计" value={`¥${items.reduce((s, it) => s + Number(it.amount || 0), 0).toFixed(2)}`} bold />
        </View>
      </View>
    )
  }
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
    const oldQ = details.old_quote
    return (
      <View>
        {type === 'requote' && oldQ && (
          <View style={{ opacity: 0.5, marginBottom: 4 }}>
            <Row label="原报价（材料费）" value={`¥${Number(oldQ.material_fee || 0).toFixed(2)}`} />
            <Row label="原报价（服务费）" value={`¥${Number(oldQ.service_fee || 0).toFixed(2)}`} />
            <Row label="原报价（物流费）" value={`¥${Number(oldQ.logistics_fee || 0).toFixed(2)}`} />
            <Row label="原报价合计" value={`¥${Number(oldQ.total || 0).toFixed(2)}`} bold />
          </View>
        )}
        <Row label="材料费" value={`¥${Number(details.material_fee || 0).toFixed(2)}`} />
        <Row label="服务费" value={`¥${Number(details.service_fee || 0).toFixed(2)}`} />
        <Row label="物流费" value={`¥${Number(details.logistics_fee || 0).toFixed(2)}`} />
        {type === 'requote' && oldQ && (
          <Row label="需补付" value={`+¥${Math.max(0, Number(details.total || 0)).toFixed(2)}`} bold color="#dc2626" />
        )}
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
  if (type === 'appeal') {
    // Appeal resolution receipt (#1576): shows the settled refund from the
    // appeal outcome (ResolveAppeal/AgreeDamage already ran settlement).
    return (
      <View>
        <Row label="实际租期" value={`${Number(details.actual_rent_days || 0)} 天`} />
        <Row label="实际租金" value={`¥${Number(details.actual_rent_amount || 0).toFixed(2)}`} />
        {Number(details.damage_deducted || 0) > 0 && (
          <Row label="损坏扣款" value={`-¥${Number(details.damage_deducted).toFixed(2)}`} color="#dc2626" />
        )}
        {Number(details.overdue_fee || 0) > 0 && (
          <Row label="逾期费用" value={`-¥${Number(details.overdue_fee).toFixed(2)}`} color="#dc2626" />
        )}
        <View style={{ borderTop: '1px solid #f4f4f5', paddingTop: 8, marginTop: 4 }}>
          <Row label="退款金额" value={`¥${Number(details.cash_refundable || 0).toFixed(2)}`} bold color="#3b82f6" />
        </View>
      </View>
    )
  }
  return null
}

function btnStyle(bgColor) {
  return { width: '100%', padding: '14px 0', borderRadius: 16, fontWeight: '700', fontSize: 15, textAlign: 'center', color: '#fff', backgroundColor: bgColor, cursor: 'pointer' }
}
