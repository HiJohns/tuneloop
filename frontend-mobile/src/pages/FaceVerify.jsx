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

  // #1823: 桌面版微信（Mac/Windows）的 <camera> 仅支持拍照，录像不可用——
  // 之前在此环境下静默走了「录像失败→仅提交照片」，用户以为已上传视频。
  const isDesktopWeapp = (() => {
    if (!env.isMiniProgram) return false
    try {
      const platform = Taro.getSystemInfoSync().platform
      return platform === 'mac' || platform === 'windows'
    } catch {
      return false
    }
  })()

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

  // #1822: 上传超时兜底——弱网（如海外直连国内）下 Taro.uploadFile 无超时
  // 参数，失败前会无限"上传中"。60s 未完成即抛错进入 fail 态（可重试/重拍）。
  const UPLOAD_TIMEOUT_MS = 60000
  const withUploadTimeout = (promise) =>
    Promise.race([
      promise,
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error('上传超时，请检查网络后重试')), UPLOAD_TIMEOUT_MS)
      ),
    ])

  // Upload photo + optional video → POST /user/face-capture (weapp 分离上传)
  const doUpload = async (imagePath, videoPath) => {
    setUploadError('')
    try {
      const base = baseUrl || '/api'
      const headers = { Authorization: 'Bearer ' + getToken() }
      // Upload image first (creates batch)
      const imgResp = await withUploadTimeout(uploadFile(`${base}/user/face-capture`, imagePath, {
        name: 'image',
        headers,
      }))
      if (!imgResp.ok) throw new Error('照片上传失败')
      const imgJson = JSON.parse(imgResp.data)
      if (imgJson.code !== 20000) throw new Error(resolveErrorMessage(imgJson, '提交失败'))
      const batchId = imgJson.data?.batch_id
      // Upload video (optional, appended to same batch)
      if (videoPath && batchId) {
        const vidResp = await withUploadTimeout(uploadFile(`${base}/user/face-capture`, videoPath, {
          name: 'video',
          formData: { batch_id: batchId },
          headers,
        }))
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
        const vp = res?.tempVideoPath || ''
        videoPathRef.current = vp
        doUpload(photoPathRef.current, vp)
      },
      fail: () => {
        // #1823: 录像拿不到素材（桌面版微信等）——不再静默降级为仅照片上传。
        videoPathRef.current = ''
        setPhase('idle')
        Taro.showModal({
          title: '未获取到视频',
          content: '当前设备未能录制视频（电脑版微信通常不支持录像）。实名认证需要动态视频，请改用手机微信完成；如需继续请仅提交照片（不推荐）。',
          confirmText: '仅提交照片',
          cancelText: '知道了',
          success: (r) => {
            if (r.confirm) doUpload(photoPathRef.current, '')
          },
        })
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
      fail: (err) => {
        // #1823: 开始录像失败要给反馈（桌面微信常不支持），不再静默卡住。
        Taro.showModal({
          title: '无法开始录像',
          content: '当前设备不支持视频录制，实名认证需要动态视频。请改用手机微信操作。',
          showCancel: false,
        })
        setPhase('photo_done')
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
    // #1822: 采集窗压缩至约 1/4 屏 + resolution=low → 录制视频为低分辨率，
    // 上传更快、服务器占用更小（全屏预览不改变录制分辨率，真正压缩靠 low）。
    const shutterLabel = phase === 'Uploading' ? '处理中...'
      : phase === 'Recording' || phase === 'Blink' ? '停止'
      : '拍照'

    // #1823: 桌面微信录像不可用 → 直接引导用手机，不进相机流程。
    if (isDesktopWeapp) {
      return (
        <View style={{ minHeight: '100vh', backgroundColor: '#f4f4f5', display: 'flex', flexDirection: 'column', alignItems: 'center', paddingTop: 60, paddingLeft: 32, paddingRight: 32 }}>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#111', marginBottom: 12 }}>请使用手机微信</Text>
          <Text style={{ fontSize: 13, color: '#52525b', textAlign: 'center', lineHeight: '20px', marginBottom: 28 }}>
            实名认证需要录制动态视频（活体检测），电脑版微信不支持视频录制。请使用手机微信打开本小程序完成实名认证。
          </Text>
          <View onClick={goBack} style={{ paddingLeft: 32, paddingRight: 32, height: 44, borderRadius: 22, backgroundColor: '#915F38', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Text style={{ fontSize: 14, color: '#fff' }}>返回</Text>
          </View>
        </View>
      )
    }

    return (
      <View style={{ position: 'relative', width: '100vw', minHeight: '100vh', backgroundColor: '#0b0b0f', display: 'flex', flexDirection: 'column', alignItems: 'center', paddingTop: 24, paddingBottom: 48, boxSizing: 'border-box' }}>
        {cameraErr ? (
          <View style={{ width: '100%', height: '100vh', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
            <Text style={{ fontSize: 15, color: '#fff', marginBottom: 16, textAlign: 'center', paddingHorizontal: 32 }}>⚠️ {cameraErr}</Text>
            <View onClick={goBack} style={{ paddingLeft: 24, paddingRight: 24, height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Text style={{ fontSize: 14, color: '#fff' }}>返回</Text>
            </View>
          </View>
        ) : (
          <>
            {/* Status bar (top) */}
            <View style={{ width: '100%', paddingLeft: 16, paddingRight: 16, zIndex: 10 }}>{renderStatusBar()}</View>

            {/* Compact camera stage (~1/4 screen area) */}
            <View style={{ marginTop: 20, width: 232, height: 320, borderRadius: 20, overflow: 'hidden', position: 'relative', backgroundColor: '#000' }}>
              <Camera
                devicePosition="front"
                resolution="low"
                style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%' }}
                onError={handleCameraError}
              />
              {/* Blink prompt (inside stage, visible during last 2s) */}
              {blinkVisible && phase !== 'Uploading' && (
                <View style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 10 }}>
                  <View style={{ backgroundColor: 'rgba(0,0,0,0.6)', borderRadius: 8, paddingLeft: 20, paddingRight: 20, paddingTop: 8, paddingBottom: 8 }}>
                    <Text style={{ fontSize: 18, color: '#fff', fontWeight: '700' }}>{actionPrompt || '请眨眨眼'}</Text>
                  </View>
                </View>
              )}
              {/* Countdown (inside stage bottom) */}
              {(phase === 'Recording' || phase === 'Blink') && countdown > 0 ? (
                <View style={{ position: 'absolute', bottom: 12, left: 0, right: 0, display: 'flex', justifyContent: 'center', zIndex: 10 }}>
                  <Text style={{ fontSize: 15, color: 'rgba(255,255,255,0.9)' }}>{countdown}s</Text>
                </View>
              ) : null}
              {/* 录像中红点指示 */}
              {(phase === 'Recording' || phase === 'Blink') && (
                <View style={{ position: 'absolute', top: 10, left: 12, display: 'flex', flexDirection: 'row', alignItems: 'center', zIndex: 10 }}>
                  <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: '#dc2626', marginRight: 5 }} />
                  <Text style={{ fontSize: 11, color: '#fff' }}>录制中</Text>
                </View>
              )}
            </View>

            {/* Instruction / upload error text under the stage */}
            <View style={{ marginTop: 14, paddingLeft: 28, paddingRight: 28, width: '100%', display: 'flex', alignItems: 'center' }}>
              {uploadError ? (
                <Text style={{ fontSize: 13, color: '#fca5a5', textAlign: 'center', lineHeight: '18px' }}>{uploadError}</Text>
              ) : phase === 'Uploading' ? (
                <Text style={{ fontSize: 13, color: 'rgba(255,255,255,0.7)', textAlign: 'center' }}>正在上传，请勿离开页面…</Text>
              ) : (
                <Text style={{ fontSize: 12, color: 'rgba(255,255,255,0.6)', textAlign: 'center', lineHeight: '18px' }}>
                  {phase === 'Recording' || phase === 'Blink'
                    ? '请正对屏幕，保持面部在框内，完成提示动作（≤5 秒）'
                    : phase === 'photo_done'
                      ? '照片已采集，即将录制动态视频'
                      : '请正对屏幕并保持光线充足，点击下方快门拍照；随后录制 ≤5 秒视频完成动作验证。视频为压缩采集，上传更快'}
                </Text>
              )}
            </View>

            {/* Photo confirmation transition panel (photo_done phase) */}
            {phase === 'photo_done' && (
              <View style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.88)', zIndex: 20 }}>
                <Image
                  src={photoPathRef.current}
                  style={{ width: 180, height: 240, borderRadius: 12, marginBottom: 24 }}
                  mode="aspectFill"
                />
                <Text style={{ fontSize: 16, color: '#fff', fontWeight: '600', marginBottom: 8 }}>图像采集完成！</Text>
                <Text style={{ fontSize: 13, color: 'rgba(255,255,255,0.8)', marginBottom: 36, paddingHorizontal: 40, textAlign: 'center', lineHeight: '20px' }}>下面还需要采集一段视频，录制过程中会提示您完成一个动作，请配合。</Text>
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
              <View style={{ marginTop: 28, display: 'flex', justifyContent: 'center', alignItems: 'center', zIndex: 10 }}>
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
                      width: 64, height: 64, borderRadius: 32,
                      border: '3px solid #fff',
                      backgroundColor: phase === 'Recording' || phase === 'Blink' ? '#dc2626' : 'rgba(255,255,255,0.25)',
                      display: 'flex', alignItems: 'center', justifyContent: 'center',
                    }}>
                    <Text style={{ fontSize: 13, color: '#fff', fontWeight: '600' }}>{shutterLabel}</Text>
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
