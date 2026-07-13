import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env, storage, eventBus } from '../platform'

export default function Success() {
  const [checking, setChecking] = useState(true)
  const [paid, setPaid] = useState(false)
  const [error, setError] = useState('')
  const params = Taro.getCurrentInstance().router?.params || {}
  const orderId = params.order_id

  useEffect(() => {
    storage.removeItem('cart')
    eventBus.emit('cartUpdated')

    if (orderId && params.status === 'check') {
      verifyPayment()
    } else {
      setChecking(false)
      setPaid(true)
    }
  }, [])

  const verifyPayment = async () => {
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/orders/${orderId}`)
      const result = await resp.json()
      if (result.code === 20000) {
        if (result.data?.status === 'paid' || result.data?.status === 'pending_shipment') {
          setPaid(true)
        } else {
          setPaid(false)
          setError(`订单状态: ${result.data?.status || '未知'}`)
        }
      } else {
        setError('查询失败')
      }
    } catch (err) {
      setError('网络错误')
    }
    setChecking(false)
  }

  if (checking) {
    return (
      <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}>
        <Text style={{ color: '#a1a1aa' }}>验证支付状态...</Text>
      </View>
    )
  }

  if (!paid) {
    return (
      <View style={{ height: '100vh', width: '100vw', overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative', backgroundColor: '#fafafa' }}>
        <View style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingHorizontal: 32 }}>
          <View style={{ width: 128, height: 128, borderRadius: '50%', backgroundColor: '#fee2e2', display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 24 }}>
            <Text style={{ fontSize: 48 }}>⚠️</Text>
          </View>
          <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', marginBottom: 8 }}>支付异常</Text>
          <Text style={{ fontSize: 16, color: '#a1a1aa', fontWeight: '500', marginBottom: 8, textAlign: 'center' }}>{error || '订单未标记已支付'}</Text>
          <Text style={{ fontSize: 14, color: '#2563eb', fontWeight: '700', textDecoration: 'underline' }} onClick={() => Taro.redirectTo({ url: `/pages-weapp/order-detail/index?id=${orderId}` })}>查看订单详情</Text>
        </View>
      </View>
    )
  }

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
          onClick={() => { Taro.redirectTo({ url: '/pages-weapp/home/index' }) }}
        >
          返回首页
        </Text>
      </View>
    </View>
  )
}
