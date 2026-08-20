import { useState, useEffect, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Picker, Image } from '@tarojs/components'
import { storage, session, env, request, wxLogin } from '../../platform'
import { apiFetch , resolveErrorMessage } from '../../services/api'
import IdPhotoUploader from '../../components/IdPhotoUploader'
import regions from '../../data/regions.json'

export default function ProfileComplete() {
  const [name, setName] = useState('')
  const [nickname, setNickname] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [avatar, setAvatar] = useState('')
  const idPhotoFrontRef = useRef(null)
  const idPhotoBackRef = useRef(null)

  const [province, setProvince] = useState('')
  const [city, setCity] = useState('')
  const [district, setDistrict] = useState('')
  const [detail, setDetail] = useState('')
  const [postalCode, setPostalCode] = useState('')

  const [saving, setSaving] = useState(false)
  // mode=member: 从购物车提交/立即租赁的员工弹窗进入 — 隐藏「用户名密码登录」
  const [mode, setMode] = useState('')
  // Two-phase registration (#1663): resume an existing pending session.
  const [resumeSid, setResumeSid] = useState('')
  const [sessionAmount, setSessionAmount] = useState(0)

  const provinceNames = regions.map(r => r.name)
  const selectedProv = regions.find(r => r.name === province)
  const cityNames = selectedProv ? selectedProv.children.map(c => c.name) : []
  const selectedCity = selectedProv ? selectedProv.children.find(c => c.name === city) : null
  const districtNames = selectedCity ? selectedCity.children.map(d => d.name) : []

  useEffect(() => {
    const params = Taro.getCurrentInstance().router?.params || {}
    if (params.phone) setPhone(params.phone)
    if (params.ref) storage.setItem('ref_code', params.ref)
    if (params.scene) {
      const decoded = decodeURIComponent(params.scene)
      if (decoded.startsWith('ref=')) storage.setItem('ref_code', decoded.slice(4))
    }
    if (params.mode) setMode(params.mode)
    // Two-phase registration (#1663): resume an existing pending session —
    // prefill the form and skip re-creating the session on submit.
    if (params.session_id) {
      setResumeSid(params.session_id)
      apiFetch(`${env.apiBaseUrl}/auth/registration-sessions/me?session_id=${params.session_id}`)
        .then(r => r.json())
        .then(res => {
          // 404 → legacy/expired session (pre-#1682, no reserved user):
          // clear the resume state so submitting creates a fresh session.
          if (res.code !== 20000) {
            setResumeSid('')
            return
          }
          if (res.data?.form_data) {
            const f = res.data.form_data
            if (f.name) setName(f.name)
            if (f.nickname) setNickname(f.nickname)
            if (f.phone) setPhone(f.phone)
            if (f.email) setEmail(f.email)
            // Resume the shipping address too — the form was fully filled
            // before; only the basic fields were restored previously.
            if (f.address) {
              if (f.address.province) setProvince(f.address.province)
              if (f.address.city) setCity(f.address.city)
              if (f.address.district) setDistrict(f.address.district)
              if (f.address.detail) setDetail(f.address.detail)
              if (f.address.postal_code) setPostalCode(f.address.postal_code)
            }
          }
          if (res.code === 20000 && res.data?.amount) setSessionAmount(res.data.amount)
        })
        .catch(() => {})
    }
  }, [])

  const handleChooseAvatar = () => {
    Taro.chooseImage({ count: 1, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      .then(res => setAvatar(res.tempFilePaths[0]))
      .catch(() => {})
  }

  const handleRegister = async () => {
    if (!name.trim()) { Taro.showToast({ title: '请输入姓名', icon: 'none' }); return }
    if (!phone.trim()) { Taro.showToast({ title: '请输入手机号', icon: 'none' }); return }
    setSaving(true)
    try {
      // Two-phase registration (#1663): submitting creates a pending session
      // (no account yet) and redirects to the payment page. The account is
      // created server-side after the membership fee callback.
      let sid = resumeSid
      let amount = sessionAmount
      if (!sid) {
        const body = { name: name.trim(), nickname: nickname.trim() || name.trim(), phone: phone.trim(), email: email.trim() }
        if (province || city || detail) {
          body.address = { province, city, district, detail, postal_code: postalCode }
        }
        // Registration binds via the exchange_token minted by wx-accounts
        // (#1644) — the raw code is single-use and already consumed. Fall back
        // to a fresh wx.login code when no token is available (expired).
        const exchangeToken = session.getItem('wx_login_token') || ''
        if (exchangeToken) {
          body.exchange_token = exchangeToken
        }
        // Always fetch a fresh wx.login code too (#1681): the exchange_token
        // cannot resolve the openid (only a code can) — the backend stores the
        // resolved openid on the session for the JSAPI prepay backfill. The
        // code and the exchange_token are independent and coexist.
        const wxCode = await wxLogin()
        if (wxCode) { body.wx_code = wxCode }
        const refCode = storage.getItem('ref_code')
        if (refCode) { body.ref = refCode }
        const res = await request(`${env.apiBaseUrl}/auth/registration-sessions`, {
          method: 'POST',
          body: JSON.stringify(body),
        })
        const result = await res.json()
        // exchange_token is single-use (5min TTL); clear it so a retry never
        // reuses an expired token and falls back to a fresh wx.login code
        // (#1648).
        session.removeItem('wx_login_token')
        if (result.code === 20000 && result.data?.session_id) {
          sid = result.data.session_id
          amount = result.data.amount
        } else {
          Taro.showToast({ title: resolveErrorMessage(result, '提交失败, 请重试'), icon: 'none', duration: 3000 })
          setSaving(false)
          return
        }
      }
      session.setItem('pending_registration_session', sid)
      Taro.redirectTo({ url: `/pages-weapp/payment/index?type=membership&session_id=${sid}&amount=${amount}` })
    } catch (err) {
      Taro.showToast({ title: '网络错误, 请重试', icon: 'none' })
    }
    setSaving(false)
  }

  const goAccountSelect = () => {
    Taro.redirectTo({ url: '/pages-weapp/account-select/index' })
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

      <Input placeholder="昵称" value={nickname} onInput={e => setNickname(e.detail.value)}
        type="nickname"
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', boxSizing: 'border-box', fontSize: 14, lineHeight: '44px', marginBottom: 12 }} />
      <Input placeholder="姓名" value={name} onInput={e => setName(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', boxSizing: 'border-box', fontSize: 14, lineHeight: '44px', marginBottom: 12 }} />
      <Input placeholder="手机号" value={phone} onInput={e => setPhone(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', boxSizing: 'border-box', fontSize: 14, lineHeight: '44px', marginBottom: 12 }} />
      <Input placeholder="邮箱（选填）" value={email} onInput={e => setEmail(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', boxSizing: 'border-box', fontSize: 14, lineHeight: '44px', marginBottom: 24 }} />

      <Text style={{ fontSize: 16, fontWeight: '700', color: '#000', width: '100%', marginBottom: 12 }}>收货地址（选填）</Text>
      <View style={{ display: 'flex', width: '100%', marginBottom: 12 }}>
        <View style={{ flex: 1, marginRight: 8 }}>
          <Picker mode="selector" range={provinceNames} value={province ? provinceNames.indexOf(province) : 0}
            onChange={e => { setProvince(provinceNames[e.detail.value]); setCity(''); setDistrict('') }}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: 44, display: 'flex', alignItems: 'center', padding: '0 16px', boxSizing: 'border-box', fontSize: 14, color: province ? '#000' : '#9ca3af' }}>
              {province || '省'}
            </View>
          </Picker>
        </View>
        <View style={{ flex: 1, marginRight: 8 }}>
          <Picker mode="selector" range={cityNames} value={city ? cityNames.indexOf(city) : 0}
            onChange={e => { setCity(cityNames[e.detail.value]); setDistrict('') }}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: 44, display: 'flex', alignItems: 'center', padding: '0 16px', boxSizing: 'border-box', fontSize: 14, color: city ? '#000' : '#9ca3af' }}>
              {city || '市'}
            </View>
          </Picker>
        </View>
        {districtNames.length > 0 && (
        <View style={{ flex: 1 }}>
          <Picker mode="selector" range={districtNames} value={district ? districtNames.indexOf(district) : 0}
            onChange={e => setDistrict(districtNames[e.detail.value])}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: 44, display: 'flex', alignItems: 'center', padding: '0 16px', boxSizing: 'border-box', fontSize: 14, color: district ? '#000' : '#9ca3af' }}>
              {district || '区'}
            </View>
          </Picker>
        </View>
        )}
      </View>
      <Input placeholder="详细地址" value={detail} onInput={e => setDetail(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', boxSizing: 'border-box', fontSize: 14, lineHeight: '44px', marginBottom: 12 }} />
      <Input placeholder="邮编（选填）" value={postalCode} onInput={e => setPostalCode(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', boxSizing: 'border-box', fontSize: 14, lineHeight: '44px', marginBottom: 24 }} />

      <Text style={{ fontSize: 16, fontWeight: '700', color: '#000', width: '100%', marginBottom: 12 }}>身份证照片（选填）</Text>
      <View style={{ display: 'flex', width: '100%', marginBottom: 24 }}>
        <View style={{ flex: 1, marginRight: 8, display: 'flex', justifyContent: 'center' }}>
          <IdPhotoUploader ref={idPhotoFrontRef} side="front" defer />
        </View>
        <View style={{ flex: 1, marginLeft: 8, display: 'flex', justifyContent: 'center' }}>
          <IdPhotoUploader ref={idPhotoBackRef} side="back" defer />
        </View>
      </View>

      <View onClick={handleRegister}
        style={{ width: '100%', height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12 }}>
        <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>{saving ? '处理中...' : '支付会员费'}</Text>
      </View>
      {(resumeSid || sessionAmount > 0) && (
        <Text style={{ fontSize: 12, color: '#a1a1aa', textAlign: 'center', display: 'block', marginBottom: 8 }}>
          会员费 ¥{(Number(sessionAmount) / 100).toFixed(2)}{resumeSid ? '（已创建支付会话）' : ''}
        </Text>
      )}
      {mode !== 'member' && (
        <Text style={{ fontSize: 14, color: '#a1a1aa', textAlign: 'center', display: 'block', marginBottom: 8 }} onClick={goAccountSelect}>用户名密码登录</Text>
      )}
      <Text style={{ fontSize: 14, color: '#a1a1aa', textAlign: 'center', display: 'block' }} onClick={() => Taro.navigateBack()}>返回</Text>
    </View>
  )
}
