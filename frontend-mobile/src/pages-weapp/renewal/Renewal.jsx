import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Input, Button, Image } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../../services/api'
import { env } from '../../platform'

function formatDate(raw) {
  if (!raw) return '-'
  const d = new Date(raw.slice(0, 10))
  if (isNaN(d.getTime())) return raw.slice(0, 10)
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

export default function Renewal() {
  const params = Taro.getCurrentInstance()?.router?.params || {}
  const orderId = params.id
  const baseUrl = env.apiBaseUrl
  // #1733: weapp <Image> 相对路径 src 被当作包内路径 → 补全域名前缀
  const IMG_BASE = env.apiBaseUrl.replace(/\/api$/, '')
  const fixImg = (url) => !url || url.startsWith('http') || url.startsWith('data:') ? url : IMG_BASE + url

  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [days, setDays] = useState(30)
  const [calcResult, setCalcResult] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [customDays, setCustomDays] = useState('')

  useEffect(() => {
    if (!orderId) return
    const load = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${orderId}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrder(result.data)
          if (result.data.instrument_id) {
            const iresp = await apiFetch(`${baseUrl}/public/instruments/${result.data.instrument_id}`)
            const iresult = await iresp.json()
            if (iresult.code === 20000) setInstrument(iresult.data)
          }
        }
      } catch (err) {
        console.error('Failed to load order:', err)
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [orderId])

  useEffect(() => {
    if (!order || !days || days <= 0) return
    const calc = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${orderId}/renewal/calculate`, {
          method: 'POST',
          body: JSON.stringify({ additional_days: days }),
        })
        const result = await resp.json()
        if (result.code === 20000) setCalcResult(result.data)
      } catch (err) {
        console.error('Failed to calculate:', err)
      }
    }
    calc()
  }, [days, order, orderId])

  const handleSubmit = async () => {
    if (submitting || !calcResult) return
    setSubmitting(true)
    try {
      // #1722: 裸 fetch 在 weapp 不存在（ReferenceError "fetch is not defined"）→
      // 改用 apiFetch（platformRequest 跨端适配 + 自动带 Authorization）。
      const resp = await apiFetch(`${baseUrl}/orders/${orderId}/renewal/confirm`, {
        method: 'POST',
        body: JSON.stringify({ additional_days: days }),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data?.success) {
        Taro.redirectTo({
          url: `/pages-weapp/payment/index?type=renewal&id=${orderId}&amount=${calcResult.total_amount}`,
        })
      } else {
        Taro.showModal({ title: '续期失败', content: resolveErrorMessage(result.data, '请重试'), showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '网络错误', content: err.message, showCancel: false })
    } finally {
      setSubmitting(false)
    }
  }

  const btnStyle = (bg) => ({
    width: '100%',
    padding: '12px 0',
    borderRadius: 16,
    fontWeight: '900',
    textAlign: 'center',
    color: '#fff',
    backgroundColor: bg,
  })

  const pickDay = (d) => {
    setDays(d)
    setCustomDays('')
  }

  const dayOptions = [7, 15, 30, 60, 90, 180, 365]

  if (loading) return <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}><Text>加载中...</Text></View>
  if (!order) return <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}><Text>订单不存在</Text></View>

  const overdueDays = calcResult?.overdue_days || 0
  const minDays = calcResult?.min_additional_days || 0

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7' }}>
      {/* Title bar removed — native navigation bar shows 续期 (#1511) */}

      <ScrollView>
        <View style={{ padding: 16, boxSizing: 'border-box' }}>

        {instrument && (
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12 }} onClick={() => Taro.navigateTo({ url: `/pages-weapp/detail/index?id=${instrument.id}` })}>
            {(instrument.cover_image || instrument.images?.[0]) && <Image src={fixImg(instrument.cover_image || instrument.images?.[0])} style={{ width: '100%', height: 160, objectFit: 'cover', borderRadius: 8, marginBottom: 12, backgroundColor: '#f4f4f5' }} mode="aspectFill" />}
            <Text style={{ fontSize: 14, fontWeight: '700', marginBottom: 8 }}>{instrument.category_name || '乐器'}</Text>
            <View style={{ display: 'flex', flexDirection: 'row', marginBottom: 4 }}><Text style={{ fontSize: 12, color: '#a1a1aa', width: 80 }}>SN</Text><Text style={{ fontSize: 12, color: '#000' }}>{instrument.sn || '-'}</Text></View>
            <View style={{ display: 'flex', flexDirection: 'row', marginBottom: 4 }}><Text style={{ fontSize: 12, color: '#a1a1aa', width: 80 }}>下单日</Text><Text style={{ fontSize: 12, color: '#000' }}>{formatDate(order.created_at)}</Text></View>
            <View style={{ display: 'flex', flexDirection: 'row', marginBottom: 4 }}><Text style={{ fontSize: 12, color: '#a1a1aa', width: 80 }}>原预期归还</Text><Text style={{ fontSize: 12, color: '#000' }}>{formatDate(order.end_date)}</Text></View>
            {overdueDays > 0 && (
              <View style={{ display: 'flex', flexDirection: 'row', marginBottom: 4 }}><Text style={{ fontSize: 12, color: '#a1a1aa', width: 80 }}>超期</Text><Text style={{ fontSize: 12, color: '#ef4444' }}>{overdueDays} 天（续期需覆盖）</Text></View>
            )}
          </View>
        )}

        <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12 }}>
          <Text style={{ fontSize: 14, fontWeight: '700', marginBottom: 8 }}>续期设置</Text>
          {minDays > 1 && (
            <Text style={{ fontSize: 12, color: '#ef4444', marginBottom: 8 }}>已超期，最少续期 {minDays} 天</Text>
          )}
          <View style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 8 }}>
            {dayOptions.map(d => {
              const disabled = d < minDays
              return (
                <View key={d}
                  onClick={() => !disabled && pickDay(d)}
                  style={{
                    padding: '8px 16px', borderRadius: 20,
                    border: days === d && !customDays ? '1px solid #2563eb' : '1px solid #d1d5db',
                    backgroundColor: disabled ? '#f3f4f6' : days === d && !customDays ? '#2563eb' : '#fff',
                  }}>
                  <Text style={{ fontSize: 13, color: disabled ? '#9ca3af' : days === d && !customDays ? '#fff' : '#374151' }}>{d}天</Text>
                </View>
              )
            })}
          </View>
          <View style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text style={{ fontSize: 12, color: '#71717a' }}>自定义:</Text>
            <Input
              type="number"
              min={minDays || 1}
              placeholder="天数"
              value={customDays}
              onInput={(e) => { setCustomDays(e.detail.value); setDays(Math.max(minDays || 1, parseInt(e.detail.value) || (minDays || 1))) }}
              style={{ width: 80, padding: '4px 8px', border: '1px solid #d1d5db', borderRadius: 8, textAlign: 'center', fontSize: 12 }}
            />
            <Text style={{ fontSize: 12, color: '#71717a' }}>天</Text>
          </View>
          {days > 0 && calcResult?.new_end_date && (
            <View style={{ marginTop: 8 }}><Text style={{ fontSize: 12, color: '#a1a1aa' }}>预期归还日: </Text><Text style={{ fontSize: 12, color: '#000', fontWeight: '500' }}>{formatDate(calcResult.new_end_date)}</Text></View>
          )}
        </View>

        {calcResult && (
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', marginBottom: 8 }}>费用明细</Text>
            {calcResult.tier_breakdown?.map((t, i) => {
              const tierRate = (t.rate * (t.discount || 1)).toFixed(2)
              return (
              <View key={i} style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4 }}>
                <Text style={{ fontSize: 12, color: '#71717a' }}>第{t.tier}阶 {t.days}天 · ¥{((tierRate || 0) / 100).toFixed(2)}/天</Text>
                <Text style={{ fontSize: 12, fontWeight: '500' }}>¥{(Number(t.subtotal || 0) / 100).toFixed(2)}</Text>
              </View>
            )})}
            <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4, borderTop: '1px solid #f3f4f6', marginTop: 4 }}>
              <Text style={{ fontSize: 12, color: '#71717a' }}>续期费</Text>
              <Text style={{ fontSize: 12, fontWeight: '500' }}>¥{(Number(calcResult.renewal_cost || 0) / 100).toFixed(2)}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 8, borderTop: '1px solid #e5e7eb', marginTop: 4 }}>
              <Text style={{ fontSize: 14, fontWeight: '700' }}>合计</Text>
              <Text style={{ fontSize: 14, fontWeight: '700' }}>¥{(Number(calcResult.total_amount || 0) / 100).toFixed(2)}</Text>
            </View>
            <Text style={{ fontSize: 11, color: '#9ca3af', marginTop: 8 }}>新到期日: {formatDate(calcResult.new_end_date)}</Text>
            {calcResult.renewal_cost <= 0 && (
              <Text style={{ fontSize: 12, color: '#ef4444', marginTop: 8 }}>当前订单定价数据不完整，请联系管理员</Text>
            )}
          </View>
        )}

        <View onClick={submitting ? undefined : handleSubmit}
          style={submitting ? { ...btnStyle('#9ca3af') } : btnStyle('#000')}>
          <Text>{submitting ? '处理中...' : `确认续期 ¥${((calcResult?.total_amount || 0) / 100).toFixed(2)}`}</Text>
        </View>
        </View>
      </ScrollView>
    </View>
  )
}
