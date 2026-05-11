import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch, getToken } from '../services/api'
import { ArrowLeft, Bell } from 'lucide-react'

export default function Messages() {
  const navigate = useNavigate()
  const [notifications, setNotifications] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchNotifications = async () => {
      try {
        const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
        const resp = await apiFetch(`${baseUrl}/notifications`)
        const result = await resp.json()
        if (result.code === 20000) {
          setNotifications(result.data?.list || [])
        }
      } catch (err) {
        console.error('Failed to fetch notifications:', err)
      }
      setLoading(false)
    }
    fetchNotifications()
  }, [])

  const markRead = async (id) => {
    try {
      const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
      await apiFetch(`${baseUrl}/notifications/${id}/read`, { method: 'POST' })
      setNotifications(prev => prev.map(n => n.id === id ? { ...n, status: 'read' } : n))
    } catch (err) {
      console.error('Failed to mark read:', err)
    }
  }

  const typeLabel = {
    damage: '定损通知',
    appeal: '申诉通知',
    refund: '退款通知',
    general: '系统通知',
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">Messages</h1>
      </div>

      <div className="p-4">
        {loading ? (
          <div className="text-center py-8 text-gray-500">Loading...</div>
        ) : notifications.length === 0 ? (
          <div className="text-center py-16">
            <Bell size={48} className="mx-auto text-gray-300 mb-4" />
            <p className="text-gray-500">No messages</p>
          </div>
        ) : (
          <div className="space-y-3">
            {notifications.map(notif => (
              <div
                key={notif.id}
                className={`bg-white rounded-xl p-4 shadow-sm ${notif.status === 'unread' ? 'border-l-4 border-brand-primary' : ''}`}
                onClick={() => notif.status === 'unread' && markRead(notif.id)}
              >
                <div className="flex justify-between items-start mb-1">
                  <span className={`text-xs px-2 py-0.5 rounded ${
                    notif.type === 'damage' ? 'bg-red-100 text-red-600' : 'bg-blue-100 text-blue-600'
                  }`}>
                    {typeLabel[notif.type] || notif.type}
                  </span>
                  {notif.status === 'unread' && (
                    <span className="w-2 h-2 rounded-full bg-brand-primary"></span>
                  )}
                </div>
                <h3 className="font-medium text-sm mt-1">{notif.title}</h3>
                <p className="text-gray-500 text-sm mt-1">{notif.content}</p>
                <p className="text-gray-400 text-xs mt-2">{new Date(notif.created_at).toLocaleString()}</p>

                {notif.type === 'damage' && (
                  <div className="flex gap-2 mt-3">
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        alert('Damage accepted. Refund process will begin.')
                      }}
                      className="flex-1 py-1.5 bg-green-500 text-white rounded text-sm"
                    >
                      Accept
                    </button>
                    <button
                      onClick={(e) => {
                        e.stopPropagation()
                        const reason = prompt('Reason for appeal:')
                        if (reason) {
                          alert('Appeal submitted.')
                        }
                      }}
                      className="flex-1 py-1.5 bg-red-500 text-white rounded text-sm"
                    >
                      Reject
                    </button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
