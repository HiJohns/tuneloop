import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../../services/api'
import { env, dialog, getInputValue, storage, session } from '../../platform'
import PageShell from '../../components/PageShell'

const USERNAME_RE = /^[A-Za-z0-9_]{4,32}$/

const labelStyle = { fontSize: 13, color: '#52525b', marginBottom: 6, display: 'block' }
const inputStyle = {
  width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 10,
  paddingLeft: 12, paddingRight: 12, boxSizing: 'border-box', fontSize: 14,
  backgroundColor: '#fff',
}
const primaryBtn = {
  width: '100%', height: 48, margin: 0, padding: 0, backgroundColor: '#915F38',
  color: '#fff', borderRadius: 12, border: 'none', fontSize: 16, fontWeight: '700',
  display: 'flex', alignItems: 'center', justifyContent: 'center',
}

function validatePassword(pw) {
  if (pw.length < 8) return '密码长度不能少于 8 位'
  if (!/[A-Z]/.test(pw)) return '密码必须包含大写字母'
  if (!/[a-z]/.test(pw)) return '密码必须包含小写字母'
  if (!/[0-9]/.test(pw)) return '密码必须包含数字'
  return ''
}

export default function Credentials() {
  const baseUrl = env.apiBaseUrl
  const [username, setUsername] = useState('')
  const [phone, setPhone] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [needLogin, setNeedLogin] = useState(false)

  useEffect(() => {
    const params = Taro.getCurrentInstance().router?.params || {}
    const queryUsername = params.username ? decodeURIComponent(params.username) : ''
    if (!getToken()) {
      setNeedLogin(true)
      setLoading(false)
      return
    }
    const load = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000 && result.data) {
          const currentPhone = result.data.phone || ''
          const currentUsername = result.data.username || ''
          setPhone(currentPhone)
          setUsername(queryUsername || (currentUsername && currentUsername !== currentPhone ? currentUsername : currentPhone))
        } else if (result.code === 40104 || result.code === 40100) {
          setNeedLogin(true)
        } else {
          dialog.alert(resolveErrorMessage(result, '加载账号信息失败'))
        }
      } catch {
        dialog.alert('网络错误，请重试')
      }
      setLoading(false)
    }
    load()
  }, [baseUrl])

  const goLogin = () => {
    Taro.switchTab({ url: '/pages-weapp/profile/index' })
  }

  const submit = async () => {
    const name = username.trim()
    if (!USERNAME_RE.test(name)) {
      dialog.alert('用户名需为 4-32 位字母、数字或下划线')
      return
    }
    if (phone && name === phone) {
      dialog.alert('用户名不能与手机号相同，请设置一个专属用户名')
      return
    }
    const pwErr = validatePassword(password)
    if (pwErr) {
      dialog.alert(pwErr)
      return
    }
    if (password !== confirmPassword) {
      dialog.alert('两次输入的密码不一致')
      return
    }
    if (!await dialog.confirm(`确认使用「${name}」作为登录用户名？`)) return

    setSaving(true)
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`, {
        method: 'PUT',
        body: JSON.stringify({ username: name, password }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        storage.removeItem('token')
        storage.removeItem('token_expiry')
        storage.removeItem('refresh_token')
        storage.removeItem('login_contexts')
        storage.removeItem('login_context')
        session.removeItem('wx_accounts_prefetched')
        await dialog.alert('设置成功，请使用新用户名和密码重新登录。', { title: '设置成功' })
        Taro.reLaunch({ url: '/pages-weapp/profile/index' })
        return
      }
      dialog.alert(resolveErrorMessage(result, '设置失败，请重试'))
    } catch {
      dialog.alert('网络错误，请重试')
    }
    setSaving(false)
  }

  return (
    <PageShell>
      <View style={{ minHeight: '100vh', backgroundColor: '#fafafa', padding: 20, boxSizing: 'border-box' }}>
        <Text style={{ display: 'block', fontSize: 22, fontWeight: '900', color: '#18181b', marginBottom: 6 }}>设置登录账号</Text>
        <Text style={{ display: 'block', fontSize: 13, color: '#71717a', lineHeight: '20px', marginBottom: 20 }}>
          用于 PC 管理后台登录。设置后请使用新的用户名和密码重新登录。
        </Text>

        {needLogin ? (
          <View style={{ backgroundColor: '#fff', borderRadius: 12, padding: 20 }}>
            <Text style={{ display: 'block', fontSize: 14, color: '#52525b', marginBottom: 16 }}>请先登录后再设置登录账号。</Text>
            <Button onClick={goLogin} style={primaryBtn}>去登录</Button>
          </View>
        ) : loading ? (
          <Text style={{ display: 'block', fontSize: 14, color: '#a1a1aa' }}>加载中...</Text>
        ) : (
          <View style={{ backgroundColor: '#fff', borderRadius: 12, padding: 20 }}>
            <Text style={labelStyle}>用户名</Text>
            <Input
              value={username}
              onInput={e => setUsername(getInputValue(e))}
              placeholder="4-32 位字母、数字或下划线"
              {...(env.isMiniProgram ? { maxlength: 32 } : { maxLength: 32 })}
              style={inputStyle}
            />
            <Text style={{ display: 'block', fontSize: 12, color: '#a1a1aa', marginTop: 6, marginBottom: 16 }}>
              用户名不能与手机号相同
            </Text>

            <Text style={labelStyle}>密码</Text>
            <Input
              value={password}
              {...(env.isMiniProgram ? { password: true, maxlength: 64 } : { type: 'password', maxLength: 64 })}
              onInput={e => setPassword(getInputValue(e))}
              placeholder="至少 8 位，含大小写字母和数字"
              style={inputStyle}
            />
            <Text style={{ display: 'block', fontSize: 12, color: '#a1a1aa', marginTop: 6, marginBottom: 16 }}>
              至少 8 位，且包含大写字母、小写字母和数字
            </Text>

            <Text style={labelStyle}>确认密码</Text>
            <Input
              value={confirmPassword}
              {...(env.isMiniProgram ? { password: true, maxlength: 64 } : { type: 'password', maxLength: 64 })}
              onInput={e => setConfirmPassword(getInputValue(e))}
              placeholder="请再次输入密码"
              style={inputStyle}
            />

            <View style={{ height: 24 }} />
            <Button onClick={submit} disabled={saving} style={{ ...primaryBtn, opacity: saving ? 0.6 : 1 }}>
              {saving ? '处理中...' : '保存登录账号'}
            </Button>
          </View>
        )}
      </View>
    </PageShell>
  )
}
