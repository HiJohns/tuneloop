import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Image, ScrollView, Button } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { dialog, env, toWeappRoute } from '../platform'

// #1977 T3 师傅详情页（样式参照乐器详情页）：
// 照片 + 姓名 + 完整介绍 + 专长年限列表；底部固定「创建维修订单」→ 创建页（师傅锁定）

const btnPrimaryStyle = {
  width: '100%', margin: 0, height: 48, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF',
  borderRadius: 10, fontSize: 15, fontWeight: 'bold',
}

export default function TechDetail() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  // 双源读取 technician_id（H5 query / weapp router params）
  const technicianId = (() => {
    if (env.isMiniProgram) {
      const p = Taro.getCurrentInstance()?.router?.params || {}
      return p.technician_id || ''
    }
    return new URLSearchParams(window.location.search).get('technician_id') || ''
  })()
  const [tech, setTech] = useState(null)
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    if (!technicianId) { setLoading(false); return }
    apiFetch(`${baseUrl}/common/repair-technicians/${technicianId}`)
      .then(r => r.json())
      .then(r => { if (r.code === 20000) setTech(r.data || {}) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [technicianId])

  const experience = (() => {
    const v = tech?.experience
    if (!v) return []
    if (Array.isArray(v)) return v
    try { return JSON.parse(v) } catch { return [] }
  })()

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修师</Text>
      </View>

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 120px', boxSizing: 'border-box' }}>
          {loading ? (
            <Text style={{ fontSize: 13, color: '#A1A1AA' }}>加载中...</Text>
          ) : !tech ? (
            <Text style={{ fontSize: 13, color: '#A1A1AA' }}>维修师不存在或已停用</Text>
          ) : (
            <>
              {/* 头部：照片 + 姓名 */}
              <View style={{ backgroundColor: '#FFFFFF', borderRadius: 12, padding: 16, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8 }}>
                {tech.avatar ? (
                  <Image src={tech.avatar} mode="aspectFill" style={{ width: 96, height: 96, borderRadius: 48 }} />
                ) : (
                  <View style={{ width: 96, height: 96, borderRadius: 48, backgroundColor: '#F4F4F5', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Text style={{ fontSize: 28, color: '#A1A1AA' }}>师</Text>
                  </View>
                )}
                <Text style={{ fontSize: 18, fontWeight: 'bold', color: '#18181B' }}>{tech.name || '维修师'}</Text>
              </View>

              {/* 专长与年限 */}
              {experience.length > 0 && (
                <View style={{ backgroundColor: '#FFFFFF', borderRadius: 12, padding: 14, marginTop: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
                  <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>专长与年限</Text>
                  {experience.map((e, i) => (
                    <View key={i} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <Text style={{ fontSize: 13, color: '#3F3F46' }}>{e.craft || '-'}</Text>
                      <Text style={{ fontSize: 12, color: '#71717A' }}>{e.years || 0} 年经验</Text>
                    </View>
                  ))}
                </View>
              )}

              {/* 详细介绍 */}
              <View style={{ backgroundColor: '#FFFFFF', borderRadius: 12, padding: 14, marginTop: 12, display: 'flex', flexDirection: 'column', gap: 6 }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>详细介绍</Text>
                <Text style={{ fontSize: 13, color: '#3F3F46' }}>{tech.bio || '暂无介绍'}</Text>
              </View>
            </>
          )}
        </View>
      </ScrollView>

      {/* 底部固定：创建维修订单（师傅锁定） */}
      {tech && (
        <View style={{ position: 'fixed', left: 0, right: 0, bottom: 0, padding: 16, backgroundColor: '#FDFBF7', boxSizing: 'border-box' }}>
          <Button onClick={() => nav(`/repair-service-create?technician_id=${tech.technician_id}`)} style={btnPrimaryStyle}>
            创建维修订单
          </Button>
        </View>
      )}
    </View>
  )
}
