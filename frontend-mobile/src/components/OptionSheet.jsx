import { View, Text, ScrollView } from '@tarojs/components'

// #1893: shared cross-end bottom-sheet option picker. Extracted from the
// StaffInstrumentForm inline sheet so instrument screens share one control.
// The option list uses a fixed pixel height (not max-height): weapp scroll-view
// needs a definite height to scroll.
export default function OptionSheet({ title, options = [], onSelect, onClose }) {
  const listHeight = Math.min(Math.max(options.length, 1) * 48, 320)
  return (
    <View className="fixed inset-0 z-50 flex items-end" style={{ backgroundColor: 'rgba(0,0,0,0.5)' }} onClick={onClose}>
      <View className="bg-white rounded-t-2xl w-full p-4" onClick={e => e.stopPropagation()}>
        <Text className="text-sm font-bold text-black mb-3">{title}</Text>
        {options.length === 0 ? (
          <View className="py-3 border-b border-gray-50">
            <Text className="text-sm text-gray-400">暂无选项</Text>
          </View>
        ) : (
          <ScrollView scrollY style={{ height: listHeight }}>
            {options.map(opt => (
              <View key={opt.id} className="py-3 border-b border-gray-50" onClick={() => onSelect(opt.id)}>
                <Text className="text-sm text-black">{opt.label}</Text>
              </View>
            ))}
          </ScrollView>
        )}
      </View>
    </View>
  )
}
