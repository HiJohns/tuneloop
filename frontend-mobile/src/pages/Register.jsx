import { useState, useEffect, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Input, Button, ScrollView } from '@tarojs/components'
import { apiFetch , resolveErrorMessage } from '../services/api'
import { dialog, env, storage, session, navigation } from '../platform'
import { getWXConfig } from '../platform/init'
import IdPhotoUploader from '../components/IdPhotoUploader'
import regions from '../data/regions.json'
import AgreementCheckbox from '../components/AgreementCheckbox'

// Register — H5 用户注册页（P-03 / #1597）
// 方案 C：IAM 管登录，tuneloop 管注册。POST /auth/register 复用 weapp 注册链路。

// #2244 审计返工：同意流会离开本路由，React Router 卸载组件导致本地表单丢失。
// 草稿策略：普通字段写 sessionStorage（刷新可恢复）；密码仅保存在模块内存
// （SPA 内跨路由保留，不落盘），注册成功后统一清除。
const REGISTER_DRAFT_KEY = 'register_form_draft'
let registerDraftMemory = null

function loadRegisterDraft() {
  if (registerDraftMemory) return registerDraftMemory
  return storage.getJSON(REGISTER_DRAFT_KEY, {}) || {}
}

export default function Register() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const baseUrl = env.apiBaseUrl

  const [initialDraft] = useState(loadRegisterDraft)
  const [name, setName] = useState(initialDraft.name || '')
  const [nickname, setNickname] = useState(initialDraft.nickname || '')
  const [phone, setPhone] = useState(initialDraft.phone || '')
  const [email, setEmail] = useState(initialDraft.email || '')
  const [password, setPassword] = useState(initialDraft.password || '')
  const [confirmPassword, setConfirmPassword] = useState(initialDraft.confirmPassword || '')
  const [province, setProvince] = useState(initialDraft.province || '')
  const [city, setCity] = useState(initialDraft.city || '')
  const [district, setDistrict] = useState(initialDraft.district || '')
  const [detail, setDetail] = useState(initialDraft.detail || '')
  const [postalCode, setPostalCode] = useState(initialDraft.postalCode || '')
  const [saving, setSaving] = useState(false)
  const [agreed, setAgreed] = useState(false) // #1923: 协议同意
  const idPhotoFrontRef = useRef(null)
  const idPhotoBackRef = useRef(null)
  const idPhotoOtherRef = useRef(null)

  // 推荐码：从 URL ?ref= 或 ?scene=ref= 读取（微信扫码场景）
  useEffect(() => {
    const ref = searchParams.get('ref')
    if (ref) storage.setItem('ref_code', ref)
    const scene = searchParams.get('scene')
    if (scene && scene.startsWith('ref=')) storage.setItem('ref_code', scene.slice(4))
  }, [])

  // #2244：同意流返回后恢复表单（密码仅内存，不写 sessionStorage）
  useEffect(() => {
    registerDraftMemory = {
      name, nickname, phone, email, password, confirmPassword,
      province, city, district, detail, postalCode,
    }
    const { password: _pw, confirmPassword: _cpw, ...persisted } = registerDraftMemory
    storage.setItem(REGISTER_DRAFT_KEY, JSON.stringify(persisted))
  }, [name, nickname, phone, email, password, confirmPassword, province, city, district, detail, postalCode])

  const provinceNames = regions.map(r => r.name)
  const selectedProv = regions.find(r => r.name === province)
  const cityNames = selectedProv ? selectedProv.children.map(c => c.name) : []
  const selectedCity = selectedProv ? selectedProv.children.find(c => c.name === city) : null
  const districtNames = selectedCity ? selectedCity.children.map(d => d.name) : []

  const goLogin = () => {
    const config = getWXConfig()
    const redirectUri = encodeURIComponent(`${navigation.getOrigin()}/callback`)
    const authUrl = `${config.iamExternalUrl}/oauth/authorize?client_id=${config.iamClientId}&redirect_uri=${redirectUri}&response_type=code`
    navigation.redirect(authUrl)
  }

  const handleRegister = async () => {
    if (!name.trim()) { dialog.alert('请输入姓名'); return }
    if (!phone.trim()) { dialog.alert('请输入手机号'); return }
    if (!/^1[3-9]\d{9}$/.test(phone.trim())) { dialog.alert('手机号格式不正确'); return }
    if (!password) { dialog.alert('请输入密码'); return }
    if (password !== confirmPassword) { dialog.alert('两次输入的密码不一致'); return }
    if (!agreed) { dialog.alert('请先阅读并同意《相关协议》'); return }
    setSaving(true)
    try {
      const body = {
        name: name.trim(),
        nickname: nickname.trim() || name.trim(),
        phone: phone.trim(),
        email: email.trim(),
        password,
        agreed_terms: true,
      }
      const refCode = storage.getItem('ref_code')
      if (refCode) body.ref = refCode

      const resp = await apiFetch(`${baseUrl}/auth/register`, {
        method: 'POST',
        body: JSON.stringify(body),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data?.access_token) {
        storage.setItem('token', result.data.access_token)
        storage.setItem('token_expiry', (Date.now() + (result.data.expires_in || 3600) * 1000).toString())
        session.removeItem('post_auth_redirect')
        // #2244: 注册成功，清除表单草稿（含内存中的密码）
        registerDraftMemory = null
        storage.removeItem(REGISTER_DRAFT_KEY)

        // 注册成功拿到 token 后上传待传身份证（defer 模式）
        try {
          if (idPhotoFrontRef.current?.uploadPending) await idPhotoFrontRef.current.uploadPending()
          if (idPhotoBackRef.current?.uploadPending) await idPhotoBackRef.current.uploadPending()
          if (idPhotoOtherRef.current?.uploadPending) await idPhotoOtherRef.current.uploadPending()
        } catch (e) { console.error('[Register] id photo upload failed', e) }

        // 保存收货地址
        if (province && city && detail) {
          try {
            await apiFetch(`${baseUrl}/user/addresses`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                recipient_name: name.trim(), phone: phone.trim(),
                province, city, district, detail,
                postal_code: postalCode, is_default: true,
              }),
            })
          } catch (e) { console.error('[Register] address save failed', e) }
        }

        // 跳转会员费支付
        const fee = result.data?.membership_fee || 99
        navigate(`/payment?type=membership&amount=${fee}`)
      } else {
        dialog.alert(resolveErrorMessage(result, '注册失败, 请重试'))
      }
    } catch (err) {
      dialog.alert('网络错误, 请重试')
    }
    setSaving(false)
  }

  return (
    <ScrollView scrollY className="h-screen">
      <View className="px-5 pt-12 pb-8">
        <View className="mb-1"><Text className="text-2xl font-bold text-center block">注册账号</Text></View>
        <View className="mb-8"><Text className="text-gray-500 text-center text-sm block">填写信息即可开始租赁</Text></View>

        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">姓名 <Text className="text-red-500">*</Text></Text></View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="请输入真实姓名" value={name} onChange={e => setName(e.target.value)} />
        </View>

        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">昵称（微信昵称，可编辑）</Text></View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="输入您的昵称" value={nickname} onChange={e => setNickname(e.target.value)} />
        </View>

        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">手机号 <Text className="text-red-500">*</Text></Text></View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="请输入手机号" value={phone} onChange={e => setPhone(e.target.value)} />
        </View>

        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">邮箱（选填）</Text></View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="请输入邮箱" value={email} onChange={e => setEmail(e.target.value)} />
        </View>

        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">密码 <Text className="text-red-500">*</Text></Text></View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm" password
            placeholder="请输入密码" value={password} onChange={e => setPassword(e.target.value)} />
        </View>

        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">确认密码 <Text className="text-red-500">*</Text></Text></View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm" password
            placeholder="请再次输入密码" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} />
        </View>

        {/* 收货地址（选填） */}
        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">收货地址（选填）</Text></View>
          <View className="flex flex-row gap-2 mb-2">
            <select style={{ width: '50%' }} className="border border-gray-300 rounded-lg px-2 py-3 text-sm bg-white"
              value={province} onChange={e => { setProvince(e.target.value); setCity(''); setDistrict('') }}>
              <option value="">省</option>
              {provinceNames.map((r, i) => <option key={i} value={r}>{r}</option>)}
            </select>
            <select style={{ width: '25%' }} className="border border-gray-300 rounded-lg px-2 py-3 text-sm bg-white"
              value={city} onChange={e => { setCity(e.target.value); setDistrict('') }}>
              <option value="">市</option>
              {cityNames.map((c, i) => <option key={i} value={c}>{c}</option>)}
            </select>
            <select style={{ width: '25%' }} className="border border-gray-300 rounded-lg px-2 py-3 text-sm bg-white"
              value={district} onChange={e => setDistrict(e.target.value)}>
              <option value="">区</option>
              {districtNames.map((d, i) => <option key={i} value={d}>{d}</option>)}
            </select>
          </View>
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm mb-2"
            placeholder="详细地址" value={detail} onChange={e => setDetail(e.target.value)} />
          <Input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm"
            placeholder="邮编（选填）" value={postalCode} onChange={e => setPostalCode(e.target.value)} />
        </View>

        {/* 身份证照片（选填，defer 上传） */}
        <View className="mb-4">
          <View className="mb-1"><Text className="text-sm font-medium text-gray-700">身份证照片（选填）</Text></View>
          <View className="flex flex-row gap-3">
            <View className="flex-1 flex justify-center">
              <IdPhotoUploader ref={idPhotoFrontRef} side="front" defer />
            </View>
            <View className="flex-1 flex justify-center">
              <IdPhotoUploader ref={idPhotoBackRef} side="back" defer />
            </View>
            <View className="flex-1 flex justify-center">
              <IdPhotoUploader ref={idPhotoOtherRef} side="other" defer />
            </View>
          </View>
        </View>

        {/* #2044: 审核提示 */}
        <View className="mb-4">
          <Text className="text-xs text-zinc-500 leading-relaxed">
            实名信息将由平台审核；完成微信核身的可即时生效，人工审核约需 1-2 个工作日，结果将通过系统消息通知您。
          </Text>
        </View>

        {/* #1923/#2244: 协议同意（必选，显性阅读同意流） */}
        <AgreementCheckbox
          agreed={agreed}
          onChange={setAgreed}
          style={{ marginBottom: 12, paddingLeft: 4, paddingRight: 4 }}
          onOpenConsent={() => navigate('/content?key=related_agreements&consent=1')}
          onOpenRead={() => navigate('/content?key=related_agreements')}
        />

        <Button className="w-full bg-blue-500 text-white py-4 rounded-xl text-lg font-medium"
          disabled={saving || !agreed} onClick={handleRegister}>
          {saving ? '注册中...' : '注  册'}
        </Button>

        <View className="mt-4 text-center">
          <Text className="text-gray-400 text-sm">已有账号？</Text>
          <Text className="text-blue-500 text-sm font-medium" onClick={goLogin}>登录 →</Text>
        </View>
      </View>
    </ScrollView>
  )
}
