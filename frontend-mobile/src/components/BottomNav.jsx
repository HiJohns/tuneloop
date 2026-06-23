import { View, Text } from '@tarojs/components'

export default function BottomNav({ tabs = [], active = '' }) {
  return (
    <View className="absolute bottom-0 left-0 right-0 bg-[#5A3B24] border-t border-[#4E321E] py-2 flex justify-around items-center z-50 shadow-2xl">
      {tabs.map((tab, i) => {
        const isActive = active === tab.key
        return (
          <View key={tab.key || i} className="flex flex-col items-center justify-center" onClick={tab.onClick}>
            <Text className="text-xl mb-0.5">{tab.icon}</Text>
            <Text className={`text-[10px] font-bold ${isActive ? 'text-white' : 'text-white/40'}`}>{tab.label}</Text>
          </View>
        )
      })}
    </View>
  )
}
