import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import RepairRecordPanel from '../components/RepairRecordPanel'

export default function RepairRequestDetail() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get('request_id')
  const baseUrl = env.apiBaseUrl

  const [request, setRequest] = useState(null)
  const [records, setRecords] = useState([])
  const [quoteAmount, setQuoteAmount] = useState('')
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)

  const fetchData = async () => {
    if (!requestId) return
    setLoading(true)
    try {
      const [reqRes, recRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-requests/${requestId}`),
        apiFetch(`${baseUrl}/repair-requests/${requestId}/records`),
      ])
      const req = await reqRes.json()
      const rec = await recRes.json()
      if (req.code === 20000) setRequest(req.data)
      if (rec.code === 20000) setRecords(rec.data?.records || [])
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
      if (r.code === 20000) { await fetchData() }
      else { alert(r.message || '操作失败') }
    } catch (err) { alert('操作失败') }
    setActionLoading(false)
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
          <Text className="text-sm font-bold text-black mb-2">报修信息</Text>
          <Text className="text-xs text-zinc-500">状态: {status}</Text>
          <Text className="text-xs text-zinc-500">描述: {request.description || '-'}</Text>
          {request.quote_amount && <Text className="text-xs text-zinc-500">报价: ¥{request.quote_amount}</Text>}
        </View>

        {/* Repair records */}
        <RepairRecordPanel instrumentId={requestId} records={records} baseUrl={baseUrl}
          onRecordAdded={fetchData} />

        {/* Actions for inspecting */}
        {status === 'inspecting' && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-2">提交报价</Text>
            <Text className="text-xs text-red-500 mb-2">* 请拍摄乐器识别码特写照片</Text>
            <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={quoteAmount} onChange={e => setQuoteAmount(e.target.value)} placeholder="报价金额（元）" type="number" />
            <Button onClick={() => handleAction('quote', { quote_amount: quoteAmount })}
              disabled={actionLoading || !quoteAmount}
              className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              提交报价
            </Button>
          </View>
        )}

        {/* Actions for repairing */}
        {status === 'repairing' && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-xs text-red-500 mb-2">* 维修完成前请拍摄识别码特写照片作为记录</Text>
            <Button onClick={() => handleAction('complete')} disabled={actionLoading}
              className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
              维修完成
            </Button>
          </View>
        )}
      </ScrollView>
    </View>
  )
}
