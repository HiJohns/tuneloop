import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { wxLogin, storage, session, env, request, navigation } from '../../platform'
import { maskOpenid } from '../../utils/wechatBind.mjs'
import { resolveLogin } from '../../services/api'

const primaryBtn = {
  backgroundColor: '#915F38',
  color: '#fff',
  borderRadius: 22,
  height: 48,
  padding: '0 32px',
  fontSize: 14,
  fontWeight: '700',
  border: 'none',
  margin: 0,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
}

const secondaryBtn = {
  ...primaryBtn,
  backgroundColor: 'transparent',
  color: '#52525b',
  border: '1px solid #d4d4d8',
  marginTop: 10,
}

export default function Bind() {
  const [status, setStatus] = useState('loading')
  const [bindToken, setBindToken] = useState('')
  const [mergeInfo, setMergeInfo] = useState(null)
  const [mergeResult, setMergeResult] = useState(null)

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
    setBindToken(token)
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
        // #2410 D3 / #2445：该微信已绑定其他账户 → 在 Bind 页直接确认合并，
        // 不再把员工引到「我的 → 编辑资料」的不可达路径。
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

  // #2445：X（管理员预建）→ obsolete，Y（员工个人账户）保留并接收网点身份。
  const doMerge = async () => {
    if (!bindToken || !mergeInfo?.merge_ticket) {
      setStatus('error')
      return
    }
    setStatus('merging')
    try {
      const res = await request(`${env.apiBaseUrl}/wechat-bind/merge`, {
        method: 'POST',
        body: JSON.stringify({ token: bindToken, merge_ticket: mergeInfo.merge_ticket }),
      })
      const result = await res.json()
      if (result.code === 20000 && result.data?.merged) {
        setMergeResult(result.data)
        setStatus('merged')
        return
      }
      setStatus('merge_error')
    } catch {
      setStatus('merge_error')
    }
  }

  // 合并后当前 token 的员工上下文已变化：清会话，重新微信登录时进入身份选择
  // （顾客 + 网点），个人中心随后显示「切换身份」。
  const goRelogin = async () => {
    storage.removeItem('token')
    storage.removeItem('token_expiry')
    storage.removeItem('refresh_token')
    storage.removeItem('login_contexts')
    storage.removeItem('login_context')
    session.removeItem('wx_accounts_prefetched')
    session.removeItem('wx_login_token')
    // 合并后 Y 具备「顾客 + 网点」上下文：直接拉起登录，进入身份选择页。
    const ok = await resolveLogin('profile')
    if (ok) Taro.reLaunch({ url: '/pages-weapp/profile/index' })
  }

  return (
    <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa', padding: 32, boxSizing: 'border-box' }}>
      {status === 'loading' && <Text style={{ color: '#a1a1aa' }}>正在绑定...</Text>}
      {status === 'merging' && <Text style={{ color: '#a1a1aa' }}>正在合并账户...</Text>}

      {status === 'success' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>✅</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>绑定成功</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24 }}>PC 端将自动刷新</Text>
          <Button onClick={() => navigation.safeBack()} style={primaryBtn}>返回</Button>
        </View>
      )}

      {status === 'needs_merge' && (
        <View style={{ alignItems: 'center', padding: 12, width: '100%', boxSizing: 'border-box' }}>
          <Text style={{ display: 'block', fontSize: 48, marginBottom: 16 }}>🔗</Text>
          <Text style={{ display: 'block', fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8, textAlign: 'center' }}>该微信已绑定个人账户</Text>
          <Text style={{ display: 'block', fontSize: 14, color: '#71717a', marginBottom: 12, textAlign: 'center', lineHeight: '22px' }}>
            继续将把管理员为你创建的员工账户合并到当前微信的个人账户：
            员工账户作废，网点身份与角色转入个人账户。合并后可重新登录并切换为员工身份。
          </Text>
          {maskOpenid(mergeInfo?.openid) ? (
            <Text style={{ display: 'block', fontSize: 12, color: '#a1a1aa', marginBottom: 16, textAlign: 'center' }}>
              绑定编号：{maskOpenid(mergeInfo?.openid)}
            </Text>
          ) : null}
          {mergeInfo?.merge_ticket ? (
            <View style={{ width: '100%', display: 'flex', flexDirection: 'column' }}>
              <Button onClick={doMerge} style={primaryBtn}>合并到我的个人账户</Button>
              <Button onClick={() => navigation.safeBack()} style={secondaryBtn}>取消</Button>
            </View>
          ) : (
            <>
              <Text style={{ display: 'block', fontSize: 13, color: '#b45309', marginBottom: 16, textAlign: 'center', lineHeight: '20px' }}>
                当前服务端未返回合并凭证，请让管理员重新生成绑定二维码后再扫码。
              </Text>
              <Button onClick={() => navigation.safeBack()} style={primaryBtn}>返回</Button>
            </>
          )}
        </View>
      )}

      {status === 'merged' && (
        <View style={{ alignItems: 'center', width: '100%', boxSizing: 'border-box' }}>
          <Text style={{ display: 'block', fontSize: 48, marginBottom: 16 }}>✅</Text>
          <Text style={{ display: 'block', fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8, textAlign: 'center' }}>合并成功</Text>
          <Text style={{ display: 'block', fontSize: 14, color: '#71717a', marginBottom: 12, textAlign: 'center', lineHeight: '22px' }}>
            员工身份已转入你的个人账户。请重新登录小程序，在「我的」页点击「切换身份」进入员工身份。
          </Text>
          {mergeResult?.needs_credentials ? (
            <>
              <Text style={{ display: 'block', fontSize: 13, color: '#b45309', marginBottom: 12, textAlign: 'center', lineHeight: '20px' }}>
                你还没有设置登录用户名/密码，设置后可用于 PC 后台登录。
              </Text>
              <Button
                onClick={() => navigation.navigateTo(
                  `/pages-weapp/credentials/index?username=${encodeURIComponent(mergeResult.suggested_username || '')}`
                )}
                style={{ ...primaryBtn, marginBottom: 10 }}
              >
                设置登录账号
              </Button>
            </>
          ) : null}
          <Button onClick={goRelogin} style={primaryBtn}>去登录</Button>
        </View>
      )}

      {status === 'merge_error' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>❌</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>合并失败</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24, textAlign: 'center', lineHeight: '22px' }}>
            合并凭证可能已过期。请让管理员重新生成绑定二维码后再扫码。
          </Text>
          <Button onClick={() => navigation.safeBack()} style={primaryBtn}>返回</Button>
        </View>
      )}

      {status === 'error' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>❌</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>绑定失败</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24 }}>请重试</Text>
          <Button onClick={() => navigation.safeBack()} style={primaryBtn}>返回</Button>
        </View>
      )}
    </View>
  )
}
