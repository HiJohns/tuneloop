import { View, Text } from '@tarojs/components'

// #2150 统一分段控制器（页面级 Tab / 单选切换）。
// 跨端（H5 + weapp）；weapp 样式禁区（#1831）：全部内联 style，禁 Tailwind 任意值/分数/变体类。
// 用法：<SegmentedTabs options={[{key,label}]} value={cur} onChange={(key)=>...} />
export default function SegmentedTabs({ options = [], value, onChange, style }) {
  return (
    <View style={{ display: 'flex', backgroundColor: '#F4F4F5', borderRadius: 10, padding: 3, gap: 3, ...(style || {}) }}>
      {options.map((t) => {
        const active = value === t.key
        return (
          <View key={t.key} onClick={() => { if (onChange) onChange(t.key) }}
            style={{
              flex: 1, height: 30, display: 'flex', alignItems: 'center', justifyContent: 'center',
              borderRadius: 8,
              backgroundColor: active ? '#171717' : 'transparent',
              boxShadow: active ? '0 1px 2px rgba(0,0,0,0.08)' : 'none',
            }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: active ? '#FFFFFF' : '#52525B' }}>{t.label}</Text>
          </View>
        )
      })}
    </View>
  )
}
