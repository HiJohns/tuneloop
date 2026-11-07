import { View, Text } from '@tarojs/components'

// Reproduces #2298: the map parameter `t` shadows the i18n `t` function.
// The smoke harness must report this as a fatal render error.
export function DefectiveTier() {
  return (
    <View>
      {[{ days: 1 }].map((t, i) => (
        <Text key={i}>{t('/天')}</Text>
      ))}
    </View>
  )
}
