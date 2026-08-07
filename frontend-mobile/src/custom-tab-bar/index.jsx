// Empty shell for WeChat custom tabBar mode (tabBar.custom: true).
// The native tabBar is hidden; the real bottom navigation UI is rendered
// by components-weapp/BottomNav inside each page. This component only
// satisfies WeChat's requirement that a custom-tab-bar component exists.
import { View } from '@tarojs/components'

export default function CustomTabBar() {
  return <View style={{ height: 0, overflow: 'hidden' }} />
}
