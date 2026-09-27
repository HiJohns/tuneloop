// #1956 阶段3b / #2084：师傅工作台独立页（RS-02 deep link 保留）。
// 区块内容抽取为共享组件 TechRepairSections（MyRepairs 内联复用）。
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView } from '@tarojs/components'
import { dialog, env, toWeappRoute } from '../platform'
import TechRepairSections from '../components/TechRepairSections'

export default function TechRepairWorkbench() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务 · 维修师工作台</Text>
      </View>

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          <TechRepairSections />
        </View>
      </ScrollView>
    </View>
  )
}
