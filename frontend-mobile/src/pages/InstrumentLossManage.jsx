import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button, ScrollView, Image } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute, uploadFile as uploadFileApi } from '../platform'
import { formatBeijingDate } from '../utils/format'

// #1959 阶段3 丢失-1：员工 weapp 丢失登记 / 找回 / 台账（LS-01/05/04）

const MAX_PHOTOS = 6
const partyLabels = { user: '用户', logistics: '物流公司', platform: '平台', site: '网点' }
const statusLabel = { available: '可租', rented: '租赁中', maintenance: '维修中', lost: '已丢失', archived: '已下架' }
const timelineLabels = {
  created: '创建', settled: '发回结算', restored: '已恢复', reviewed: '评价',
}

const cardStyle = {
  display: 'flex', flexDirection: 'column', gap: 6,
  backgroundColor: '#FFFFFF', borderRadius: 12, padding: 12, marginBottom: 10,
}
const btnPrimaryStyle = {
  width: '100%', margin: 0, height: 40, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF',
  borderRadius: 8, fontSize: 14, fontWeight: 'bold',
}
const btnDangerStyle = { ...btnPrimaryStyle, backgroundColor: '#DC2626' }
const btnSecondaryStyle = {
  width: '100%', margin: 0, height: 36, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#F4F4F5', color: '#3F3F46',
  borderRadius: 8, fontSize: 13, fontWeight: 'bold',
}
const inputStyle = {
  width: '100%', height: 36, boxSizing: 'border-box', backgroundColor: '#FAFAFA',
  border: '1px solid #E4E4E7', borderRadius: 8, padding: '0 10px', fontSize: 13,
}
const labelStyle = { fontSize: 12, color: '#71717A' }
const parties = ['user', 'logistics', 'platform', 'site']

export default function InstrumentLossManage() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [instruments, setInstruments] = useState([])
  const [records, setRecords] = useState([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState('') // `${id}:lost|restore`
  const [busy, setBusy] = useState(false)
  // 丢失表单
  const [party, setParty] = useState('user')
  const [ratio, setRatio] = useState('100')
  const [desc, setDesc] = useState('')
  const [compYuan, setCompYuan] = useState('')
  const [burdenYuan, setBurdenYuan] = useState('')
  // 恢复表单
  const [damaged, setDamaged] = useState(false)
  const [restoreDesc, setRestoreDesc] = useState('')
  const [restorePhotos, setRestorePhotos] = useState([])
  const baseUrl = env.apiBaseUrl

  const fetchAll = async () => {
    setLoading(true)
    try {
      const [iRes, rRes] = await Promise.all([
        apiFetch(`${baseUrl}/instruments?page=1&pageSize=100`),
        apiFetch(`${baseUrl}/instrument-loss`),
      ])
      const i = await iRes.json()
      const r = await rRes.json()
      if (i.code === 20000) setInstruments(i.data?.list || [])
      if (r.code === 20000) setRecords(r.data?.list || [])
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setLoading(false)
  }

  useEffect(() => { fetchAll() }, [])

  const lostItems = instruments.filter(i2 => i2.stock_status === 'lost')
  const managedItems = instruments.filter(i2 => i2.stock_status !== 'lost' && i2.stock_status !== 'archived')

  const uploadOne = async (file) => {
    const authHeaders = { Authorization: 'Bearer ' + (getToken() || '') }
    if (env.isMiniProgram) {
      const resp = await uploadFileApi(`${baseUrl}/upload`, file, { headers: authHeaders })
      const r = JSON.parse(resp.data)
      if (r.code === 20000) return r.data.file_key
      throw new Error(r.message || 'upload failed')
    }
    const fd = new FormData()
    fd.append('file', file)
    const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', headers: authHeaders, body: fd })
    const r = await resp.json()
    if (r.code === 20000) return r.data.file_key
    throw new Error(resolveErrorMessage(r, 'upload failed'))
  }

  const choosePhotos = async () => {
    try {
      const res = await Taro.chooseImage({ count: MAX_PHOTOS - restorePhotos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setRestorePhotos(p => [...p, ...(res.tempFilePaths || [])].slice(0, MAX_PHOTOS))
    } catch (err) { console.error('choose image failed:', err) }
  }

  const toggle = (id, mode) => {
    if (expanded === `${id}:${mode}`) { setExpanded(''); return }
    setParty('user'); setRatio('100'); setDesc(''); setCompYuan(''); setBurdenYuan('')
    setDamaged(false); setRestoreDesc(''); setRestorePhotos([])
    setExpanded(`${id}:${mode}`)
  }

  const submitLost = async (inst) => {
    if (!desc.trim()) { dialog.alert('请填写丢失描述'); return }
    const r = parseInt(ratio || '0', 10)
    if (isNaN(r) || r < 0 || r > 100) { dialog.alert('责任比例须为 0-100'); return }
    const comp = Math.round((parseFloat(compYuan) || 0) * 100)
    const burdenRaw = parseFloat(burdenYuan)
    setBusy(true)
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/${inst.id}/lost`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          description: desc.trim(), responsible_party: party, user_ratio: r,
          compensation_cents: comp,
          ...(burdenRaw > 0 ? { user_burden_cents: Math.round(burdenRaw * 100) } : {}),
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('丢失登记完成')
        setExpanded('')
        fetchAll()
      } else dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const submitRestore = async (inst) => {
    setBusy(true)
    try {
      const photos = []
      for (const f of restorePhotos) photos.push(await uploadOne(f))
      const resp = await apiFetch(`${baseUrl}/instruments/${inst.id}/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ damaged: damaged, description: restoreDesc.trim(), photos }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        const d = result.data || {}
        dialog.alert(d.restored_damaged ? '已恢复上架（有损坏，已留痕）' : '已恢复上架（无损坏）')
        setExpanded('')
        fetchAll()
      } else dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const renderLostForm = (inst) => (
    <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
      <Text style={labelStyle}>责任方</Text>
      <View style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {parties.map(p2 => (
          <View key={p2} onClick={() => { setParty(p2); setRatio(p2 === 'user' ? '100' : '0') }}
            style={{
              padding: '6px 12px', borderRadius: 8,
              backgroundColor: party === p2 ? '#171717' : '#F4F4F5',
            }}>
            <Text style={{ fontSize: 12, color: party === p2 ? '#FFFFFF' : '#52525B' }}>{partyLabels[p2]}</Text>
          </View>
        ))}
      </View>
      <Text style={labelStyle}>用户责任比例（%）</Text>
      <Input style={inputStyle} type="number" value={ratio} onInput={e => setRatio(getInputValue(e))} />
      <Text style={labelStyle}>赔偿金额（元，按乐器价值填写）</Text>
      <Input style={inputStyle} type="digit" value={compYuan} onInput={e => setCompYuan(getInputValue(e))} />
      <Text style={labelStyle}>用户承担金额（元，留空=按比例计算）</Text>
      <Input style={inputStyle} type="digit" value={burdenYuan} onInput={e => setBurdenYuan(getInputValue(e))} />
      <Text style={labelStyle}>丢失描述（必填）</Text>
      <Input style={inputStyle} value={desc} onInput={e => setDesc(getInputValue(e))} placeholder="描述丢失经过" />
      <Button disabled={busy} onClick={() => submitLost(inst)}
        style={{ ...btnDangerStyle, opacity: busy ? 0.5 : 1 }}>
        {busy ? '处理中...' : '提交丢失登记'}
      </Button>
    </View>
  )

  const renderRestoreForm = (inst) => (
    <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
      <Text style={labelStyle}>是否有损坏（恢复上架，损坏仅留痕）</Text>
      <View style={{ display: 'flex', gap: 6 }}>
        {[{ v: false, label: '无损坏' }, { v: true, label: '有损坏' }].map(o => (
          <View key={String(o.v)} onClick={() => setDamaged(o.v)}
            style={{ padding: '6px 12px', borderRadius: 8, backgroundColor: damaged === o.v ? '#171717' : '#F4F4F5' }}>
            <Text style={{ fontSize: 12, color: damaged === o.v ? '#FFFFFF' : '#52525B' }}>{o.label}</Text>
          </View>
        ))}
      </View>
      <Text style={labelStyle}>找回说明</Text>
      <Input style={inputStyle} value={restoreDesc} onInput={e => setRestoreDesc(getInputValue(e))} placeholder="（可选）" />
      <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {restorePhotos.map((f, i) => (
          <View key={i} style={{ width: 56, height: 56, borderRadius: 8, overflow: 'hidden' }}>
            <Image src={env.isMiniProgram ? f : URL.createObjectURL(f)} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
          </View>
        ))}
        {restorePhotos.length < MAX_PHOTOS && (
          <View onClick={choosePhotos}
            style={{ width: 56, height: 56, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Text style={{ fontSize: 18, color: '#A1A1AA' }}>＋</Text>
          </View>
        )}
      </View>
      <Button disabled={busy} onClick={() => submitRestore(inst)}
        style={{ ...btnPrimaryStyle, opacity: busy ? 0.5 : 1 }}>
        {busy ? '处理中...' : '确认恢复'}
      </Button>
    </View>
  )

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>乐器丢失 / 恢复</Text>
      </View>

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>丢失中（{lostItems.length}）</Text>
            <Text onClick={fetchAll} style={{ fontSize: 12, color: '#71717A' }}>刷新</Text>
          </View>
          {loading ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
          ) : lostItems.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无丢失乐器</Text>
          ) : lostItems.map(inst => (
            <View key={inst.id} style={cardStyle}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>SN: {inst.sn || '-'}</Text>
                <Text style={{ fontSize: 12, color: '#DC2626' }}>已丢失</Text>
              </View>
              <Button onClick={() => toggle(inst.id, 'restore')} style={btnSecondaryStyle}>
                {expanded === `${inst.id}:restore` ? '收起' : '恢复'}
              </Button>
              {expanded === `${inst.id}:restore` && renderRestoreForm(inst)}
            </View>
          ))}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>在管乐器（{managedItems.length}）</Text>
          </View>
          {!loading && managedItems.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无在管乐器</Text>
          ) : managedItems.map(inst => (
            <View key={inst.id} style={cardStyle}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>SN: {inst.sn || '-'}</Text>
                <Text style={{ fontSize: 12, color: '#71717A' }}>{statusLabel[inst.stock_status] || inst.stock_status}</Text>
              </View>
              <Button onClick={() => toggle(inst.id, 'lost')} style={btnDangerStyle}>
                {expanded === `${inst.id}:lost` ? '收起' : '丢失登记'}
              </Button>
              {expanded === `${inst.id}:lost` && renderLostForm(inst)}
            </View>
          ))}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>丢失台账（{records.length}）</Text>
          </View>
          {!loading && records.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无丢失记录</Text>
          ) : records.map(r2 => (
            <View key={r2.id} style={cardStyle}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 12, fontWeight: 'bold', color: '#18181B' }}>
                  {partyLabels[r2.responsible_party] || r2.responsible_party} · 比例 {r2.user_ratio}%
                </Text>
                <Text style={{ fontSize: 12, color: r2.restored_at ? '#16A34A' : '#DC2626' }}>
                  {r2.restored_at ? '已恢复' : '丢失中'}
                </Text>
              </View>
              <Text style={{ fontSize: 11, color: '#71717A' }}>
                赔偿 ¥{((r2.compensation_cents || 0) / 100).toFixed(2)} · 承担 ¥{((r2.user_burden_cents || 0) / 100).toFixed(2)}
              </Text>
              <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
                {r2.created_at ? formatBeijingDate(r2.created_at) : '-'} {r2.reverse_note ? ` · ${r2.reverse_note}` : ''}
              </Text>
            </View>
          ))}
        </View>
      </ScrollView>
    </View>
  )
}
