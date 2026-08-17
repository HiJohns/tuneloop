import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView } from '@tarojs/components'
import { env } from '../platform'
import { getToken } from '../services/api'

// Setting — 设置页（#1686）：协议条款入口 + 编辑资料入口。
// 内容页统一走 /content?key=xxx（ContentPage 渲染，后台 ContentEdit 可编辑）。
export default function Setting() {
  const navigate = useNavigate()
  const [hasToken, setHasToken] = useState(false)

  useEffect(() => {
    setHasToken(!!getToken())
  }, [])

  // #1686: cross-end nav — H5 short path vs weapp full path.
  const nav = (to) => {
    if (!env.isMiniProgram) { navigate(to); return }
    const [path, query] = to.split('?')
    const page = { '/content': 'content', '/profile/edit': 'profile/edit' }[path]
    if (!page) { navigate(to); return }
    Taro.navigateTo({ url: `/pages-weapp/${page}/index${query ? '?' + query : ''}` })
  }

  const rows = [
    { icon: '✏️', label: '编辑资料', onClick: () => hasToken && nav('/profile/edit') },
    { icon: '📄', label: '租用服务协议', onClick: () => nav('/content?key=rental_agreement') },
    { icon: '📄', label: '用户协议', onClick: () => nav('/content?key=user_agreement') },
    { icon: '🔒', label: '隐私协议', onClick: () => nav('/content?key=privacy_policy') },
    { icon: '🪪', label: '数字证书授权使用协议', onClick: () => nav('/content?key=digital_certificate') },
    { icon: '⚖️', label: '《乐器损耗与赔偿标准》细则', onClick: () => nav('/content?key=damage_standard') },
  ]

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7' }}>
      <ScrollView style={{ width: '100%' }}>
        <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 divide-y divide-zinc-100">
          {rows.map((row, i) => (
            <View
              key={i}
              className="flex justify-between items-center py-3.5 active:opacity-60"
              style={{ opacity: row.label === '编辑资料' && !hasToken ? 0.4 : 1 }}
              onClick={row.onClick}
            >
              <View className="flex items-center gap-2">
                <Text className="text-lg">{row.icon}</Text>
                <Text className="text-base font-bold text-zinc-800">{row.label}</Text>
              </View>
              <Text className="text-sm text-zinc-300">❯</Text>
            </View>
          ))}
        </View>
      </ScrollView>
    </View>
  )
}
