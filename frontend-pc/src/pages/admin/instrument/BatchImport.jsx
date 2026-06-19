import { useState, useRef } from 'react'
import { Steps, Button, Upload, message, Card, Table, Alert, Progress, Typography, Space, Tag, Tooltip, Input, Modal, Breadcrumb, Image } from 'antd'
import { UploadOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined, EditOutlined, SwapOutlined, HomeOutlined, SettingOutlined } from '@ant-design/icons'
import { instrumentsApi, api } from '../../../services/api'
import { useNavigate } from 'react-router-dom'

const { Title, Text } = Typography

function downloadTemplate() {
  const headers = ['识别码', '分类', '品牌', '型号', '产地', '级别']
  const BOM = '\uFEFF'
  const csv = BOM + '\uFEFF' + headers.join(',') + '\n'
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = '乐器导入模板.csv'; a.click()
  URL.revokeObjectURL(url)
}

export default function BatchImport() {
  const navigate = useNavigate()
  const [currentStep, setCurrentStep] = useState(0)
  const [csvFileList, setCsvFileList] = useState([])
  const [loading, setLoading] = useState(false)
  const [previewData, setPreviewData] = useState(null)
  const [sessionId, setSessionId] = useState(null)
  const [importResult, setImportResult] = useState(null)
  const [executing, setExecuting] = useState(false)
  const [validatedData, setValidatedData] = useState(null)
  const [instrumentList, setInstrumentList] = useState([])
  const [mediaDialog, setMediaDialog] = useState(null)

  const steps = [
    { title: '上传 CSV' },
    { title: '校验并提交' },
    { title: '上传图片' },
    { title: '完成' },
  ]

  const handlePreview = async () => {
    if (csvFileList.length === 0) {
      message.error('请先上传 CSV 文件')
      return
    }
    setLoading(true)
    try {
      const result = await instrumentsApi.batchImportPreview(csvFileList[0].originFileObj)
      if (result.code === 20000) {
        setPreviewData(result.data)
        setSessionId(result.data.session_id)
        setValidatedData(result.data.rows)
        setCurrentStep(1)
      } else {
        message.error(result.message || '校验失败')
      }
    } catch (err) {
      message.error('校验失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async () => {
    setExecuting(true)
    try {
      const result = await instrumentsApi.batchImport(sessionId)
      if (result.code === 20000) {
        setImportResult(result.data)
        setInstrumentList(result.data.results || [])
        setCurrentStep(2)
      } else {
        message.error(result.message || '提交失败')
      }
    } catch (err) {
      message.error('提交失败: ' + err.message)
    } finally {
      setExecuting(false)
    }
  }

  const getColumns = () => {
    if (!previewData?.rows?.length) return []
    const firstRow = previewData.rows[0]
    const cols = [
      { title: '行号', dataIndex: 'row', key: 'row', width: 60 },
      { title: '识别码', dataIndex: 'sn', key: 'sn', width: 120 },
    ]
    if (firstRow.fields) {
      const fieldKeys = Object.keys(firstRow.fields).filter(k => !k.startsWith('_'))
      fieldKeys.forEach(key => {
        if (key === 'sn') return
        cols.push({
          title: key,
          key: key,
          width: 100,
          render: (_, record) => {
            const val = record.fields?.[key] || ''
            const hasError = record.errors?.some(e => e.includes(key))
            return <div className={hasError ? 'bg-red-50' : ''}>{val || '-'}</div>
          },
        })
      })
    }
    cols.push({
      title: '状态',
      key: 'status',
      width: 80,
      render: (_, record) => {
        if (!record.valid) {
          return <Tooltip title={record.errors?.join('; ')}><CloseCircleOutlined style={{ color: '#ff4d4f' }} /></Tooltip>
        }
        if (record.fields?._warning_level) {
          return <Tooltip title={record.fields._warning_level}><WarningOutlined style={{ color: '#faad14' }} /></Tooltip>
        }
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />
      },
    })
    cols.push({
      title: '错误',
      key: 'errors',
      render: (_, record) => record.errors?.map((e, i) => <Tag key={i} color="error">{e}</Tag>),
    })
    return cols
  }

  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <Card title="上传 CSV 文件">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Alert
                message="格式说明"
                description={
                  <div>
                    <p>CSV 文件包含以下列：<strong>识别码、分类、品牌、型号、产地、级别</strong></p>
                    <p>创建方式：在 Excel 中编辑数据后，另存为 CSV UTF-8（逗号分隔）格式。</p>
                    <p><a onClick={downloadTemplate} style={{ cursor: 'pointer' }}>📄 下载模板文件</a></p>
                  </div>
                }
                type="info"
                showIcon
              />
              <Upload
                fileList={csvFileList}
                onChange={({ fileList }) => setCsvFileList(fileList)}
                beforeUpload={() => false}
                accept=".csv"
                maxCount={1}
              >
                <Button icon={<UploadOutlined />}>选择 CSV 文件</Button>
              </Upload>
              <div style={{ marginTop: 16 }}>
                <Button type="primary" onClick={handlePreview} disabled={csvFileList.length === 0 || loading} loading={loading}>
                  校验数据
                </Button>
              </div>
            </Space>
          </Card>
        )

      case 1:
        return (
          <>
            <Card title="校验结果" style={{ marginBottom: 24 }}>
              <Space wrap>
                <Text>总数: <strong>{previewData?.total_count || 0}</strong></Text>
                <Text>有效: <span style={{ color: '#52c41a' }}>{previewData?.valid_count || 0}</span></Text>
                <Text>错误: <span style={{ color: '#ff4d4f' }}>{previewData?.error_count || 0}</span></Text>
              </Space>
            </Card>
            {previewData?.rows && (
              <Table
                dataSource={previewData.rows}
                columns={getColumns()}
                rowKey="row"
                pagination={{ pageSize: 10 }}
                scroll={{ x: 1000 }}
                size="small"
                rowClassName={(record) => !record.valid ? 'bg-red-50' : ''}
              />
            )}
            <div style={{ marginTop: 16 }}>
              {previewData?.error_count > 0 ? (
                <Text type="danger">CSV 中存在错误，请修正后重新上传</Text>
              ) : (
                <Button type="primary" onClick={handleSubmit} loading={executing}>
                  提交并创建乐器
                </Button>
              )}
            </div>
          </>
        )

      case 2:
        return (
          <>
            <Card title="上传媒体文件">
              <Table
                dataSource={instrumentList}
                rowKey="id"
                pagination={false}
                size="small"
                columns={[
                  { title: '识别码', dataIndex: 'sn', key: 'sn', width: 120 },
                  { title: '分类', dataIndex: 'category_name', key: 'category_name', width: 100 },
                  { title: '展示图片', key: 'images', width: 80,
                    render: (_, r) => r._images?.length ? <Image src={r._images[0]} width={40} height={40} style={{ objectFit: 'cover' }} /> : '-'
                  },
                  { title: '海报', key: 'poster', width: 80,
                    render: (_, r) => r._poster ? <Image src={r._poster} width={40} height={40} style={{ objectFit: 'cover' }} /> : '-'
                  },
                  { title: '视频', key: 'video', width: 80,
                    render: (_, r) => r._video ? <video src={r._video} style={{ width: 40, height: 40, objectFit: 'cover' }} /> : '-'
                  },
                  { title: '操作', key: 'action', width: 80,
                    render: (_, r) => <Button size="small" icon={<SettingOutlined />} onClick={() => setMediaDialog(r)}>设置</Button>
                  },
                ]}
              />
            </Card>
            <div style={{ marginTop: 16 }}>
              <Button onClick={() => setCurrentStep(3)}>完成</Button>
            </div>

            <Modal title={`媒体设置 - ${mediaDialog?.sn || ''}`} open={!!mediaDialog} onCancel={() => setMediaDialog(null)} footer={null} width={600} destroyOnClose>
              {mediaDialog && (
                <MediaUploader instrument={mediaDialog} onUpdate={(updates) => {
                  setInstrumentList(prev => prev.map(r => r.id === mediaDialog.id ? { ...r, ...updates } : r))
                }} />
              )}
            </Modal>
          </>
        )

      case 3:
        return (
          <Card title="导入完成">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Alert message={`成功创建 ${instrumentList.length} 部乐器`} type="success" showIcon style={{ marginBottom: 16 }} />
              <Text>展示图片: {instrumentList.filter(r => r._images?.length > 0).length} 件</Text>
              <Text>海报: {instrumentList.filter(r => r._poster).length} 件</Text>
              <Text>视频: {instrumentList.filter(r => r._video).length} 件</Text>
            </Space>
          </Card>
        )

      default:
        return null
    }
  }

  return (
    <div style={{ padding: 24 }}>
      <Breadcrumb items={[
        { title: <><HomeOutlined /> Tuneloop</> },
        { title: <a onClick={() => navigate('/instruments')}>乐器管理</a> },
        { title: '批量导入乐器' },
      ]} style={{ marginBottom: 16 }} />
      <Card>
        <Title level={3}>批量导入乐器</Title>
        <Steps current={currentStep} style={{ marginBottom: 24 }}>
          {steps.map(step => <Steps.Step key={step.title} title={step.title} />)}
        </Steps>
        {renderStepContent()}
        <div style={{ marginTop: 24, textAlign: 'right' }}>
          <Space>
            {currentStep === 3 && (
              <Button type="primary" onClick={() => navigate('/instruments')}>
                返回列表
              </Button>
            )}
          </Space>
        </div>
      </Card>
    </div>
  )
}

const uploadFile = async (file) => {
  const formData = new FormData()
  formData.append('file', file)
  const res = await api.post('/upload', formData, { headers: { 'Content-Type': 'multipart/form-data' } })
  return res?.data?.url || res?.url || ''
}

function MediaUploader({ instrument, onUpdate }) {
  const [saving, setSaving] = useState(false)
  const [images, setImages] = useState(instrument._images || [])
  const [poster, setPoster] = useState(instrument._poster || '')
  const [video, setVideo] = useState(instrument._video || '')

  const handleSave = async () => {
    setSaving(true)
    try {
      await api.put(`/instruments/${instrument.id}`, { images, poster, video })
      message.success('保存成功')
      onUpdate({ _images: images, _poster: poster, _video: video })
    } catch (e) {
      message.error('保存失败: ' + (e.message || ''))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <div>
        <Text strong>展示图片</Text>
        <Upload multiple accept="image/*" showUploadList={false} beforeUpload={async (file) => {
          const url = await uploadFile(file)
          setImages(prev => [...prev, url])
          return false
        }}>
          <Button size="small" icon={<UploadOutlined />} style={{ marginLeft: 8 }}>添加</Button>
        </Upload>
        <div style={{ marginTop: 8 }}>
          {images.map((url, i) => (
            <span key={i} className="inline-block relative mr-1 mb-1">
              <Image src={url} width={60} height={60} style={{ objectFit: 'cover', borderRadius: 4 }} />
              <a style={{ position: 'absolute', top: -4, right: -4, color: 'red', cursor: 'pointer', fontSize: 14 }}
                onClick={() => setImages(prev => prev.filter((_, j) => j !== i))}>×</a>
            </span>
          ))}
        </div>
      </div>
      <div>
        <Text strong>海报</Text>
        <Upload accept="image/*" showUploadList={false} beforeUpload={async (file) => {
          const url = await uploadFile(file); setPoster(url); return false
        }}>
          <Button size="small" icon={<UploadOutlined />} style={{ marginLeft: 8 }}>上传</Button>
        </Upload>
        {poster && <Image src={poster} width={120} style={{ marginTop: 8, borderRadius: 4 }} />}
      </div>
      <div>
        <Text strong>视频</Text>
        <Upload accept="video/*" showUploadList={false} beforeUpload={async (file) => {
          const url = await uploadFile(file); setVideo(url); return false
        }}>
          <Button size="small" icon={<UploadOutlined />} style={{ marginLeft: 8 }}>上传</Button>
        </Upload>
        {video && <video src={video} controls style={{ width: '100%', maxHeight: 200, marginTop: 8 }} />}
      </div>
      <Button type="primary" loading={saving} onClick={handleSave}>保存</Button>
    </Space>
  )
}
