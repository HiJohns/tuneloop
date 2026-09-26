// 统一底部导航（#2083）：tabs 由本组件按当前账户角色构造，各页只传 active/badges
// 与 navigate（H5 页面注入 useNavigate；shared 页在两端分支渲染，weapp 分支不渲染本组件）。
// tenant 可选：H5 首页白标透传。角色来源 GET /site-members/me（= 网点角色 ∪ JWT fn_roles）；
// 纯维修师傅隐藏「租赁」（#1884）。
import { useEffect, useState } from 'react'
import { View, Text } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { isStaffRole, isPureTechnician } from '../utils/role'

export default function BottomNav({ active = '', badges = {}, tenant = '', navigate }) {
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

  const go = (url) => { if (typeof navigate === 'function') navigate(url) }
  const isStaff = isStaffRole()
  const isPureTech = roles ? isPureTechnician(roles) : false
  const withTenant = (url) => (tenant ? `${url}${url.includes('?') ? '&' : '?'}tenant=${tenant}` : url)

  const tabs = [
    { key: 'home', icon: '🏪', label: '首页', onClick: () => go('/') },
    ...(isPureTech ? [] : [{ key: 'rent', icon: '🪕', label: '租赁', onClick: () => go(isStaff ? '/staff/orders' : '/my-leases') }]),
    { key: 'service', icon: '🛠️', label: '维修', onClick: () => go(withTenant(isStaff ? '/my-repairs' : '/tech-list')) },
    { key: 'profile', icon: '👤', label: '我的', onClick: () => go(withTenant('/profile')) },
  ]

  return (
    <View className="absolute bottom-0 left-0 right-0 py-2 flex justify-around items-center z-50 shadow-2xl"
      style={{ backgroundColor: '#5A3B24', borderTop: '1px solid #4E321E' }}
    >
      {tabs.map((tab, i) => {
        const isActive = active === tab.key
        const badge = badges[tab.key]
        return (
          <View key={tab.key || i} className="flex flex-col items-center justify-center relative flex-1 py-1.5" onClick={tab.onClick}>
            <View className="text-3xl mb-0.5 relative">
              {tab.icon}
              {badge > 0 && (
                <View className="absolute -top-1 -right-2 text-white font-black h-4 rounded-full flex items-center justify-center px-1"
                  style={{ border: '1px solid #5A3B24' }}
                >
                  {badge > 99 ? '99+' : badge}
                </View>
              )}
            </View>
            <Text className={`font-bold ${isActive ? 'text-white' : ''}`} style={!isActive ? { color: 'rgba(255,255,255,0.4)' } : undefined}>{tab.label}</Text>
          </View>
        )
      })}
    </View>
  )
}
