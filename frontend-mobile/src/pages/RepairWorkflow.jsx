import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { Button, Image, ScrollView, Text, Textarea, View } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, uploadFile, storage, session, previewImage } from '../platform'
import { formatBeijingDateTimeShort } from '../utils/format'
import { parsePhotos, photoSrc } from '../utils/media'

const statusLabels = {
  repair_pending: '待维修', repair_in_progress: '维修中', repair_completed: '已修复',
}

const damageStatusLabels = {
  pending: '待确认', completed: '已完成', agreed: '已确认', appealed: '申诉中', cancelled: '已撤销', resolved: '已解决',
}

export default function RepairWorkflow() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const instrumentId = searchParams.get('instrument_id')
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [records, setRecords] = useState([])
  const [damage, setDamage] = useState(null)
  const [comment, setComment] = useState('')
  const [capturedPhotos, setCapturedPhotos] = useState([])
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [submittingRecord, setSubmittingRecord] = useState(false)

  const token = getToken()
  const currentUserId = token ? JSON.parse(atob(token.split('.')[1]))?.sub || '' : ''

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setCapturedPhotos(prev => [...prev, ...files].slice(0, 10))
  }

  const handlePhotoCaptureWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: 10 - capturedPhotos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setCapturedPhotos(prev => [...prev, ...(res.tempFilePaths || [])].slice(0, 10))
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const removePhoto = (index) => {
    setCapturedPhotos(prev => prev.filter((_, i) => i !== index))
  }

  const previewPhoto = (file) => {
    const url = env.isMiniProgram ? file : URL.createObjectURL(file)
    previewImage({ urls: [url], current: url })
  }

  const fetchData = async () => {
    if (!instrumentId) return
    setLoading(true)
    try {
      const [instRes, recRes] = await Promise.all([
        apiFetch(`${baseUrl}/instruments/${instrumentId}`),
        apiFetch(`${baseUrl}/repair/${instrumentId}/records`),
      ])
      const inst = await instRes.json()
      const rec = await recRes.json()
      if (inst.code === 20000) setInstrument(inst.data)
      if (rec.code === 20000) {
        setRecords(rec.data?.records || [])
        setDamage(rec.data?.damage || null)
      }
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchData() }, [instrumentId])

  const handleAction = async (action) => {
    if (!instrumentId) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/${action}`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        await fetchData()
      } else {
        dialog.alert(resolveErrorMessage(result, '操作失败'))
      }
    } catch (err) {
      dialog.alert('操作失败: ' + (err.message || ''))
    }
    setActionLoading(false)
  }

  const handleTakeover = async () => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/takeover`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        await fetchData()
      } else {
        dialog.alert(resolveErrorMessage(result, '接手失败'))
      }
    } catch (err) {
      dialog.alert('接手失败: ' + (err.message || ''))
    }
    setActionLoading(false)
  }

  const goBack = () => {
    if (env.isMiniProgram) Taro.navigateBack()
    else navigate(-1)
  }

  if (!instrumentId) {
    return (
      <View style={{ backgroundColor: "#FDFBF7" }} className="h-screen flex items-center justify-center p-4">
        <Text className="text-zinc-400">请扫描或选择乐器</Text>
      </View>
    )
  }

  if (loading) {
    return <View className="h-screen bg-zinc-50 flex items-center justify-center"><Text className="text-zinc-400">加载中...</Text></View>
  }

  if (!instrument) {
    return <View className="h-screen bg-zinc-50 flex items-center justify-center"><Text className="text-zinc-400">乐器不存在</Text></View>
  }

  const status = instrument.repair_status
  const workerId = instrument.repair_worker_id
  const isMyJob = workerId === currentUserId
  const isValid = ['repair_pending', 'repair_in_progress', 'repair_completed'].includes(status)

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      {!env.isMiniProgram && (
        <View className="bg-white px-4 py-3 border-b border-zinc-100 flex items-center gap-2">
          <Text className="text-lg mr-2" onClick={goBack}>{'<'}</Text>
          <Text className="text-lg font-bold flex-1">维修 - {instrument.sn || ''}</Text>
          <Text className={`text-xs px-2 py-1 rounded-full font-bold ${status === 'repair_completed' ? 'bg-green-100 text-green-700' : status === 'repair_in_progress' ? 'bg-yellow-100 text-yellow-700' : 'bg-blue-100 text-blue-700'}`}>
            {statusLabels[status] || status}
          </Text>
        </View>
      )}

      <ScrollView scrollY className="flex-1 min-h-0 overflow-y-auto">
        <View style={{ padding: '0 16px', boxSizing: 'border-box' }}>
        {/* Instrument info */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black">乐器信息</Text>
          <View className="mt-2 text-sm" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <Text className="text-zinc-500">编号: <Text className="text-black">{instrument.sn || '-'}</Text></Text>
            <Text className="text-zinc-500">类别: <Text className="text-black">{instrument.category_name || '-'}</Text></Text>
            {instrument.repair_worker_name && <Text className="text-zinc-500">负责人: <Text className="text-black">{instrument.repair_worker_name}</Text></Text>}
          </View>
        </View>

        {/* (#1866) Damage info */}
        {damage && (damage.damage_description || damage.notes) && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
            <Text className="text-sm font-bold text-black mb-2">定损信息</Text>
            <View style={{ display: 'flex', flexDirection: 'column', gap: 4 }} className="text-sm">
              {damage.lease_id && <Text className="text-zinc-500">来源订单: <Text className="text-black">{damage.lease_id.slice(0, 8)}</Text></Text>}
              {damage.status && <Text className="text-zinc-500">状态: <Text className="text-black">{damageStatusLabels[damage.status] || damage.status}</Text></Text>}
              {damage.damage_amount != null && <Text className="text-zinc-500">赔偿金额: <Text className="text-black">¥{(damage.damage_amount / 100).toFixed(2)}</Text></Text>}
              {damage.damage_description && <Text className="text-zinc-500">损坏描述: <Text className="text-black">{damage.damage_description}</Text></Text>}
              {damage.notes && <Text className="text-zinc-500">员工评语: <Text className="text-black">{damage.notes}</Text></Text>}
            </View>
          </View>
        )}

        {/* Repair records */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black">维修记录 ({records.length})</Text>
          {records.length === 0 ? (
            <View style={{ marginTop: 8 }}>
              <Text className="text-xs text-zinc-400">暂无记录</Text>
            </View>
          ) : (
            <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {records.map(r => {
                const recordPhotos = parsePhotos(r.photos)
                return (
                  <View key={r.id} className="border-b border-zinc-100 pb-3">
                    {r.comment && (
                      <View><Text className="text-sm text-black">{r.comment}</Text></View>
                    )}
                    {recordPhotos.length > 0 && (
                      <View className="flex flex-wrap gap-1 mt-2">
                        {recordPhotos.map((p, i) => (
                          <Image key={i} src={photoSrc(p)} mode="aspectFill"
                            className="w-16 h-16 rounded object-cover"
                            onClick={() => previewImage({ urls: recordPhotos.map(photoSrc), current: photoSrc(p) })} />
                        ))}
                      </View>
                    )}
                    <View className="mt-1">
                      <Text className="text-xs text-zinc-400">
                        {formatBeijingDateTimeShort(r.created_at)}{r.worker_name ? ` · ${r.worker_name}` : ''}
                      </Text>
                    </View>
                  </View>
                )
              })}
            </View>
          )}
        </View>

        {/* Status-specific actions */}
        {status === 'repair_pending' && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-600">此乐器等待维修</Text>
            <Button onClick={() => handleAction('start')} disabled={actionLoading || submittingRecord}
              className="w-full mt-3 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '处理中...' : '开始维修'}
            </Button>
          </View>
        )}

        {status === 'repair_in_progress' && isMyJob && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-2">添加记录</Text>
            <Textarea className="w-full border border-zinc-300 rounded-lg p-3 text-sm"
              value={comment} onInput={e => setComment(getInputValue(e))} placeholder="输入评论..." />
            
            {/* Photo Capture Section */}
            <View className="mt-3">
              <Text className="text-sm font-bold text-black mb-2">📷 拍照存档</Text>
              <View className="grid grid-cols-3 gap-2 mb-2">
                {capturedPhotos.map((file, i) => (
                  <View key={i} className="relative aspect-square rounded-lg overflow-hidden border" onClick={() => previewPhoto(file)}>
                    <Image src={env.isMiniProgram ? file : URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                    <Button onClick={(e) => { e.stopPropagation(); removePhoto(i) }}
                      className="absolute top-1 right-1 rounded-full w-5 h-5 flex items-center justify-center">
                      <Text className="text-white text-xs">✕</Text>
                    </Button>
                  </View>
                ))}
                {capturedPhotos.length < 10 && (
                  env.isMiniProgram ? (
                    <View className="aspect-square border-2 border-dashed border-zinc-300 rounded-lg flex flex-col items-center justify-center text-zinc-400" onClick={handlePhotoCaptureWeapp}>
                      <Text className="text-2xl">📷</Text><Text className="text-xs mt-1">拍摄</Text>
                    </View>
                  ) : (
                    <label className="aspect-square border-2 border-dashed border-zinc-300 rounded-lg flex flex-col items-center justify-center cursor-pointer text-zinc-400">
                      <Text className="text-2xl">📷</Text><Text className="text-xs mt-1">拍摄</Text>
                      <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
                    </label>
                  )
                )}
              </View>
              <Text className="text-xs text-zinc-400">已拍摄 {capturedPhotos.length} 张，最多 10 张</Text>
            </View>

            {/* Submit Record Button */}
            <Button onClick={async () => {
              if (!comment && capturedPhotos.length === 0) { dialog.alert('请输入评论或拍照'); return }
              setSubmittingRecord(true)
              try {
                const photoUrls = []
                const token = storage.getItem('token') || session.getItem('token')
                for (const file of capturedPhotos) {
                  const uploadResp = await uploadFile(`${baseUrl}/upload`, file, {
                    headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
                  })
                  const uploadResult = env.isMiniProgram ? JSON.parse(uploadResp.data || '{}') : await uploadResp.json()
                  if (uploadResult.code === 20000 && uploadResult.data?.url) photoUrls.push(uploadResult.data.url)
                }
                const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/records`, {
                  method: 'POST',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({ comment, photos: photoUrls }),
                })
                const r = await resp.json()
                if (r.code === 20000) { setComment(''); setCapturedPhotos([]); await fetchData() }
                else { dialog.alert(resolveErrorMessage(r, '提交失败')) }
              } catch (err) { dialog.alert('提交失败: ' + (err.message || '')) }
              setSubmittingRecord(false)
            }} disabled={submittingRecord || actionLoading}
              className="w-full mt-3 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              {submittingRecord ? '处理中...' : '提交记录'}
            </Button>

            <Button onClick={() => handleAction('complete')} disabled={actionLoading || submittingRecord}
              className="w-full mt-3 py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '处理中...' : '维修完成'}
            </Button>
          </View>
        )}

        {status === 'repair_in_progress' && !isMyJob && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-600">此乐器由 {instrument.repair_worker_name || '其他师傅'} 负责处理中</Text>
            <Button onClick={handleTakeover} disabled={actionLoading || submittingRecord}
              className="w-full mt-3 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '处理中...' : '接手'}
            </Button>
          </View>
        )}

        {status === 'repair_completed' && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-600">乐器已修复，等待验收</Text>
            <View className="flex gap-2 mt-3">
              <Button onClick={() => handleAction('accept')} disabled={actionLoading || submittingRecord}
                className="flex-1 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
                验收通过
              </Button>
              <Button onClick={async () => {
                let reason = ''
                if (env.isMiniProgram) {
                  const res = await Taro.showModal({ title: '验收不通过', editable: true, placeholderText: '请输入不通过原因' })
                  reason = res.confirm ? (res.content || '') : ''
                } else {
                  reason = prompt('请输入不通过原因')
                }
                if (!reason) return
                try {
                  const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/reject`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ comment: reason }),
                  })
                  const r = await resp.json()
                  if (r.code === 20000) { await fetchData() }
                  else { dialog.alert(resolveErrorMessage(r)) }
                } catch {}
              }} className="flex-1 py-3 bg-red-500 text-white rounded-xl font-bold text-sm text-center">
                验收不通过
              </Button>
            </View>
          </View>
        )}

        {!isValid && !damage && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-400 text-center">乐器状态正常，不需要维修</Text>
          </View>
        )}
        </View>
      </ScrollView>
    </View>
  )
}
