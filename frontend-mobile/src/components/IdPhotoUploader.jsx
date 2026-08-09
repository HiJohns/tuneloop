// IdPhotoUploader — 身份证单面上传组件（H5 + weapp 共用，#1599）
// Props:
//   side: 'front' | 'back'
//   initialUrl: 已上传的照片 URL（从 GET /user/id-photos 或 /users/me 预填）
//   onChange(url): 上传成功后回调新 URL；点击删除后回调 ''
//   defer: 延迟上传模式（注册页专用）——仅本地预览，通过 uploadPending() 显式上传
import { useState, useRef, useImperativeHandle, forwardRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image } from '@tarojs/components'
import { uploadFile, env, storage, session } from '../platform'

const IdPhotoUploader = forwardRef(function IdPhotoUploader({ side, initialUrl = '', onChange, defer = false }, ref) {
  const [url, setUrl] = useState(initialUrl || '')
  const [uploading, setUploading] = useState(false)
  const [pendingFile, setPendingFile] = useState(null)
  const fileInputRef = useRef(null)

  const getToken = () => storage.getItem('token') || session.getItem('token')

  const uploadToServer = async (fileOrPath) => {
    setUploading(true)
    try {
      const base = env.apiBaseUrl || '/api'
      let resp
      if (env.isMiniProgram) {
        resp = await uploadFile(`${base}/user/id-photo`, fileOrPath, {
          name: 'file',
          formData: { side },
          headers: { Authorization: 'Bearer ' + getToken() },
        })
        if (!resp.ok) throw new Error('upload failed')
        const json = JSON.parse(resp.data)
        if (json.code === 20000 && json.data?.url) {
          setUrl(json.data.url)
          if (onChange) onChange(json.data.url)
        } else {
          throw new Error(json.message || 'upload failed')
        }
      } else {
        const headers = { Authorization: 'Bearer ' + getToken() }
        if (fileOrPath instanceof File) {
          const fd = new FormData()
          fd.append('file', fileOrPath)
          fd.append('side', side)
          const fetchResp = await fetch(`${base}/user/id-photo`, { method: 'POST', body: fd, headers })
          const json = await fetchResp.json()
          if (json.code === 20000 && json.data?.url) {
            setUrl(json.data.url)
            if (onChange) onChange(json.data.url)
          } else {
            throw new Error(json.message || 'upload failed')
          }
        }
      }
    } catch (err) {
      if (env.isMiniProgram) {
        Taro.showToast({ title: '身份证上传失败', icon: 'none' })
      } else {
        alert('身份证上传失败')
      }
    } finally {
      setUploading(false)
    }
  }

  // Defer mode: keep the file locally until uploadPending() is called.
  const handleSelect = (fileOrPath) => {
    if (defer) {
      setPendingFile(fileOrPath)
      setUrl(env.isMiniProgram ? fileOrPath : URL.createObjectURL(fileOrPath))
      return
    }
    uploadToServer(fileOrPath)
  }

  useImperativeHandle(ref, () => ({
    // Explicit upload for defer mode (registration pages): returns URL or null.
    uploadPending: async () => {
      if (!pendingFile) return null
      setUploading(true)
      try {
        const base = env.apiBaseUrl || '/api'
        if (env.isMiniProgram) {
          const resp = await uploadFile(`${base}/user/id-photo`, pendingFile, {
            name: 'file',
            formData: { side },
            headers: { Authorization: 'Bearer ' + getToken() },
          })
          if (!resp.ok) throw new Error('upload failed')
          const json = JSON.parse(resp.data)
          if (json.code === 20000 && json.data?.url) {
            setPendingFile(null)
            return json.data.url
          }
          throw new Error(json.message || 'upload failed')
        }
        const fd = new FormData()
        fd.append('file', pendingFile)
        fd.append('side', side)
        const fetchResp = await fetch(`${base}/user/id-photo`, {
          method: 'POST',
          body: fd,
          headers: { Authorization: 'Bearer ' + getToken() },
        })
        const json = await fetchResp.json()
        if (json.code === 20000 && json.data?.url) {
          setPendingFile(null)
          return json.data.url
        }
        throw new Error(json.message || 'upload failed')
      } catch (err) {
        if (env.isMiniProgram) {
          Taro.showToast({ title: '身份证上传失败', icon: 'none' })
        } else {
          alert('身份证上传失败')
        }
        return null
      } finally {
        setUploading(false)
      }
    },
  }))

  const handleWeappChoose = () => {
    Taro.chooseImage({ count: 1, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      .then(res => {
        const path = res.tempFilePaths?.[0]
        if (path) handleSelect(path)
      })
      .catch(() => {})
  }

  const handleH5File = (e) => {
    const file = e.target.files?.[0]
    if (file) handleSelect(file)
    e.target.value = ''
  }

  const handleRemove = async () => {
    // Best-effort server-side delete; keep the UI consistent even if it fails
    try {
      const base = env.apiBaseUrl || '/api'
      const headers = { Authorization: 'Bearer ' + getToken() }
      if (env.isMiniProgram) {
        await Taro.request({ url: `${base}/user/id-photo?side=${side}`, method: 'DELETE', header: headers })
      } else {
        await fetch(`${base}/user/id-photo?side=${side}`, { method: 'DELETE', headers })
      }
    } catch { /* ignore */ }
    setUrl('')
    setPendingFile(null)
    if (onChange) onChange('')
  }

  const label = side === 'front' ? '正面' : '反面'

  return (
    <View className="flex flex-col items-center">
      {url ? (
        <View className="relative w-32">
          {env.isMiniProgram ? (
            <Image src={url} mode="aspectFill" className="w-32 h-20 rounded-lg" style={{ width: 128, height: 80 }} />
          ) : (
            <img src={url} alt={`身份证${label}`} className="w-32 h-20 object-cover rounded-lg" />
          )}
          <View className="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center text-xs"
            onClick={handleRemove}>✕</View>
        </View>
      ) : (
        <View className="w-32 h-20 border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center"
          onClick={env.isMiniProgram ? handleWeappChoose : () => fileInputRef.current?.click()}>
          <Text className="text-gray-400 text-xs">{label}</Text>
          <Text className="text-gray-300 text-xs mt-1">{uploading ? '上传中...' : '点击上传'}</Text>
        </View>
      )}
      {!env.isMiniProgram && (
        <input ref={fileInputRef} type="file" accept="image/jpeg,image/png,image/webp"
          className="hidden" onChange={handleH5File} />
      )}
    </View>
  )
})

export default IdPhotoUploader
