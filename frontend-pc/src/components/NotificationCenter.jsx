import { useCallback, useEffect, useRef, useState } from 'react'
import { Badge, Button, Empty, List, Popover, Spin, Tag, Typography } from 'antd'
import { BellOutlined, CheckOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { notificationsApi } from '../services/api'

// #2114 PC 消息中心：顶栏铃铛 + 未读徽标 + 下拉列表 + 按 action_type 动作直达。
// 复用既有 /notifications 端点（notifications 表）。
const ACTION_ROUTES = {
  invite_manage: '/staff/invites',
}

const TYPE_TAG = {
  invite_application: { color: 'orange', text: '邀请申请' },
  invite_rejected: { color: 'red', text: '申请被拒' },
  staff_invite: { color: 'blue', text: '加入邀请' },
}

function parseActionData(raw) {
  if (!raw) return null
  try {
    return typeof raw === 'string' ? JSON.parse(raw) : raw
  } catch {
    return null
  }
}

export default function NotificationCenter() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [unread, setUnread] = useState(0)
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  const timerRef = useRef(null)

  const refreshCount = useCallback(async () => {
    try {
      const resp = await notificationsApi.unreadCount()
      if (resp.code === 20000) setUnread(resp.data?.count || 0)
    } catch {
      /* non-critical */
    }
  }, [])

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await notificationsApi.list()
      if (resp.code === 20000) setList(resp.data?.list || [])
    } catch {
      /* non-critical */
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refreshCount()
    timerRef.current = setInterval(refreshCount, 60000)
    return () => clearInterval(timerRef.current)
  }, [refreshCount])

  const handleOpenChange = (next) => {
    setOpen(next)
    if (next) {
      fetchList()
      refreshCount()
    }
  }

  const handleClickItem = async (item) => {
    if (item.status !== 'read') {
      try {
        await notificationsApi.markRead(item.id)
        setList((prev) => prev.map((n) => (n.id === item.id ? { ...n, status: 'read' } : n)))
        setUnread((c) => Math.max(0, c - 1))
      } catch {
        /* non-critical */
      }
    }
    const route = ACTION_ROUTES[item.action_type]
    if (route) {
      setOpen(false)
      navigate(route)
    }
  }

  const handleMarkAll = async () => {
    try {
      await notificationsApi.markAllRead()
      setList((prev) => prev.map((n) => ({ ...n, status: 'read' })))
      setUnread(0)
    } catch {
      /* non-critical */
    }
  }

  const content = (
    <div style={{ width: 340 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
        <Typography.Text strong>消息中心</Typography.Text>
        <Button type="link" size="small" icon={<CheckOutlined />} onClick={handleMarkAll} disabled={unread === 0}>
          全部已读
        </Button>
      </div>
      {loading ? (
        <div style={{ textAlign: 'center', padding: 24 }}><Spin /></div>
      ) : list.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无消息" />
      ) : (
        <List
          size="small"
          style={{ maxHeight: 400, overflowY: 'auto' }}
          dataSource={list}
          renderItem={(item) => {
            const tag = TYPE_TAG[item.type] || { color: 'default', text: '' }
            const route = ACTION_ROUTES[item.action_type]
            return (
              <List.Item
                key={item.id}
                onClick={() => handleClickItem(item)}
                style={{
                  cursor: 'pointer',
                  background: item.status === 'read' ? 'transparent' : '#f0f7ff',
                  borderRadius: 6,
                  padding: '8px 10px',
                }}
              >
                <List.Item.Meta
                  title={
                    <span style={{ fontSize: 13 }}>
                      {item.status !== 'read' && (
                        <span style={{ display: 'inline-block', width: 6, height: 6, borderRadius: '50%', background: '#1677ff', marginRight: 6, verticalAlign: 'middle' }} />
                      )}
                      {item.title}
                      {tag.text && <Tag color={tag.color} style={{ marginLeft: 6 }}>{tag.text}</Tag>}
                    </span>
                  }
                  description={
                    <span style={{ fontSize: 12, color: '#888' }}>
                      {item.content}
                      {(route || item.action_type === 'staff_invite') && (
                        <span style={{ color: '#1677ff', marginLeft: 4 }}>
                          {route ? '· 去处理' : '· 在「我的 → 系统消息」处理'}
                        </span>
                      )}
                    </span>
                  }
                />
              </List.Item>
            )
          }}
        />
      )}
    </div>
  )

  return (
    <Popover
      content={content}
      trigger="click"
      open={open}
      onOpenChange={handleOpenChange}
      placement="bottomRight"
      overlayInnerStyle={{ padding: 12 }}
    >
      <Badge count={unread} size="small" offset={[-2, 2]}>
        <Button type="text" icon={<BellOutlined style={{ fontSize: 18 }} />} aria-label="消息中心" />
      </Badge>
    </Popover>
  )
}
