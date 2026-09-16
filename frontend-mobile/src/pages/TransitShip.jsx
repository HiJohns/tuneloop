// 中转发货页（#1931/#1934 Sub4）—— 填物流公司/单号/物流费 → last_mile
// PUT /forwarding/sessions/:id/last-mile { tracking_company, tracking_number, logistics_fee }
import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Input, Text, View, Button } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue } from '../platform'

export default function TransitShip() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const sessionId = params.get('session') || ''
  const baseUrl = env.apiBaseUrl

  const [company, setCompany] = useState('')
  const [number, setNumber] = useState('')
  const [fee, setFee] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const valid = company.trim() && number.trim() && fee !== '' && !isNaN(Number(fee))

  const handleSubmit = async () => {
    if (!valid) { dialog.alert('请填写完整物流信息'); return }
    setSubmitting(true)
    try {
      const resp = await apiFetch(`${baseUrl}/forwarding/sessions/${sessionId}/last-mile`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tracking_company: company.trim(),
          tracking_number: number.trim(),
          logistics_fee: Number(fee),
        }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        dialog.alert('转发已发出，等待顾客/商户确认')
        navigate('/transit-workbench')
      } else {
        dialog.alert(resolveErrorMessage(r, '提交失败'))
      }
    } catch (e) {
      dialog.alert('提交失败: ' + (e.message || ''))
    }
    setSubmitting(false)
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      {!env.isMiniProgram && (
      <View style={{ padding: '16px 16px 8px', backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
        <Text style={{ fontSize: 20, fontWeight: 900, color: '#000' }}>中转发货</Text>
      </View>
      )}
      <View style={{ padding: 16, display: 'flex', flexDirection: 'column' }}>
        <Text style={{ fontSize: 13, color: '#6b7280', marginBottom: 12 }}>填写承运物流信息（含运费）→ 状态变更为「派送中」。</Text>

        <View style={{ marginBottom: 12 }}>
          <Text style={{ display: 'block', fontSize: 13, fontWeight: 600, color: '#6b7280', marginBottom: 4 }}>物流公司</Text>
          <Input
            style={{ backgroundColor: '#fff', border: '1px solid #e4e4e7', borderRadius: 12, padding: '12px', fontSize: 14 }}
            value={company}
            onInput={e => setCompany(getInputValue(e))}
            placeholder="如 顺丰/EMS/德邦"
          />
        </View>
        <View style={{ marginBottom: 12 }}>
          <Text style={{ display: 'block', fontSize: 13, fontWeight: 600, color: '#6b7280', marginBottom: 4 }}>物流单号</Text>
          <Input
            style={{ backgroundColor: '#fff', border: '1px solid #e4e4e7', borderRadius: 12, padding: '12px', fontSize: 14 }}
            value={number}
            onInput={e => setNumber(getInputValue(e))}
            placeholder="输入单号"
          />
        </View>
        <View style={{ marginBottom: 12 }}>
          <Text style={{ display: 'block', fontSize: 13, fontWeight: 600, color: '#6b7280', marginBottom: 4 }}>物流费（元）</Text>
          <Input
            style={{ backgroundColor: '#fff', border: '1px solid #e4e4e7', borderRadius: 12, padding: '12px', fontSize: 14 }}
            value={fee}
            onInput={e => setFee(getInputValue(e))}
            type="digit"
            placeholder="如 12.50"
          />
        </View>

        <Button
          onClick={handleSubmit}
          disabled={!valid || submitting}
          style={{ width: '100%', margin: 0, height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', borderRadius: 999, backgroundColor: valid ? '#B98E5F' : '#d4d4d8', color: '#fff', fontWeight: 800, fontSize: 16, letterSpacing: '0.05em', border: 'none' }}
        >
          {submitting ? '处理中...' : valid ? '发起转发' : '请先完整填写物流信息'}
        </Button>
      </View>
    </View>
  )
}
