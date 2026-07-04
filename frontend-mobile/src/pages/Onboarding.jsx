import { useState, useEffect } from 'react'
import { View, Text, Input, Button, Picker, ScrollView } from '@tarojs/components'
import { api, addressesApi } from '../services/api'
import { storage, navigation } from '../platform'

const REGIONS = {
  '北京市': { '北京市': ['海淀区', '朝阳区', '东城区', '西城区', '丰台区', '石景山区'] },
  '上海市': { '上海市': ['浦东新区', '黄浦区', '徐汇区', '静安区', '长宁区'] },
  '广东省': { '广州市': ['天河区', '越秀区', '海珠区', '白云区'], '深圳市': ['南山区', '福田区', '罗湖区', '宝安区'] },
}

const provinceList = Object.keys(REGIONS)
const getCityList = (p) => REGIONS[p] ? Object.keys(REGIONS[p]) : []
const getDistrictList = (p, c) => REGIONS[p]?.[c] || []

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
  const [idPhotoUrl, setIdPhotoUrl] = useState('')
  const [uploading, setUploading] = useState(false)
  const [pointAmount, setPointAmount] = useState('')

  const [provinceIdx, setProvinceIdx] = useState(0)
  const [cityIdx, setCityIdx] = useState(0)
  const [districtIdx, setDistrictIdx] = useState(0)

  const currentCities = getCityList(provinceList[provinceIdx])
  const currentDistricts = getDistrictList(provinceList[provinceIdx], currentCities[cityIdx])

  const onProvinceChange = (e) => {
    const idx = parseInt(e.detail.value, 10)
    setProvinceIdx(idx)
    setCityIdx(0)
    setDistrictIdx(0)
    setProvince(provinceList[idx])
    setCity(currentCities[0] || '')
    setDistrict((getDistrictList(provinceList[idx], currentCities[0]) || [])[0] || '')
  }

  const onCityChange = (e) => {
    const idx = parseInt(e.detail.value, 10)
    setCityIdx(idx)
    setDistrictIdx(0)
    setCity(currentCities[idx])
    setDistrict(currentDistricts[0] || '')
  }

  const onDistrictChange = (e) => {
    const idx = parseInt(e.detail.value, 10)
    setDistrictIdx(idx)
    setDistrict(currentDistricts[idx])
  }

  useEffect(() => {
    checkStatus()
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

  const loadExistingAddress = async () => {
    try {
      const resp = await api.get('/user/addresses')
      if (resp?.code === 20000 && resp.data?.list?.length > 0) {
        const a = resp.data.list[0]
        setRecipientName(a.recipient_name || '')
        setPhone(a.phone || '')
        setProvince(a.province || '')
        setCity(a.city || '')
        setDistrict(a.district || '')
        setDetail(a.detail || '')
        setHasExistingAddress(true)
        if (a.province) {
          const pi = provinceList.indexOf(a.province)
          if (pi >= 0) setProvinceIdx(pi)
        }
        if (a.city) {
          const ci = getCityList(provinceList[provinceIdx]).indexOf(a.city)
          if (ci >= 0) setCityIdx(ci)
        }
      }
    } catch { /* no address */ }
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
      if (json.code === 20000) setIdPhotoUrl(json.data?.url || 'uploaded')
    } catch { /* silent */ }
    setUploading(false)
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
        <View className="mb-1"><Text className="text-2xl font-bold text-center block">欢迎来到音租</Text></View>
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
              <Picker mode="selector" range={provinceList} value={provinceIdx} onChange={onProvinceChange}>
                <View className="flex-1 border border-gray-300 rounded-lg px-4 py-3 text-sm text-gray-700">
                  {provinceList[provinceIdx] || '省'}
                </View>
              </Picker>
              <Picker mode="selector" range={currentCities} value={cityIdx} onChange={onCityChange}>
                <View className="flex-1 border border-gray-300 rounded-lg px-4 py-3 text-sm text-gray-700">
                  {currentCities[cityIdx] || '市'}
                </View>
              </Picker>
              <Picker mode="selector" range={currentDistricts} value={districtIdx} onChange={onDistrictChange}>
                <View className="flex-1 border border-gray-300 rounded-lg px-4 py-3 text-sm text-gray-700">
                  {currentDistricts[districtIdx] || '区'}
                </View>
              </Picker>
            </View>

            <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
              placeholder="详细地址" value={detail} onInput={e => setDetail(e.detail.value)} />
          </View>
        </View>

        {/* Step 3: ID Photo */}
        <View className="mb-6">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">身份证照片（可选）</Text></View>
          <View className="border-2 border-dashed border-gray-300 rounded-lg p-6 text-center">
            {idPhotoUrl ? (
              <View><Text className="text-green-600 text-sm">✓ 已上传</Text></View>
            ) : (
              <View onClick={() => { const el = document.querySelector('#id-photo-input'); if (el) el.click() }}>
                <View><Text className="text-gray-400 text-sm">{uploading ? '上传中...' : '点击上传'}</Text></View>
                <View><Text className="text-gray-400 text-xs">支持 JPG/PNG，最大 5MB</Text></View>
                <input type="file" id="id-photo-input" accept="image/*" className="hidden" onChange={handleIDPhotoUpload} disabled={uploading} />
              </View>
            )}
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
