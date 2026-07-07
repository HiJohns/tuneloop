import { useState } from 'react'
import { View, Text, Input } from '@tarojs/components'

const TABS = [
  { key: 'home', icon: '🏪', label: '首页' },
  { key: 'rent', icon: '🪕', label: '租赁' },
  { key: 'service', icon: '🛠️', label: '维修' },
  { key: 'profile', icon: '👤', label: '我的' },
]

export default function TestMinimal() {
  return (
    <View style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      {/* Search bar */}
      <View style={{
        marginTop: 80, marginLeft: 'auto', marginRight: 'auto',
        width: 250, height: 42, borderRadius: 999,
        display: 'flex', alignItems: 'center',
        backgroundColor: 'rgba(255,255,255,0.2)',
        border: '1px solid rgba(255,255,255,0.3)',
        paddingLeft: 16, paddingRight: 16,
      }}>
        <Text style={{ fontSize: 14, marginRight: 10, color: 'rgba(255,255,255,0.7)' }}>🔍</Text>
        <Input placeholder="搜索乐器..." style={{ flex: 1, fontSize: 14, color: '#fff' }} />
      </View>

      {/* Background placeholder */}
      <View style={{ flex: 1 }} />

      {/* Bottom nav — inline styles to avoid \# escapes */}
      <View style={{
        display: 'flex', justifyContent: 'space-around', alignItems: 'center',
        backgroundColor: '#5A3B24', borderTop: '1px solid #4E321E',
        paddingTop: 10, paddingBottom: 10,
        position: 'absolute', bottom: 0, left: 0, right: 0,
        zIndex: 50,
      }}>
        {TABS.map(tab => (
          <View key={tab.key} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
            <Text style={{ fontSize: 20 }}>{tab.icon}</Text>
            <Text style={{ fontSize: 10, fontWeight: 700, color: tab.key === 'home' ? '#fff' : 'rgba(255,255,255,0.4)' }}>{tab.label}</Text>
          </View>
        ))}
      </View>
    </View>
  )
}
