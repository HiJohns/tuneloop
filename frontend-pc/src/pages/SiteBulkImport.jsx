import { useState } from 'react'
import { Steps, Button, Upload, message, Card, Table, Alert, Space, Typography, Tag } from 'antd'
import { UploadOutlined, CheckCircleOutlined, CloseCircleOutlined, DownloadOutlined, ArrowLeftOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { bulkImportApi } from '../services/api'

const { Title, Text } = Typography

export default function SiteBulkImport() {
  const navigate = useNavigate()
  const [currentStep, setCurrentStep] = useState(0)
  const [fileList, setFileList] = useState([])
  const [loading, setLoading] = useState(false)
  const [previewData, setPreviewData] = useState(null)
  const [importResult, setImportResult] = useState(null)
  const [executing, setExecuting] = useState(false)

  const steps = [
    { title: '上传 CSV' },
    { title: '数据校验' },
    { title: '确认导入' },
    { title: '完成' },
  ]

  const handlePreview = async () => {
    if (fileList.length === 0) {
      message.error('请先上传 CSV 文件')
      return
    }
    setLoading(true)
    try {
      const result = await bulkImportApi.previewOrganizations(fileList[0].originFileObj)
      if (result.code === 20000) {
        setPreviewData(result.data)
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

  const downloadTemplate = async () => {
    try {
      const token = localStorage.getItem('token')
      const response = await fetch('/api/admin/bulk-import/template/organizations', {
        headers: { 'Authorization': `Bearer ${token}` }
      })
      if (!response.ok) throw new Error('下载模板失败')
      const blob = await response.blob()
      const link = document.createElement('a')
      link.href = URL.createObjectURL(blob)
      link.download = 'bulk_sites_template.csv'
      link.click()
      message.success('模板下载成功')
    } catch (err) {
      message.error('下载模板失败: ' + err.message)
    }
  }

  const handleExecuteImport = async () => {
    if (fileList.length === 0) {
      message.error('请先上传 CSV 文件')
      return
    }
    setExecuting(true)
    setCurrentStep(3)
    try {
      const result = await bulkImportApi.importOrganizations(fileList[0].originFileObj)
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

  const getColumns = () => [
    { title: '行号', dataIndex: 'row', key: 'row', width: 60 },
    { title: '编码', dataIndex: 'key', key: 'key', width: 150 },
    { title: '操作', dataIndex: 'action', key: 'action', width: 100, render: (action) => {
      const colorMap = { created: 'green', updated: 'blue', failed: 'red', skipped: 'orange' }
      return <Tag color={colorMap[action] || 'default'}>{action === 'created' ? '创建' : action === 'updated' ? '更新' : action === 'failed' ? '失败' : '跳过'}</Tag>
    }},
    { title: '说明', dataIndex: 'reason', key: 'reason' },
  ]

  const renderStepContent = () => {
    switch (currentStep) {
      case 0:
        return (
          <Card title="上传网点 CSV 文件">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Alert
                message="CSV 格式要求：organization_code, name, parent_code, type"
                description="parent_code 为空表示顶级组织。type 可选：merchant / site"
                type="info"
                showIcon
              />
              <Button icon={<DownloadOutlined />} onClick={downloadTemplate}>
                下载模板
              </Button>
              <Upload
                fileList={fileList}
                onChange={({ fileList }) => setFileList(fileList)}
                beforeUpload={() => false}
                accept=".csv"
                maxCount={1}
              >
                <Button icon={<UploadOutlined />}>选择 CSV 文件</Button>
              </Upload>
              <div style={{ marginTop: 16 }}>
                <Button type="primary" onClick={handlePreview} disabled={fileList.length === 0 || loading} loading={loading}>
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
                <Text>总数: <strong>{previewData?.summary?.total || 0}</strong></Text>
                <Text>将创建: <span style={{ color: '#52c41a' }}>{previewData?.summary?.created || 0}</span></Text>
                <Text>将更新: <span style={{ color: '#1890ff' }}>{previewData?.summary?.updated || 0}</span></Text>
                <Text>错误: <span style={{ color: '#ff4d4f' }}>{previewData?.summary?.failed || 0}</span></Text>
              </Space>
            </Card>
            {previewData?.details && (
              <Table
                dataSource={previewData.details}
                columns={getColumns()}
                rowKey="row"
                pagination={{ pageSize: 10 }}
                size="small"
                rowClassName={(record) => record.action === 'failed' ? 'bg-red-50' : ''}
              />
            )}
          </>
        )
      case 2:
        return (
          <Card title="确认导入">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Alert
                message={`即将导入 ${previewData?.summary?.total || 0} 条记录：创建 ${previewData?.summary?.created || 0} 条，更新 ${previewData?.summary?.updated || 0} 条`}
                type="info"
                showIcon
              />
              {previewData?.summary?.failed > 0 && (
                <Alert message={`警告：${previewData.summary.failed} 条记录存在错误，将被跳过`} type="warning" showIcon />
              )}
              <Button type="primary" onClick={handleExecuteImport} loading={executing}>
                确认导入
              </Button>
            </Space>
          </Card>
        )
      case 3:
        return (
          <Card title="导入结果">
            {executing && <Alert message="正在导入，请稍候..." type="info" />}
            {importResult && (
              <>
                <Alert
                  message={`导入完成：创建 ${importResult.summary?.created || 0} 条，更新 ${importResult.summary?.updated || 0} 条，失败 ${importResult.summary?.failed || 0} 条`}
                  type={importResult.summary?.failed > 0 ? 'warning' : 'success'}
                  style={{ marginBottom: 24 }}
                  showIcon
                />
                {importResult.details?.filter(r => r.action === 'failed').length > 0 && (
                  <Card title="失败明细" type="inner" size="small">
                    <Table
                      dataSource={importResult.details.filter(r => r.action === 'failed')}
                      columns={getColumns()}
                      rowKey="row"
                      pagination={{ pageSize: 5 }}
                      size="small"
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
        <div style={{ marginBottom: 16 }}>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/organization/sites')}>
            返回网点管理
          </Button>
        </div>
        <Title level={3}>批量导入网点</Title>
        <Steps current={currentStep} style={{ marginBottom: 24 }}>
          {steps.map(step => <Steps.Step key={step.title} title={step.title} />)}
        </Steps>
        {renderStepContent()}
        <div style={{ marginTop: 24, textAlign: 'right' }}>
          <Space>
            {currentStep === 1 && previewData?.summary?.failed === 0 && (
              <Button type="primary" onClick={() => setCurrentStep(2)}>
                下一步：确认导入
              </Button>
            )}
            {currentStep === 1 && previewData?.summary?.failed > 0 && (
              <Button type="primary" onClick={() => setCurrentStep(2)}>
                下一步：确认导入（跳过错误）
              </Button>
            )}
            {currentStep === 3 && !executing && (
              <Button type="primary" onClick={() => navigate('/organization/sites')}>
                返回网点管理
              </Button>
            )}
          </Space>
        </div>
      </Card>
    </div>
  )
}
