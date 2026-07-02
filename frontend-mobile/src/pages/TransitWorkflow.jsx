import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView, Button, Image } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { Camera } from 'lucide-react'

export default function TransitWorkflow() {
  const navigate = useNavigate()
  const [orders, setOrders] = useState([])
  const [selected, setSelected] = useState(null)
  const [photos, setPhotos] = useState([])
  const [company, setCompany] = useState('')
  const [number, setNumber] = useState('')
  const [actionLoading, setActionLoading] = useState(false)
  const [step, setStep] = useState('list') // list / receive / repack
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    apiFetch(`${baseUrl}/transit-orders?status=dispatching`).then(r => r.json()).then(r => {
      if (r.code === 20000) setOrders(r.data?.list || [])
    }).catch(() => {})
  }, [])

  const handleReceive = async () => {
    if (!selected || photos.length === 0) { alert('请先拍照'); return }
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/transit-orders/${selected}/receive`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ photos }),
      })
      const r = await resp.json()
      if (r.code === 20000) { setStep('repack') } else { alert(r.message) }
    } catch {}
    setActionLoading(false)
  }

  const handleRepack = async () => {
    if (!company || !number) { alert('请填写物流信息'); return }
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/transit-orders/${selected}/repack`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ company, number, photos }),
      })
      const r = await resp.json()
      if (r.code === 20000) { alert('转包完成'); navigate(-1) } else { alert(r.message) }
    } catch {}
    setActionLoading(false)
  }

  if (step === 'receive' || step === 'repack') {
    return (
      <View className="h-screen bg-zinc-50">
        <View className="bg-white px-4 py-3 border-b border-zinc-100 flex items-center">
          <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
          <Text className="text-lg font-bold flex-1">{step === 'receive' ? '收货拆包' : '转包发货'}</Text>
        </View>
        <ScrollView scrollY className="flex-1 px-4 min-h-0">
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-3">
            <Text className="text-sm font-bold text-black mb-2">拆包拍照</Text>
            <View className="grid grid-cols-3 gap-2">
              {photos.map((f, i) => (
                <View key={i} className="aspect-square bg-zinc-200 rounded-lg overflow-hidden">
                  <Image src={typeof f === 'string' ? f : URL.createObjectURL(f)} className="w-full h-full object-cover" mode="aspectFill" />
                </View>
              ))}
              <label className="aspect-square border-2 border-dashed border-zinc-300 rounded-lg flex items-center justify-center active:opacity-60">
                <Camera size={24} className="text-zinc-400" />
                <input type="file" accept="image/*" capture="environment" multiple className="hidden"
                  onChange={e => setPhotos(p => [...p, ...Array.from(e.target.files || [])].slice(0, 10))} />
              </label>
            </View>
          </View>
          {step === 'repack' && (
            <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-3">
              <Text className="text-sm font-bold text-black">转包信息</Text>
              <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
                value={company} onChange={e => setCompany(e.target.value)} placeholder="物流公司" />
              <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
                value={number} onChange={e => setNumber(e.target.value)} placeholder="物流单号" />
            </View>
          )}
          <Button onClick={step === 'receive' ? handleReceive : handleRepack} disabled={actionLoading}
            className="w-full mt-4 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
            {actionLoading ? '处理中...' : step === 'receive' ? '确认收货' : '确认转包'}
          </Button>
        </ScrollView>
      </View>
    )
  }

  return (
    <View className="h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg font-bold">中转收货</Text>
      </View>
      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        <View className="mt-4 space-y-3">
          {orders.length === 0 ? (
            <Text className="text-center text-zinc-400 py-8">暂无待收货的中转订单</Text>
          ) : orders.map(o => (
            <View key={o.id} className="bg-white rounded-2xl shadow-sm p-4 active:opacity-80"
              onClick={() => { setSelected(o.id); setStep('receive'); setPhotos([]) }}>
              <Text className="text-sm font-bold text-black">中转单 #{o.id?.slice(0, 8)}</Text>
              <Text className="text-xs text-zinc-400 mt-1">{o.transit_order_number || ''}</Text>
            </View>
          ))}
        </View>
      </ScrollView>
    </View>
  )
}
