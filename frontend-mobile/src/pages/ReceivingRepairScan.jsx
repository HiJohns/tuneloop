import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Button } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function ReceivingRepairScan() {
  const navigate = useNavigate()
  const [sn, setSn] = useState('')
  const [result, setResult] = useState(null)
  const [searching, setSearching] = useState(false)
  const baseUrl = env.apiBaseUrl

  const handleSearch = async () => {
    if (!sn.trim()) return
    setSearching(true)
    try {
      // Check repair requests first
      const resp = await apiFetch(`${baseUrl}/repair-requests?status=shipping`)
      const data = await resp.json()
      if (data.code === 20000) {
        const matches = (data.data?.list || []).filter(r => r.sn === sn.trim())
        if (matches.length > 0) {
          setResult({ type: 'repair_request', items: matches })
          setSearching(false)
          return
        }
      }

      // If not found in repair requests, search instruments
      const instrResp = await apiFetch(`${baseUrl}/instruments?sn=${sn.trim()}`)
      const instrData = await instrResp.json()
      if (instrData.code === 20000 && (instrData.data?.list || []).length > 0) {
        setResult({ type: 'instrument', items: instrData.data.list })
      } else {
        // Not found anywhere - handle offline
        setResult({ type: 'not_found' })
      }
    } catch {}
    setSearching(false)
  }

  return (
    <View className="h-screen bg-zinc-50 p-4">
      <View className="flex items-center mb-4">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1">收货识别</Text>
      </View>

      <View className="bg-white rounded-2xl shadow-sm p-4 mb-4">
        <Text className="text-sm font-bold text-black mb-2">输入/扫描乐器识别码</Text>
        <View className="flex gap-2">
          <input className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
            value={sn} onChange={e => setSn(e.target.value)} placeholder="扫描或输入识别码" />
          <Button onClick={handleSearch} disabled={searching}
            className="px-4 py-2 bg-black text-white rounded-lg text-sm font-bold">
            查询
          </Button>
        </View>
      </View>

      {result && (
        <View className="bg-white rounded-2xl shadow-sm p-4">
          {result.type === 'repair_request' && (
            <>
              <Text className="text-sm font-bold text-green-700 mb-2">匹配到报修单</Text>
              {result.items.map(r => (
                <View key={r.id} className="border-b border-zinc-50 py-2">
                  <Text className="text-sm">报修单 #{r.id?.slice(0, 8)}</Text>
                  <Text className="text-xs text-zinc-400">{r.description}</Text>
                  <View className="flex gap-2 mt-2">
                    <Button onClick={() => navigate(`/repair-request?request_id=${r.id}`)}
                      className="flex-1 py-1.5 bg-green-600 text-white rounded-lg text-xs font-bold">
                      进入质检
                    </Button>
                    <Button onClick={() => alert('已线下处理')} 
                      className="flex-1 py-1.5 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">
                      不匹配
                    </Button>
                  </View>
                </View>
              ))}
            </>
          )}
          {result.type === 'instrument' && (
            <>
              <Text className="text-sm font-bold text-blue-700 mb-2">匹配到租赁乐器</Text>
              <Text className="text-xs text-zinc-500">此识别码属于租赁乐器，将按租归还-接收定损流程处理</Text>
              <Button onClick={() => navigate(-1)}
                className="w-full mt-3 py-2 bg-blue-600 text-white rounded-lg text-sm font-bold">
                进入接收流程
              </Button>
            </>
          )}
          {result.type === 'not_found' && (
            <>
              <Text className="text-sm font-bold text-red-700 mb-2">未找到匹配记录</Text>
              <Text className="text-xs text-zinc-500">此识别码在报修单和乐器表中均未找到。请线下通知用户补全手续和付款。</Text>
            </>
          )}
        </View>
      )}
    </View>
  )
}
