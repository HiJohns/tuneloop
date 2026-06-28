import { useState, useEffect } from 'react'
import { api, pointsApi, addressesApi } from '../services/api'
import { storage, navigation } from '../platform'

export default function Onboarding() {
  const [loading, setLoading] = useState(true)
  const [step, setStep] = useState(1)
  const [submitting, setSubmitting] = useState(false)

  const [name, setName] = useState('')
  const [recipientName, setRecipientName] = useState('')
  const [phone, setPhone] = useState('')
  const [province, setProvince] = useState('')
  const [city, setCity] = useState('')
  const [district, setDistrict] = useState('')
  const [detail, setDetail] = useState('')
  const [idPhotoUrl, setIdPhotoUrl] = useState('')
  const [uploading, setUploading] = useState(false)
  const [pointAmount, setPointAmount] = useState('')

  useEffect(() => {
    checkStatus()
  }, [])

  const checkStatus = async () => {
    try {
      const resp = await api.get('/user/onboarding')
      if (resp?.data?.onboarding_completed) {
        navigation.redirect('/')
        return
      }
      if (resp?.data?.name) {
        setName(resp.data.name || '')
      }
    } catch {
      // offline fallback
    } finally {
      setLoading(false)
    }
  }

  const handleIDPhotoUpload = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    try {
      const formData = new FormData()
      formData.append('file', file)
      const resp = await fetch('/api/user/id-photo', {
        method: 'POST',
        headers: { Authorization: 'Bearer ' + storage.getItem('auth_token') },
        body: formData,
      })
      const json = await resp.json()
      if (json.code === 20000) {
        setIdPhotoUrl(json.data?.url || 'uploaded')
      }
    } catch {
      // silent
    } finally {
      setUploading(false)
    }
  }

  const handleSaveAddress = async () => {
    if (!recipientName || !phone || !detail) return
    try {
      await addressesApi.create({ recipient_name: recipientName, phone, province, city, district, detail })
    } catch {
      // address save failed silently
    }
  }

  const handlePurchasePoints = async () => {
    const amount = parseFloat(pointAmount)
    if (!amount || amount <= 0) return
    try {
      await api.post('/user/points/purchase', { amount })
    } catch {
      // purchase failed silently
    }
  }

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      if (recipientName && phone && detail) {
        await handleSaveAddress()
      }
      if (parseFloat(pointAmount) > 0) {
        await handlePurchasePoints()
      }

      await api.put('/user/onboarding', { name: name || undefined })
      navigation.redirect('/')
    } catch {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen bg-gray-50">
        <div className="text-gray-500">加载中...</div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gradient-to-b from-blue-50 to-white">
      <div className="px-5 pt-12 pb-8">
        <h1 className="text-2xl font-bold text-center mb-1">欢迎来到音租</h1>
        <p className="text-gray-500 text-center text-sm mb-8">完善您的信息，开启租赁之旅</p>

        {/* Step 1: Nickname */}
        <div className="mb-6">
          <label className="block text-sm font-medium text-gray-700 mb-1">昵称（可选）</label>
          <input
            className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            placeholder="输入您的昵称"
            value={name}
            onChange={e => setName(e.target.value)}
          />
        </div>

        {/* Step 2: Address */}
        <div className="mb-6">
          <label className="block text-sm font-medium text-gray-700 mb-1">收货地址（可选）</label>
          <div className="space-y-2">
            <input
              className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
              placeholder="收件人姓名"
              value={recipientName}
              onChange={e => setRecipientName(e.target.value)}
            />
            <input
              className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
              placeholder="手机号"
              value={phone}
              onChange={e => setPhone(e.target.value)}
            />
            <div className="flex gap-2">
              <input
                className="flex-1 border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                placeholder="省"
                value={province}
                onChange={e => setProvince(e.target.value)}
              />
              <input
                className="flex-1 border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                placeholder="市"
                value={city}
                onChange={e => setCity(e.target.value)}
              />
              <input
                className="flex-1 border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                placeholder="区"
                value={district}
                onChange={e => setDistrict(e.target.value)}
              />
            </div>
            <input
              className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
              placeholder="详细地址"
              value={detail}
              onChange={e => setDetail(e.target.value)}
            />
          </div>
        </div>

        {/* Step 3: ID Photo */}
        <div className="mb-6">
          <label className="block text-sm font-medium text-gray-700 mb-1">身份证照片（可选）</label>
          <div className="border-2 border-dashed border-gray-300 rounded-lg p-6 text-center">
            {idPhotoUrl ? (
              <div className="text-green-600 text-sm">✓ 已上传</div>
            ) : (
              <label className="cursor-pointer block">
                <div className="text-gray-400 text-sm mb-1">{uploading ? '上传中...' : '点击上传'}</div>
                <div className="text-gray-400 text-xs">支持 JPG/PNG，最大 5MB</div>
                <input type="file" accept="image/*" className="hidden" onChange={handleIDPhotoUpload} disabled={uploading} />
              </label>
            )}
          </div>
        </div>

        {/* Step 4: Points Purchase */}
        <div className="mb-8">
          <label className="block text-sm font-medium text-gray-700 mb-1">预购点数（可选）</label>
          <div className="flex gap-2">
            {[100, 300, 500].map(amt => (
              <button
                key={amt}
                className={`flex-1 py-3 rounded-lg text-sm font-medium border transition ${
                  pointAmount === String(amt)
                    ? 'bg-blue-500 text-white border-blue-500'
                    : 'bg-white text-gray-700 border-gray-300'
                }`}
                onClick={() => setPointAmount(String(amt))}
              >
                ¥{amt}
              </button>
            ))}
          </div>
          <input
            className="w-full mt-2 border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            placeholder="自定义金额"
            type="number"
            min="0"
            value={pointAmount}
            onChange={e => setPointAmount(e.target.value)}
          />
        </div>

        <button
          className="w-full bg-blue-500 text-white py-4 rounded-xl text-lg font-medium active:bg-blue-600 disabled:opacity-50"
          disabled={submitting}
          onClick={handleSubmit}
        >
          {submitting ? '提交中...' : '开始使用'}
        </button>

        <button
          className="w-full text-gray-400 text-sm py-3 mt-2"
          onClick={() => navigation.redirect('/')}
        >
          跳过，稍后再说
        </button>
      </div>
    </div>
  )
}
