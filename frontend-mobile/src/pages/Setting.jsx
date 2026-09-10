import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView } from '@tarojs/components'
import { env } from '../platform'
import { getToken } from '../services/api'

// Setting — 协议条款页（#1686 → 2026-09 调整）：纯协议入口列表。
// 编辑资料入口已移至个人中心顶层菜单（不再与顶层重复）。
// 内容页统一走 /content?key=xxx（ContentPage 渲染，后台 ContentEdit 可编辑）。
export default function Setting() {
  const navigate = useNavigate()

  // #1686: cross-end nav — H5 short path vs weapp full path.
  const nav = (to) => {
    if (!env.isMiniProgram) { navigate(to); return }
    const [path, query] = to.split('?')
    const page = { '/content': 'content' }[path]
    if (!page) { navigate(to); return }
    Taro.navigateTo({ url: `/pages-weapp/${page}/index${query ? '?' + query : ''}` })
  }

  // #1840: rows follow the canonical 10-item sequence (positions 3–10;
  // 联系我们/商务合作 live on the profile page at positions 1–2).
  const rows = [
    { icon: '📜', label: '平台规则文档', onClick: () => nav('/content?key=platform_rules') },
    { icon: '📄', label: '租用服务协议', onClick: () => nav('/content?key=rental_agreement') },
    { icon: '⚖️', label: '《乐器损耗与赔偿标准》细则', onClick: () => nav('/content?key=damage_standard') },
    { icon: '📄', label: '个人信息查询授权书', onClick: () => nav('/content?key=user_agreement') },
    { icon: '🔒', label: '个人信息保护政策', onClick: () => nav('/content?key=privacy_policy') },
    { icon: '🪪', label: '数字证书授权使用协议', onClick: () => nav('/content?key=digital_certificate') },
    { icon: '📋', label: '平台入驻审核要求与规范', onClick: () => nav('/content?key=merchant_audit_requirements') },
    { icon: '📝', label: '商家入驻协议', onClick: () => nav('/content?key=merchant_agreement') },
  ]

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7' }}>
      <ScrollView style={{ width: '100%' }}>
        <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 divide-y divide-zinc-100">
          {rows.map((row, i) => (
            <View
              key={i}
              className="flex justify-between items-center py-3.5"
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
