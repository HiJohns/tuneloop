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
  const idPhotoOtherRef = useRef(null)

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
  // #1807: 第三证件类型（学生证/教师证/工作证/其他）
  const [otherIdType, setOtherIdType] = useState('')
  // #1845: resume 会话时回填服务端已上传的证件侧（defer 本地无预览，
  // 以 id_photos map 键为凭）→ 满足身份证必填校验。
  const [uploadedSides, setUploadedSides] = useState({})
  // #1845: 其他证件（学生证）槽本轮是否已选图（onSelect 回调，供豁免判定与标签）。
  const [otherPicked, setOtherPicked] = useState(false)

  const ID_TYPE_OPTIONS = ['学生证', '教职工证', '教师证', '工作证', '其他']

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
            // #1845: resume 会话回填第三证件类型（学生证豁免判定依赖）。
            if (f.id_photo_other_type) setOtherIdType(f.id_photo_other_type)
            // #1845: 已上传证件侧回填（id_photos: {front|back|other: key}）。
            // 服务端照片随会话保留，本次不再重复上传，仅用于必填校验。
            if (f.id_photos && typeof f.id_photos === 'object') {
              const sides = {}
              Object.keys(f.id_photos).forEach(side => {
                if (side === 'front' || side === 'back' || side === 'other') sides[side] = true
              })
              if (Object.keys(sides).length > 0) setUploadedSides(sides)
            }
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
    // #1845: 实名证件必填——学生证豁免：选择「学生证」类型且学生证已传
    //（本轮选图 或 resume 会话已传）的，跳过身份证正反面（不做年龄判定）；
    // 其余证件类型仍须身份证正反面。
    const otherReady = otherPicked || uploadedSides.other
    const studentExempt = otherIdType === '学生证' && otherReady
    const frontReady = uploadedSides.front || !!idPhotoFrontRef.current?.hasFile()
    const backReady = uploadedSides.back || !!idPhotoBackRef.current?.hasFile()
    if (!studentExempt && (!frontReady || !backReady)) {
      Taro.showToast({ title: '请先上传身份证正反面照片', icon: 'none' })
      return
    }
    if (studentExempt) {
      // 学生证豁免路径：支付完成后引导人脸识别（FaceVerify），标记供 Payment 读取。
      session.setItem('reg_student_exempt', '1')
    } else {
      session.removeItem('reg_student_exempt')
    }
    setSaving(true)
    try {
      // Two-phase registration (#1663): submitting creates a pending session
      // (no account yet) and redirects to the payment page. The account is
      // created server-side after the membership fee callback.
      let sid = resumeSid
      let amount = sessionAmount
      if (!sid) {
        const body = { name: name.trim(), nickname: nickname.trim() || name.trim(), phone: phone.trim(), email: email.trim() }
        if (otherIdType) { body.id_photo_other_type = otherIdType } // #1807: 第三证件类型
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
          // #1807: 新建 session 后立即更新 state —— IdPhotoUploader 的
          // sessionUpload.sessionId 随之 rerender 拿到新 sid，否则 uploadPending()
          // 走 user/id-photo 端点（两阶段注册用户未创建 → 401 上传失败）。
          setResumeSid(sid)
        } else {
          Taro.showToast({ title: resolveErrorMessage(result, '提交失败, 请重试'), icon: 'none', duration: 3000 })
          setSaving(false)
          return
        }
      }
      session.setItem('pending_registration_session', sid)
      // Upload ID photos to the session (#1787) — only slots that actually
      // carry a locally picked image are uploaded (#1845): empty slots return
      // 'skip' and must NOT be counted as failures (previously any unselected
      // slot made the flow show a false "证件照上传失败" dialog).
      const slots = [
        { label: '身份证正面', ref: idPhotoFrontRef, uploaded: uploadedSides.front },
        { label: '身份证反面', ref: idPhotoBackRef, uploaded: uploadedSides.back },
        { label: '其他证件', ref: idPhotoOtherRef, uploaded: uploadedSides.other },
      ]
      const failedSides = []
      for (const slot of slots) {
        const hasLocal = !!slot.ref.current?.hasFile()
        if (!hasLocal && slot.uploaded) continue // resume 会话已传，无本地新图
        if (!hasLocal) continue // 未选图 = skip，不尝试上传
        // #1807: 显式传 sid（新建 session 后 prop 更新有批处理延迟，直接传参
        // 保证走 session 端点而非 user/id-photo 匿名 401）。
        const outcome = await slot.ref.current?.uploadPending(sid)
        if (outcome === null) failedSides.push(slot.label)
      }
      if (failedSides.length > 0) {
        const { confirm } = await Taro.showModal({
          title: '证件照上传失败',
          content: `${failedSides.join('、')}上传失败，可在注册后于『编辑资料』补传。`,
          confirmText: '继续支付',
          cancelText: '重试',
        })
        if (!confirm) {
          setSaving(false)
          return
        }
      }
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

      <View style={{ width: '100%', display: 'flex', alignItems: 'center', paddingTop: 12, paddingBottom: 12, border: '1px solid #d4d4d8', borderRadius: 12, paddingLeft: 16, paddingRight: 16, boxSizing: 'border-box', marginBottom: 12 }}>
        <Input placeholder="昵称" value={nickname} onInput={e => setNickname(e.detail.value)}
          type="nickname"
          style={{ flex: 1, fontSize: 14, height: '20px', padding: 0 }} />
      </View>
      <View style={{ width: '100%', display: 'flex', alignItems: 'center', paddingTop: 12, paddingBottom: 12, border: '1px solid #d4d4d8', borderRadius: 12, paddingLeft: 16, paddingRight: 16, boxSizing: 'border-box', marginBottom: 12 }}>
        <Input placeholder="姓名" value={name} onInput={e => setName(e.detail.value)}
          style={{ flex: 1, fontSize: 14, height: '20px', padding: 0 }} />
      </View>
      <View style={{ width: '100%', display: 'flex', alignItems: 'center', paddingTop: 12, paddingBottom: 12, border: '1px solid #d4d4d8', borderRadius: 12, paddingLeft: 16, paddingRight: 16, boxSizing: 'border-box', marginBottom: 12 }}>
        <Input placeholder="手机号" value={phone} onInput={e => setPhone(e.detail.value)}
          style={{ flex: 1, fontSize: 14, height: '20px', padding: 0 }} />
      </View>
      <View style={{ width: '100%', display: 'flex', alignItems: 'center', paddingTop: 12, paddingBottom: 12, border: '1px solid #d4d4d8', borderRadius: 12, paddingLeft: 16, paddingRight: 16, boxSizing: 'border-box', marginBottom: 24 }}>
        <Input placeholder="邮箱（选填）" value={email} onInput={e => setEmail(e.detail.value)}
          style={{ flex: 1, fontSize: 14, height: '20px', padding: 0 }} />
      </View>

      <Text style={{ fontSize: 16, fontWeight: '700', color: '#000', width: '100%', marginBottom: 12 }}>收货地址（选填）</Text>
      <View style={{ display: 'flex', width: '100%', marginBottom: 12 }}>
        <View style={{ flex: 1, marginRight: 8 }}>
          <Picker mode="selector" range={provinceNames} value={province ? provinceNames.indexOf(province) : 0}
            onChange={e => { setProvince(provinceNames[e.detail.value]); setCity(''); setDistrict('') }}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: '44px', display: 'flex', alignItems: 'center', padding: '0 16px', boxSizing: 'border-box', fontSize: 14, color: province ? '#000' : '#9ca3af' }}>
              {province || '省'}
            </View>
          </Picker>
        </View>
        <View style={{ flex: 1, marginRight: 8 }}>
          <Picker mode="selector" range={cityNames} value={city ? cityNames.indexOf(city) : 0}
            onChange={e => { setCity(cityNames[e.detail.value]); setDistrict('') }}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: '44px', display: 'flex', alignItems: 'center', padding: '0 16px', boxSizing: 'border-box', fontSize: 14, color: city ? '#000' : '#9ca3af' }}>
              {city || '市'}
            </View>
          </Picker>
        </View>
        {districtNames.length > 0 && (
        <View style={{ flex: 1 }}>
          <Picker mode="selector" range={districtNames} value={district ? districtNames.indexOf(district) : 0}
            onChange={e => setDistrict(districtNames[e.detail.value])}>
            <View style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: '44px', display: 'flex', alignItems: 'center', padding: '0 16px', boxSizing: 'border-box', fontSize: 14, color: district ? '#000' : '#9ca3af' }}>
              {district || '区'}
            </View>
          </Picker>
        </View>
        )}
      </View>
      <View style={{ width: '100%', display: 'flex', alignItems: 'center', paddingTop: 12, paddingBottom: 12, border: '1px solid #d4d4d8', borderRadius: 12, paddingLeft: 16, paddingRight: 16, boxSizing: 'border-box', marginBottom: 12 }}>
        <Input placeholder="详细地址" value={detail} onInput={e => setDetail(e.detail.value)}
          style={{ flex: 1, fontSize: 14, height: '20px', padding: 0 }} />
      </View>
      <View style={{ width: '100%', display: 'flex', alignItems: 'center', paddingTop: 12, paddingBottom: 12, border: '1px solid #d4d4d8', borderRadius: 12, paddingLeft: 16, paddingRight: 16, boxSizing: 'border-box', marginBottom: 24 }}>
        <Input placeholder="邮编（选填）" value={postalCode} onInput={e => setPostalCode(e.detail.value)}
          style={{ flex: 1, fontSize: 14, height: '20px', padding: 0 }} />
      </View>

      {/* #1845: 默认身份证必填；选择「学生证」类型并已传学生证后提示可豁免 */}
      <Text style={{ fontSize: 16, fontWeight: '700', color: '#000', width: '100%', marginBottom: 12 }}>
        身份证照片{(otherIdType === '学生证' && (otherPicked || uploadedSides.other))
          ? <Text style={{ color: '#a16207' }}>（学生证已传可免填）</Text>
          : <Text style={{ color: '#ef4444' }}>（必填）</Text>}
      </Text>
      {/* #1807: 正反面一行（各 ~48%） */}
      <View style={{ display: 'flex', width: '100%', marginBottom: 12, justifyContent: 'space-between' }}>
        <View style={{ width: '48%', display: 'flex', justifyContent: 'center' }}>
          <IdPhotoUploader ref={idPhotoFrontRef} side="front" defer sessionUpload={{ sessionId: resumeSid || undefined }} />
        </View>
        <View style={{ width: '48%', display: 'flex', justifyContent: 'center' }}>
          <IdPhotoUploader ref={idPhotoBackRef} side="back" defer sessionUpload={{ sessionId: resumeSid || undefined }} />
        </View>
      </View>
      {/* #1807/#1845: 第三证件标题改版——学校师生专属文案；证件类型（在上，宽度与上传框一致）+ 上传框靠左 */}
      <Text style={{ fontSize: 14, fontWeight: '600', color: '#000', width: '100%', marginBottom: 4 }}>学生证、教职工等其他证件</Text>
      <Text style={{ fontSize: 12, color: '#a16207', width: '100%', marginBottom: 8, lineHeight: '18px' }}>
        （学校师生专属：上传学生证/教职工证，享绿色通道及特殊政策！）
      </Text>
      <View style={{ display: 'flex', width: '100%', marginBottom: 8 }}>
        <Picker mode="selector" range={ID_TYPE_OPTIONS} value={otherIdType ? ID_TYPE_OPTIONS.indexOf(otherIdType) : 0}
          onChange={e => setOtherIdType(ID_TYPE_OPTIONS[e.detail.value])}>
          <View className="w-32" style={{ border: '1px solid #d4d4d8', borderRadius: 12, height: '44px', display: 'flex', alignItems: 'center', paddingLeft: 12, paddingRight: 12, boxSizing: 'border-box', fontSize: 13, color: otherIdType ? '#000' : '#9ca3af' }}>
            {otherIdType ? `证件类型：${otherIdType}` : '证件类型'}
          </View>
        </Picker>
      </View>
      <View style={{ display: 'flex', width: '100%', marginBottom: 24 }}>
        <IdPhotoUploader ref={idPhotoOtherRef} side="other" defer sessionUpload={{ sessionId: resumeSid || undefined }} leftAligned onSelect={() => setOtherPicked(true)} onClear={() => setOtherPicked(false)} />
      </View>

      <View onClick={handleRegister}
        style={{ width: '100%', paddingTop: 13, paddingBottom: 13, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12 }}>
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
