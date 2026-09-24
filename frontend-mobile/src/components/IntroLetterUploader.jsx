// IntroLetterUploader — 学生证介绍信上传（H5 + weapp 共用，#2057 裁定3）
// 走统一媒体管线 POST /upload（content_image），onChange 回传 file_key。
// Props:
//   initialKey: 已存存储键（展示「已上传」占位）
//   onChange(key, url): 上传成功回调；删除后回调 ('', '')
//   leftAligned: 左对齐（与 IdPhotoUploader 一致）
import { useState, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image } from '@tarojs/components'
import { dialog, uploadFile, env, storage, session } from '../platform'
import { resolveErrorMessage } from '../services/api'

export default function IntroLetterUploader({ initialKey = '', onChange, leftAligned = false }) {
  const [url, setUrl] = useState('')
  const [uploading, setUploading] = useState(false)
  const fileInputRef = useRef(null)

  const getToken = () => storage.getItem('token') || session.getItem('token')

  const resolveImageUrl = (u) => {
    if (!u || !u.startsWith('/')) return u
    if (!env.isMiniProgram) return u
    const base = (env.apiBaseUrl || '').replace(/\/api$/, '')
    return base + u
  }

  const uploadToServer = async (fileOrPath) => {
    setUploading(true)
    try {
      const base = env.apiBaseUrl || '/api'
      const headers = { Authorization: 'Bearer ' + getToken() }
      let json
      if (env.isMiniProgram) {
        const resp = await uploadFile(`${base}/upload`, fileOrPath, { name: 'file', headers })
        if (!resp.ok) throw new Error('upload failed')
        json = JSON.parse(resp.data)
      } else {
        const fd = new FormData()
        fd.append('file', fileOrPath)
        const fetchResp = await fetch(`${base}/upload`, { method: 'POST', body: fd, headers })
        json = await fetchResp.json()
      }
      if (json.code === 20000 && json.data?.file_key) {
        setUrl(json.data.url || '')
        if (onChange) onChange(json.data.file_key, json.data.url || '')
      } else {
        throw new Error(resolveErrorMessage(json, '介绍信上传失败'))
      }
    } catch (err) {
      if (env.isMiniProgram) Taro.showToast({ title: '介绍信上传失败', icon: 'none' })
      else dialog.alert('介绍信上传失败')
    } finally {
      setUploading(false)
    }
  }

  const handleWeappChoose = () => {
    Taro.chooseImage({ count: 1, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      .then(res => {
        const path = res.tempFilePaths?.[0]
        if (path) uploadToServer(path)
      })
      .catch(() => {})
  }

  const handleH5File = (e) => {
    const file = e.target.files?.[0]
    if (file) uploadToServer(file)
    e.target.value = ''
  }

  const handleRemove = () => {
    setUrl('')
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
}
