import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button, Picker } from '@tarojs/components'
import { useNavigate } from 'react-router-dom'
import { apiFetch, getToken, resolveErrorMessage } from '../../../services/api'
import { env, dialog, getInputValue, wxLogin as wxLoginCode } from '../../../platform'
import { parseJWT } from '../../../platform/init'
import IdPhotoUploader from '../../../components/IdPhotoUploader'

const ID_TYPE_OPTIONS = ['学生证', '教师证', '工作证', '其他']

export default function EditProfile() {
  const [name, setName] = useState('')
  const [nickname, setNickname] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [idPhotoFront, setIdPhotoFront] = useState('')
  const [idPhotoBack, setIdPhotoBack] = useState('')
  const [idPhotoOther, setIdPhotoOther] = useState('')
  const [idPhotoOtherType, setIdPhotoOtherType] = useState('') // #1807: 第三证件类型
  const [realName, setRealName] = useState('')
  const [idCardNo, setIdCardNo] = useState('')
  const [faceVerified, setFaceVerified] = useState(false)
  const [faceVerifiedAt, setFaceVerifiedAt] = useState('')
  const [idVerifyStatus, setIdVerifyStatus] = useState('') // #1792 T4: 五态聚合
  const [saving, setSaving] = useState(false)
  const [bindingWx, setBindingWx] = useState(false)
  const baseUrl = env.apiBaseUrl
  const navigate = useNavigate()

  // #1807: 进入独立「人脸识别模式」页（自拍照片 + 动态视频采样）。
  // H5 用 react-router 路径；weapp 用完整 /pages-weapp/... 路径（#1665 跨端导航规则）。
  const goFaceVerify = () => {
    if (!env.isMiniProgram) return navigate('/face-verify')
    Taro.navigateTo({ url: '/pages-weapp/face-verify/index' })
  }

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
          setIdPhotoOtherType(result.data.id_photo_other_type || '') // #1807
          setRealName(result.data.real_name || '')
          setIdCardNo(result.data.id_card_no || '')
          setFaceVerified(result.data.face_verified || false)
          setFaceVerifiedAt(result.data.face_verified_at || '')
          setIdVerifyStatus(result.data.id_verify_status || '')
        }
      } catch (e) {
        // Silently ignore fetch errors on mount
      }
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
          // #1807: 第三证件类型
          ...(idPhotoOtherType ? { id_photo_other_type: idPhotoOtherType } : {}),
          // #1807: real_name/id_card_no 不再由顾客提交——实名信息由员工在
          // 审核流程根据身份证照核对填写（face_review approve）。
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        // H5 (Vite) has no Taro runtime — use platform dialog/navigation
        if (env.isMiniProgram) {
          Taro.showToast({ title: '保存成功', icon: 'success' })
          // eslint-disable-next-line no-undef
          setTimeout(() => Taro.navigateBack(), 800)
        } else {
          dialog.toast('保存成功')
          // eslint-disable-next-line no-undef
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
          {/* #1807: 正反面一行（各 ~48%） */}
          <View style={{ display: 'flex', flexDirection: 'row', justifyContent: 'space-between', marginBottom: 12 }}>
            <View style={{ width: '48%', display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="front" initialUrl={idPhotoFront} onChange={setIdPhotoFront} />
            </View>
            <View style={{ width: '48%', display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="back" initialUrl={idPhotoBack} onChange={setIdPhotoBack} />
            </View>
          </View>
          {/* #1807: 其他证件小节标题 + 证件类型（在上，宽度与上传框一致）+ 上传框靠左 */}
          <Text style={{ fontSize: 13, color: '#6b7280', marginBottom: 8 }}>其他证件</Text>
          <View style={{ marginBottom: 8 }}>
            <Picker mode="selector" range={ID_TYPE_OPTIONS} value={idPhotoOtherType ? ID_TYPE_OPTIONS.indexOf(idPhotoOtherType) : 0}
              onChange={e => setIdPhotoOtherType(ID_TYPE_OPTIONS[e.detail.value])}>
              <View className="w-32" style={{ border: '1px solid #d4d4d8', borderRadius: 8, height: '44px', display: 'flex', alignItems: 'center', paddingLeft: 12, paddingRight: 12, boxSizing: 'border-box', fontSize: 13, color: idPhotoOtherType ? '#000' : '#9ca3af' }}>
                {idPhotoOtherType ? `证件类型：${idPhotoOtherType}` : '证件类型'}
              </View>
            </Picker>
          </View>
          <View>
            <IdPhotoUploader side="other" initialUrl={idPhotoOther} onChange={setIdPhotoOther} leftAligned />
          </View>
        </View>
        {/* 实名认证区块 (#1787) */}
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 10 }}>实名认证</Text>
          {faceVerified ? (
            <View style={{ padding: 12, backgroundColor: '#f0fdf4', borderRadius: 8, borderWidth: 1, borderColor: '#bbf7d0' }}>
              <Text style={{ fontSize: 13, color: '#16a34a', fontWeight: '600' }}>✅ 已认证</Text>
              {realName && <Text style={{ fontSize: 12, color: '#6b7280', marginTop: 4 }}>姓名：{realName}</Text>}
              {idCardNo && <Text style={{ fontSize: 12, color: '#6b7280' }}>身份证：{maskIdCard(idCardNo)}</Text>}
              {faceVerifiedAt && <Text style={{ fontSize: 12, color: '#9ca3af', marginTop: 4 }}>认证时间：{new Date(faceVerifiedAt).toLocaleString()}</Text>}
            </View>
          ) : idVerifyStatus === 'pending_review' ? (
            <View style={{ padding: 12, backgroundColor: '#fefce8', borderRadius: 8, borderWidth: 1, borderColor: '#fde68a' }}>
              <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600' }}>⚠️ 实名认证审核中</Text>
              <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>已提交人脸采样，平台员工审核通过后即完成实名认证（预计 1-2 个工作日）。</Text>
            </View>
          ) : (
            <View>
              {/* #1811: 按 id_verify_status 细分文案 + 分行（weapp Text 不渲染 \n） */}
              <View style={{ padding: 12, backgroundColor: '#fefce8', borderRadius: 8, borderWidth: 1, borderColor: '#fde68a' }}>
                {idVerifyStatus === 'none' ? (
                  <>
                    <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600' }}>⚠️ 尚未完成实名认证</Text>
                    <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>请先在上方上传身份证照片</Text>
                  </>
                ) : idVerifyStatus === 'rejected' ? (
                  <>
                    <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600' }}>⚠️ 审核未通过</Text>
                    <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>请重新发起人脸识别</Text>
                    <View onClick={goFaceVerify} style={{ marginTop: 8, padding: 4 }}>
                      <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600', textDecorationLine: 'underline' }}>发起人脸识别 ›</Text>
                    </View>
                  </>
                ) : (
                  <>
                    <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600' }}>⚠️ 已上传身份证照片</Text>
                    <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>请完成人脸识别</Text>
                    <View onClick={goFaceVerify} style={{ marginTop: 8, padding: 4 }}>
                      <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600', textDecorationLine: 'underline' }}>发起人脸识别 ›</Text>
                    </View>
                  </>
                )}
              </View>
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

// #1807: 身份证号掩码展示（如 110***********1234）。
function maskIdCard(no) {
  if (!no || no.length < 8) return no || ''
  return no.slice(0, 3) + '***********' + no.slice(-4)
}
