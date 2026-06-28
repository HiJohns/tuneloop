import { View, Text } from '@tarojs/components'

export default function BottomNav({ tabs = [], active = '', badges = {} }) {
  return (
    <View className="absolute bottom-0 left-0 right-0 bg-[#5A3B24] border-t border-[#4E321E] py-2 flex justify-around items-center z-50 shadow-2xl">
      {tabs.map((tab, i) => {
        const isActive = active === tab.key
        const badge = badges[tab.key]
        return (
          <View key={tab.key || i} className="flex flex-col items-center justify-center relative" onClick={tab.onClick}>
            <View className="text-xl mb-0.5 relative">
              {tab.icon}
              {badge > 0 && (
                <View className="absolute -top-1 -right-2 bg-[#FF2A55] text-white text-[9px] font-black min-w-[16px] h-4 rounded-full flex items-center justify-center px-1 border border-[#5A3B24]">
                  {badge > 99 ? '99+' : badge}
                </View>
              )}
            </View>
            <Text className={`text-[10px] font-bold ${isActive ? 'text-white' : 'text-white/40'}`}>{tab.label}</Text>
          </View>
        )
      })}
    </View>
  )
}
