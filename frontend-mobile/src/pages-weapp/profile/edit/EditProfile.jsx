import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../../../services/api'
import { env, dialog, getInputValue, wxLogin as wxLoginCode } from '../../../platform'
import { parseJWT } from '../../../platform/init'
import IdPhotoUploader from '../../../components/IdPhotoUploader'

export default function EditProfile() {
  const [name, setName] = useState('')
  const [nickname, setNickname] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [idPhotoFront, setIdPhotoFront] = useState('')
  const [idPhotoBack, setIdPhotoBack] = useState('')
  const [idPhotoOther, setIdPhotoOther] = useState('')
  const [realName, setRealName] = useState('')
  const [idCardNo, setIdCardNo] = useState('')
  const [faceVerified, setFaceVerified] = useState(false)
  const [faceVerifiedAt, setFaceVerifiedAt] = useState('')
  const [saving, setSaving] = useState(false)
  const [bindingWx, setBindingWx] = useState(false)
  const [showFaceVerify, setShowFaceVerify] = useState(false)
  const [faceVerifyLoading, setFaceVerifyLoading] = useState(false)
  const baseUrl = env.apiBaseUrl

  const token = getToken()
  const claims = token ? parseJWT(token) : {}
  // 员工判定统一口径（#1639 / #1700）：对齐后端 GetBusinessRole——
  // role == 'USER'（含 oid/tid 非空的顾客）→ 顾客；仅非 USER 角色才视为员工
  const isStaff = !!claims.role && claims.role !== 'USER'

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          setName(result.data.name || '')
          // WeChat nickname is only the default value; the user may edit it
          // freely (#1588). phone/email prefill from current profile.
          setNickname(result.data.nickname || result.data.wx_nickname || result.data.name || '')
          setPhone(result.data.phone || '')
          setEmail(result.data.email || '')
          setIdPhotoFront(result.data.id_photo_front || '')
          setIdPhotoBack(result.data.id_photo_back || '')
          setIdPhotoOther(result.data.id_photo_other || '')
          setRealName(result.data.real_name || '')
          setIdCardNo(result.data.id_card_no || '')
          setFaceVerified(result.data.face_verified || false)
          setFaceVerifiedAt(result.data.face_verified_at || '')
        }
      } catch {}
    }
    fetchUser()
  }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name, nickname, phone, email,
          // #1686: id photos were never submitted — backend supports them.
          ...(idPhotoFront ? { id_photo_front: idPhotoFront } : {}),
          ...(idPhotoBack ? { id_photo_back: idPhotoBack } : {}),
          ...(idPhotoOther ? { id_photo_other: idPhotoOther } : {}),
          ...(realName ? { real_name: realName } : {}),
          ...(idCardNo ? { id_card_no: idCardNo } : {}),
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        // H5 (Vite) has no Taro runtime — use platform dialog/navigation
        if (env.isMiniProgram) {
          Taro.showToast({ title: '保存成功', icon: 'success' })
          setTimeout(() => Taro.navigateBack(), 800)
        } else {
          dialog.toast('保存成功')
          setTimeout(() => window.history.back(), 800)
        }
      } else {
        dialog.toast(resolveErrorMessage(result, '保存失败'))
      }
    } catch {
      dialog.toast('网络错误')
    }
    setSaving(false)
  }

  // 员工绑定微信（#1639 计划 §五）：wx.login 拿 code → POST /users/me/wx-bind
  const handleBindWx = async () => {
    if (bindingWx) return
    setBindingWx(true)
    try {
      const code = await wxLoginCode()
      if (!code) { dialog.toast('获取微信登录凭证失败'); setBindingWx(false); return }
      const resp = await apiFetch(`${baseUrl}/users/me/wx-bind`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.toast('微信绑定成功')
      } else {
        dialog.toast(resolveErrorMessage(result, '微信绑定失败'))
      }
    } catch {
      dialog.toast('网络错误')
    }
    setBindingWx(false)
  }

  // Face verification (#1787)
  const handleFaceVerify = async () => {
    if (!realName.trim() || !idCardNo.trim()) {
      Taro.showToast({ title: '请先填写姓名和身份证号', icon: 'none' })
      return
    }
    setFaceVerifyLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/user/face-verify/token`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: realName.trim(), id_card_no: idCardNo.trim() }),
      })
      const result = await resp.json()
      if (result.code === 40012) {
        Taro.showToast({ title: '人脸认证暂未开通', icon: 'none' })
        setFaceVerifyLoading(false)
        return
      }
      if (result.code !== 20000 || !result.data?.biz_token) {
        Taro.showToast({ title: '获取认证令牌失败', icon: 'none' })
        setFaceVerifyLoading(false)
        return
      }
      // On weapp: show instructions (the actual plugin integration requires
      // 慧眼小程序插件 to be added in the WeChat backend).
      Taro.showModal({
        title: '人脸认证',
        content: '请确保在小程序后台已添加「慧眼人脸核身」插件。点击确定后将调起人脸核身。',
        confirmText: '确定',
        cancelText: '取消',
      }).then(({ confirm }) => {
        if (confirm) {
          // TODO: integrate 慧眼 plugin — navigateTo plugin://faceid/verify?token=...
          Taro.showToast({ title: '请先添加慧眼插件', icon: 'none' })
        }
      })
    } catch {
      Taro.showToast({ title: '网络错误', icon: 'none' })
    }
    setFaceVerifyLoading(false)
  }

  const handleQueryFaceResult = async (bizToken) => {
    try {
      const resp = await apiFetch(`${baseUrl}/user/face-verify/result`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ biz_token: bizToken }),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data?.passed) {
        setFaceVerified(true)
        setFaceVerifiedAt(new Date().toISOString())
        Taro.showToast({ title: '认证成功', icon: 'success' })
      } else {
        Taro.showToast({ title: '认证未通过，请重试', icon: 'none' })
      }
    } catch {
      Taro.showToast({ title: '查询认证结果失败', icon: 'none' })
    }
  }

  return (
    <View style={{ height: '100vh', backgroundColor: '#f4f4f5', display: 'flex', flexDirection: 'column' }}>
      <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 12, padding: 16 }}>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>姓名</Text>
          <Input value={name} onInput={e => setName(getInputValue(e))}
            placeholder={name ? '' : '请输入真实姓名'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>昵称</Text>
          <Input value={nickname} onInput={e => setNickname(getInputValue(e))}
            placeholder={nickname ? '' : '请输入昵称'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>手机号</Text>
          <Input value={phone} onInput={e => setPhone(getInputValue(e))}
            placeholder={phone ? '' : '请输入手机号'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>邮箱</Text>
          <Input value={email} onInput={e => setEmail(getInputValue(e))}
            placeholder={email ? '' : '请输入邮箱'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 10 }}>身份证照片</Text>
          <View style={{ display: 'flex', flexDirection: 'row', flexWrap: 'wrap', justifyContent: 'space-between' }}>
            <View style={{ width: '30%', display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="front" initialUrl={idPhotoFront} onChange={setIdPhotoFront} />
            </View>
            <View style={{ width: '30%', display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="back" initialUrl={idPhotoBack} onChange={setIdPhotoBack} />
            </View>
            <View style={{ width: '30%', display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="other" initialUrl={idPhotoOther} onChange={setIdPhotoOther} />
            </View>
          </View>
        </View>
        {/* 实名认证区块 (#1787) */}
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 10 }}>实名认证</Text>
          {faceVerified ? (
            <View style={{ padding: 12, backgroundColor: '#f0fdf4', borderRadius: 8, borderWidth: 1, borderColor: '#bbf7d0' }}>
              <Text style={{ fontSize: 13, color: '#16a34a', fontWeight: '600' }}>✅ 已认证</Text>
              {realName && <Text style={{ fontSize: 12, color: '#6b7280', marginTop: 4 }}>姓名：{realName}</Text>}
              {idCardNo && <Text style={{ fontSize: 12, color: '#6b7280' }}>身份证：{idCardNo}</Text>}
              {faceVerifiedAt && <Text style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>认证时间：{new Date(faceVerifiedAt).toLocaleString()}</Text>}
            </View>
          ) : (
            <View>
              <View style={{ marginBottom: 10 }}>
                <Text style={{ fontSize: 12, color: '#6b7280', marginBottom: 4 }}>姓名</Text>
                <Input value={realName} onInput={e => setRealName(getInputValue(e))}
                  placeholder="请输入真实姓名"
                  style={{ width: '100%', height: 40, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 13, boxSizing: 'border-box' }} />
              </View>
              <View style={{ marginBottom: 10 }}>
                <Text style={{ fontSize: 12, color: '#6b7280', marginBottom: 4 }}>身份证号</Text>
                <Input value={idCardNo} onInput={e => setIdCardNo(getInputValue(e))}
                  placeholder="请输入18位身份证号"
                  style={{ width: '100%', height: 40, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 13, boxSizing: 'border-box' }} />
              </View>
              <View onClick={!faceVerifyLoading ? handleFaceVerify : undefined}
                style={{ width: '100%', height: 40, backgroundColor: faceVerifyLoading ? '#d4d4d8' : '#915F38', borderRadius: 20, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Text style={{ color: '#fff', fontSize: 13, fontWeight: '600' }}>{faceVerifyLoading ? '获取认证中...' : '发起人脸认证'}</Text>
              </View>
              {!env.isMiniProgram && (
                <Text style={{ fontSize: 12, color: '#9ca3af', marginTop: 8, textAlign: 'center' }}>请在微信小程序中完成人脸认证</Text>
              )}
            </View>
          )}
        </View>
        {isStaff && (
          <View style={{ marginBottom: 20 }}>
            <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 10 }}>微信账户</Text>
            <View onClick={handleBindWx}
              style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', boxSizing: 'border-box' }}>
              <Text style={{ fontSize: 14, color: '#915F38', fontWeight: '600' }}>{bindingWx ? '绑定中...' : '绑定微信账户'}</Text>
            </View>
          </View>
        )}
        <Button onClick={handleSave}
          style={{ width: '100%', height: 44, backgroundColor: '#915F38', color: '#fff', borderRadius: 22, fontSize: 16, fontWeight: '700', lineHeight: '44px', border: 'none', marginTop: 8, boxSizing: 'border-box' }}>
          {saving ? '保存中...' : '保存'}
        </Button>
      </View>
    </View>
  )
}
