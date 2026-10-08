import { useNavigate } from 'react-router-dom'
import { View, Text } from '@tarojs/components'
import { env } from '../platform'
import BottomNav from '../components/BottomNav'
import BottomNavWeapp from '../components-weapp/BottomNav'

// #2175: 商城占位页（底条 tab）——「租赁」tab 改为「商城」，
// 「我的租赁」降为个人中心二级入口。
export default function Mall() {
  const navigate = useNavigate()
  return (
    <View style={{ display: 'flex', flexDirection: 'column', height: '100vh', backgroundColor: '#FDFBF7' }}>
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', textAlign: 'center' }}>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>商城</Text>
        </View>
      )}
      <View style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingBottom: 60 }}>
        <Text style={{ fontSize: 15, color: '#A1A1AA' }}>建设中，敬请期待</Text>
      </View>
      {env.isMiniProgram
        ? <BottomNavWeapp active="mall" />
        : <BottomNav active="mall" navigate={navigate} />}
    </View>
  )
}
