// IntroLetterUploader — 学生证介绍信上传（H5 + weapp 共用，#2057 裁定3 / #2314）
// Props:
//   initialKey: 已存存储键
//   onChange(key, url): 上传成功回调；删除后回调 ('', '')
//   defer: 注册页延迟上传；通过 uploadPending() 显式上传
//   sessionUpload: { sessionId } — 注册会话匿名上传（side=intro）
import { useState, useRef, forwardRef, useImperativeHandle } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image } from '@tarojs/components'
import { dialog, uploadFile, env } from '../platform'
import { resolveErrorMessage, resolveUploadError } from '../services/api'
import { t } from '../i18n'

const IntroLetterUploader = forwardRef(function IntroLetterUploader({ initialKey = '', onChange, defer = false, sessionUpload, leftAligned = false, onSelect }, ref) {
  const [url, setUrl] = useState('')
  const [uploading, setUploading] = useState(false)
  const [pendingFile, setPendingFile] = useState(null)
  const fileInputRef = useRef(null)

  const resolveImageUrl = (u) => {
    if (!u || !u.startsWith('/')) return u
    if (!env.isMiniProgram) return u
    const base = (env.apiBaseUrl || '').replace(/\/api$/, '')
    return base + u
  }

  const uploadToServer = async (fileOrPath, overrideSessionId) => {
    setUploading(true)
    try {
      const base = env.apiBaseUrl || '/api'
      const useSession = overrideSessionId || sessionUpload?.sessionId
      const uploadUrl = useSession
        ? `${base}/auth/registration-sessions/${useSession}/id-photo`
        : `${base}/upload`
      const resp = await uploadFile(uploadUrl, fileOrPath, {
        name: 'file',
        formData: useSession ? { side: 'intro' } : {},
        auth: !useSession,
      })
      if (!resp.ok) throw new Error('介绍信上传失败')
      const json = env.isMiniProgram ? JSON.parse(resp.data) : await resp.json()
      const authErr = resolveUploadError(resp, json)
      if (authErr) throw new Error(authErr)
      if (json.code === 20000) {
        const key = json.data?.file_key || ''
        setUrl(json.data?.url || '')
        setPendingFile(null)
        if (onChange) onChange(key, json.data?.url || '')
        return key
      }
      throw new Error(resolveErrorMessage(json, '介绍信上传失败'))
    } catch (err) {
      if (env.isMiniProgram) dialog.toast('介绍信上传失败')
      else dialog.alert('介绍信上传失败')
      return null
    } finally {
      setUploading(false)
    }
  }

  const handleSelect = (fileOrPath) => {
    if (defer) {
      setPendingFile(fileOrPath)
      setUrl(env.isMiniProgram ? fileOrPath : URL.createObjectURL(fileOrPath))
      if (onSelect) onSelect(fileOrPath)
      return
    }
    uploadToServer(fileOrPath)
  }

  useImperativeHandle(ref, () => ({
    hasFile: () => !!(pendingFile || url || initialKey),
    uploadPending: async (overrideSessionId) => {
      if (!pendingFile) return 'skip'
      const key = await uploadToServer(pendingFile, overrideSessionId)
      return key || null
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

  const handleRemove = () => {
    setUrl('')
    setPendingFile(null)
    if (onChange) onChange('', '')
  }

  const hasValue = !!url || !!initialKey

  return (
    <View className={`flex flex-col ${leftAligned ? 'items-start' : 'items-center'}`}>
      {hasValue ? (
        <View className="relative w-32">
          {url && env.isMiniProgram ? (
            <Image src={resolveImageUrl(url)} mode="aspectFill" className="w-32 h-20 rounded-lg" style={{ width: 128, height: 80 }} />
          ) : url ? (
            <img src={url} alt="介绍信" className="w-32 h-20 object-cover rounded-lg" />
          ) : (
            <View className="w-32 h-20 rounded-lg flex items-center justify-center" style={{ width: 128, height: 80, backgroundColor: '#f0fdf4', border: '1px solid #bbf7d0' }}>
              <Text className="text-xs" style={{ color: '#16a34a' }}>已上传</Text>
            </View>
          )}
          <View className="absolute -top-2 -right-2 w-5 h-5 bg-red-500 text-white rounded-full flex items-center justify-center text-xs"
            onClick={handleRemove}>✕</View>
        </View>
      ) : (
        <View className="w-32 h-20 border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center"
          onClick={env.isMiniProgram ? handleWeappChoose : () => fileInputRef.current?.click()}>
          <Text className="text-gray-400 text-xs">介绍信</Text>
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

export default IntroLetterUploader
