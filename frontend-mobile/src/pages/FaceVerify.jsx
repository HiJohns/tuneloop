// FaceVerify — 人脸识别模式独立页（#1807）
// 编辑资料页身份证照未验证时，黄色警告 + 链接进入本页。
// 采集信息取决于人脸识别+身份证校验所需：自拍照片（必选）+ 动态视频（可选，
// 活体佐证）→ POST /user/face-capture → 平台员工人工审核（未实装腾讯云）。
import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { useNavigate } from 'react-router-dom'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { env, dialog } from '../platform'
import FaceCaptureUploader from '../components/FaceCaptureUploader'

const STATUS_TEXT = {
  none: '未上传身份证',
  uploaded: '已上传身份证，待提交人脸采样',
  pending_review: '已提交，等待平台员工审核',
  verified: '已实名认证',
  rejected: '审核未通过，请重新提交',
}

export default function FaceVerify() {
  const [status, setStatus] = useState('')
  const [hasIdPhoto, setHasIdPhoto] = useState(false)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl

  const fetchStatus = async () => {
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`)
      const r = await resp.json()
      if (r.code === 20000) {
        setStatus(r.data.id_verify_status || '')
        setHasIdPhoto(!!(r.data.id_photo_front || r.data.id_photo_back || r.data.id_photo_other))
      }
    } catch {
      dialog.toast('加载失败，请重试')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchStatus() }, [])

  const goBack = () => {
    if (!env.isMiniProgram) return navigate('/profile/edit')
    Taro.navigateBack()
  }

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#f4f4f5' }}>
      {/* H5 手写标题条（weapp 用原生导航栏，见 #1511 规则） */}
      {!env.isMiniProgram && (
        <View style={{ padding: 12, paddingLeft: 16, backgroundColor: '#fff', display: 'flex', alignItems: 'center' }}>
          <View onClick={goBack} style={{ marginRight: 8, padding: 4 }}>
            <Text style={{ fontSize: 20, color: '#6b7280' }}>‹</Text>
          </View>
          <Text style={{ fontSize: 16, fontWeight: '700', color: '#111' }}>人脸识别</Text>
        </View>
      )}

      <View style={{ margin: 16, backgroundColor: '#fff', borderRadius: 12, padding: 16 }}>
        <Text style={{ fontSize: 14, fontWeight: '700', color: '#111', marginBottom: 8 }}>实名认证</Text>
        {loading ? (
          <Text style={{ fontSize: 13, color: '#9ca3af' }}>加载中...</Text>
        ) : status === 'verified' ? (
          <View style={{ padding: 12, backgroundColor: '#f0fdf4', borderRadius: 8, borderWidth: 1, borderColor: '#bbf7d0' }}>
            <Text style={{ fontSize: 13, color: '#16a34a', fontWeight: '600' }}>✅ 已实名认证</Text>
          </View>
        ) : !hasIdPhoto ? (
          <View style={{ padding: 12, backgroundColor: '#fefce8', borderRadius: 8, borderWidth: 1, borderColor: '#fde68a' }}>
            <Text style={{ fontSize: 13, color: '#b45309', fontWeight: '600' }}>⚠️ 请先上传身份证照片</Text>
            <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>人脸识别前需先上传身份证件照。</Text>
            <View onClick={goBack} style={{ marginTop: 8 }}>
              <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600', textDecorationLine: 'underline' }}>去上传身份证 ›</Text>
            </View>
          </View>
        ) : status === 'pending_review' ? (
          <View style={{ padding: 12, backgroundColor: '#fefce8', borderRadius: 8, borderWidth: 1, borderColor: '#fde68a' }}>
            <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600' }}>⚠️ 实名认证审核中</Text>
            <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>已提交人脸采样，平台员工审核通过后即完成实名认证（预计 1-2 个工作日）。</Text>
          </View>
        ) : (
          <View>
            <View style={{ padding: 12, backgroundColor: '#f4f4f5', borderRadius: 8, marginBottom: 12 }}>
              <Text style={{ fontSize: 12, color: '#6b7280' }}>
                请完成以下人脸采样。提交后由平台员工依据身份证照核对填写实名信息（真实姓名、身份证号等），审核通过即完成实名认证。
              </Text>
            </View>
            <FaceCaptureUploader
              initialStatus={status}
              onSubmitSuccess={() => {
                setStatus('pending_review')
                dialog.toast('提交成功，等待审核')
                setTimeout(() => { if (env.isMiniProgram) { Taro.navigateBack() } else { navigate('/profile/edit') } }, 800)
              }}
            />
          </View>
        )}
      </View>

      {!loading && status === 'rejected' && (
        <View style={{ margin: 16, marginTop: 0, padding: 12, backgroundColor: '#fef2f2', borderRadius: 8, borderWidth: 1, borderColor: '#fecaca' }}>
          <Text style={{ fontSize: 12, color: '#dc2626' }}>审核未通过，请重新提交人脸采样素材。</Text>
        </View>
      )}
    </View>
  )
}
