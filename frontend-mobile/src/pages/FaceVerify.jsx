// FaceVerify — 人脸识别模式独立页（#1807 → #1811 重构）
// 编辑资料页身份证照未验证时，黄色警告 + 链接进入本页。
// weapp：全屏 Camera 前摄预览 + 快门（拍照+自动录像+眨眼引导）→ 自动上传 → 自动返回。
// H5：保留卡片式 FaceCaptureUploader（下期统一）。
import { useState, useEffect, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Camera, Image } from '@tarojs/components'
import { useNavigate } from 'react-router-dom'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { env, dialog, uploadFile, storage, getCameraContext } from '../platform'
import { session } from '../platform'
import FaceCaptureUploader from '../components/FaceCaptureUploader'

const ACTION_PROMPTS = ['请眨眨眼', '请左右转头', '请张嘴', '请微笑']

export default function FaceVerify() {
  const [status, setStatus] = useState('')
  const [hasIdPhoto, setHasIdPhoto] = useState(false)
  const [loading, setLoading] = useState(true)
  const [cameraErr, setCameraErr] = useState('')
  // Phase: idle → photo_done → recording → blink → uploading → fail(保留素材可重试)
  const [phase, setPhase] = useState('idle')
  const [countdown, setCountdown] = useState(0)
  const [blinkVisible, setBlinkVisible] = useState(false)
  const [uploadError, setUploadError] = useState('')
  const [actionPrompt, setActionPrompt] = useState('')
  const photoPathRef = useRef('')
  const videoPathRef = useRef('')
  const countdownRef = useRef(null)
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

  // Cleanup countdown on unmount
  useEffect(() => {
    return () => { if (countdownRef.current) clearInterval(countdownRef.current) }
  }, [])

  const goBack = () => {
    if (!env.isMiniProgram) return navigate('/profile/edit')
    Taro.navigateBack()
  }

  const getToken = () => storage.getItem('token') || session.getItem('token')

  // Camera error handler
  const handleCameraError = (e) => {
    setCameraErr(e.detail?.errMsg || '摄像头授权失败，请在小程序设置中允许使用摄像头')
  }

  // Upload photo + optional video → POST /user/face-capture (weapp 分离上传)
  const doUpload = async (imagePath, videoPath) => {
    setUploadError('')
    try {
      const base = baseUrl || '/api'
      const headers = { Authorization: 'Bearer ' + getToken() }
      // Upload image first (creates batch)
      const imgResp = await uploadFile(`${base}/user/face-capture`, imagePath, {
        name: 'image',
        headers,
      })
      if (!imgResp.ok) throw new Error('照片上传失败')
      const imgJson = JSON.parse(imgResp.data)
      if (imgJson.code !== 20000) throw new Error(resolveErrorMessage(imgJson, '提交失败'))
      const batchId = imgJson.data?.batch_id
      // Upload video (optional, appended to same batch)
      if (videoPath && batchId) {
        const vidResp = await uploadFile(`${base}/user/face-capture`, videoPath, {
          name: 'video',
          formData: { batch_id: batchId },
          headers,
        })
        if (!vidResp.ok) throw new Error('视频上传失败')
        const vidJson = JSON.parse(vidResp.data)
        if (vidJson.code !== 20000) throw new Error(resolveErrorMessage(vidJson, '视频上传失败'))
      }
      setStatus('pending_review')
      dialog.toast('提交成功，等待审核')
      setTimeout(() => {
        if (env.isMiniProgram) { Taro.navigateBack() } else { navigate('/profile/edit') }
      }, 800)
    } catch (err) {
      // P5: 上传失败进入 fail 态并保留已录素材（photo/video temp paths），
      // 供「重试上传」直接复用——不丢失已录素材状态。
      setUploadError(err.message || '上传失败，请重试')
      setPhase('fail')
    }
  }

  // P5: fail 态「重试上传」——复用保留的 photo/video ref 重新上传（不重拍）。
  const handleRetryUpload = () => {
    if (!photoPathRef.current) {
      setUploadError('未找到已拍摄素材，请重新拍摄')
      return
    }
    setPhase('Uploading')
    doUpload(photoPathRef.current, videoPathRef.current)
  }

  // P5: fail 态「重新拍摄」——清空保留素材，回到 idle 可重新采集。
  const handleRetake = () => {
    photoPathRef.current = ''
    videoPathRef.current = ''
    setUploadError('')
    setActionPrompt('')
    setPhase('idle')
  }

  const handleStopRecord = () => {
    if (countdownRef.current) {
      clearInterval(countdownRef.current)
      countdownRef.current = null
    }
    setBlinkVisible(false)
    setPhase('Uploading')
    const cam = getCameraContext()
    cam.stopRecord({
      success: (res) => {
        const vp = res?.tempFilePath || ''
        videoPathRef.current = vp
        doUpload(photoPathRef.current, vp)
      },
      fail: () => {
        // 录像失败 → 仅上传照片（视频素材不可用）
        videoPathRef.current = ''
        doUpload(photoPathRef.current, '')
      },
    })
  }

  // Shutter button: idle → take photo → photo_done; Recording/Blink → stop early
  const handleShutter = () => {
    if (phase === 'idle') {
      const cam = getCameraContext()
      cam.takePhoto({
        quality: 'high',
        success: (res) => {
          const photoPath = res?.tempImagePath
          if (!photoPath) return
          photoPathRef.current = photoPath
          setPhase('photo_done')
        },
      })
    } else if (phase === 'Recording' || phase === 'Blink') {
      handleStopRecord()
    }
  }

  // Start 5s recording with countdown + blink/action prompt in last 2s.
  // Extracted per plan 3.4 for readability and future testability.
  const startRecording = () => {
    const cam = getCameraContext()
    cam.startRecord({
      success: () => {
        setPhase('Recording')
        setCountdown(5)
        let remaining = 5
        countdownRef.current = setInterval(() => {
          remaining -= 1
          setCountdown(remaining)
          if (remaining <= 2) setBlinkVisible(true)
          if (remaining <= 0) {
            clearInterval(countdownRef.current)
            countdownRef.current = null
            handleStopRecord()
          }
        }, 1000)
      },
    })
  }

  // Continue from photo_done → recording: pick random action + start record
  const handleContinueRecord = () => {
    setActionPrompt(ACTION_PROMPTS[Math.floor(Math.random() * ACTION_PROMPTS.length)])
    startRecording()
  }

  // ---- Status-based message bar (shown in both weapp and H5) ----
  const renderStatusBar = () => {
    if (loading) return null
    if (status === 'verified') {
      return (
        <View style={{ padding: 12, backgroundColor: '#f0fdf4', borderRadius: 8, borderWidth: 1, borderColor: '#bbf7d0', marginBottom: 12 }}>
          <Text style={{ fontSize: 13, color: '#16a34a', fontWeight: '600' }}>✅ 已实名认证</Text>
        </View>
      )
    }
    if (!hasIdPhoto) {
      return (
        <View style={{ padding: 12, backgroundColor: '#fefce8', borderRadius: 8, borderWidth: 1, borderColor: '#fde68a', marginBottom: 12 }}>
          <Text style={{ fontSize: 13, color: '#b45309', fontWeight: '600' }}>⚠️ 请先上传身份证照片</Text>
          <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>人脸识别前需先上传身份证件照。</Text>
          <View onClick={goBack} style={{ marginTop: 8 }}>
            <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600', textDecorationLine: 'underline' }}>去上传身份证 ›</Text>
          </View>
        </View>
      )
    }
    if (status === 'pending_review') {
      return (
        <View style={{ padding: 12, backgroundColor: '#fefce8', borderRadius: 8, borderWidth: 1, borderColor: '#fde68a', marginBottom: 12 }}>
          <Text style={{ fontSize: 13, color: '#d97706', fontWeight: '600' }}>⚠️ 实名认证审核中</Text>
          <Text style={{ fontSize: 12, color: '#b45309', marginTop: 4 }}>已提交人脸采样，平台员工审核通过后即完成实名认证（预计 1-2 个工作日）。</Text>
        </View>
      )
    }
    if (status === 'rejected') {
      return (
        <View style={{ padding: 12, backgroundColor: '#fef2f2', borderRadius: 8, borderWidth: 1, borderColor: '#fecaca', marginBottom: 12 }}>
          <Text style={{ fontSize: 12, color: '#dc2626', fontWeight: '600' }}>审核未通过，请重新发起人脸识别。</Text>
        </View>
      )
    }
    return null
  }

  // ---- weapp: full-screen camera mode ----
  if (env.isMiniProgram) {
    // Status-only modes: don't show camera
    if (loading || status === 'verified' || !hasIdPhoto || status === 'pending_review') {
      return (
        <View style={{ minHeight: '100vh', backgroundColor: '#f4f4f5' }}>
          <View style={{ margin: 16, backgroundColor: '#fff', borderRadius: 12, padding: 16 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#111', marginBottom: 8 }}>实名认证</Text>
            {renderStatusBar()}
          </View>
        </View>
      )
    }

    // Camera mode: idle / photo_done / recording / blink / uploading
    const shutterLabel = phase === 'Uploading' ? '处理中...'
      : phase === 'Recording' || phase === 'Blink' ? '停止'
      : '拍照'

    return (
      <View style={{ position: 'relative', width: '100vw', height: '100vh', backgroundColor: '#000' }}>
        {/* Full-screen front camera — getCameraContext() targets first <Camera> on page */}
        <Camera
          devicePosition="front"
          style={{ width: '100%', height: '100%' }}
          onError={handleCameraError}
        />

        {/* Camera error overlay */}
        {cameraErr ? (
          <View style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.85)' }}>
            <Text style={{ fontSize: 16, color: '#fff', marginBottom: 12 }}>⚠️ {cameraErr}</Text>
            <View onClick={goBack} style={{ padding: '8px 24px', backgroundColor: '#915F38', borderRadius: 20 }}>
              <Text style={{ fontSize: 14, color: '#fff' }}>返回</Text>
            </View>
          </View>
        ) : (
          <>
            {/* Status bar overlay (top) */}
            <View style={{ position: 'absolute', top: 48, left: 16, right: 16, zIndex: 10 }}>
              {renderStatusBar()}
            </View>

            {/* Blink prompt (center-top, visible during last 2s) */}
            {blinkVisible && phase !== 'Uploading' && (
              <View style={{ position: 'absolute', top: '35%', left: 0, right: 0, display: 'flex', justifyContent: 'center', zIndex: 10 }}>
                <View style={{ backgroundColor: 'rgba(0,0,0,0.6)', borderRadius: 8, padding: '8px 20px' }}>
                  <Text style={{ fontSize: 18, color: '#fff', fontWeight: '700' }}>{actionPrompt || '请眨眨眼'}</Text>
                </View>
              </View>
            )}

            {/* Upload error */}
            {uploadError ? (
              <View style={{ position: 'absolute', bottom: 140, left: 16, right: 16, zIndex: 10 }}>
                <View style={{ backgroundColor: 'rgba(220,38,38,0.9)', borderRadius: 8, padding: 10 }}>
                  <Text style={{ fontSize: 13, color: '#fff', textAlign: 'center' }}>{uploadError}</Text>
                </View>
              </View>
            ) : null}

            {/* Countdown (when recording) */}
            {(phase === 'Recording' || phase === 'Blink') && countdown > 0 ? (
              <View style={{ position: 'absolute', bottom: 130, left: 0, right: 0, display: 'flex', justifyContent: 'center', zIndex: 10 }}>
                <Text style={{ fontSize: 14, color: 'rgba(255,255,255,0.8)' }}>{countdown}s</Text>
              </View>
            ) : null}

            {/* Photo confirmation transition panel (photo_done phase) */}
            {phase === 'photo_done' && (
              <View style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.85)', zIndex: 20 }}>
                <Image
                  src={photoPathRef.current}
                  style={{ width: 200, height: 266, borderRadius: 12, marginBottom: 24 }}
                  mode="aspectFill"
                />
                <Text style={{ fontSize: 16, color: '#fff', fontWeight: '600', marginBottom: 8 }}>图像采集完成！</Text>
                <Text style={{ fontSize: 13, color: 'rgba(255,255,255,0.8)', marginBottom: 40, paddingHorizontal: 32, textAlign: 'center', lineHeight: '20px' }}>下面还需要采集一段视频，录制过程中会提示您完成一个动作，请配合。</Text>
                <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}>
                  <View
                    onClick={handleRetake}
                    style={{
                      paddingLeft: 22, paddingRight: 22, height: 44,
                      borderRadius: 22, backgroundColor: 'rgba(255,255,255,0.25)',
                      border: '1px solid rgba(255,255,255,0.6)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                      marginRight: 16,
                    }}>
                    <Text style={{ fontSize: 14, color: '#fff', fontWeight: '600' }}>重新拍摄</Text>
                  </View>
                  <View
                    onClick={handleContinueRecord}
                    style={{
                      paddingLeft: 22, paddingRight: 22, height: 44,
                      borderRadius: 22, backgroundColor: '#915F38',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                    }}>
                    <Text style={{ fontSize: 14, color: '#fff', fontWeight: '600' }}>继续录视频</Text>
                  </View>
                </View>
              </View>
            )}

            {/* Bottom controls: fail → 重试上传/重新拍摄; photo_done → hidden (panel above); else → shutter */}
            {phase !== 'photo_done' && (
              <View style={{ position: 'absolute', bottom: 60, left: 0, right: 0, display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 10 }}>
                {phase === 'fail' ? (
                  <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}>
                    {/* 重新拍摄（左，次要） */}
                    <View
                      onClick={handleRetake}
                      style={{
                        paddingLeft: 22, paddingRight: 22, height: 44,
                        borderRadius: 22, backgroundColor: 'rgba(255,255,255,0.25)',
                        border: '1px solid rgba(255,255,255,0.6)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        marginRight: 16,
                      }}>
                      <Text style={{ fontSize: 14, color: '#fff', fontWeight: '600' }}>重新拍摄</Text>
                    </View>
                    {/* 重试上传（右，主要） */}
                    <View
                      onClick={handleRetryUpload}
                      style={{
                        paddingLeft: 22, paddingRight: 22, height: 44,
                        borderRadius: 22, backgroundColor: '#915F38',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                      }}>
                      <Text style={{ fontSize: 14, color: '#fff', fontWeight: '600' }}>重试上传</Text>
                    </View>
                  </View>
                ) : (
                  <View
                    onClick={phase === 'Uploading' ? undefined : handleShutter}
                    style={{
                      width: 68, height: 68, borderRadius: 34,
                      border: '4px solid #fff',
                      backgroundColor: phase === 'Recording' || phase === 'Blink' ? '#dc2626' : 'rgba(255,255,255,0.3)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                    }}>
                    <Text style={{ fontSize: 14, color: '#fff', fontWeight: '600' }}>{shutterLabel}</Text>
                  </View>
                )}
              </View>
            )}
          </>
        )}
      </View>
    )
  }

  // ---- H5: card-based with FaceCaptureUploader (kept from #1807, #1811 C requirement) ----
  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#f4f4f5' }}>
      {/* H5 手写标题条（weapp 用原生导航栏，见 #1511 规则） */}
      <View style={{ padding: 12, paddingLeft: 16, backgroundColor: '#fff', display: 'flex', alignItems: 'center' }}>
        <View onClick={goBack} style={{ marginRight: 8, padding: 4 }}>
          <Text style={{ fontSize: 20, color: '#6b7280' }}>‹</Text>
        </View>
        <Text style={{ fontSize: 16, fontWeight: '700', color: '#111' }}>人脸识别</Text>
      </View>

      <View style={{ margin: 16, backgroundColor: '#fff', borderRadius: 12, padding: 16 }}>
        <Text style={{ fontSize: 14, fontWeight: '700', color: '#111', marginBottom: 8 }}>实名认证</Text>
        {renderStatusBar()}
        {!loading && status !== 'verified' && hasIdPhoto && status !== 'pending_review' && (
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
                setTimeout(() => { navigate('/profile/edit') }, 800)
              }}
            />
          </View>
        )}
      </View>
    </View>
  )
}
