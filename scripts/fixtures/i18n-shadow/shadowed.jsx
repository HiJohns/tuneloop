import { View, Text } from '@tarojs/components'
import { t } from '../../../frontend-mobile/src/i18n'

export default function ShadowedFixture({ tiers }) {
  return (
    <View>
      {tiers.map((t, i) => (
        <Text key={i}>第{t.tier}阶梯</Text>
      ))}
      <Text>未遮蔽文案</Text>
    </View>
  )
}
