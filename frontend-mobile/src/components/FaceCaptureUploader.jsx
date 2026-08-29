// FaceCaptureUploader — 实名核身自拍采集组件（H5 + weapp 共用，#1792 T4）
// 图（必选）+ 可选视频 → POST /user/face-capture
// Props:
//   initialStatus: id_verify_status（none/uploaded/pending_review/verified/rejected）
//   onSubmitSuccess(): 提交成功回调（刷新状态）
import { useState, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { uploadFile, env } from '../platform'
import { resolveErrorMessage } from '../services/api'
import { storage, session } from '../platform'

const FaceCaptureUploader = ({ initialStatus = '', onSubmitSuccess }) => {
  const [imagePath, setImagePath] = useState('')
  const [videoPath, setVideoPath] = useState('')
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
          if (f) setImagePath(f.tempFilePath)
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
          if (f) setVideoPath(f.tempFilePath)
        })
        .catch(() => {})
    } else {
      videoInputRef.current?.click()
    }
  }

  const handleH5Image = (e) => {
    const file = e.target.files?.[0]
    if (file) setImagePath(file)
    e.target.value = ''
  }

  const handleH5Video = (e) => {
    const file = e.target.files?.[0]
    if (file) setVideoPath(file)
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
      let resp
      if (env.isMiniProgram) {
        // weapp：uploadFile 一次传一个文件——图必选、视频可选（两个文件分两次传）。
        // 简化：weapp 端只传图片（视频可选降级，后端 image 必选）。
        resp = await uploadFile(`${base}/user/face-capture`, imagePath, {
          name: 'image',
          headers,
        })
        if (!resp.ok) throw new Error('upload failed')
        const json = JSON.parse(resp.data)
        if (json.code !== 20000) throw new Error(resolveErrorMessage(json, '提交失败'))
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
      {/* 状态展示 */}
      {initialStatus && initialStatus !== 'none' && (
        <View style={{ padding: 8, backgroundColor: initialStatus === 'rejected' ? '#fef2f2' : '#f4f4f5', borderRadius: 8, marginBottom: 8 }}>
          <Text style={{ fontSize: 12, color: initialStatus === 'rejected' ? '#dc2626' : '#6b7280' }}>
            {statusText[initialStatus] || initialStatus}
          </Text>
        </View>
      )}

      {/* 自拍选择 */}
      <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', marginBottom: 8 }}>
        <View
          onClick={submitting ? undefined : pickImage}
          style={{ flex: 1, height: 36, border: '1px solid #d4d4d8', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 8, boxSizing: 'border-box' }}>
          <Text style={{ fontSize: 12, color: imagePath ? '#16a34a' : '#6b7280' }}>{imagePath ? '已选照片' : '选择自拍照片'}</Text>
        </View>
        <View
          onClick={submitting ? undefined : pickVideo}
          style={{ flex: 1, height: 36, border: '1px solid #d4d4d8', borderRadius: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', boxSizing: 'border-box' }}>
          <Text style={{ fontSize: 12, color: videoPath ? '#16a34a' : '#9ca3af' }}>{videoPath ? '已选视频' : '选择视频（可选）'}</Text>
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
        style={{ width: '100%', height: 40, backgroundColor: submitting || !imagePath ? '#d4d4d8' : '#915F38', borderRadius: 20, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ color: '#fff', fontSize: 13, fontWeight: '600' }}>{submitting ? '提交中...' : '提交核身素材'}</Text>
      </View>
      {error && <Text style={{ fontSize: 12, color: '#dc2626', marginTop: 6 }}>{error}</Text>}
      <Text style={{ fontSize: 11, color: '#9ca3af', marginTop: 6 }}>
        {initialStatus === 'pending_review' ? '已提交，等待平台审核' : '提交后将进行核身比对；腾讯云未配置时将转为人工审核'}
      </Text>
    </View>
  )
}

export default FaceCaptureUploader
