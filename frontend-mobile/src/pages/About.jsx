// About — 关于页：显示当前小程序版本 + 服务器版本（版本归属核对）。
// weapp 走原生导航栏（title=关于）；H5 手写标题条 + 返回。
import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'
import { View, Text } from '@tarojs/components'
import { env } from '../platform'
import { apiFetch } from '../services/api'

export default function About() {
  const navigate = useNavigate()
  const [serverVersion, setServerVersion] = useState('')
  const [serverBuild, setServerBuild] = useState('')
  const baseUrl = env.apiBaseUrl || '/api'

  const goBack = () => {
    if (!env.isMiniProgram) { navigate(-1); return }
    Taro.navigateBack()
  }

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/config`)
        const json = await resp.json()
        if (!cancelled && json.code === 20000 && json.data?.version) {
          setServerVersion(json.data.version)
          setServerBuild(json.data.build || '')
        } else if (!cancelled) {
          setServerVersion('—')
        }
      } catch {
        if (!cancelled) setServerVersion('获取失败')
      }
    }
    load()
    return () => { cancelled = true }
  }, [baseUrl])

  const rows = [
    { label: '小程序版本', value: env.version ? `v${env.version}` : 'dev' },
    {
      label: '服务器版本',
      value: serverVersion
        ? serverBuild && serverBuild !== 'dev'
          ? `v${serverVersion} (build ${serverBuild})`
          : `v${serverVersion}`
        : '加载中…',
    },
  ]

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7' }}>
      {/* H5 手写标题条（weapp 用原生导航栏，见 #1511 规则） */}
      {!env.isMiniProgram && (
        <View style={{ paddingTop: 12, paddingBottom: 12, paddingLeft: 16, backgroundColor: '#fff', display: 'flex', alignItems: 'center' }}>
          <View onClick={goBack} style={{ marginRight: 8, padding: 4 }}>
            <Text style={{ fontSize: 20, color: '#6b7280' }}>‹</Text>
          </View>
          <Text style={{ fontSize: 16, fontWeight: '700', color: '#111' }}>关于</Text>
        </View>
      )}
      <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 divide-y divide-zinc-100">
        {rows.map((row, i) => (
          <View key={i} className="flex justify-between items-center py-3.5">
            <View className="flex items-center gap-2">
              <Text className="text-base font-bold text-zinc-800">{row.label}</Text>
            </View>
            <Text className="text-sm text-zinc-500">{row.value}</Text>
          </View>
        ))}
      </View>
      <Text style={{ display: 'block', textAlign: 'center', fontSize: 12, color: '#d4d4d8', marginTop: 24, paddingLeft: 32, paddingRight: 32, lineHeight: '18px' }}>
        本页用于版本归属核对：小程序版本为前端构建标识，服务器版本由后端 /api/config 提供。
      </Text>
    </View>
  )
}
