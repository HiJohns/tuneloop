import { useState, useEffect, useRef } from 'react'
import { Card, Button, message } from 'antd'
import { SaveOutlined } from '@ant-design/icons'
import ReactQuill from 'react-quill'
import 'react-quill/dist/quill.snow.css'
import { api } from '../../services/api'

const HANDBOOK_KEY = 'membership_handbook'

// #1830 增量: backend-editable membership handbook. Stored via the generic
// global settings store (same chain as the content editor), read by the
// mobile membership center from the public settings endpoint.
export default function MembershipHandbookPage() {
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [initialValue, setInitialValue] = useState('')
  const quillRef = useRef(null)
  const draftRef = useRef('')

  useEffect(() => {
    const load = async () => {
      try {
        const res = await api.get(`/public/settings/${HANDBOOK_KEY}`)
        if (res.code === 20000) {
          const value = res.data?.value || ''
          setInitialValue(value)
          draftRef.current = value
        } else {
          message.error(res.message || '加载会员手册失败')
        }
      } catch {
        message.error('加载会员手册失败')
      }
      setLoading(false)
    }
    load()
  }, [])

  const imageHandler = async () => {
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
          const quill = quillRef.current?.getEditor?.()
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

  const save = async () => {
    setSaving(true)
    try {
      const res = await api.put(`/admin/content/${HANDBOOK_KEY}`, { value: draftRef.current ?? '' })
      if (res.code === 20000) {
        message.success('会员手册已保存')
      } else {
        message.error(res.message || '保存失败')
      }
    } catch {
      message.error('保存失败')
    }
    setSaving(false)
  }

  return (
    <Card title="会员规则与权益手册（会员中心底部展示，所有会员统一）">
      {loading ? (
        <div style={{ padding: 24, color: '#999' }}>加载中...</div>
      ) : (
        <div>
          <ReactQuill
            ref={quillRef}
            theme="snow"
            defaultValue={initialValue}
            onChange={val => { draftRef.current = val }}
            placeholder="请输入会员规则与权益手册内容（支持富文本与图片）"
            style={{ marginBottom: 12, height: 420 }}
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
                  image: imageHandler,
                },
              },
            }}
          />
          <div style={{ height: 48 }} />
          <Button type="primary" icon={<SaveOutlined />} onClick={save} loading={saving}>
            保存
          </Button>
          <div style={{ marginTop: 12, color: '#999', fontSize: 12 }}>
            保存后小程序/H5「我的-会员中心」底部折叠区块即时更新；清空保存则回退到内置默认文案。
          </div>
        </div>
      )}
    </Card>
  )
}
