import { useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button, ScrollView, Image } from '@tarojs/components'
import { apiFetch, resolveErrorMessage, resolveUploadError } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute, uploadFile as uploadFileApi } from '../platform'
import { yuanToCents } from '../utils/money'

// #2615 登记页：列表页唯一入口 → 本页（SN 检索选择乐器 + 表单 + 照片）。
// D5：多步流程 = 独立路由；D7：照片走 Taro.chooseImage → platform.uploadFile，≤6 张；
// D8：参数统一 query（本页无入参）。D6：危险色只用于「提交丢失登记」确认按钮。

const MAX_PHOTOS = 6
const partyLabels = { user: '用户', logistics: '物流公司', platform: '平台', site: '网点' }
const parties = ['user', 'logistics', 'platform', 'site']

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
const inputStyle = {
  width: '100%', height: 36, boxSizing: 'border-box', backgroundColor: '#FAFAFA',
  border: '1px solid #E4E4E7', borderRadius: 8, padding: '0 10px', fontSize: 13,
}
const labelStyle = { fontSize: 12, color: '#71717A' }

export default function InstrumentLossRegister() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const baseUrl = env.apiBaseUrl
  const [keyword, setKeyword] = useState('')
  const [options, setOptions] = useState([])
  const [selected, setSelected] = useState(null)
  const [party, setParty] = useState('user')
  const [ratio, setRatio] = useState('100')
  const [compYuan, setCompYuan] = useState('')
  const [burdenYuan, setBurdenYuan] = useState('')
  const [desc, setDesc] = useState('')
  const [photos, setPhotos] = useState([])
  const [busy, setBusy] = useState(false)
  const debounceRef = useRef(null)

  const search = async (kw) => {
    const query = String(kw || '').trim()
    if (!query) { setOptions([]); return }
    try {
      const res = await apiFetch(`${baseUrl}/instruments?sn=${encodeURIComponent(query)}&exclude_status=lost,archived,sold&page=1&pageSize=20`)
      const d = await res.json()
      if (d.code === 20000) setOptions(d.data?.list || [])
      else dialog.alert(resolveErrorMessage(d))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
  }

  const onKeywordInput = (value) => {
    setKeyword(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => { search(value) }, 300)
  }

  const pickInstrument = (inst) => {
    setSelected(inst)
    setOptions([])
  }

  const chooseParty = (p) => {
    setParty(p)
    // 与现网一致：只有「用户」责任默认 100%，其余责任方默认 0%
    setRatio(p === 'user' ? '100' : '0')
  }

  const uploadOne = async (file) => {
    const resp = await uploadFileApi(`${baseUrl}/upload`, file)
    const r = env.isMiniProgram ? JSON.parse(resp.data) : await resp.json()
    const authErr = resolveUploadError(resp, r)
    if (authErr) throw new Error(authErr)
    if (r.code === 20000) return r.data.file_key
    throw new Error(resolveErrorMessage(r, 'upload failed'))
  }

  const choosePhotos = async () => {
    try {
      const res = await Taro.chooseImage({
        count: MAX_PHOTOS - photos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'],
      })
      setPhotos((p) => [...p, ...(res.tempFilePaths || [])].slice(0, MAX_PHOTOS))
    } catch (err) { console.error('choose image failed:', err) }
  }

  const submit = async () => {
    if (!selected) { dialog.alert('请选择丢失的乐器'); return }
    if (!String(desc || '').trim()) { dialog.alert('请填写丢失描述'); return }
    const r = parseInt(ratio || '0', 10)
    if (isNaN(r) || r < 0 || r > 100) { dialog.alert('责任比例须为 0-100'); return }
    const comp = yuanToCents(parseFloat(compYuan) || 0)
    const burdenRaw = parseFloat(burdenYuan)
    setBusy(true)
    try {
      const fileKeys = []
      for (const f of photos) fileKeys.push(await uploadOne(f))
      const resp = await apiFetch(`${baseUrl}/instruments/${selected.id}/lost`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          description: String(desc).trim(),
          responsible_party: party,
          user_ratio: r,
          compensation_cents: comp,
          ...(burdenRaw > 0 ? { user_burden_cents: yuanToCents(burdenRaw) } : {}),
          photos: fileKeys,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        await dialog.alert('丢失登记完成')
        nav(-1) // 回列表，列表 useDidShow 刷新
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>登记丢失</Text>
        </View>
      )}

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>选择乐器</Text>
            <Input
              data-testid="loss-inst-sn"
              style={inputStyle}
              value={keyword}
              placeholder="输入 SN 检索乐器"
              onInput={(e) => onKeywordInput(getInputValue(e))}
            />
            {options.length > 0 && (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                {options.map((inst) => (
                  <View
                    key={inst.id}
                    data-testid="loss-inst-option"
                    onClick={() => pickInstrument(inst)}
                    style={{ padding: '8px 10px', borderRadius: 8, backgroundColor: '#F4F4F5' }}
                  >
                    <Text style={{ fontSize: 12, color: '#3F3F46' }}>SN: {inst.sn || '-'}</Text>
                  </View>
                ))}
              </View>
            )}
            {selected ? (
              <Text data-testid="loss-selected-inst" style={{ fontSize: 12, color: '#18181B' }}>
                已选：SN: {selected.sn || '-'}
              </Text>
            ) : (
              <Text style={{ fontSize: 12, color: '#A1A1AA' }}>请选择乐器</Text>
            )}
          </View>

          <View style={cardStyle}>
            <Text style={labelStyle}>责任方</Text>
            <View style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {parties.map((p) => (
                <View
                  key={p}
                  data-testid={`loss-party-${p}`}
                  onClick={() => chooseParty(p)}
                  style={{ padding: '6px 12px', borderRadius: 8, backgroundColor: party === p ? '#171717' : '#F4F4F5' }}
                >
                  <Text style={{ fontSize: 12, color: party === p ? '#FFFFFF' : '#52525B' }}>{partyLabels[p]}</Text>
                </View>
              ))}
            </View>
            <Text data-testid="loss-party-current" style={labelStyle}>当前责任方：{partyLabels[party]}</Text>

            <Text style={labelStyle}>用户责任比例（%）</Text>
            <Input data-testid="loss-ratio" style={inputStyle} type="number" value={ratio} onInput={(e) => setRatio(getInputValue(e))} />

            <Text style={labelStyle}>赔偿金额（元，按乐器价值填写）</Text>
            <Input style={inputStyle} type="digit" value={compYuan} onInput={(e) => setCompYuan(getInputValue(e))} />

            <Text style={labelStyle}>用户承担金额（元，留空=按比例计算）</Text>
            <Input style={inputStyle} type="digit" value={burdenYuan} onInput={(e) => setBurdenYuan(getInputValue(e))} />

            <Text style={labelStyle}>丢失描述（必填）</Text>
            <Input data-testid="loss-desc" style={inputStyle} value={desc} placeholder="描述丢失经过" onInput={(e) => setDesc(getInputValue(e))} />

            <Text style={labelStyle}>现场照片（最多 {MAX_PHOTOS} 张）</Text>
            <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {photos.map((f, i) => (
                <View key={i} style={{ width: 56, height: 56, borderRadius: 8, overflow: 'hidden' }}>
                  <Image src={env.isMiniProgram ? f : URL.createObjectURL(f)} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
                </View>
              ))}
              {photos.length < MAX_PHOTOS && (
                <View
                  onClick={choosePhotos}
                  style={{ width: 56, height: 56, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
                >
                  <Text style={{ fontSize: 18, color: '#A1A1AA' }}>＋</Text>
                </View>
              )}
            </View>

            <Button data-testid="loss-submit" disabled={busy} onClick={submit}
              style={{ ...btnDangerStyle, opacity: busy ? 0.5 : 1 }}>
              {busy ? '处理中...' : '提交丢失登记'}
            </Button>
          </View>
        </View>
      </ScrollView>
    </View>
  )
}
