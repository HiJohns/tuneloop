import { View, Text } from '@tarojs/components'

export default function BottomNav({ tabs = [], active = '', badges = {} }) {
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
