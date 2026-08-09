import { useState, useEffect, useRef } from 'react'
import { Upload, Button, message, Image } from 'antd'
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons'
import { api } from '../services/api'

// IdPhotoDisplay — PC 端身份证查看/替换/删除组件 (#1599)
// Props:
//   side: 'front' | 'back'
//   initialUrl: 初始照片 URL
//   uploadEndpoint: 上传端点（如 /admin/user-management/:id/id-photo）
//   deleteEndpoint: 删除端点（如 /admin/user-management/:id/id-photo）
//   readOnly: 只读模式（业务场景查看，如订单/报修详情）
export default function IdPhotoDisplay({ side, initialUrl = '', uploadEndpoint, deleteEndpoint, readOnly = false }) {
  const [url, setUrl] = useState(initialUrl || '')
  const [uploading, setUploading] = useState(false)
  const label = side === 'front' ? '正面' : '反面'

  useEffect(() => {
    setUrl(initialUrl || '')
  }, [initialUrl])

  const getToken = () => localStorage.getItem('token') || sessionStorage.getItem('token')

  const authFetch = async (url, options = {}) => {
    const headers = { ...(options.headers || {}) }
    const token = getToken()
    if (token) headers.Authorization = `Bearer ${token}`
    return fetch(url, { ...options, headers })
  }

  const handleUpload = async (options) => {
    setUploading(true)
    const formData = new FormData()
    formData.append('file', options.file)
    formData.append('side', side)
    try {
      const resp = await authFetch(uploadEndpoint, { method: 'POST', body: formData })
      const json = await resp.json()
      if (json.code === 20000 && json.data?.url) {
        setUrl(json.data.url)
        message.success(`${label}照片已更新`)
      } else {
        message.error(json.message || '上传失败')
      }
    } catch {
      message.error('上传失败')
    } finally {
      setUploading(false)
      if (options.onSuccess) options.onSuccess()
    }
  }

  const handleDelete = async () => {
    try {
      const resp = await authFetch(`${deleteEndpoint}?side=${side}`, { method: 'DELETE' })
      const json = await resp.json()
      if (json.code === 20000) {
        setUrl('')
        message.success(`${label}照片已删除`)
      } else {
        message.error(json.message || '删除失败')
      }
    } catch {
      message.error('删除失败')
    }
  }

  return (
    <div className="id-photo-item" style={{ textAlign: 'center' }}>
      <div style={{ marginBottom: 8 }}>
        {url ? (
          <Image
            src={url}
            alt={`身份证${label}`}
            width={160}
            height={100}
            style={{ objectFit: 'cover', borderRadius: 8, cursor: 'pointer' }}
            fallback="/placeholder.png"
          />
        ) : (
          <div style={{
            width: 160, height: 100, border: '2px dashed #d9d9d9', borderRadius: 8,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: '#999', fontSize: 12, background: '#fafafa',
          }}>
            未上传
          </div>
        )}
      </div>
      <div style={{ fontSize: 12, color: '#666' }}>{label}</div>
      {!readOnly && (
        <div style={{ marginTop: 8 }}>
          <Upload
            accept="image/jpeg,image/png,image/webp"
            showUploadList={false}
            customRequest={handleUpload}
            disabled={uploading}
          >
            <Button size="small" icon={<PlusOutlined />} loading={uploading}>
              {url ? '替换' : '上传'}
            </Button>
          </Upload>
          {url && (
            <Button size="small" danger icon={<DeleteOutlined />} onClick={handleDelete} style={{ marginLeft: 8 }}>
              删除
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
