import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button, Image, Video, Input, Textarea } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import RepairRecordPanel from '../components/RepairRecordPanel'

export default function RepairRequestDetail() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get('request_id')
  const baseUrl = env.apiBaseUrl

  const [request, setRequest] = useState(null)
  const [records, setRecords] = useState([])
  const [roles, setRoles] = useState([])
  const [quotes, setQuotes] = useState([])
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [quoteForm, setQuoteForm] = useState({ material_fee: '', service_fee: '', logistics_fee: '', duration: '', comment: '' })
  const [editingQuoteId, setEditingQuoteId] = useState(null)
  const [showQuoteForm, setShowQuoteForm] = useState(false)
  const [trackingCompany, setTrackingCompany] = useState('')
  const [trackingNumber, setTrackingNumber] = useState('')
  const [returnCompany, setReturnCompany] = useState('')
  const [returnNumber, setReturnNumber] = useState('')
  const [unpackPhotos, setUnpackPhotos] = useState([])

  const token = getToken()
  const isCustomer = (() => {
    if (!token) return true
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload?.role === 'USER' || !payload?.role
    } catch { return true }
  })()
  const isTechnician = roles.includes('repair_technician')
  const isSiteStaff = roles.some(r => ['site_admin', 'site_member'].includes(r))

  const fetchData = async () => {
    if (!requestId) return
    setLoading(true)
    try {
      const [reqRes, recRes, roleRes, quoteRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-requests/${requestId}`),
        apiFetch(`${baseUrl}/repair-requests/${requestId}/records`),
        apiFetch(`${baseUrl}/site-members/me`),
        apiFetch(`${baseUrl}/repair-requests/${requestId}/quotes`),
      ])
      const req = await reqRes.json()
      const rec = await recRes.json()
      const role = await roleRes.json()
      const q = await quoteRes.json()
      if (req.code === 20000) setRequest(req.data)
      if (rec.code === 20000) setRecords(rec.data?.records || [])
      if (role.code === 20000) setRoles(role.data?.roles || [])
      if (q.code === 20000) setQuotes(q.data?.list || [])
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchData() }, [requestId])

  const handleAction = async (action, body = {}) => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${requestId}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        setQuoteForm({ material_fee: '', service_fee: '', logistics_fee: '', duration: '', comment: '' })
        setEditingQuoteId(null)
        setShowQuoteForm(false)
        await fetchData()
      } else {
        alert(r.message || '操作失败')
      }
    } catch (err) { alert('操作失败') }
    setActionLoading(false)
  }

  const handleAcceptQuote = async (quoteId) => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${requestId}/quotes/${quoteId}/accept`, { method: 'POST' })
      const r = await resp.json()
      if (r.code === 20000) {
        navigate(`/repair-quote?request_id=${requestId}`)
      } else {
        alert(r.message || '操作失败')
      }
    } catch { alert('操作失败') }
    setActionLoading(false)
  }

  const handleSubmitTracking = async () => {
    if (!trackingCompany || !trackingNumber) { alert('请填写物流公司和单号'); return }
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${requestId}/tracking`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tracking_company: trackingCompany, tracking_number: trackingNumber }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        setTrackingCompany('')
        setTrackingNumber('')
        await fetchData()
      } else {
        alert(r.message || '提交失败')
      }
    } catch { alert('提交失败') }
    setActionLoading(false)
  }

  const handleAppeal = async () => {
    setActionLoading(true)
    try {
      const reason = prompt('请输入申诉原因')
      if (!reason) { setActionLoading(false); return }
      const resp = await apiFetch(`${baseUrl}/repair-appeals`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          category: 'repair',
          object_type: 'repair_request',
          object_id: requestId,
          description: reason,
        }),
      })
      const r = await resp.json()
      if (r.code === 20000) { await fetchData() }
      else { alert(r.message || '申诉提交失败') }
    } catch { alert('操作失败') }
    setActionLoading(false)
  }

  const handleConfirmReceipt = async () => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${requestId}/confirm-receipt`, { method: 'POST' })
      const r = await resp.json()
      if (r.code === 20000) { await fetchData() }
      else { alert(r.message || '操作失败') }
    } catch { alert('操作失败') }
    setActionLoading(false)
  }

  const handleReturnShipping = async () => {
    if (!returnCompany || !returnNumber) { alert('请填写物流公司和单号'); return }
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${requestId}/return-shipping`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ return_company: returnCompany, return_tracking_number: returnNumber }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        setReturnCompany('')
        setReturnNumber('')
        await fetchData()
      } else {
        alert(r.message || '提交失败')
      }
    } catch { alert('提交失败') }
    setActionLoading(false)
  }

  const handlePhotoUpload = async (e) => {
    const file = e.target?.files?.[0] || e.detail?.value?.[0]
    if (!file) return
    const fd = new FormData()
    fd.append('file', file)
    try {
      const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', body: fd })
      const r = await resp.json()
      if (r.code === 20000) {
        setUnpackPhotos(p => [...p, r.data.file_key])
      }
    } catch {}
  }

  if (!requestId) return <View className="h-screen flex items-center justify-center"><Text>请选择报修单</Text></View>
  if (loading) return <View className="h-screen flex items-center justify-center"><Text className="text-zinc-400">加载中...</Text></View>
  if (!request) return <View className="h-screen flex items-center justify-center"><Text className="text-zinc-400">报修单不存在</Text></View>

  const status = request.status

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100 flex items-center gap-2">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1">报修详情</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        {/* Request info */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <View><Text className="text-sm font-bold text-black">报修信息</Text></View>
          <View className="space-y-2 mt-3">
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">识别码</Text>
              <Text className="text-xs text-zinc-600">{request.instrument_sn || '-'}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">状态</Text>
              <Text className="text-xs text-zinc-600">{status}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">乐器类别</Text>
              <Text className="text-xs text-zinc-600">{request.instrument_type || '-'}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">品牌</Text>
              <Text className="text-xs text-zinc-600">{request.brand || '-'}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">型号</Text>
              <Text className="text-xs text-zinc-600">{request.model || '-'}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">描述</Text>
              <Text className="text-xs text-zinc-600">{request.description || '-'}</Text>
            </View>
            {isCustomer && (
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">报修人</Text>
              <Text className="text-xs text-zinc-600">{request.reporter_name || '-'}</Text>
            </View>
            )}
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">商户</Text>
              <Text className="text-xs text-zinc-600">{request.merchant_name || '-'}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">网点</Text>
              <Text className="text-xs text-zinc-600">{request.site_name || '-'}</Text>
            </View>
            <View className="flex justify-between items-center">
              <Text className="text-xs text-zinc-400">创建时间</Text>
              <Text className="text-xs text-zinc-600">{request.created_at ? new Date(request.created_at).toLocaleString() : '-'}</Text>
            </View>
          </View>
        </View>

        {/* Media: photos and video */}
        {request.photos && JSON.parse(request.photos).length > 0 && (
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <View><Text className="text-sm font-bold text-black mb-2">图片</Text></View>
          <View className="flex flex-wrap gap-2">
            {JSON.parse(request.photos).map((p, i) => (
              <Image key={i} src={`/uploads/media/${p}`} className="w-24 h-24 rounded-lg object-cover" mode="aspectFill" />
            ))}
          </View>
        </View>
        )}
        {request.video_url && (
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <View><Text className="text-sm font-bold text-black mb-2">视频</Text></View>
          <Video src={`/uploads/media/${request.video_url}`} className="w-full h-48 rounded-lg" controls />
        </View>
        )}

        {/* Repair records panel — for repair requester and technician */}
        {(isCustomer || isTechnician) && (
          <RepairRecordPanel instrumentId={requestId} records={records} baseUrl={baseUrl}
            onRecordAdded={fetchData} />
        )}

        {/* ========== PENDING ASSESSMENT ========== */}
        {status === 'pending_assessment' && isTechnician && (
          <>
          {/* Quote list (always shown) */}
          {quotes.length > 0 && (
            <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
              <Text className="text-sm font-bold text-black mb-3">本网点报价 ({quotes.filter(q => q.status === 'pending').length})</Text>
              {quotes.filter(q => q.status === 'pending').map(q => (
                <View key={q.id} className="border border-zinc-200 rounded-xl p-3 mb-3">
                  <View className="space-y-1">
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">报价单号</Text>
                      <Text className="text-xs text-zinc-700">{q.quote_no}</Text>
                    </View>
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">材料费</Text>
                      <Text className="text-xs text-zinc-700">¥{q.material_fee}</Text>
                    </View>
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">服务费</Text>
                      <Text className="text-xs text-zinc-700">¥{q.service_fee}</Text>
                    </View>
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">物流费</Text>
                      <Text className="text-xs text-zinc-700">¥{q.logistics_fee}</Text>
                    </View>
                    {q.duration && (
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">工期</Text>
                      <Text className="text-xs text-zinc-700">{q.duration}</Text>
                    </View>
                    )}
                    {q.comment && (
                    <View className="mt-1">
                      <Text className="text-xs text-zinc-500">备注：{q.comment}</Text>
                    </View>
                    )}
                  </View>
                </View>
              ))}
            </View>
          )}

          {/* Quote form (hidden by default, shown when adding/editing) */}
          {showQuoteForm && (
            <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
              <Text className="text-sm font-bold text-black mb-3">
                {editingQuoteId ? '编辑报价' : '提交报价'}
              </Text>
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.material_fee} onInput={e => setQuoteForm(p => ({ ...p, material_fee: e.detail?.value || e.target?.value || '' }))}
                placeholder="材料费（元）" type="number" />
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.service_fee} onInput={e => setQuoteForm(p => ({ ...p, service_fee: e.detail?.value || e.target?.value || '' }))}
                placeholder="服务费（元）" type="number" />
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.logistics_fee} onInput={e => setQuoteForm(p => ({ ...p, logistics_fee: e.detail?.value || e.target?.value || '' }))}
                placeholder="物流费（元）" type="number" />
              <View className="flex items-center gap-2 mb-2">
                <Input className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
                  value={quoteForm.duration} onInput={e => setQuoteForm(p => ({ ...p, duration: e.detail?.value || e.target?.value || '' }))}
                  placeholder="工期" type="number" />
                <Text className="text-sm text-zinc-500">天</Text>
              </View>
              <Textarea className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2 min-h-[60px]"
                value={quoteForm.comment} onInput={e => setQuoteForm(p => ({ ...p, comment: e.detail?.value || e.target?.value || '' }))}
                placeholder="报价备注（禁止含联系方式）" />
              <View className="flex gap-2">
                <Button onClick={() => handleAction('quotes', {
                  material_fee: Number(quoteForm.material_fee),
                  service_fee: Number(quoteForm.service_fee),
                  logistics_fee: Number(quoteForm.logistics_fee),
                  duration: quoteForm.duration ? quoteForm.duration + '天' : '',
                  comment: quoteForm.comment,
                })}
                  disabled={actionLoading || !quoteForm.material_fee || !quoteForm.service_fee}
                  className="flex-1 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
                  {editingQuoteId ? '保存修改' : '提交报价'}
                </Button>
                <Button onClick={() => { setShowQuoteForm(false); setEditingQuoteId(null); setQuoteForm({ material_fee: '', service_fee: '', logistics_fee: '', duration: '', comment: '' }) }}
                  className="py-3 px-4 border border-zinc-300 rounded-xl font-bold text-sm text-zinc-600 text-center">
                  取消
                </Button>
              </View>
            </View>
          )}

          {/* Add / Edit buttons */}
          {!showQuoteForm && (
            <View className="space-y-2 mt-2 mb-4">
              <Button onClick={() => { setShowQuoteForm(true); setEditingQuoteId(null); setQuoteForm({ material_fee: '', service_fee: '', logistics_fee: '', duration: '', comment: '' }) }}
                className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
                添加报价
              </Button>
              {quotes.filter(q => q.status === 'pending').slice(-1).map(q => (
                <Button key={q.id} onClick={() => {
                  setEditingQuoteId(q.id)
                  setQuoteForm({
                    material_fee: String(q.material_fee || ''),
                    service_fee: String(q.service_fee || ''),
                    logistics_fee: String(q.logistics_fee || ''),
                    duration: q.duration ? q.duration.replace('天', '') : '',
                    comment: q.comment || '',
                  })
                  setShowQuoteForm(true)
                }}
                  className="w-full py-2 border border-zinc-300 rounded-xl font-bold text-sm text-zinc-600 text-center">
                  编辑最新报价
                </Button>
              ))}
            </View>
          )}
          </>
        )}

        {status === 'pending_assessment' && isCustomer && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">报价列表</Text>
            {quotes.length === 0 ? (
              <Text className="text-xs text-zinc-400">暂无报价</Text>
            ) : (
              quotes.filter(q => q.status === 'pending').map(q => (
                <View key={q.id} className="border border-zinc-200 rounded-xl p-3 mb-3">
                  <View className="space-y-1">
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">报价单号</Text>
                      <Text className="text-xs text-zinc-700">{q.quote_no}</Text>
                    </View>
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">材料费</Text>
                      <Text className="text-xs text-zinc-700">¥{q.material_fee}</Text>
                    </View>
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">服务费</Text>
                      <Text className="text-xs text-zinc-700">¥{q.service_fee}</Text>
                    </View>
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">物流费</Text>
                      <Text className="text-xs text-zinc-700">¥{q.logistics_fee}</Text>
                    </View>
                    {q.duration && (
                    <View className="flex justify-between">
                      <Text className="text-xs text-zinc-500">工期</Text>
                      <Text className="text-xs text-zinc-700">{q.duration}</Text>
                    </View>
                    )}
                    {q.comment && (
                    <View className="mt-1">
                      <Text className="text-xs text-zinc-500">备注：{q.comment}</Text>
                    </View>
                    )}
                  </View>
                  <Button onClick={() => handleAcceptQuote(q.id)} disabled={actionLoading}
                    className="w-full mt-2 py-2 bg-blue-600 text-white rounded-xl font-bold text-sm text-center">
                    接受此报价
                  </Button>
                </View>
              ))
            )}
          </View>
        )}

        {status === 'pending_payment' && isCustomer && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">待付款</Text>
            <Text className="text-xs text-zinc-500 mb-3">您已接受报价，请前往支付。</Text>
            <Button onClick={() => navigate(`/repair-quote?request_id=${requestId}`)}
              className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              去支付
            </Button>
          </View>
        )}

        {status === 'pending_ship' && isCustomer && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">待发送</Text>
            <View className="bg-zinc-50 rounded-xl p-3 mb-3">
              <Text className="text-xs text-zinc-500 mb-1">收货信息</Text>
              {request.merchant_type === 'controlled' ? (
                <View className="space-y-1">
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">地址</Text>
                    <Text className="text-xs text-zinc-700 text-right">{request.transit_site_address || '-'}</Text>
                  </View>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">电话</Text>
                    <Text className="text-xs text-zinc-700">{request.transit_site_phone || '-'}</Text>
                  </View>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">中转单号</Text>
                    <Text className="text-xs text-zinc-700">{request.transit_order_number || '-'}</Text>
                  </View>
                </View>
              ) : (
                <View className="space-y-1">
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">商户</Text>
                    <Text className="text-xs text-zinc-700">{request.merchant_name || '-'}</Text>
                  </View>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">网点</Text>
                    <Text className="text-xs text-zinc-700">{request.site_name || '-'}</Text>
                  </View>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">地址</Text>
                    <Text className="text-xs text-zinc-700 text-right">{request.site_address || '-'}</Text>
                  </View>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-400">电话</Text>
                    <Text className="text-xs text-zinc-700">{request.site_phone || '-'}</Text>
                  </View>
                </View>
              )}
            </View>
            <Text className="text-xs text-red-500 mb-2">* 请将{request.merchant_type === 'controlled' ? '中转单号' : '物流单号'}写入物流留言</Text>
            <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={trackingCompany} onInput={e => setTrackingCompany(e.detail?.value || e.target?.value || '')}
              placeholder="物流公司" />
            <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={trackingNumber} onInput={e => setTrackingNumber(e.detail?.value || e.target?.value || '')}
              placeholder="物流单号" />
            <Button onClick={handleSubmitTracking} disabled={actionLoading || !trackingCompany || !trackingNumber}
              className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '提交中...' : '提交发货'}
            </Button>
          </View>
        )}

        {/* ========== REPAIRING ========== */}
        {status === 'repairing' && isTechnician && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">维修操作</Text>
            <Button onClick={() => handleAction('complete')} disabled={actionLoading}
              className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center mb-3">
              维修完成
            </Button>
            <View className="border-t border-zinc-200 pt-3">
              <Text className="text-xs text-zinc-500 mb-2">如需调整报价，可重新报价一次：</Text>
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.material_fee} onInput={e => setQuoteForm(p => ({ ...p, material_fee: e.detail?.value || e.target?.value || '' }))}
                placeholder="材料费（元）" type="number" />
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.service_fee} onInput={e => setQuoteForm(p => ({ ...p, service_fee: e.detail?.value || e.target?.value || '' }))}
                placeholder="服务费（元）" type="number" />
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.logistics_fee} onInput={e => setQuoteForm(p => ({ ...p, logistics_fee: e.detail?.value || e.target?.value || '' }))}
                placeholder="物流费（元）" type="number" />
              <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
                value={quoteForm.duration} onInput={e => setQuoteForm(p => ({ ...p, duration: e.detail?.value || e.target?.value || '' }))}
                placeholder="工期（如：3个工作日）" />
              <Button onClick={() => handleAction('requote', {
                material_fee: Number(quoteForm.material_fee),
                service_fee: Number(quoteForm.service_fee),
                logistics_fee: Number(quoteForm.logistics_fee),
                duration: quoteForm.duration,
              })}
                disabled={actionLoading || !quoteForm.material_fee || !quoteForm.service_fee}
                className="w-full py-2 bg-yellow-600 text-white rounded-xl font-bold text-sm text-center">
                重新报价
              </Button>
            </View>
          </View>
        )}

        {/* ========== RETURNED ========== */}
        {status === 'returned' && isCustomer && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">已发回</Text>
            <Text className="text-xs text-zinc-500 mb-3">乐器已发回，请确认收货。</Text>
            <Button onClick={handleConfirmReceipt} disabled={actionLoading}
              className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center mb-3">
              确认收货
            </Button>
            <Button onClick={handleAppeal} disabled={actionLoading}
              className="w-full py-2 bg-red-500 text-white rounded-xl font-bold text-sm text-center">
              申诉
            </Button>
          </View>
        )}

        {/* ========== TRANSIT PROCESSING (transit staff) ========== */}
        {status === 'transit_processing' && isSiteStaff && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">中转处理</Text>
            <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={quoteForm.material_fee} onInput={e => setQuoteForm(p => ({ ...p, material_fee: e.detail?.value || e.target?.value || '' }))}
              placeholder="中转服务费（元）" type="number" />
            <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={quoteForm.service_fee} onInput={e => setQuoteForm(p => ({ ...p, service_fee: e.detail?.value || e.target?.value || '' }))}
              placeholder="中转物流费（元）" type="number" />
            <Button onClick={() => handleAction('transit-process', {
              transit_service_fee: Number(quoteForm.material_fee),
              transit_logistics_fee: Number(quoteForm.service_fee),
            })}
              disabled={actionLoading || !quoteForm.material_fee}
              className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              提交中转处理
            </Button>
          </View>
        )}

        {/* ========== SHIPPING (staff receive) ========== */}
        {status === 'shipping' && isSiteStaff && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">收货处理</Text>
            <View className="bg-zinc-50 rounded-xl p-3 mb-3 space-y-1">
              <Text className="text-xs text-zinc-500">物流信息</Text>
              {request.tracking_company && <Text className="text-xs text-zinc-700">物流公司：{request.tracking_company}</Text>}
              {request.tracking_number && <Text className="text-xs text-zinc-700">物流单号：{request.tracking_number}</Text>}
              {!request.tracking_company && <Text className="text-xs text-zinc-400">暂无物流信息</Text>}
            </View>
            {request.merchant_type === 'controlled' ? (
              <>
                <View className="flex flex-wrap gap-2 mb-3">
                  {unpackPhotos.length > 0 && unpackPhotos.map((p, i) => (
                    <Image key={i} src={`/uploads/media/${p}`} className="w-16 h-16 rounded object-cover" mode="aspectFill" />
                  ))}
                  <View className="w-16 h-16 border-2 border-dashed border-zinc-300 rounded flex items-center justify-center" onClick={() => { const el = document.createElement('input'); el.type='file'; el.accept='image/*'; el.onchange=e=>handlePhotoUpload(e); el.click() }}>
                    <Text className="text-2xl text-zinc-300">+</Text>
                  </View>
                </View>
                <Button onClick={() => handleAction('transit-relay', { direction: 'in', transit_order_number: request.transit_order_number || '', unpack_photos: unpackPhotos })}
                  disabled={actionLoading}
                  className="w-full py-3 bg-blue-600 text-white rounded-xl font-bold text-sm text-center">
                  中转处理
                </Button>
              </>
            ) : (
              <Button onClick={() => handleAction('receive')} disabled={actionLoading}
                className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
                确认收货
              </Button>
            )}
          </View>
        )}

        {/* ========== TRANSIT IN (staff receive) ========== */}
        {status === 'transit_in' && isSiteStaff && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">转入收货</Text>
            <Button onClick={() => handleAction('receive')} disabled={actionLoading}
              className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
              确认收货
            </Button>
          </View>
        )}

        {/* ========== RETURN PENDING (staff fill return logistics) ========== */}
        {status === 'return_pending' && isSiteStaff && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">发回物流</Text>
            <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={returnCompany} onInput={e => setReturnCompany(e.detail?.value || e.target?.value || '')}
              placeholder="物流公司" />
            <Input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={returnNumber} onInput={e => setReturnNumber(e.detail?.value || e.target?.value || '')}
              placeholder="物流单号" />
            <Button onClick={handleReturnShipping} disabled={actionLoading || !returnCompany || !returnNumber}
              className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              提交发回
            </Button>
          </View>
        )}

        {/* ========== TRANSIT OUT (staff relay out) ========== */}
        {status === 'transit_out' && isSiteStaff && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-3">转出中转</Text>
            <View className="flex flex-wrap gap-2 mb-3">
              {unpackPhotos.length > 0 && unpackPhotos.map((p, i) => (
                <Image key={i} src={`/uploads/media/${p}`} className="w-16 h-16 rounded object-cover" mode="aspectFill" />
              ))}
              <View className="w-16 h-16 border-2 border-dashed border-zinc-300 rounded flex items-center justify-center" onClick={() => { const el = document.createElement('input'); el.type='file'; el.accept='image/*'; el.onchange=e=>handlePhotoUpload(e); el.click() }}>
                <Text className="text-2xl text-zinc-300">+</Text>
              </View>
            </View>
            <Button onClick={() => handleAction('transit-relay', { direction: 'out', transit_order_number: request.transit_order_number || '', unpack_photos: unpackPhotos })}
              disabled={actionLoading}
              className="w-full py-3 bg-blue-600 text-white rounded-xl font-bold text-sm text-center">
              转出处理
            </Button>
          </View>
        )}
      </ScrollView>
    </View>
  )
}
