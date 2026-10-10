import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { env, getInputValue } from '../platform'

// #2193: 加入网点（邀请码）——共享组件，H5(Profile) 与 weapp(pages-weapp/Profile) 复用，
// 避免两端各写一份漂移。
export default function JoinSiteModal({ visible, onClose }) {
  const [code, setCode] = useState('')
  const [joining, setJoining] = useState(false)
  if (!visible) return null
  const submit = async () => {
    if (!code.trim()) { Taro.showToast({ title: '请输入邀请码', icon: 'none' }); return }
    setJoining(true)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/user/accept-invite`, {
        method: 'POST',
        body: JSON.stringify({ code: code.trim() }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '加入成功', icon: 'success' })
        setCode('')
        onClose()
      } else {
        Taro.showToast({ title: resolveErrorMessage(result, '加入失败'), icon: 'none' })
      }
    } catch {
      Taro.showToast({ title: '网络错误，请重试', icon: 'none' })
    }
    setJoining(false)
  }
  return (
    <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.45)' }}>
      <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 20, width: '85%', boxSizing: 'border-box' }}>
        <Text style={{ fontSize: 17, fontWeight: '900', color: '#18181b', display: 'block', marginBottom: 8 }}>加入网点</Text>
        <Text style={{ fontSize: 12, color: '#71717a', display: 'block', marginBottom: 14 }}>请输入管理员提供的邀请码（员工身份将加到你的当前账户）</Text>
        <Input
          value={code}
          onInput={e => setCode(getInputValue(e))}
          placeholder="邀请码"
          style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, marginBottom: 16, boxSizing: 'border-box' }}
        />
        <View onClick={joining ? undefined : submit} style={{ width: '100%', height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 8 }}>
          <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>{joining ? '处理中...' : '确认加入'}</Text>
        </View>
        <View onClick={onClose} style={{ width: '100%', height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Text style={{ color: '#a1a1aa', fontSize: 14 }}>取消</Text>
        </View>
      </View>
    </View>
  )
}
