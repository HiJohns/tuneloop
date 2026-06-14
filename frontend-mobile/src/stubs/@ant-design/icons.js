import { View, Text } from '@tarojs/components'

function createIcon(emoji) {
  const IconComponent = ({ style, className, ...props }) => (
    <Text style={{ fontSize: 16, ...style }} className={className} {...props}>
      {emoji}
    </Text>
  )
  return IconComponent
}

export const EnvironmentOutlined = createIcon('📍')
export const PhoneOutlined = createIcon('📞')
export const ClockCircleOutlined = createIcon('⏱')
export const CheckCircleFilled = createIcon('✓')
