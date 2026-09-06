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
  const lastBatchIdRef = useRef('')
  const countdownRef = useRef(null)
  // 上传管理（#1821）：task 句柄 / 停滞看门狗 / 部件标识 / 处理标记
  const uploadTaskRef = useRef(null)
  const stallTimerRef = useRef(null)
  const lastProgressAtRef = useRef(0)
  const uploadPartRef = useRef('')
  const stallHandledRef = useRef(false)
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl

  // #1821: 桌面版微信（Mac/Windows）的 <camera> 仅支持拍照，录像不可用——
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
    return () => {
      if (countdownRef.current) clearInterval(countdownRef.current)
      if (stallTimerRef.current) clearInterval(stallTimerRef.current)
    }
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

  const [uploadProgress, setUploadProgress] = useState(null)

  // #1821: 上传管理三件套——
  // 1) 总超时：照片 90s / 视频 300s
  // 2) 停滞看门狗：黑屏/后台/断流时微信不再派发进度事件且定时器被挂起；
  //    90s 无任何进度 → abort 任务 + 弹窗，绝不留下无限"正在上传"
  // 3) 进度百分比：显示真实上传进度
  //
  // 语义约定（audit #1821 Bug1/2/3）：
  // - stallHandledRef 只表示「停滞看门狗已弹窗接管 UI」——由 handleUploadStall
  //   置 true，startUploadPart 重置；wrapUploadTimeout 硬超时不得置它，
  //   否则超时会被上层 catch 当"已停滞处理"吞掉 → 图片无限上传中 / 视频假成功
  const STALL_MS = 90000

  const clearStallWatchdog = () => {
    if (stallTimerRef.current) {
      clearInterval(stallTimerRef.current)
      stallTimerRef.current = null
    }
  }

  // abort 当前底层上传任务（Taro.uploadFile 的 UploadTask），
  // 硬超时与停滞看门狗共用（audit Bug1/2：超时也必须中止传输，防半包批次）。
  const abortActiveUpload = () => {
    try {
      if (uploadTaskRef.current && typeof uploadTaskRef.current.abort === 'function') {
        uploadTaskRef.current.abort()
      }
    } catch {}
    uploadTaskRef.current = null
  }

  const startUploadPart = (name) => {
    uploadPartRef.current = name
    lastProgressAtRef.current = Date.now()
    stallHandledRef.current = false
    setUploadProgress(null)
    clearStallWatchdog()
    stallTimerRef.current = setInterval(() => {
      if (Date.now() - lastProgressAtRef.current > STALL_MS) {
        handleUploadStall()
      }
    }, 5000)
  }

  const onUploadProgress = (p) => {
    lastProgressAtRef.current = Date.now()
    setUploadProgress(p)
  }

  const wrapUploadTimeout = (promise, ms) =>
    Promise.race([
      promise,
      new Promise((_, reject) =>
        setTimeout(() => {
          clearStallWatchdog()
          abortActiveUpload()
          reject(new Error('上传超时，请检查网络后重试'))
        }, ms)
      ),
    ])

  const uploadImagePart = async () => {
    startUploadPart('image')
    const headers = { Authorization: 'Bearer ' + getToken() }
    try {
      const imgResp = await wrapUploadTimeout(
        uploadFile(`${baseUrl}/user/face-capture`, photoPathRef.current, {
          name: 'image',
          headers,
          onProgress: onUploadProgress,
          onStart: (t) => { uploadTaskRef.current = t },
        }),
        90000
      )
      clearStallWatchdog()
      uploadTaskRef.current = null
      if (!imgResp.ok) throw new Error('照片上传失败')
      const imgJson = JSON.parse(imgResp.data)
      if (imgJson.code !== 20000) throw new Error(resolveErrorMessage(imgJson, '提交失败'))
      const batchId = imgJson.data?.batch_id || ''
      lastBatchIdRef.current = batchId
      return batchId
    } catch (err) {
      clearStallWatchdog()
      // audit #1821 Bug1：停滞（watchdog abort，stallHandledRef=true）时 UI 已由
      // handleUploadStall 接管（fail 态弹窗）→ 静默返回空 batchId 即可；
      // 其余错误（含硬超时）必须 throw → doUpload 外层 catch 进 fail 态，
      // 绝不静默卡在 Uploading（旧代码超时误置 stallHandledRef 导致无限"正在上传"）。
      if (stallHandledRef.current) return ''
      throw err
    }
  }

  const uploadVideoPart = async (batchId) => {
    startUploadPart('video')
    const headers = { Authorization: 'Bearer ' + getToken() }
    try {
      const vidResp = await wrapUploadTimeout(
        uploadFile(`${baseUrl}/user/face-capture`, videoPathRef.current, {
          name: 'video',
          formData: { batch_id: batchId },
          headers,
          onProgress: onUploadProgress,
          onStart: (t) => { uploadTaskRef.current = t },
        }),
        300000
      )
      clearStallWatchdog()
      uploadTaskRef.current = null
      if (!vidResp.ok) throw new Error('视频上传失败')
      const vidJson = JSON.parse(vidResp.data)
      if (vidJson.code !== 20000) throw new Error(resolveErrorMessage(vidJson, '视频上传失败'))
    } catch (err) {
      clearStallWatchdog()
      // audit #1821 Bug2/3：任何失败（硬超时/停滞/服务端错误）一律 throw——
      // 由 doUpload 层统一分流（停滞时 handleUploadStall 已弹 askVideoFail，
      // doUpload 依据 stallHandledRef 不重复弹窗、不 finishSubmit）。
      // 旧代码停滞时静默 return 会令 doUpload 误以为成功 → finishSubmit 假成功。
      throw err
    }
  }

  const finishSubmit = (toastText) => {
    clearStallWatchdog()
    setStatus('pending_review')
    setUploadProgress(null)
    dialog.toast(toastText || '提交成功，等待审核')
    setTimeout(() => {
      if (env.isMiniProgram) { Taro.navigateBack() } else { navigate('/profile/edit') }
    }, 800)
  }

  // #1821: 视频上传失败/超时/停滞（慢网）→ 照片批次已建好，询问是否仅提交照片，
  // 不静默降级也不整体重来（重试只补视频，不重复传照片/不新建批次）。
  function askVideoFail(batchId, err) {
    setUploadProgress(null)
    Taro.showModal({
      title: '视频上传未完成',
      content: `照片已提交成功，但动态视频上传失败${err && err.message ? '：' + err.message : ''}。您可重试视频（网络较慢时可能需要几分钟），或仅提交当前照片进入审核。`,
      confirmText: '重试视频',
      cancelText: '仅提交照片',
      success: async (r) => {
        if (!r.confirm) {
          finishSubmit('已提交照片，等待审核（未含视频）')
          return
        }
        setPhase('Uploading')
        try {
          await uploadVideoPart(batchId)
          finishSubmit()
        } catch (e2) {
          if (!stallHandledRef.current) askVideoFail(batchId, e2)
        }
      },
    })
  }

  // 停滞看门狗触发：abort 后按当前部件分流。
  // audit #1821 Bug3：abort → task fail → uploadVideoPart/uploadImagePart
  // 的 catch 会 throw → doUpload catch 依据 stallHandledRef 不再重复弹窗，
  // 也不会 finishSubmit——UI 完全由本函数弹的模态/fail 态接管，杜绝
  // 「弹窗一闪而过 + 假成功跳页」竞态。
  function handleUploadStall() {
    clearStallWatchdog()
    abortActiveUpload()
    stallHandledRef.current = true
    setUploadProgress(null)
    if (uploadPartRef.current === 'video' && lastBatchIdRef.current) {
      askVideoFail(lastBatchIdRef.current, new Error('网络停滞（可能是锁屏/断流），请重试或仅提交照片'))
    } else {
      setUploadError('上传停滞（网络中断或锁屏挂起），请重试')
      setPhase('fail')
    }
  }

  // Upload photo + optional video → POST /user/face-capture (weapp 分离上传)
  const doUpload = async () => {
    setUploadError('')
    setUploadProgress(null)
    try {
      const batchId = await uploadImagePart()
      if (!batchId) return // 停滞已处理（fail 态弹窗）
      if (videoPathRef.current && batchId) {
        try {
          await uploadVideoPart(batchId)
          finishSubmit()
        } catch (err) {
          if (!stallHandledRef.current) askVideoFail(batchId, err)
        }
      } else {
        finishSubmit()
      }
    } catch (err) {
      // P5: 上传失败进入 fail 态并保留已录素材（photo/video temp paths），
      // 供「重试上传」直接复用——不丢失已录素材状态。
      setUploadProgress(null)
      clearStallWatchdog()
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
    doUpload()
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
        doUpload()
      },
      fail: () => {
        // #1821: 录像拿不到素材（桌面版微信等）——不再静默降级为仅照片上传。
        videoPathRef.current = ''
        setPhase('idle')
        Taro.showModal({
          title: '未获取到视频',
          content: '当前设备未能录制视频（电脑版微信通常不支持录像）。实名认证需要动态视频，请改用手机微信完成；如需继续请仅提交照片（不推荐）。',
          confirmText: '仅提交照片',
          cancelText: '知道了',
          success: (r) => {
            if (r.confirm) doUpload()
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
        // #1821: 开始录像失败要给反馈（桌面微信常不支持），不再静默卡住。
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
    // #1821: 采集窗压缩至约 1/4 屏 + resolution=low → 录制视频为低分辨率，
    // 上传更快、服务器占用更小（全屏预览不改变录制分辨率，真正压缩靠 low）。
    const shutterLabel = phase === 'Uploading' ? '处理中...'
      : phase === 'Recording' || phase === 'Blink' ? '停止'
      : '拍照'

    // #1821: 桌面微信录像不可用 → 直接引导用手机，不进相机流程。
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
                <Text style={{ fontSize: 13, color: 'rgba(255,255,255,0.7)', textAlign: 'center', lineHeight: '18px' }}>
                  {uploadProgress && uploadProgress.total > 0
                    ? `正在上传 ${Math.min(100, Math.round((uploadProgress.loaded / uploadProgress.total) * 100))}%（网络较慢时可能需要几分钟，请勿离开）`
                    : '正在上传，请勿离开页面…'}
                </Text>
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
