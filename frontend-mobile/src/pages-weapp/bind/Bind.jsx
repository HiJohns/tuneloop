import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { wxLogin, storage, env, request, navigation } from '../../platform'
import { maskOpenid } from '../../utils/wechatBind.mjs'

export default function Bind() {
  const [status, setStatus] = useState('loading')
  const [mergeInfo, setMergeInfo] = useState(null)

  useEffect(() => {
    const params = Taro.getCurrentInstance().router?.params || {}
    // Two entry paths: direct query param (?token=) and mini-program code
    // scene (scene=bind_<token>, URL-encoded). See #1640.
    let token = params.token || ''
    if (!token && params.scene) {
      const decoded = decodeURIComponent(params.scene)
      if (decoded.startsWith('bind_')) token = decoded.slice(5)
    }
    if (!token) {
      setStatus('error')
      return
    }
    doBind(token)
  }, [])

  const doBind = async (token) => {
    try {
      const code = await wxLogin()
      if (!code) { setStatus('error'); return }
      const res = await request(`${env.apiBaseUrl}/auth/wx-accounts?code=${encodeURIComponent(code)}`)
      const result = await res.json()
      const openid = result?.data?.openid
      if (result.code === 20000 && openid) {
        const payload = { token, wx_openid: openid }
        if (result.data.exchange_token) payload.exchange_token = result.data.exchange_token
        const confRes = await request(`${env.apiBaseUrl}/wechat-bind/confirm`, {
          method: 'POST',
          body: JSON.stringify(payload),
        })
        const confResult = await confRes.json()
        if (confResult.code === 20000) {
          setStatus('success')
          return
        }
        // #2410 D3：该微信已绑定其他账户 → 明确引导合并，不显示绑定成功。
        if (confResult.code === 40900 && confResult.data?.needs_merge) {
          setMergeInfo(confResult.data)
          setStatus('needs_merge')
          return
        }
      }
      setStatus('error')
    } catch {
      setStatus('error')
    }
  }

  return (
    <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa', padding: 32 }}>
      {status === 'loading' && <Text style={{ color: '#a1a1aa' }}>正在绑定...</Text>}
      {status === 'success' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>✅</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>绑定成功</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24 }}>PC 端将自动刷新</Text>
          <Button onClick={() => navigation.safeBack()} style={{ backgroundColor: '#915F38', color: '#fff', borderRadius: 22, padding: '12px 32px', fontSize: 14, fontWeight: '700', border: 'none' }}>
            返回
          </Button>
        </View>
      )}
      {status === 'needs_merge' && (
        <View style={{ alignItems: 'center', padding: 12, width: '100%', boxSizing: 'border-box' }}>
          <View style={{ display: 'block', width: '100%', textAlign: 'center' }}>
            <Text style={{ display: 'block', fontSize: 48, marginBottom: 16 }}>🔗</Text>
          </View>
          <View style={{ display: 'block', width: '100%' }}>
            <Text style={{ display: 'block', fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8, textAlign: 'center' }}>该微信已绑定其他账户</Text>
          </View>
          <View style={{ display: 'block', width: '100%' }}>
            <Text style={{ display: 'block', fontSize: 14, color: '#71717a', marginBottom: 12, textAlign: 'center', lineHeight: '22px' }}>
              请前往「我的 → 编辑资料 → 微信绑定」确认合并；合并后当前账户的组织关系与密码将迁移至该微信账户。
            </Text>
          </View>
          {maskOpenid(mergeInfo?.openid) ? (
            <View style={{ display: 'block', width: '100%' }}>
              <Text style={{ display: 'block', fontSize: 12, color: '#a1a1aa', marginBottom: 16, textAlign: 'center' }}>
                绑定编号：{maskOpenid(mergeInfo?.openid)}
              </Text>
            </View>
          ) : null}
          <Button onClick={() => navigation.safeBack()} style={{ backgroundColor: '#915F38', color: '#fff', borderRadius: 22, padding: '12px 32px', fontSize: 14, fontWeight: '700', border: 'none', margin: 0 }}>
            返回
          </Button>
        </View>
      )}
      {status === 'error' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>❌</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>绑定失败</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24 }}>请重试</Text>
          <Button onClick={() => navigation.safeBack()} style={{ backgroundColor: '#915F38', color: '#fff', borderRadius: 22, padding: '12px 32px', fontSize: 14, fontWeight: '700', border: 'none' }}>
            返回
          </Button>
        </View>
      )}
    </View>
  )
}
