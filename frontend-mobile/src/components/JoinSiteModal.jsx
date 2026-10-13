import { useState, useEffect } from 'react'
import { View, Text } from '@tarojs/components'
import { dialog, env } from '../platform'
import { apiFetch, resolveErrorMessage } from '../services/api'

// #2216: 加入网点（定向邀请）——共享组件，H5(Profile) 与 weapp(pages-weapp/Profile) 复用。
// 仅列出「对我的 pending 定向邀请」，一键接受（复用 /user/invitations/:id/accept）。
// 匿名邀请码方式已下线。
export default function JoinSiteModal({ visible, onClose, onAccepted }) {
  const [invites, setInvites] = useState([])
  const [loading, setLoading] = useState(false)
  const [acceptingId, setAcceptingId] = useState('')

  useEffect(() => {
    if (!visible) return
    let cancelled = false
    const load = async () => {
      setLoading(true)
      try {
        const resp = await apiFetch(`${env.apiBaseUrl}/user/invitations/pending`)
        const result = await resp.json()
        if (!cancelled && result.code === 20000) setInvites(result.data?.list || [])
      } catch {
        if (!cancelled) setInvites([])
      }
      if (!cancelled) setLoading(false)
    }
    load()
    return () => { cancelled = true }
  }, [visible])

  if (!visible) return null

  const accept = async (invite) => {
    setAcceptingId(invite.id)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/user/invitations/${invite.id}/accept`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.toast('加入成功', { icon: 'success' })
        const rest = invites.filter(i => i.id !== invite.id)
        setInvites(rest)
        if (rest.length === 0) {
          if (onAccepted) onAccepted()
          else onClose()
        }
      } else {
        dialog.toast(resolveErrorMessage(result, '加入失败'))
      }
    } catch {
      dialog.toast('网络错误，请重试')
    }
    setAcceptingId('')
  }

  return (
    <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.45)' }}>
      <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 20, width: '85%', boxSizing: 'border-box' }}>
        <Text style={{ fontSize: 17, fontWeight: '900', color: '#18181b', display: 'block', marginBottom: 8 }}>加入网点</Text>
        <Text style={{ fontSize: 12, color: '#71717a', display: 'block', marginBottom: 14 }}>管理员邀请你加入以下网点，确认后员工身份将加到你的当前账户</Text>

        {loading ? (
          <Text style={{ fontSize: 14, color: '#a1a1aa', display: 'block', textAlign: 'center', padding: '16px 0' }}>加载中...</Text>
        ) : invites.length === 0 ? (
          <Text style={{ fontSize: 14, color: '#a1a1aa', display: 'block', textAlign: 'center', padding: '16px 0' }}>暂无待接受的邀请</Text>
        ) : (
          invites.map(invite => (
            <View key={invite.id} style={{ border: '1px solid #e4e4e7', borderRadius: 10, padding: 12, marginBottom: 10, display: 'flex', flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' }}>
              <View style={{ flex: 1, minWidth: 0 }}>
                <Text style={{ fontSize: 15, fontWeight: '700', color: '#18181b', display: 'block' }}>{invite.name || '商户'}</Text>
                <Text style={{ fontSize: 12, color: '#71717a', display: 'block', marginTop: 2 }}>{invite.role_label || invite.role}</Text>
              </View>
              <View
                onClick={acceptingId ? undefined : () => accept(invite)}
                style={{ paddingLeft: 16, paddingRight: 16, height: 34, backgroundColor: acceptingId === invite.id ? '#c4b5a5' : '#915F38', borderRadius: 17, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
              >
                <Text style={{ color: '#fff', fontSize: 13, fontWeight: '700' }}>{acceptingId === invite.id ? '处理中...' : '接受'}</Text>
              </View>
            </View>
          ))
        )}

        <View onClick={onClose} style={{ width: '100%', height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', marginTop: 4 }}>
          <Text style={{ color: '#a1a1aa', fontSize: 14 }}>关闭</Text>
        </View>
      </View>
    </View>
  )
}
