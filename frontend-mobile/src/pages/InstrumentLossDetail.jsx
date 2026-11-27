import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button, ScrollView, Image } from '@tarojs/components'
import { apiFetch, resolveErrorMessage, resolveUploadError } from '../services/api'
import { dialog, env, getInputValue, previewImage, toWeappRoute, uploadFile as uploadFileApi, useDidShow } from '../platform'
import { formatCents } from '../utils/money'
import { formatBeijingDateTimeShort } from '../utils/format'
import { photoSrc } from '../utils/media'

// #2615 详情页：列表行 → 本页（记录全文 + 照片预览 + 丢失中记录的「恢复上架」）。
//   - 已恢复记录：展示恢复留痕（时间/损坏/说明/照片），**不渲染恢复按钮**
//   - 丢失中记录：底部行动条「恢复上架」→ 展开恢复表单 → POST /instruments/:id/restore
// D8：参数用 useSearchParams().get('id')（weapp 无路径参数）；query 由 toWeappRoute 透传。
// D5：恢复表单是本页内的表单展开（非独立步骤），提交后 toast + 返回列表。

const MAX_PHOTOS = 6
const partyLabels = { user: '用户', logistics: '物流公司', platform: '平台', site: '网点' }

const cardStyle = {
  display: 'flex', flexDirection: 'column', gap: 8,
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
const valueStyle = { fontSize: 13, color: '#18181B' }
const thumbStyle = { width: 56, height: 56, borderRadius: 8, overflow: 'hidden' }

export default function InstrumentLossDetail() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [searchParams] = useSearchParams()
  const recordID = searchParams.get('id') || ''
  const baseUrl = env.apiBaseUrl

  const [record, setRecord] = useState(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [restoreOpen, setRestoreOpen] = useState(false)
  const [damaged, setDamaged] = useState(false)
  const [restoreDesc, setRestoreDesc] = useState('')
  const [restorePhotos, setRestorePhotos] = useState([])
  const [busy, setBusy] = useState(false)

  const load = async () => {
    if (!recordID) { setLoading(false); setNotFound(true); return }
    setLoading(true)
    try {
      const res = await apiFetch(`${baseUrl}/instrument-loss/${encodeURIComponent(recordID)}`)
      const d = await res.json()
      if (d.code === 20000) { setRecord(d.data || null); setNotFound(false) }
      else { setRecord(null); setNotFound(true) }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
      setNotFound(true)
    }
    setLoading(false)
  }

  useDidShow(() => { load() })

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
        count: MAX_PHOTOS - restorePhotos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'],
      })
      setRestorePhotos((p) => [...p, ...(res.tempFilePaths || [])].slice(0, MAX_PHOTOS))
    } catch (err) { console.error('choose image failed:', err) }
  }

  const submitRestore = async () => {
    if (!record) return
    setBusy(true)
    try {
      const photos = []
      for (const f of restorePhotos) photos.push(await uploadOne(f))
      const resp = await apiFetch(`${baseUrl}/instruments/${record.instrument_id}/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ damaged: Boolean(damaged), description: String(restoreDesc || '').trim(), photos }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        const d = result.data || {}
        dialog.alert(d.restored_damaged ? '已恢复上架（有损坏，已留痕）' : '已恢复上架（无损坏）')
        setRestoreOpen(false)
        nav(-1)
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const brief = record?.instrument_brief || {}
  const restored = Boolean(record?.restored_at)
  const photos = Array.isArray(record?.photos) ? record.photos : []
  const restorePhotosSaved = Array.isArray(record?.restore_photos) ? record.restore_photos : []
  // #2615 B1：后端存的是裸 file_key（/upload 返回）或 /uploads/... 相对路径，weapp
  // <Image> 不吃相对路径（静默不显示）；统一走 utils/media 的 photoSrc 补 origin。
  const photoUrls = photos.map(photoSrc)
  const restorePhotoUrls = restorePhotosSaved.map(photoSrc)

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>丢失记录详情</Text>
        </View>
      )}

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          {loading ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
          ) : notFound || !record ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>未找到该丢失记录</Text>
          ) : (
            <>
              <View style={cardStyle}>
                <View style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                  {brief.cover_image ? (
                    <Image src={photoSrc(brief.cover_image)} mode="aspectFill" style={{ width: 56, height: 56, borderRadius: 8 }} />
                  ) : (
                    <View style={{ width: 56, height: 56, borderRadius: 8, backgroundColor: '#F4F4F5' }} />
                  )}
                  <View style={{ display: 'flex', flexDirection: 'column', gap: 2, flex: 1, minWidth: 0 }}>
                    <Text style={{ fontSize: 14, fontWeight: 'bold', color: '#18181B' }}>SN: {brief.sn || '-'}</Text>
                    <Text style={{ fontSize: 12, color: '#71717A' }}>
                      {brief.category_name || '-'}{brief.level_name ? ` · ${brief.level_name}` : ''}
                    </Text>
                    <Text style={{ fontSize: 12, color: restored ? '#16A34A' : '#DC2626' }}>
                      {restored ? '已恢复' : '丢失中'}
                    </Text>
                  </View>
                </View>
              </View>

              <View style={cardStyle}>
                <Text style={labelStyle}>丢失时间</Text>
                <Text style={valueStyle}>{formatBeijingDateTimeShort(record.created_at)}</Text>
                <Text style={labelStyle}>责任方 · 比例</Text>
                <Text style={valueStyle}>
                  {partyLabels[record.responsible_party] || record.responsible_party} · {record.user_ratio}%
                </Text>
                <Text style={labelStyle}>赔偿 / 用户承担</Text>
                <Text style={valueStyle}>
                  ¥{formatCents(record.compensation_cents || 0)} / ¥{formatCents(record.user_burden_cents || 0)}
                </Text>
                <Text style={labelStyle}>丢失描述</Text>
                <Text style={valueStyle}>{record.description || '-'}</Text>
                <Text style={labelStyle}>现场照片</Text>
                {photos.length === 0 ? (
                  <Text style={{ fontSize: 12, color: '#A1A1AA' }}>无</Text>
                ) : (
                  <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {photos.map((key, i) => (
                      <View
                        key={`${key}-${i}`}
                        data-testid="loss-photo"
                        style={thumbStyle}
                        onClick={() => previewImage({ urls: photoUrls, current: photoSrc(key) })}
                      >
                        <Image src={photoSrc(key)} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
                      </View>
                    ))}
                  </View>
                )}
              </View>

              {restored && (
                <View style={cardStyle}>
                  <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#16A34A' }}>恢复留痕</Text>
                  <Text style={labelStyle}>恢复时间</Text>
                  <Text style={valueStyle}>{formatBeijingDateTimeShort(record.restored_at)}</Text>
                  <Text style={labelStyle}>是否有损坏</Text>
                  <Text style={valueStyle}>{record.restored_damaged ? '有损坏（已留痕）' : '无损坏'}</Text>
                  <Text style={labelStyle}>找回说明</Text>
                  <Text style={valueStyle}>{record.restore_description || '-'}</Text>
                  {restorePhotosSaved.length > 0 && (
                    <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                      {restorePhotosSaved.map((key, i) => (
                        <View
                          key={`r-${key}-${i}`}
                          data-testid="loss-restore-photo"
                          style={thumbStyle}
                          onClick={() => previewImage({ urls: restorePhotoUrls, current: photoSrc(key) })}
                        >
                          <Image src={photoSrc(key)} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
                        </View>
                      ))}
                    </View>
                  )}
                </View>
              )}

              {!restored && (
                <View style={cardStyle}>
                  <Button data-testid="loss-open-restore" onClick={() => setRestoreOpen((v) => !v)} style={btnPrimaryStyle}>
                    {restoreOpen ? '收起恢复表单' : '恢复上架'}
                  </Button>
                  {restoreOpen && (
                    <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
                      <Text style={labelStyle}>是否有损坏（恢复上架，损坏仅留痕）</Text>
                      <View style={{ display: 'flex', gap: 6 }}>
                        {[{ v: false, label: '无损坏' }, { v: true, label: '有损坏' }].map((o) => (
                          <View
                            key={String(o.v)}
                            data-testid={`loss-damaged-${o.v ? 'yes' : 'no'}`}
                            onClick={() => setDamaged(o.v)}
                            style={{ padding: '6px 12px', borderRadius: 8, backgroundColor: damaged === o.v ? '#171717' : '#F4F4F5' }}
                          >
                            <Text style={{ fontSize: 12, color: damaged === o.v ? '#FFFFFF' : '#52525B' }}>{o.label}</Text>
                          </View>
                        ))}
                      </View>
                      <Text style={labelStyle}>找回说明</Text>
                      <Input data-testid="loss-restore-desc" style={inputStyle} value={restoreDesc} placeholder="（可选）" onInput={(e) => setRestoreDesc(getInputValue(e))} />
                      <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                        {restorePhotos.map((f, i) => (
                          <View key={i} style={thumbStyle}>
                            <Image src={env.isMiniProgram ? f : URL.createObjectURL(f)} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
                          </View>
                        ))}
                        {restorePhotos.length < MAX_PHOTOS && (
                          <View
                            onClick={choosePhotos}
                            style={{ width: 56, height: 56, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
                          >
                            <Text style={{ fontSize: 18, color: '#A1A1AA' }}>＋</Text>
                          </View>
                        )}
                      </View>
                      <Button data-testid="loss-submit-restore" disabled={busy} onClick={submitRestore}
                        style={{ ...btnDangerStyle, opacity: busy ? 0.5 : 1 }}>
                        {busy ? '处理中...' : '确认恢复'}
                      </Button>
                    </View>
                  )}
                </View>
              )}
            </>
          )}
        </View>
      </ScrollView>
    </View>
  )
}
