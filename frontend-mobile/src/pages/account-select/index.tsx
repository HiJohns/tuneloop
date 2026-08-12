import { View, Text } from '@tarojs/components'
import { useNavigate } from 'react-router-dom'

// H5 placeholder for /account-select (#1639): H5 keeps IAM OAuth login and
// does not implement WeChat account switching (separate beaconiam QR-login
// issue). Route exists so weapp-style links never 404 on H5.
export default function AccountSelectPlaceholder() {
  const navigate = useNavigate()
  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#fafafa', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', padding: 32 }}>
      <Text style={{ fontSize: 40, marginBottom: 16 }}>👤</Text>
      <Text style={{ fontSize: 16, fontWeight: '700', color: '#18181b', marginBottom: 8 }}>请使用网页版登录</Text>
      <Text style={{ fontSize: 13, color: '#a1a1aa', textAlign: 'center', marginBottom: 24 }}>
        微信账户切换仅在小程序中支持，请打开微信小程序后重试
      </Text>
      <View onClick={() => navigate('/')}
        style={{ width: 200, height: 40, backgroundColor: '#915F38', borderRadius: 20, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>返回首页</Text>
      </View>
    </View>
  )
}
