import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Picker, Image } from '@tarojs/components'
import { storage, env, request, eventBus, wxLogin } from '../../platform'
import regions from '../../data/regions.json'

export default function ProfileComplete() {
  const [username, setUsername] = useState('')
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [avatar, setAvatar] = useState('')

  const [province, setProvince] = useState('')
  const [city, setCity] = useState('')
  const [district, setDistrict] = useState('')
  const [detail, setDetail] = useState('')
  const [postalCode, setPostalCode] = useState('')

  const [saving, setSaving] = useState(false)

  const provinceNames = regions.map(r => r.name)
  const selectedProv = regions.find(r => r.name === province)
  const cityNames = selectedProv ? selectedProv.children.map(c => c.name) : []
  const selectedCity = selectedProv ? selectedProv.children.find(c => c.name === city) : null
  const districtNames = selectedCity ? selectedCity.children.map(d => d.name) : []

  useEffect(() => {
    const params = Taro.getCurrentInstance().router?.params || {}
    if (params.phone) setPhone(params.phone)
    if (params.ref) storage.setItem('ref_code', params.ref)
  }, [])

  const handleChooseAvatar = () => {
    Taro.chooseImage({ count: 1, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      .then(res => setAvatar(res.tempFilePaths[0]))
      .catch(() => {})
  }

  const handleRegister = async () => {
    if (!username.trim()) { Taro.showToast({ title: '请输入用户名', icon: 'none' }); return }
    if (!name.trim()) { Taro.showToast({ title: '请输入姓名', icon: 'none' }); return }
    if (!phone.trim()) { Taro.showToast({ title: '请输入手机号', icon: 'none' }); return }
    if (!password.trim() || password.length < 6) { Taro.showToast({ title: '密码至少6位', icon: 'none' }); return }
    if (password !== confirmPassword) { Taro.showToast({ title: '两次密码不一致', icon: 'none' }); return }
    setSaving(true)
    try {
      const body = { username: username.trim(), name: name.trim(), phone: phone.trim(), email: email.trim(), password: password.trim() }
      const wxCode = await wxLogin()
      if (wxCode) { body.wx_code = wxCode }
      const refCode = storage.getItem('ref_code')
      if (refCode) { body.ref = refCode }
      const res = await request(`${env.apiBaseUrl}/auth/register`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      const result = await res.json()
      if (result.code === 20000 && result.data?.access_token) {
        storage.setItem('token', result.data.access_token)
        storage.setItem('token_expiry', (Date.now() + (result.data.expires_in || 3600) * 1000).toString())
        if (avatar) {
          try {
            await Taro.uploadFile({
              url: `${env.apiBaseUrl}/users/me/avatar`,
              filePath: avatar,
              name: 'file',
              header: { 'Authorization': 'Bearer ' + result.data.access_token },
            })
          } catch {}
        }
        if (province || city || detail) {
          if (!province || !city || !detail) {
            Taro.showToast({ title: '收货地址信息不完整，未保存', icon: 'none', duration: 2500 })
          } else {
            try {
              await request(`${env.apiBaseUrl}/user/addresses`, {
                method: 'POST',
                headers: { 'Authorization': 'Bearer ' + result.data.access_token, 'Content-Type': 'application/json' },
                body: JSON.stringify({
                  recipient_name: name.trim(), phone: phone.trim(),
                  province, city, district, detail,
                  postal_code: postalCode, is_default: true,
                }),
              })
              Taro.showToast({ title: '收货地址已保存', icon: 'success', duration: 1500 })
            } catch (e) { console.error('[Register] address save failed', e) }
          }
        }
        eventBus.emit('loginSuccess')
        const pages = Taro.getCurrentPages()
        if (pages.length > 1) {
          Taro.navigateBack()
        } else {
          Taro.redirectTo({ url: '/pages-weapp/profile/index' })
        }
      } else {
        Taro.showToast({ title: result.message || '注册失败, 请重试', icon: 'none', duration: 3000 })
      }
    } catch (err) {
      Taro.showToast({ title: '网络错误, 请重试', icon: 'none' })
    }
    setSaving(false)
  }

  return (
    <View style={{ height: '100vh', backgroundColor: '#fafafa', display: 'flex', flexDirection: 'column', alignItems: 'center', padding: 32 }}>
      <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', marginBottom: 8, marginTop: 32 }}>注册账号</Text>
      <Text style={{ fontSize: 14, color: '#a1a1aa', marginBottom: 24 }}>填写信息即可开始租赁</Text>

      <View onClick={handleChooseAvatar}
        style={{ width: 72, height: 72, borderRadius: 999, backgroundColor: '#e4e4e7', display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 24, overflow: 'hidden' }}>
        {avatar ? (
          <Image src={avatar} style={{ width: '100%', height: '100%' }} mode="aspectFill" />
        ) : (
          <Text style={{ fontSize: 28 }}>📷</Text>
        )}
      </View>

      <Input placeholder="用户名" value={username} onInput={e => setUsername(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="姓名" value={name} onInput={e => setName(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="手机号" value={phone} onInput={e => setPhone(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="邮箱（选填）" value={email} onInput={e => setEmail(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="密码（至少6位）" password value={password} onInput={e => setPassword(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="确认密码" password value={confirmPassword} onInput={e => setConfirmPassword(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 24 }} />

      <Text style={{ fontSize: 16, fontWeight: '700', color: '#000', width: '100%', marginBottom: 12 }}>收货地址（选填）</Text>
      <View style={{ display: 'flex', width: '100%', marginBottom: 12 }}>
        <View style={{ flex: 1, marginRight: 8 }}>
          <Picker mode="selector" range={provinceNames} value={province ? provinceNames.indexOf(province) : 0}
            onChange={e => { setProvince(provinceNames[e.detail.value]); setCity(''); setDistrict('') }}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, padding: '11px 16px', fontSize: 14, color: province ? '#000' : '#9ca3af' }}>
              {province || '省'}
            </View>
          </Picker>
        </View>
        <View style={{ flex: 1, marginRight: 8 }}>
          <Picker mode="selector" range={cityNames} value={city ? cityNames.indexOf(city) : 0}
            onChange={e => { setCity(cityNames[e.detail.value]); setDistrict('') }}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, padding: '11px 16px', fontSize: 14, color: city ? '#000' : '#9ca3af' }}>
              {city || '市'}
            </View>
          </Picker>
        </View>
        {districtNames.length > 0 && (
        <View style={{ flex: 1 }}>
          <Picker mode="selector" range={districtNames} value={district ? districtNames.indexOf(district) : 0}
            onChange={e => setDistrict(districtNames[e.detail.value])}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, padding: '11px 16px', fontSize: 14, color: district ? '#000' : '#9ca3af' }}>
              {district || '区'}
            </View>
          </Picker>
        </View>
        )}
      </View>
      <Input placeholder="详细地址" value={detail} onInput={e => setDetail(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="邮编（选填）" value={postalCode} onInput={e => setPostalCode(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 24 }} />

      <View onClick={handleRegister}
        style={{ width: '100%', height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12 }}>
        <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>{saving ? '注册中...' : '注册'}</Text>
      </View>
      <Text style={{ fontSize: 14, color: '#a1a1aa' }} onClick={() => Taro.navigateBack()}>返回</Text>
    </View>
  )
}
