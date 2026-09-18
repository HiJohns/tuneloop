import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Image, ScrollView, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { dialog, env, toWeappRoute } from '../platform'

// #1976 T2 维修 Tab 首屏 = 师傅列表（样式参照乐器列表）
// 右上角『我的维修』：无活跃会话置灰（可点）/ 有活跃会话点亮+个数 → 现存维修页
// v3「乐器报修」并存入口保留（RS-10，现存维修页内含双 Tab）

function parseExperience(v) {
  if (!v) return []
  if (Array.isArray(v)) return v
  try { return JSON.parse(v) } catch { return [] }
}

export default function TechList() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [techs, setTechs] = useState([])
  const [activeCount, setActiveCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const load = async () => {
      try {
        const res = await apiFetch(`${baseUrl}/common/repair-technicians`)
        const r = await res.json()
        if (r.code === 20000) setTechs(r.data?.list || [])
      } catch (e) {
        dialog.alert('加载维修师失败，请稍后重试')
      }
      // 『我的维修』活跃会话计数（未登录/非师傅 → 0 → 置灰）
      if (getToken()) {
        try {
          const cRes = await apiFetch(`${baseUrl}/common/repair-technicians/active-session-count`)
          const c = await cRes.json()
          if (c.code === 20000) setActiveCount(c.data?.count || 0)
        } catch { /* 忽略：保持置灰 */ }
      }
      setLoading(false)
    }
    load()
  }, [])

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {/* 头部：标题 + 『我的维修』 */}
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修师</Text>
        <View
          onClick={() => nav('/my-repairs')}
          style={{
            display: 'flex', alignItems: 'center', gap: 4, padding: '6px 12px', borderRadius: 16,
            backgroundColor: activeCount > 0 ? '#171717' : '#F4F4F5',
          }}
        >
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: activeCount > 0 ? '#FFFFFF' : '#A1A1AA' }}>
            我的维修
          </Text>
          {activeCount > 0 && (
            <Text style={{ fontSize: 12, fontWeight: 'bold', color: '#FFFFFF' }}>{activeCount}</Text>
          )}
        </View>
      </View>

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          {loading ? (
            <Text style={{ fontSize: 13, color: '#A1A1AA' }}>加载中...</Text>
          ) : techs.length === 0 ? (
            <Text style={{ fontSize: 13, color: '#A1A1AA' }}>暂无维修师</Text>
          ) : techs.map(t => {
            const exp = parseExperience(t.experience)
            const summary = exp.slice(0, 2).map(e => `${e.craft} ${e.years} 年`).join(' · ')
            return (
              <View
                key={t.technician_id}
                onClick={() => nav(`/tech-detail?technician_id=${t.technician_id}`)}
                style={{
                  display: 'flex', alignItems: 'center', gap: 12,
                  backgroundColor: '#FFFFFF', borderRadius: 12, padding: 12, marginBottom: 10,
                }}
              >
                {t.avatar ? (
                  <Image src={t.avatar} mode="aspectFill" style={{ width: 64, height: 64, borderRadius: 32 }} />
                ) : (
                  <View style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: '#F4F4F5', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Text style={{ fontSize: 20, color: '#A1A1AA' }}>师</Text>
                  </View>
                )}
                <View style={{ display: 'flex', flexDirection: 'column', gap: 4, flex: 1, minWidth: 0 }}>
                  <Text style={{ fontSize: 15, fontWeight: 'bold', color: '#18181B' }}>{t.name || '维修师'}</Text>
                  {summary ? <Text style={{ fontSize: 12, color: '#71717A' }}>{summary}</Text> : null}
                  {t.bio ? (
                    <Text style={{ fontSize: 11, color: '#A1A1AA' }} numberOfLines={1}>{t.bio}</Text>
                  ) : null}
                </View>
                <Text style={{ fontSize: 18, color: '#D4D4D8' }}>›</Text>
              </View>
            )
          })}

          {/* v3「乐器报修」并存入口（RS-10；现存维修页内含「乐器报修 / 维修服务」双 Tab） */}
          <View
            onClick={() => nav('/my-repairs')}
            style={{ marginTop: 8, backgroundColor: '#FFFFFF', borderRadius: 12, padding: 14, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}
          >
            <View style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>乐器报修</Text>
              <Text style={{ fontSize: 11, color: '#A1A1AA' }}>已租乐器的报修工单（v3，与维修服务并存）</Text>
            </View>
            <Text style={{ fontSize: 18, color: '#D4D4D8' }}>›</Text>
          </View>
        </View>
      </ScrollView>
    </View>
  )
}
