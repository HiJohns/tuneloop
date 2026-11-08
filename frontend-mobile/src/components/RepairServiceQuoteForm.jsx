// #2293：维修师报价共享表单（工作台内联 + 维修服务详情页共用，跨端 Taro 控件）。
import { useState } from 'react'
import { View, Text, Input, Button } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue } from '../platform'
import { yuanToCents } from '../utils/money'

const baseUrl = env.apiBaseUrl
const labelStyle = { fontSize: 12, color: '#52525B', marginTop: 4 }
const inputStyle = {
  width: '100%', height: 40, border: '1px solid #E4E4E7', borderRadius: 8,
  padding: '0 10px', fontSize: 14, boxSizing: 'border-box', backgroundColor: '#FFFFFF',
}
const btnStyle = {
  width: '100%', height: 44, borderRadius: 10, backgroundColor: '#18181B', color: '#FFFFFF',
  fontSize: 14, fontWeight: 'bold', display: 'flex', alignItems: 'center',
  justifyContent: 'center', margin: 0,
}

export default function RepairServiceQuoteForm({ serviceId, onSubmitted, defaultValues = {}, submitLabel = '提交报价' }) {
  const [repairYuan, setRepairYuan] = useState(defaultValues.repairYuan || '')
  const [materialYuan, setMaterialYuan] = useState(defaultValues.materialYuan || '')
  const [logiYuan, setLogiYuan] = useState(defaultValues.logiYuan || '')
  const [submitting, setSubmitting] = useState(false)

  const submit = async () => {
    const repairCents = yuanToCents(repairYuan)
    const materialCents = yuanToCents(materialYuan === '' ? '0' : materialYuan)
    const logiCents = yuanToCents(logiYuan === '' ? '0' : logiYuan)
    if (repairCents <= 0) { dialog.alert('请填写正确的修理费'); return }
    if (materialCents < 0) { dialog.alert('请填写正确的料钱'); return }
    if (logiCents < 0) { dialog.alert('请填写正确的物流费预估'); return }
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${serviceId}/quote`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          quote_repair_cents: repairCents,
          quote_material_cents: materialCents,
          quote_logistics_cents: logiCents,
        }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('报价已提交，等待用户支付')
        if (onSubmitted) onSubmitted()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  return (
    <View style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <Text style={labelStyle}>修理费（元）</Text>
      <Input style={inputStyle} type="digit" value={repairYuan}
        onInput={e => setRepairYuan(getInputValue(e))} placeholder="如 200" />
      <Text style={labelStyle}>料钱（元；材料/配件费，可空）</Text>
      <Input style={inputStyle} type="digit" value={materialYuan}
        onInput={e => setMaterialYuan(getInputValue(e))} placeholder="如 30" />
      <Text style={labelStyle}>物流费预估（元）</Text>
      <Input style={inputStyle} type="digit" value={logiYuan}
        onInput={e => setLogiYuan(getInputValue(e))} placeholder="如 50" />
      <Button disabled={submitting} onClick={submit}
        style={{ ...btnStyle, opacity: submitting ? 0.5 : 1 }}>
        {submitting ? '处理中...' : submitLabel}
      </Button>
    </View>
  )
}
