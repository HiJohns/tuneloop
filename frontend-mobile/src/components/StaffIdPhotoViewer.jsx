import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image } from '@tarojs/components'
import { env, storage, session } from '../platform'

// StaffIdPhotoViewer — 员工只读查看用户身份证 (#1599)
// Props:
//   userId: 目标用户 ID（本地 users.id）
//   showLabel: 是否显示「身份证」标题（默认 true）
// 加载失败或未上传时静默不渲染（不打扰业务操作流）。
export default function StaffIdPhotoViewer({ userId, showLabel = true }) {
  const [photos, setPhotos] = useState(null)

  useEffect(() => {
    if (!userId) return
    let cancelled = false
    const load = async () => {
      try {
        const base = env.apiBaseUrl || '/api'
        const token = storage.getItem('token') || session.getItem('token')
        let resp
        if (env.isMiniProgram) {
          resp = await Taro.request({
            url: `${base}/user/${userId}/id-photos`,
            method: 'GET',
            header: { Authorization: 'Bearer ' + token },
          })
          if (resp.statusCode !== 200) return
          const json = typeof resp.data === 'string' ? JSON.parse(resp.data) : resp.data
          if (json.code === 20000 && !cancelled) setPhotos(json.data)
        } else {
          resp = await fetch(`${base}/user/${userId}/id-photos`, {
            headers: { Authorization: 'Bearer ' + token },
          })
          if (!resp.ok) return
          const json = await resp.json()
          if (json.code === 20000 && !cancelled) setPhotos(json.data)
        }
      } catch { /* silent */ }
    }
    load()
    return () => { cancelled = true }
  }, [userId])

  if (!photos) return null
  if (!photos.front && !photos.back) return null

  return (
    <View className="bg-white rounded-2xl p-4">
      {showLabel && <Text className="font-black text-black mb-3">身份证</Text>}
      <View className="flex flex-row gap-3">
        {photos.front && (
          <View className="flex-1">
            {env.isMiniProgram ? (
              <Image src={photos.front} mode="aspectFill" className="w-full rounded-lg" style={{ height: 80 }} />
            ) : (
              <img src={photos.front} alt="身份证正面" className="w-full h-20 object-cover rounded-lg" />
            )}
            <Text className="block text-center text-xs text-zinc-400 mt-1">正面</Text>
          </View>
        )}
        {photos.back && (
          <View className="flex-1">
            {env.isMiniProgram ? (
              <Image src={photos.back} mode="aspectFill" className="w-full rounded-lg" style={{ height: 80 }} />
            ) : (
              <img src={photos.back} alt="身份证背面" className="w-full h-20 object-cover rounded-lg" />
            )}
            <Text className="block text-center text-xs text-zinc-400 mt-1">背面</Text>
          </View>
        )}
      </View>
    </View>
  )
}
