import { useState, useEffect } from 'react'
import { View, Text, Input, Button, ScrollView } from '@tarojs/components'
import { api, addressesApi } from '../services/api'
import { storage, navigation } from '../platform'
import regions from '../data/regions.json'

export default function Onboarding() {
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [hasExistingAddress, setHasExistingAddress] = useState(false)

  const [name, setName] = useState('')
  const [recipientName, setRecipientName] = useState('')
  const [phone, setPhone] = useState('')
  const [province, setProvince] = useState('')
  const [city, setCity] = useState('')
  const [district, setDistrict] = useState('')
  const [detail, setDetail] = useState('')
  const [idPhotoFront, setIdPhotoFront] = useState(null)
  const [idPhotoBack, setIdPhotoBack] = useState(null)
  const [idPhotoFrontUrl, setIdPhotoFrontUrl] = useState('')
  const [idPhotoBackUrl, setIdPhotoBackUrl] = useState('')
  const [pointAmount, setPointAmount] = useState('')

  useEffect(() => {
    checkStatus()
    loadProfile()
    loadExistingAddress()
  }, [])

  const checkStatus = async () => {
    try {
      const resp = await api.get('/user/onboarding')
      if (resp?.data?.onboarding_completed) {
        navigation.redirect('/')
        return
      }
      if (resp?.data?.name) setName(resp.data.name || '')
    } catch { /* offline */ }
    setLoading(false)
  }

  const loadProfile = async () => {
    try {
      const resp = await api.get('/users/me')
      if (resp?.code === 20000 && resp.data) {
        if (resp.data.name && !recipientName) setRecipientName(resp.data.name)
        if (resp.data.phone && !phone) setPhone(resp.data.phone)
      }
    } catch { /* no profile */ }
  }

  const loadExistingAddress = async () => {
    try {
      const resp = await api.get('/user/addresses')
      if (resp?.code === 20000 && resp.data?.list?.length > 0) {
        const a = resp.data.list[0]
        setRecipientName(a.recipient_name || '')
        setPhone(a.phone || '')

      }
    } catch { /* no address */ }
  }

  const handleIDPhotoSelect = (side, e) => {
    const file = e.target.files?.[0]
    if (!file) return
    const url = URL.createObjectURL(file)
    if (side === 'front') {
      setIdPhotoFront(file)
      setIdPhotoFrontUrl(url)
    } else {
      setIdPhotoBack(file)
      setIdPhotoBackUrl(url)
    }
    e.target.value = ''
  }

  const clearIDPhoto = (side) => {
    if (side === 'front') {
      if (idPhotoFrontUrl) URL.revokeObjectURL(idPhotoFrontUrl)
      setIdPhotoFront(null)
      setIdPhotoFrontUrl('')
    } else {
      if (idPhotoBackUrl) URL.revokeObjectURL(idPhotoBackUrl)
      setIdPhotoBack(null)
      setIdPhotoBackUrl('')
    }
  }

  const uploadIDPhoto = async (file) => {
    const formData = new FormData()
    formData.append('file', file)
    const resp = await fetch('/api/user/id-photo', {
      method: 'POST',
      headers: { Authorization: 'Bearer ' + storage.getItem('token') },
      body: formData,
    })
    const json = await resp.json()
    return json.code === 20000
  }

  const handleSaveAddress = async () => {
    if (!recipientName || !phone || !detail) return
    try {
      if (hasExistingAddress) {
        await api.put('/user/addresses/' + (await api.get('/user/addresses')).data?.list?.[0]?.id, {
          recipient_name: recipientName, phone, province, city, district, detail,
        })
      } else {
        await addressesApi.create({ recipient_name: recipientName, phone, province, city, district, detail })
      }
    } catch { /* silent */ }
  }

  const handlePurchasePoints = async () => {
    const amount = parseFloat(pointAmount)
    if (!amount || amount <= 0) return
    try { await api.post('/user/points/purchase', { amount }) } catch { /* silent */ }
  }

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      if (recipientName && phone && detail) await handleSaveAddress()
      if (parseFloat(pointAmount) > 0) await handlePurchasePoints()
      if (idPhotoFront) await uploadIDPhoto(idPhotoFront)
      if (idPhotoBack) await uploadIDPhoto(idPhotoBack)
      await api.put('/user/onboarding', { name: name || undefined })
      navigation.redirect('/')
    } catch { setSubmitting(false) }
  }

  if (loading) {
    return (
      <View className="flex items-center justify-center h-screen bg-gray-50">
        <Text className="text-gray-500">加载中...</Text>
      </View>
    )
  }

  return (
    <ScrollView scrollY className="h-screen bg-gradient-to-b from-blue-50 to-white">
      <View className="px-5 pt-12 pb-8">
        <View className="mb-1"><Text className="text-2xl font-bold text-center block">欢迎来到 Tuneloop</Text></View>
        <View className="mb-8"><Text className="text-gray-500 text-center text-sm block">完善您的信息，开启租赁之旅</Text></View>

        {/* Step 1: Nickname */}
        <View className="mb-6">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">昵称（可选）</Text></View>
          <Input
            className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="输入您的昵称"
            value={name}
            onInput={e => setName(e.detail.value)}
          />
        </View>

        {/* Step 2: Address */}
        <View className="mb-6">
          <View className="mb-1">
            <Text className="text-sm font-medium text-gray-700">
              收货地址{hasExistingAddress ? '（已有地址，可修改）' : '（可选）'}
            </Text>
          </View>
          <View className="space-y-2">
            <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
              placeholder="收件人姓名" value={recipientName} onInput={e => setRecipientName(e.detail.value)} />
            <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
              placeholder="手机号" value={phone} onInput={e => setPhone(e.detail.value)} />

            <View className="flex flex-row gap-2">
              <select className="border border-gray-300 rounded-lg px-2 py-3 text-sm bg-white w-1/2"
                value={province} onChange={e => { setProvince(e.target.value); setCity(''); setDistrict('') }}>
                <option value="">省</option>
                {regions.map((r, i) => <option key={i} value={r.name}>{r.name}</option>)}
              </select>
              <select className="border border-gray-300 rounded-lg px-2 py-3 text-sm bg-white w-1/4"
                value={city} onChange={e => { setCity(e.target.value); setDistrict('') }}>
                <option value="">市</option>
                {(() => {
                  const prov = regions.find(r => r.name === province)
                  return prov ? prov.children.map((c, i) => <option key={i} value={c.name}>{c.name}</option>) : null
                })()}
              </select>
              <select className="border border-gray-300 rounded-lg px-2 py-3 text-sm bg-white w-1/4"
                value={district} onChange={e => setDistrict(e.target.value)}>
                <option value="">区</option>
                {(() => {
                  const prov = regions.find(r => r.name === province)
                  if (!prov) return null
                  const cit = prov.children.find(c => c.name === city)
                  return cit ? cit.children.map((d, i) => <option key={i} value={d.name}>{d.name}</option>) : null
                })()}
              </select>
            </View>

            <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
              placeholder="详细地址" value={detail} onInput={e => setDetail(e.detail.value)} />
          </View>
        </View>

        {/* Step 3: ID Photo */}
        <View className="mb-6">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">身份证照片（可选）</Text></View>
          <View className="flex flex-row gap-4">
            {/* Front side */}
            <View className="flex-1 border-2 border-dashed border-gray-300 rounded-lg p-3 text-center relative">
              {idPhotoFrontUrl ? (
                <View className="relative">
                  <img src={idPhotoFrontUrl} alt="身份证正面" className="w-full h-32 object-cover rounded" />
                  <View className="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center text-xs"
                    onClick={() => clearIDPhoto('front')}>✕</View>
                </View>
              ) : (
                <View className="h-32 flex flex-col items-center justify-center" onClick={() => document.getElementById('id-front-input').click()}>
                  <Text className="text-gray-400 text-xs">正面</Text>
                  <Text className="text-gray-300 text-xs mt-1">点击上传</Text>
                </View>
              )}
              <input type="file" id="id-front-input" accept="image/*" className="hidden" onChange={(e) => handleIDPhotoSelect('front', e)} />
            </View>
            {/* Back side */}
            <View className="flex-1 border-2 border-dashed border-gray-300 rounded-lg p-3 text-center relative">
              {idPhotoBackUrl ? (
                <View className="relative">
                  <img src={idPhotoBackUrl} alt="身份证背面" className="w-full h-32 object-cover rounded" />
                  <View className="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center text-xs"
                    onClick={() => clearIDPhoto('back')}>✕</View>
                </View>
              ) : (
                <View className="h-32 flex flex-col items-center justify-center" onClick={() => document.getElementById('id-back-input').click()}>
                  <Text className="text-gray-400 text-xs">背面</Text>
                  <Text className="text-gray-300 text-xs mt-1">点击上传</Text>
                </View>
              )}
              <input type="file" id="id-back-input" accept="image/*" className="hidden" onChange={(e) => handleIDPhotoSelect('back', e)} />
            </View>
          </View>
        </View>

        {/* Step 4: Points */}
        <View className="mb-8">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">预购点数（可选）</Text></View>
          <View className="flex flex-row gap-2">
            {[100, 300, 500].map(amt => (
              <Button key={amt}
                className={`flex-1 py-3 rounded-lg text-sm font-medium border ${pointAmount === String(amt) ? 'bg-blue-500 text-white border-blue-500' : 'bg-white text-gray-700 border-gray-300'}`}
                onClick={() => setPointAmount(String(amt))}
              >¥{amt}</Button>
            ))}
          </View>
          <Input className="w-full mt-2 border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="自定义金额" type="number" value={pointAmount} onInput={e => setPointAmount(e.detail.value)} />
        </View>

        <Button className="w-full bg-blue-500 text-white py-4 rounded-xl text-lg font-medium"
          disabled={submitting} onClick={handleSubmit}>
          {submitting ? '提交中...' : '开始使用'}
        </Button>

        <Button className="w-full text-gray-400 text-sm py-3 mt-2"
          onClick={() => navigation.redirect('/')}>跳过，稍后再说</Button>
      </View>
    </ScrollView>
  )
}
