// 统一底部导航（#2083）：tabs 由本组件按当前账户角色构造，各页只传 active/badges。
// 角色来源 GET /site-members/me（= 网点角色 ∪ JWT fn_roles）；纯维修师傅隐藏「租赁」
//（#1884）。此前各页自行拼装 tabs 导致显隐口径不一致（Profile 漏过滤的事故根因）。
import { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { isStaffRole, isPureTechnician } from '../utils/role'

export default function BottomNav({ active = '', badges = {}, tenant }) {
  const [roles, setRoles] = useState(null)

  useEffect(() => {
    if (!getToken()) return
    let cancelled = false
    apiFetch(`${env.apiBaseUrl}/site-members/me`)
      .then(r => r.json())
      .then(res => { if (!cancelled && res.code === 20000) setRoles(res.data?.roles || []) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])

  const isStaff = isStaffRole()
  const isPureTech = roles ? isPureTechnician(roles) : false

  // 兼容 Home 的 tenant 透传（原 switchTab helper 行为）：显式传入 tenant 时暂存
  // tab_params 供租赁页读取；未传（其它页）不动 tab_params。
  const goTab = (url) => {
    if (tenant !== undefined) {
      try { Taro.setStorageSync('tab_params', { tenant: tenant || '' }) } catch {}
    }
    Taro.switchTab({ url })
  }

  const goService = () => {
    if (active === 'service') return
    const url = isStaff ? '/pages-weapp/my-repairs/index' : '/pages-weapp/tech-list/index'
    if (Taro.getCurrentPages().length >= 9) {
      Taro.reLaunch({ url })
    } else {
      Taro.navigateTo({ url })
    }
  }

  const tabs = [
    { key: 'home', icon: '🏪', label: '首页', onClick: () => goTab('/pages-weapp/home/index') },
    ...(isPureTech ? [] : [{ key: 'rent', icon: '🪕', label: '租赁', onClick: () => goTab('/pages-weapp/my-leases/index') }]),
    { key: 'service', icon: '🛠️', label: '维修', onClick: goService },
    { key: 'profile', icon: '👤', label: '我的', onClick: () => goTab('/pages-weapp/profile/index') },
  ]

  return (
    <View style={{ position: 'absolute', bottom: 0, left: 0, right: 0, backgroundColor: '#5A3B24', borderTop: '1px solid #4E321E', paddingTop: 8, paddingBottom: 8, display: 'flex', justifyContent: 'space-around', alignItems: 'center', zIndex: 50 }}>
      {tabs.map((tab, i) => {
        const isActive = active === tab.key
        const badge = badges[tab.key]
        return (
          <View key={tab.key || i} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', position: 'relative', flex: 1, paddingTop: 6, paddingBottom: 6 }} onClick={tab.onClick}>
            <View style={{ fontSize: 28, marginBottom: 2, position: 'relative' }}>
              {tab.icon}
              {badge > 0 && (
                <View style={{ position: 'absolute', top: -4, right: -8, backgroundColor: '#FF2A55', color: '#fff', fontSize: 9, fontWeight: '900', minWidth: 16, height: 16, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', paddingLeft: 2, paddingRight: 2, border: '1px solid #5A3B24' }}>
                  {badge > 99 ? '99+' : badge}
                </View>
              )}
            </View>
            <Text style={{ fontSize: 10, fontWeight: '700', color: isActive ? '#fff' : 'rgba(255,255,255,0.4)' }}>{tab.label}</Text>
          </View>
        )
      })}
    </View>
  )
}
