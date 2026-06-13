import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { message } from 'antd'
import { notificationApi } from '../services/api'
import { ArrowLeft, Bell } from 'lucide-react'
import { View, Text, Button, ScrollView } from '@tarojs/components'

const typeConfig = {
  damage: { bg: 'bg-red-100', text: 'text-red-600', label: '定损通知' },
  appeal: { bg: 'bg-orange-100', text: 'text-orange-600', label: '申诉通知' },
  refund: { bg: 'bg-green-100', text: 'text-green-600', label: '退款通知' },
  payment: { bg: 'bg-blue-100', text: 'text-blue-600', label: '支付通知' },
  order: { bg: 'bg-gray-100', text: 'text-gray-600', label: '系统通知' },
}

export default function Messages() {
  const navigate = useNavigate()
  const [notifications, setNotifications] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchNotifications()
  }, [])

  const fetchNotifications = async () => {
    try {
      const resp = await notificationApi.list()
      setNotifications(resp?.data?.list || [])
    } catch (err) {
      console.error('Failed to fetch notifications:', err)
    }
    setLoading(false)
  }

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
      message.success('已全部标记为已读')
    } catch (err) {
      console.error('Failed to mark all read:', err)
    }
  }

  const handleClick = (notif) => {
    navigate(`/messages/${notif.id}`)
  }

  const unreadCount = notifications.filter(n => n.status === 'unread').length

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </Button>
        <Text className="text-lg font-bold flex-1">消息</Text>
        {unreadCount > 0 && (
          <Button onClick={markAllRead} className="text-sm text-white/80">全部已读</Button>
        )}
      </View>

      <ScrollView className="p-4">
        {loading ? (
          <Text className="text-center py-8 text-gray-500 block">加载中...</Text>
        ) : notifications.length === 0 ? (
          <View className="text-center py-16">
            <Bell size={48} className="mx-auto text-gray-300 mb-4" />
            <Text className="text-gray-500">暂无消息</Text>
          </View>
        ) : (
          <View>
            {unreadCount > 0 && (
              <Text className="text-sm text-gray-500 mb-2">{unreadCount} 条未读</Text>
            )}
            <View className="space-y-3">
              {notifications.map(notif => {
                const type = typeConfig[notif.type] || typeConfig.order
                return (
                  <View
                    key={notif.id}
                    className={`bg-white rounded-xl p-4 shadow-sm cursor-pointer ${
                      notif.status === 'unread' ? 'border-l-4 border-brand-primary' : ''
                    }`}
                    onClick={() => handleClick(notif)}
                  >
                    <View className="flex justify-between items-start mb-1">
                      <Text className={`text-xs px-2 py-0.5 rounded ${type.bg} ${type.text}`}>
                        {type.label}
                      </Text>
                      {notif.status === 'unread' && (
                        <Text className="w-2 h-2 rounded-full bg-brand-primary" />
                      )}
                    </View>
                    <Text className="font-medium text-sm mt-1">{notif.title}</Text>
                    <Text className="text-gray-500 text-sm mt-1 line-clamp-2">{notif.content}</Text>
                    <Text className="text-gray-400 text-xs mt-2">{new Date(notif.created_at).toLocaleString()}</Text>
                  </View>
                )
              })}
            </View>
          </View>
        )}
      </ScrollView>
    </View>
  )
}
