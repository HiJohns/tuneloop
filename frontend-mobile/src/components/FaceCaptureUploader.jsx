// FaceCaptureUploader — 实名核身人脸采样组件（H5 + weapp 共用，#1792 T4/#1807）
// 自拍照片（必选）+ 动态视频（可选，活体佐证）→ POST /user/face-capture
// UI 与 IdPhotoUploader 一致：虚线小窗口 → 点击采样 → 返回显示缩略图。
// Props:
//   initialStatus: id_verify_status（none/uploaded/pending_review/verified/rejected）
//   onSubmitSuccess(): 提交成功回调（刷新状态）
import { useState, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image } from '@tarojs/components'
import { uploadFile, env } from '../platform'
import { resolveErrorMessage } from '../services/api'
import { storage, session } from '../platform'

const FaceCaptureUploader = ({ initialStatus = '', onSubmitSuccess }) => {
  const [imagePath, setImagePath] = useState('')
  const [imagePreview, setImagePreview] = useState('')
  const [videoPath, setVideoPath] = useState('')
  const [videoPreview, setVideoPreview] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const imageInputRef = useRef(null)
  const videoInputRef = useRef(null)

  const getToken = () => storage.getItem('token') || session.getItem('token')

  const pickImage = () => {
    if (env.isMiniProgram) {
      Taro.chooseMedia({ count: 1, mediaType: ['image'], sourceType: ['album', 'camera'] })
        .then(res => {
          const f = res.tempFiles?.[0]
          if (f) { setImagePath(f.tempFilePath); setImagePreview(f.tempFilePath) }
        })
        .catch(() => {})
    } else {
      imageInputRef.current?.click()
    }
  }

  const pickVideo = () => {
    if (env.isMiniProgram) {
      Taro.chooseMedia({ count: 1, mediaType: ['video'], sourceType: ['album', 'camera'] })
        .then(res => {
          const f = res.tempFiles?.[0]
          if (f) {
            setVideoPath(f.tempFilePath)
            // 视频用 thumbTempFilePath 缩略图（chooseMedia 返回首帧）
            setVideoPreview(f.thumbTempFilePath || f.tempFilePath)
          }
        })
        .catch(() => {})
    } else {
      videoInputRef.current?.click()
    }
  }

  const handleH5Image = (e) => {
    const file = e.target.files?.[0]
    if (file) { setImagePath(file); setImagePreview(URL.createObjectURL(file)) }
    e.target.value = ''
  }

  const handleH5Video = (e) => {
    const file = e.target.files?.[0]
    if (file) { setVideoPath(file); setVideoPreview(URL.createObjectURL(file)) }
    e.target.value = ''
  }

  const submit = async () => {
    if (!imagePath) {
      setError('请先选择自拍照片')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const base = env.apiBaseUrl || '/api'
      const headers = { Authorization: 'Bearer ' + getToken() }
      if (env.isMiniProgram) {
        // #1792 修复: weapp 分离上传（Taro.uploadFile 一次仅一个文件）——
        // 先传 image（创建批次拿 batch_id），再传 video（带 batch_id 追加）。
        const resp = await uploadFile(`${base}/user/face-capture`, imagePath, {
          name: 'image',
          headers,
        })
        if (!resp.ok) throw new Error('upload failed')
        const json = JSON.parse(resp.data)
        if (json.code !== 20000) throw new Error(resolveErrorMessage(json, '提交失败'))
        const batchId = json.data?.batch_id
        // 可选视频：追加到同一批次（不再静默丢弃）。
        if (videoPath && batchId) {
          const videoResp = await uploadFile(`${base}/user/face-capture`, videoPath, {
            name: 'video',
            formData: { batch_id: batchId },
            headers,
          })
          if (!videoResp.ok) throw new Error('video upload failed')
          const videoJson = JSON.parse(videoResp.data)
          if (videoJson.code !== 20000) throw new Error(resolveErrorMessage(videoJson, '视频上传失败'))
        }
      } else {
        // H5：FormData 一次带 image + 可选 video。
        const formData = new FormData()
        formData.append('image', imagePath)
        if (videoPath) formData.append('video', videoPath)
        const fetchResp = await fetch(`${base}/user/face-capture`, {
          method: 'POST',
          headers,
          body: formData,
        })
        const json = await fetchResp.json()
        if (json.code !== 20000) throw new Error(resolveErrorMessage(json, '提交失败'))
      }
      setImagePath('')
      setVideoPath('')
      if (onSubmitSuccess) onSubmitSuccess()
    } catch (err) {
      setError(err.message || '提交失败，请重试')
    } finally {
      setSubmitting(false)
    }
  }

  const statusText = {
    none: '未上传身份证',
    uploaded: '已上传，待自拍',
    pending_review: '审核中',
    verified: '已认证',
    rejected: '审核未通过',
  }

  return (
    <View style={{ marginTop: 8 }}>
      {/* 人脸采样标题（#1807：必要步骤——未实装腾讯云时也须完成采样等待人工审核） */}
      <Text style={{ fontSize: 13, fontWeight: '700', color: '#6b7280', marginBottom: 6 }}>人脸采样（必填）</Text>
      {/* 状态展示 */}
      {initialStatus && initialStatus !== 'none' && (
        <View style={{ padding: 8, backgroundColor: initialStatus === 'rejected' ? '#fef2f2' : '#f4f4f5', borderRadius: 8, marginBottom: 8 }}>
          <Text style={{ fontSize: 12, color: initialStatus === 'rejected' ? '#dc2626' : '#6b7280' }}>
            {statusText[initialStatus] || initialStatus}
          </Text>
        </View>
      )}

      {/* 人脸采样：虚线小窗口 → 点击采样 → 缩略图（与身份证照上传一致） */}
      <View style={{ display: 'flex', flexDirection: 'row', marginBottom: 8 }}>
        {/* 自拍照片（必选） */}
        <View
          onClick={submitting ? undefined : pickImage}
          style={{ width: 128, height: 80, border: '2px dashed #d4d4d8', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 12, overflow: 'hidden', backgroundColor: '#fafafa', flexShrink: 0 }}>
          {imagePreview ? (
            <Image src={imagePreview} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
          ) : (
            <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
              <Text style={{ fontSize: 22 }}>📷</Text>
              <Text style={{ fontSize: 11, color: '#9ca3af', marginTop: 2 }}>自拍照片（必选）</Text>
            </View>
          )}
        </View>
        {/* 动态视频（可选，活体佐证） */}
        <View
          onClick={submitting ? undefined : pickVideo}
          style={{ width: 128, height: 80, border: '2px dashed #d4d4d8', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden', backgroundColor: '#fafafa', flexShrink: 0 }}>
          {videoPreview ? (
            <Image src={videoPreview} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
          ) : (
            <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
              <Text style={{ fontSize: 22 }}>🎥</Text>
              <Text style={{ fontSize: 11, color: '#9ca3af', marginTop: 2 }}>动态视频（可选）</Text>
            </View>
          )}
        </View>
      </View>
      {/* H5 文件输入（weapp 分支不渲染——无裸 HTML 交互控件问题） */}
      {!env.isMiniProgram && (
        <>
          <input ref={imageInputRef} type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }} onChange={handleH5Image} />
          <input ref={videoInputRef} type="file" accept="video/mp4,video/quicktime,video/webm" style={{ display: 'none' }} onChange={handleH5Video} />
        </>
      )}

      <View
        onClick={submitting ? undefined : submit}
        style={{ width: '100%', paddingTop: 13, paddingBottom: 13, backgroundColor: submitting || !imagePath ? '#d4d4d8' : '#915F38', borderRadius: 20, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ color: '#fff', fontSize: 13, fontWeight: '600' }}>{submitting ? '处理中...' : '发起人脸识别'}</Text>
      </View>
      {error && <Text style={{ fontSize: 12, color: '#dc2626', marginTop: 6 }}>{error}</Text>}
      <Text style={{ fontSize: 11, color: '#9ca3af', marginTop: 6 }}>
        {initialStatus === 'pending_review' ? '已提交，等待平台审核' : '人脸采样为实名认证必要步骤：提交自拍照片（+动态视频）后由平台员工审核，通过后完成实名认证'}
      </Text>
    </View>
  )
}

export default FaceCaptureUploader
