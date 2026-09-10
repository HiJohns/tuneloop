import { useState, useEffect, useRef } from 'react'
import { Card, Tabs, Button, message } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import ReactQuill from 'react-quill'
import 'react-quill/dist/quill.snow.css'
import { api } from '../../services/api'

// #1840: display order is the canonical 10-item sequence (user-confirmed);
// rental_notice removed from the admin list (its setting data stays in DB).
const KEYS = [
  { key: 'contact_us', title: '联系我们' },
  { key: 'cooperation', title: '商务合作' },
  { key: 'platform_rules', title: '平台规则文档' },
  { key: 'rental_agreement', title: '租用服务协议' },
  { key: 'damage_standard', title: '《乐器损耗与赔偿标准》细则' },
  { key: 'user_agreement', title: '个人信息查询授权书' },
  { key: 'privacy_policy', title: '个人信息保护政策' },
  { key: 'digital_certificate', title: '数字证书授权使用协议' },
  { key: 'merchant_audit_requirements', title: '平台入驻审核要求与规范' },
  { key: 'merchant_agreement', title: '商家入驻协议' },
]

// Upload an image via /api/upload and insert its URL at the current cursor.
// Each editor tab gets its own ref (issue-1687): AntD Tabs mount editors
// lazily and keep them mounted, so a single shared ref would point to the
// last-mounted instance — image clicks in other tabs would insert into the
// wrong editor.
const imageHandler = async (quillRefs, currentKey) => {
  const input = document.createElement('input')
  input.type = 'file'
  input.accept = 'image/*'
  input.onchange = async () => {
    const file = input.files?.[0]
    if (!file) return
    try {
      const formData = new FormData()
      formData.append('file', file)
      const res = await api.uploadFile('/upload', formData)
      if (res.code === 20000 && res.data?.url) {
        const quill = quillRefs.current?.[currentKey]?.getEditor?.()
        if (!quill) return
        const range = quill.getSelection(true)
        const index = range?.index != null ? range.index : quill.getLength()
        quill.insertEmbed(index, 'image', res.data.url)
        quill.setSelection(index + 1)
      } else {
        message.error(res.message || '图片上传失败')
      }
    } catch { message.error('图片上传失败') }
  }
  input.click()
}

export default function ContentEdit() {
  const [loading, setLoading] = useState({})
  const [values, setValues] = useState({})
  const quillRefs = useRef({})
  // #1687: keystrokes must NOT re-render the parent — any re-render rebuilds
  // the Quill editor and the box disappears. onChange writes to this ref only;
  // save() reads from it.
  const draftRefs = useRef({})

  useEffect(() => {
    KEYS.forEach(k => load(k.key))
  }, [])

  const load = async (key) => {
    setLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.get(`/public/settings/${key}`)
      if (res.code === 20000) {
        setValues(prev => ({ ...prev, [key]: res.data?.value || '' }))
      }
    } catch { message.error(`加载${key}失败`) }
    setLoading(prev => ({ ...prev, [key]: false }))
  }

  const save = async (key) => {
    setLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.put(`/admin/content/${key}`, { value: draftRefs.current[key] ?? values[key] ?? '' })
      if (res.code === 20000) {
        message.success(`${KEYS.find(k => k.key === key)?.title}已保存`)
      } else { message.error(res.message || '保存失败') }
    } catch { message.error('保存失败') }
    setLoading(prev => ({ ...prev, [key]: false }))
  }

  const items = KEYS.map(k => ({
    key: k.key,
    label: k.title,
    children: (
      <div>
        {loading[k.key] !== false ? <div style={{ padding: 24, color: '#999' }}>加载中...</div> : (
        <ReactQuill
          ref={el => { quillRefs.current[k.key] = el }}
          theme="snow"
          // Uncontrolled (#1687 regression): controlled value+onChange causes
          // the editor to rebuild on every keystroke — the box disappears.
          defaultValue={values[k.key] || ''}
          onChange={val => { draftRefs.current[k.key] = val }}
          placeholder={`请输入${k.title}内容`}
          style={{ marginBottom: 12, height: 300 }}
          modules={{
            toolbar: {
              container: [
                [{ header: [1, 2, 3, false] }],
                ['bold', 'italic', 'underline', 'strike'],
                [{ list: 'ordered' }, { list: 'bullet' }],
                [{ align: [] }],
                ['link', 'image'],
                ['clean'],
              ],
              handlers: {
                image: () => imageHandler(quillRefs, k.key),
              },
            },
          }}
        />
        )}
        <div style={{ height: 48 }} />
        <Button type="primary" icon={<SaveOutlined />} onClick={() => save(k.key)} loading={loading[k.key]}>
          保存
        </Button>
      </div>
    ),
  }))

  return (
    <div className="p-6">
      <Card title="内容编辑">
        <Tabs items={items} />
      </Card>
    </div>
  )
}
