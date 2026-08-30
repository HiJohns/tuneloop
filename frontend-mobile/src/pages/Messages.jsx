import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { notificationApi } from '../services/api'
import { dialog, env } from '../platform'
import { ArrowLeft, Bell } from 'lucide-react'
import { View, Text, ScrollView } from '@tarojs/components'
const typeConfig = {
  damage: { bgColor: '#fee2e2', textColor: '#dc2626', label: '定损通知' },
  appeal: { bgColor: '#ffedd5', textColor: '#ea580c', label: '申诉通知' },
  refund: { bgColor: '#dcfce7', textColor: '#16a34a', label: '退款通知' },
  payment: { bgColor: '#dbeafe', textColor: '#2563eb', label: '支付通知' },
  order: { bgColor: '#f4f4f5', textColor: '#52525b', label: '系统通知' },
  invoice: { bgColor: '#f3e8ff', textColor: '#9333ea', label: '发票通知' },
}

export default function Messages() {
  const navigate = useNavigate()
  const [notifications, setNotifications] = useState([])
  const [loading, setLoading] = useState(true)

  const fetchNotifications = async () => {
    try {
      const resp = await notificationApi.list()
      // #1807: api.js request → processApiResponse 已将 {code,data:{list}} 解包为
      // 数组返回（data.data.list 分支）——resp 本身就是数组；res?.data?.list
      // 会得到 undefined → 空列表（此前误改导致消息页「暂无消息」）。
      setNotifications(Array.isArray(resp) ? resp : [])
    } catch (err) {
      console.error('Failed to fetch notifications:', err)
    }
    setLoading(false)
  }

  useEffect(() => {
    fetchNotifications()
  }, [])

  // #1807: weapp 从详情页返回（navigateBack）时列表不重新挂载——
  // useDidShow 在页面每次显示时刷新，打开详情后返回未读标记才消失。
  Taro.useDidShow(() => {
    if (env.isMiniProgram) fetchNotifications()
  })

  const markRead = async (id) => {
    try {
      await notificationApi.markRead(id)
      setNotifications(prev => prev.map(n => n.id === id ? { ...n, status: 'read' } : n))
    } catch (err) {
      console.error('Failed to mark read:', err)
    }
  }

  const markAllRead = async () => {
    try {
      await notificationApi.markAllRead()
      setNotifications(prev => prev.map(n => ({ ...n, status: 'read' })))
      dialog.toast('已全部标记为已读')
    } catch (err) {
      console.error('Failed to mark all read:', err)
    }
  }

  const handleClick = (notif) => {
    if (env.isMiniProgram) {
      Taro.navigateTo({ url: `/pages-weapp/message-detail/index?id=${notif.id}` })
    } else {
      navigate(`/message-detail?id=${notif.id}`)
    }
  }

  const unreadCount = notifications.filter(n => n.status === 'unread').length

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-20">
      {/* 手写顶条仅 H5（无原生导航栏）；weapp 用原生导航栏（#1706） */}
      {!env.isMiniProgram && (
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <ArrowLeft size={20} className="text-black cursor-pointer" onClick={() => navigate(-1)} />
        <Text className="text-lg font-black text-black flex-1">消息</Text>
        {unreadCount > 0 && (
          <Text className="text-sm text-brand-primary ml-auto cursor-pointer" onClick={markAllRead}>全部已读</Text>
        )}
      </View>
      )}
      {env.isMiniProgram && unreadCount > 0 && (
        <View style={{ padding: '12px 16px 0', display: 'flex', justifyContent: 'flex-end' }}>
          <Text style={{ fontSize: 14, color: '#915F38', cursor: 'pointer' }} onClick={markAllRead}>全部已读</Text>
        </View>
      )}

      <ScrollView style={{ flex: 1 }}>
        <View style={{ padding: 16, boxSizing: 'border-box' }}>
        {loading ? (
          <Text style={{ textAlign: 'center', padding: '32px 0', color: '#71717a', display: 'block' }}>加载中...</Text>
        ) : notifications.length === 0 ? (
          <View style={{ textAlign: 'center', padding: '64px 0' }}>
            <Bell size={48} style={{ color: '#d1d5db', margin: '0 auto 16px' }} />
            <Text style={{ color: '#71717a' }}>暂无消息</Text>
          </View>
        ) : (
          <View>
            {unreadCount > 0 && (
              <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 8 }}>{unreadCount} 条未读</Text>
            )}
            <View>
              {notifications.map(notif => {
                const type = typeConfig[notif.type] || typeConfig.order
                return (
                  <View
                    key={notif.id}
                    style={{ backgroundColor: '#fff', borderRadius: 12, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', marginBottom: 12, maxWidth: 600, marginLeft: 'auto', marginRight: 'auto', boxSizing: 'border-box', overflow: 'hidden',
                      borderLeft: notif.status === 'unread' ? '4px solid #915F38' : 'none' }}
                    onClick={() => handleClick(notif)}
                  >
                    <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 4 }}>
                      <Text style={{ fontSize: 12, paddingHorizontal: 8, paddingVertical: 2, borderRadius: 4, backgroundColor: type.bgColor || '#f4f4f5', color: type.textColor || '#71717a' }}>
                        {type.label}
                      </Text>
                      {notif.status === 'unread' && (
                        <View style={{ width: 8, height: 8, borderRadius: 4, backgroundColor: '#915F38' }} />
                      )}
                    </View>
                    <Text style={{ fontWeight: '500', fontSize: 14, marginTop: 4 }}>{notif.title}</Text>
                    <Text style={{ color: '#71717a', fontSize: 14, marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' }}>{notif.content}</Text>
                    <Text style={{ color: '#a1a1aa', fontSize: 12, marginTop: 8 }}>{new Date(notif.created_at).toLocaleString()}</Text>
                  </View>
                )
              })}
            </View>
          </View>
        )}
        </View>
      </ScrollView>
    </View>
  )
}
