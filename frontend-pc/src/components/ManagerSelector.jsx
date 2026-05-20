import { useState } from 'react'
import { AutoComplete, Button, Input, Space, Tabs, Alert } from 'antd'
import { UserOutlined } from '@ant-design/icons'
import { getToken } from '../services/api'

export default function ManagerSelector({ value, onChange, conflictOptions, conflictMessage }) {
  const [mode, setMode] = useState('search')
  const [searchResults, setSearchResults] = useState([])
  const [selected, setSelected] = useState(value?.id ? value : null)
  const [fields, setFields] = useState({ username: '', name: '', email: '', phone: '' })
  const [msg, setMsg] = useState('')

  const hasExisting = !!(selected?.id)

  const handleSearch = (val) => {
    if (!val || val.length < 2) { setSearchResults([]); return }
    const token = getToken()
    fetch(`/api/iam/users/search?q=${encodeURIComponent(val)}&limit=10`, {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then(r => r.json())
      .then(resp => {
        if (resp.code === 20000) {
          const users = resp.data?.users || []
          setSearchResults(users.map(u => ({
            value: u.id,
            label: (
              <div style={{ padding: '4px 0' }}>
                <div style={{ fontWeight: 'bold' }}>{u.name}</div>
                <div style={{ fontSize: 12, color: '#666' }}>{u.username} {u.email} {u.phone}</div>
              </div>
            ),
            user: u,
          })))
        }
      })
      .catch(() => {})
  }

  const handleSelect = (value, option) => {
    const user = option.user || searchResults.find(r => r.value === value)?.user
    if (user) {
      const s = { id: user.id, name: user.name, email: user.email || '', phone: user.phone || '', username: user.username || '', isNew: false }
      setSelected(s)
      onChange?.(s)
      setSearchResults([])
      setMsg('')
    }
  }

  const handleCreateSubmit = () => {
    if (fields.name && fields.email) {
      const s = { id: null, ...fields, isNew: true }
      setSelected(s)
      onChange?.(s)
    }
  }

  const handleClear = () => {
    setSelected(null)
    setFields({ username: '', name: '', email: '', phone: '' })
    onChange?.({ id: null, name: '', email: '', phone: '', username: '', isNew: false })
    setMsg('')
  }

  const displayMsg = conflictMessage || msg

  if (hasExisting) {
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <UserOutlined style={{ fontSize: 18, color: '#1890ff' }} />
        <span style={{ fontWeight: 500 }}>{selected.name}</span>
        {selected.email && <span style={{ color: '#999' }}>({selected.email})</span>}
        <Button type="link" danger onClick={handleClear}>X</Button>
      </div>
    )
  }

  return (
    <div>
      {displayMsg && (
        <Alert message={displayMsg} type="warning" showIcon closable style={{ marginBottom: 12 }} onClose={() => setMsg('')} />
      )}
      <Tabs activeKey={mode} onChange={setMode} size="small">
        <Tabs.TabPane tab="搜索" key="search">
          <AutoComplete
            style={{ width: '100%' }}
            placeholder="输入用户名、邮箱或手机号搜索"
            options={conflictOptions || searchResults}
            onSearch={handleSearch}
            onSelect={handleSelect}
          />
        </Tabs.TabPane>
        <Tabs.TabPane tab="创建" key="create">
          <Space direction="vertical" style={{ width: '100%' }}>
            <Input placeholder="姓名" value={fields.name} onChange={e => setFields({ ...fields, name: e.target.value })} />
            <Input placeholder="用户名" value={fields.username} onChange={e => setFields({ ...fields, username: e.target.value })} />
            <Input placeholder="邮箱" value={fields.email} onChange={e => setFields({ ...fields, email: e.target.value })} />
            <Input placeholder="电话" value={fields.phone} onChange={e => setFields({ ...fields, phone: e.target.value })} />
            <Button type="primary" block onClick={handleCreateSubmit} disabled={!fields.name || !fields.email}>添加</Button>
          </Space>
        </Tabs.TabPane>
      </Tabs>
    </div>
  )
}
