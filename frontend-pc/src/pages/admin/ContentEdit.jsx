import { useState, useEffect, useRef } from 'react'
import { Card, Tabs, Button, message } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import ReactQuill from 'react-quill'
import 'react-quill/dist/quill.snow.css'
import { api } from '../../services/api'

const KEYS = [
  { key: 'rental_notice', title: '租赁须知' },
  { key: 'contact_us', title: '联系我们' },
  { key: 'cooperation', title: '商务合作' },
  { key: 'rental_agreement', title: '租用服务协议' },
  { key: 'user_agreement', title: '用户协议' },
  { key: 'privacy_policy', title: '隐私协议' },
  { key: 'digital_certificate', title: '数字证书授权使用协议' },
  { key: 'damage_standard', title: '《乐器损耗与赔偿标准》细则' },
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

  useEffect(() => {
    KEYS.forEach(k => load(k.key))
  }, [])

  const load = async (key) => {
    setLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.get(`/settings/${key}`)
      if (res.code === 20000) {
        setValues(prev => ({ ...prev, [key]: res.data?.value || '' }))
      }
    } catch { message.error(`加载${key}失败`) }
    setLoading(prev => ({ ...prev, [key]: false }))
  }

  const save = async (key) => {
    setLoading(prev => ({ ...prev, [key]: true }))
    try {
      const res = await api.put(`/admin/content/${key}`, { value: values[key] || '' })
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
        {loading[k.key] ? <div style={{ padding: 24, color: '#999' }}>加载中...</div> : (
        <ReactQuill
          ref={el => { quillRefs.current[k.key] = el }}
          theme="snow"
          // Uncontrolled (#1687 regression): controlled value+onChange causes
          // the editor to rebuild on every keystroke — the box disappears.
          defaultValue={values[k.key] || ''}
          onChange={val => setValues(prev => ({ ...prev, [k.key]: val }))}
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
