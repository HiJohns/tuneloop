import { useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { storage, eventBus } from '../platform'

export default function Success() {
  useEffect(() => {
    storage.removeItem('cart')
    eventBus.emit('cartUpdated')
  }, [])

  return (
    <View style={{ height: '100vh', width: '100vw', overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative', backgroundColor: '#fafafa' }}>
      <View style={{ flex: '1 1 0%', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingLeft: 32, paddingRight: 32 }}>
        <View style={{ width: 128, height: 128, borderRadius: '50%', backgroundColor: '#dcfce7', display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 24 }}>
          <Text style={{ fontSize: 48 }}>🎉</Text>
        </View>
        <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', letterSpacing: '0.025em', marginBottom: 8 }}>付款完成！</Text>
        <Text style={{ fontSize: 16, color: '#a1a1aa', fontWeight: '500', marginBottom: 32 }}>感谢您的租赁，祝您使用愉快</Text>
        <Text
          style={{ color: '#2563eb', fontWeight: '700', fontSize: 14, borderBottom: '1px solid #2563eb', paddingBottom: 2 }}
          onClick={() => { Taro.navigateTo({ url: '/pages-weapp/home/index' }) }}
        >
          返回首页
        </Text>
      </View>
    </View>
  )
}
