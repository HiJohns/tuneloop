import { useState } from 'react'
import { Steps, Button, Upload, message, Card, Table, Alert, Progress, List, Typography, Space, Tag, Tooltip, Input, Modal } from 'antd'
import { UploadOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined, EditOutlined, SwapOutlined } from '@ant-design/icons'
import { instrumentsApi } from '../../../services/api'

const { Title, Text } = Typography

export default function BatchImport() {
  const [currentStep, setCurrentStep] = useState(0)
  const [csvFileList, setCsvFileList] = useState([])
  const [zipFileList, setZipFileList] = useState([])
  const [loading, setLoading] = useState(false)
  const [previewData, setPreviewData] = useState(null)
  const [sessionId, setSessionId] = useState(null)
  const [mediaResult, setMediaResult] = useState(null)
  const [importResult, setImportResult] = useState(null)
  const [executing, setExecuting] = useState(false)
  const [editingCell, setEditingCell] = useState(null)
  const [editValue, setEditValue] = useState('')
  const [renameModal, setRenameModal] = useState({ visible: false, file: '', newName: '' })

  const steps = [
    { title: '上传 CSV' },
    { title: '数据校验' },
    { title: '上传图片' },
    { title: '确认导入' },
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

  const handleUploadMedia = async () => {
    if (zipFileList.length === 0) {
      setCurrentStep(3)
      return
    }
    setLoading(true)
    try {
      const result = await instrumentsApi.batchImportMedia(zipFileList[0].originFileObj, sessionId)
      if (result.code === 20000) {
        setMediaResult(result.data)
        setCurrentStep(3)
      } else {
        message.error(result.message || '图片上传失败')
      }
    } catch (err) {
      message.error('图片上传失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleExecuteImport = async () => {
    setExecuting(true)
    setCurrentStep(4)
    try {
      const result = await instrumentsApi.batchImport(sessionId)
      if (result.code === 20000) {
        setImportResult(result.data)
      } else {
        message.error(result.message || '导入失败')
      }
    } catch (err) {
      message.error('导入失败: ' + err.message)
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
            const isEditing = editingCell?.row === record.row && editingCell?.field === key
            return (
              <div
                className={hasError ? 'bg-red-50' : ''}
                onDoubleClick={() => {
                  setEditingCell({ row: record.row, field: key })
                  setEditValue(val)
                }}
              >
                {isEditing ? (
                  <Input
                    size="small"
                    value={editValue}
                    onChange={e => setEditValue(e.target.value)}
                    onPressEnter={() => {
                      if (record.fields) record.fields[key] = editValue
                      setEditingCell(null)
                    }}
                    onBlur={() => {
                      if (record.fields) record.fields[key] = editValue
                      setEditingCell(null)
                    }}
                    autoFocus
                  />
                ) : (
                  <Tooltip title="双击编辑">
                    <span>{val || '-'}</span>
                    <EditOutlined style={{ marginLeft: 4, fontSize: 10, color: '#999' }} />
                  </Tooltip>
                )}
              </div>
            )
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
                <Text>状态: <strong>{previewData?.can_import ? '可导入' : '存在错误'}</strong></Text>
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
          </>
        )

      case 2:
        return (
          <Card title="上传图片 ZIP 包（可选）">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Alert message="图片命名规则：识别码_序号.jpg（如 SN001_1.jpg）" type="info" showIcon />
              <Upload
                fileList={zipFileList}
                onChange={({ fileList }) => setZipFileList(fileList)}
                beforeUpload={() => false}
                accept=".zip"
                maxCount={1}
              >
                <Button icon={<UploadOutlined />}>选择 ZIP 文件</Button>
              </Upload>
              <div style={{ marginTop: 16 }}>
                <Button type="primary" onClick={handleUploadMedia} loading={loading}>
                  上传并继续
                </Button>
              </div>
            </Space>
          </Card>
        )

      case 3:
        return (
          <Card title="确认导入">
            <Space direction="vertical" style={{ width: '100%' }}>
              {mediaResult && (
                <Alert
                  message={`图片匹配：${mediaResult.matched_count || 0} 匹配，${mediaResult.unmatched_count || 0} 未匹配`}
                  type={mediaResult.unmatched_count > 0 ? 'warning' : 'success'}
                  showIcon
                />
              )}
              {mediaResult?.unmatched_files?.length > 0 && (
                <Card title="未匹配文件" type="inner" size="small">
                  <List
                    size="small"
                    dataSource={mediaResult.unmatched_files}
                    renderItem={f => (
                      <List.Item
                        actions={[
                          <Button
                            size="small"
                            icon={<SwapOutlined />}
                            onClick={() => setRenameModal({ visible: true, file: f, newName: f })}
                          >
                            改名
                          </Button>
                        ]}
                      >
                        <Text type="secondary">{f}</Text>
                      </List.Item>
                    )}
                  />
                </Card>
              )}
              <Alert message={`即将导入 ${previewData?.valid_count || 0} 条有效记录`} type="info" />
              <Button type="primary" onClick={handleExecuteImport} disabled={!previewData?.can_import}>
                确认导入
              </Button>
            </Space>
          </Card>
        )

      case 4:
        return (
          <Card title="导入结果">
            {executing && <Progress percent={100} status="active" />}
            {importResult && (
              <>
                <Alert
                  message={`成功导入 ${importResult.success_count || 0} 个，失败 ${importResult.fail_count || 0} 个`}
                  type={importResult.fail_count > 0 ? 'warning' : 'success'}
                  style={{ marginBottom: 24 }}
                />
                {importResult.results?.filter(r => r.status === 'failed').length > 0 && (
                  <Card title="失败项" type="inner">
                    <List
                      size="small"
                      dataSource={importResult.results.filter(r => r.status === 'failed')}
                      renderItem={item => <List.Item><Text type="danger">{item.sn}: {item.error}</Text></List.Item>}
                    />
                  </Card>
                )}
              </>
            )}
          </Card>
        )

      default:
        return null
    }
  }

  return (
    <div style={{ padding: 24 }}>
      <Card>
        <Title level={3}>批量导入乐器</Title>
        <Steps current={currentStep} style={{ marginBottom: 24 }}>
          {steps.map(step => <Steps.Step key={step.title} title={step.title} />)}
        </Steps>
        {renderStepContent()}
        <div style={{ marginTop: 24, textAlign: 'right' }}>
          <Space>
            {currentStep === 1 && (
              <Button type="primary" onClick={() => setCurrentStep(2)} disabled={!previewData?.can_import}>
                下一步：上传图片
              </Button>
            )}
            {currentStep === 3 && (
              <Button type="primary" onClick={handleExecuteImport} disabled={!previewData?.can_import}>
                确认导入
              </Button>
            )}
            {currentStep === 4 && !executing && (
              <Button type="primary" onClick={() => window.location.href = '/instruments/list'}>
                查看乐器列表
              </Button>
            )}
          </Space>
        </div>
        <Modal
          title="重命名文件"
          open={renameModal.visible}
          onOk={() => {
            if (mediaResult?.unmatched_files) {
              const idx = mediaResult.unmatched_files.indexOf(renameModal.file)
              if (idx >= 0) {
                mediaResult.unmatched_files[idx] = renameModal.newName
              }
            }
            setRenameModal({ visible: false, file: '', newName: '' })
            message.success('已重命名，请重新上传 ZIP')
          }}
          onCancel={() => setRenameModal({ visible: false, file: '', newName: '' })}
        >
          <Input
            value={renameModal.newName}
            onChange={e => setRenameModal({ ...renameModal, newName: e.target.value })}
            placeholder="新文件名（格式：识别码_序号.jpg）"
          />
        </Modal>
      </Card>
    </div>
  )
}
