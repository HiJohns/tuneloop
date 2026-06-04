import { useState, useEffect } from 'react'
import { AutoComplete, Button, Input, Space, Tabs, Alert, Checkbox, Modal, Typography } from 'antd'
import { UserOutlined } from '@ant-design/icons'
import api from '../services/api'

export default function ManagerSelector({ value, onChange, conflictOptions, conflictMessage, createReason, onCreatingChange }) {
  const [mode, setMode] = useState('search')
  const [searchResults, setSearchResults] = useState([])
  const [selected, setSelected] = useState(value?.id ? value : null)
  const [fields, setFields] = useState({ username: '', name: '', email: '', phone: '' })
  const [msg, setMsg] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [skipActivation, setSkipActivation] = useState(false)

  useEffect(() => {
    onCreatingChange?.(submitting)
  }, [submitting])

  const hasExisting = !!(selected?.id)

  useEffect(() => {
    setSelected(value?.id ? value : null)
  }, [value])

  const handleSearch = (val) => {
    if (!val || val.length < 2) { setSearchResults([]); return }
    api.get(`/iam/users/search?q=${encodeURIComponent(val)}&limit=10`)
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
      .catch(err => console.error('ManagerSelector search failed:', err))
  }

  const handleSelect = (value, option) => {
    const user = option.user || searchResults.find(r => r.value === value)?.user
    if (user) {
      const s = { id: user.id, name: user.name, email: user.email || '', phone: user.phone || '', username: user.username || '' }
      setSelected(s)
      onChange?.(s)
      setSearchResults([])
      setMsg('')
    }
  }

  const handleCreateSubmit = async () => {
    if (!fields.name || !fields.email) return
    setSubmitting(true)
    setMsg('')
    try {
      console.log('%c[ManagerSelector] Creating user', 'color: blue;', {
        email: fields.email,
        skipActivation,
        timestamp: new Date().toISOString()
      })

      const resp = await api.post('/iam/users', {
        name: fields.name,
        email: fields.email,
        phone: fields.phone,
        username: fields.username,
        reason: createReason || '管理员创建',
        skip_activation: skipActivation,
      })

      console.log('%c[ManagerSelector] Create response', 'color: green;', {
        code: resp.code,
        hasInitialPassword: !!resp.data?.initial_password,
        hasId: !!resp.data?.id,
        timestamp: new Date().toISOString()
      })
      if (resp.code === 20000 && resp.data?.id) {
        const s = {
          id: resp.data.id,
          name: fields.name,
          email: fields.email,
          phone: fields.phone,
          username: fields.username,
          isNewlyCreated: true,
          skipActivation,
        }
        setSelected(s)
        onChange?.(s)
        setSearchResults([])

        if (skipActivation && resp.data?.initial_password) {
          Modal.success({
            title: '管理员已创建',
            content: (
              <div>
                <p>初始密码（请立即复制保存）：</p>
                <div style={{ textAlign: 'center', margin: '12px 0' }}>
                  <Typography.Text copyable code style={{ fontSize: 18, padding: '4px 12px' }}>
                    {resp.data.initial_password}
                  </Typography.Text>
                </div>
                <p style={{ color: '#ff4d4f' }}>此为仅显示一次密码，请妥善保存。</p>
              </div>
            ),
          })
        }
      } else if (resp.code === 40900) {
        const conflicts = resp.data?.conflicts || []
        const options = conflicts.map(u => ({
          value: u.id,
          label: `${u.name || ''} (${u.email || ''})${u.phone ? ' ' + u.phone : ''}`,
          user: u,
        }))
        setSearchResults(options)
        setMode('search')
        setMsg('用户已存在，请在搜索中选择')
      } else {
        setMsg(resp.message || '创建失败')
      }
    } catch (err) {
      console.error('ManagerSelector create failed:', err)
      setMsg('创建失败: ' + (err.message || '网络错误'))
    } finally {
      setSubmitting(false)
    }
  }

  const handleClear = () => {
    setSelected(null)
    setFields({ username: '', name: '', email: '', phone: '' })
    onChange?.({ id: null, name: '', email: '', phone: '', username: '' })
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
            <Checkbox checked={skipActivation} onChange={e => setSkipActivation(e.target.checked)}>
              跳过邮箱验证（直接激活）
            </Checkbox>
            <Button type="primary" block onClick={handleCreateSubmit} disabled={!fields.name || !fields.email} loading={submitting}>提交</Button>
          </Space>
        </Tabs.TabPane>
      </Tabs>
    </div>
  )
}
