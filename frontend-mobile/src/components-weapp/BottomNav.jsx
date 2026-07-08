import { View, Text } from '@tarojs/components'

export default function BottomNav({ tabs = [], active = '', badges = {} }) {
  return (
    <View style={{ position: 'absolute', bottom: 0, left: 0, right: 0, backgroundColor: '#5A3B24', borderTop: '1px solid #4E321E', paddingTop: 8, paddingBottom: 8, display: 'flex', justifyContent: 'space-around', alignItems: 'center', zIndex: 50 }}>
      {tabs.map((tab, i) => {
        const isActive = active === tab.key
        const badge = badges[tab.key]
        return (
          <View key={tab.key || i} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', position: 'relative' }} onClick={tab.onClick}>
            <View style={{ fontSize: 20, marginBottom: 2, position: 'relative' }}>
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
