import { useState, useEffect } from 'react'
import { Steps, Button, Upload, message, Card, Table, Alert, Spin, Progress, List, Typography, Space } from 'antd'
import { UploadOutlined, CheckCircleOutlined, WarningOutlined, CloseCircleOutlined } from '@ant-design/icons'
import { instrumentsApi } from '../../services/api'

const { Step } = Steps
const { Title, Text } = Typography

export default function BatchImport() {
  const [currentStep, setCurrentStep] = useState(0)
  const [fileList, setFileList] = useState([])
  const [loading, setLoading] = useState(false)
  const [previewData, setPreviewData] = useState(null)
  const [importResult, setImportResult] = useState(null)
  const [executing, setExecuting] = useState(false)
  const [executeProgress, setExecuteProgress] = useState(0)

  const steps = [
    {
      title: '上传文件',
      content: '上传 ZIP 文件',
    },
    {
      title: '预检',
      content: '预览导入数据',
    },
    {
      title: '执行导入',
      content: '执行批量导入',
    },
    {
      title: '完成',
      content: '查看导入结果',
    },
  ]

  // Step 1: File Upload
  const handleUploadChange = ({ fileList }) => {
    setFileList(fileList)
  }

  const handlePreview = async () => {
    if (fileList.length === 0) {
      message.error('请先上传 ZIP 文件')
      return
    }

    setLoading(true)
    try {
      const result = await instrumentsApi.batchImportPreview(fileList[0].originFileObj)
      if (result.code === 20000) {
        setPreviewData(result.data.preview)
        setCurrentStep(1)
      } else {
        message.error(result.message || '预检失败')
      }
    } catch (err) {
      console.error('Preview error:', err)
      message.error('预检失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  // Step 2: Preview
  const columns = [
    {
      title: '识别码',
      dataIndex: 'sn',
      key: 'sn',
    },
    {
      title: '分类',
      dataIndex: 'category_name',
      key: 'category_name',
    },
    {
      title: '网点',
      dataIndex: 'site_name',
      key: 'site_name',
    },
    {
      title: '级别',
      dataIndex: 'level_name',
      key: 'level_name',
    },
    {
      title: '状态',
      key: 'status',
      render: (_, record) => {
        if (record._error_category || record._error_site) {
          return <CloseCircleOutlined style={{ color: '#ff4d4f' }} />
        }
        if (record._warning_level) {
          return <WarningOutlined style={{ color: '#faad14' }} />
        }
        return <CheckCircleOutlined style={{ color: '#52c41a' }} />
      },
    },
  ]

  // Step 3: Execute Import
  const handleExecuteImport = async () => {
    if (!previewData?.can_import) {
      message.error('存在错误，无法导入')
      return
    }

    setExecuting(true)
    setExecuteProgress(0)
    setCurrentStep(2)

    // Simulate progress
    const progressInterval = setInterval(() => {
      setExecuteProgress((prev) => {
        if (prev >= 90) {
          clearInterval(progressInterval)
          return 90
        }
        return prev + 10
      })
    }, 200)

    try {
      const result = await instrumentsApi.batchImport(fileList[0].originFileObj)
      
      clearInterval(progressInterval)
      setExecuteProgress(100)

      if (result.code === 20000) {
        setImportResult(result.data)
        setCurrentStep(3)
      } else {
        message.error(result.message || '导入失败')
      }
    } catch (err) {
      console.error('Import error:', err)
      message.error('导入失败: ' + err.message)
    } finally {
      setExecuting(false)
    }
  }

  // Step renderers
  const renderStepContent = () => {
    switch (currentStep) {
      case 0: // Upload
        return (
          <Card title="上传 ZIP 文件">
            <Upload
              fileList={fileList}
              onChange={handleUploadChange}
              beforeUpload={() => false}
              accept=".zip"
              maxCount={1}
            >
              <Button icon={<UploadOutlined />}>选择 ZIP 文件</Button>
            </Upload>
            <div style={{ marginTop: 24 }}>
              <Button
                type="primary"
                onClick={handlePreview}
                disabled={fileList.length === 0 || loading}
                loading={loading}
              >
                预检
              </Button>
            </div>
          </Card>
        )

      case 1: // Preview
        return (
          <>
            <Card title="预检结果" style={{ marginBottom: 24 }}>
              <Space wrap>
                <Text>总数量: <strong>{previewData?.total_count || 0}</strong></Text>
                <Text>错误: <span style={{ color: '#ff4d4f' }}>{previewData?.error_count || 0}</span></Text>
                <Text>警告: <span style={{ color: '#faad14' }}>{previewData?.warning_count || 0}</span></Text>
                <Text>状态: <strong>{previewData?.can_import ? '可以导入' : '存在错误，无法导入'}</strong></Text>
              </Space>
            </Card>

            {previewData?.instruments && (
              <Table
                dataSource={previewData.instruments}
                columns={columns}
                rowKey={(record) => record.sn || Math.random()}
                pagination={{ pageSize: 10 }}
                scroll={{ x: 800 }}
              />
            )}

            {previewData?.image_directories && previewData.image_directories.length > 0 && (
              <Card title="图片目录" style={{ marginTop: 24 }}>
                <List
                  dataSource={previewData.image_directories}
                  renderItem={(dir) => <List.Item>{dir}</List.Item>}
                />
              </Card>
            )}

            {previewData?.instruments?.some(inst => inst._error_category || inst._error_site || inst._warning_level) && (
              <Card title="详细错误/警告" style={{ marginTop: 24 }}>
                {previewData.instruments.map((inst, idx) => {
                  const errors = []
                  if (inst._error_category) errors.push(<Alert key={`err-cat-${idx}`} message={inst._error_category} type="error" showIcon />)
                  if (inst._error_site) errors.push(<Alert key={`err-site-${idx}`} message={inst._error_site} type="error" showIcon />)
                  if (inst._warning_level) errors.push(<Alert key={`warn-level-${idx}`} message={inst._warning_level} type="warning" showIcon />)
                  return errors.length > 0 ? <div key={`alerts-${idx}`} style={{ marginBottom: 16 }}>{errors}</div> : null
                })}
              </Card>
            )}
          </>
        )

      case 2: // Execute
        return (
          <Card title="执行批量导入">
            {executing && <Spin tip="正在导入..." />}
            <Progress percent={executeProgress} status={executing ? 'active' : 'success'} />
            {executeProgress === 100 && !executing && <Alert message="导入完成" type="success" />}
          </Card>
        )

      case 3: // Results
        return (
          <Card title="导入结果">
            {importResult && (
              <>
                <Alert
                  message={`成功导入 ${importResult.success_count || 0} 个乐器`}
                  type="success"
                  style={{ marginBottom: 24 }}
                />
                {importResult.errors && importResult.errors.length > 0 && (
                  <Card title="失败项" type="inner">
                    <List
                      dataSource={importResult.errors}
                      renderItem={(error) => (
                        <List.Item>
                          <Text type="danger">{error.sn}: {error.reason}</Text>
                        </List.Item>
                      )}
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
          {steps.map((step) => (
            <Step key={step.title} title={step.title} />
          ))}
        </Steps>

        {renderStepContent()}

        <div style={{ marginTop: 24, textAlign: 'right' }}>
          <Space>
            {currentStep > 0 && currentStep < 3 && (
              <Button onClick={() => setCurrentStep(currentStep - 1)}>上一步</Button>
            )}
            {currentStep === 1 && (
              <Button
                type="primary"
                onClick={handleExecuteImport}
                disabled={!previewData?.can_import || executing}
              >
                开始导入
              </Button>
            )}
            {currentStep === 3 && (
              <Button
                type="primary"
                onClick={() => window.location.href = '/instruments/list'}
              >
                查看乐器列表
              </Button>
            )}
          </Space>
        </div>
      </Card>
    </div>
  )
}