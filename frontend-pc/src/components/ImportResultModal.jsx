import { Modal, Alert, Typography, List } from 'antd'
import Row from 'antd/es/grid/row'
import Col from 'antd/es/grid/col'
import { CheckCircleOutlined, CloseCircleOutlined, WarningOutlined } from '@ant-design/icons'

const { Text, Paragraph } = Typography

export default function ImportResultModal({ visible, onClose, importResult }) {
  if (!importResult) return null

  const {
    success = 0,
    failed = 0,
    total = 0,
    errors = []
  } = importResult

  const successRate = total > 0 ? ((success / total) * 100).toFixed(1) : 0

  return (
    <Modal
      title="导入结果"
      visible={visible}
      onOk={onClose}
      onCancel={onClose}
      width={600}
      footer={[
        <button key="close" onClick={onClose} className="ant-btn ant-btn-primary">
          确定
        </button>
      ]}
    >
      <div className="mb-6">
        <Row gutter={16} className="mb-4">
          <Col span={8}>
            <div className="text-center p-4 bg-gray-50 rounded">
              <div className="text-2xl font-bold text-blue-600">{total}</div>
              <div className="text-sm text-gray-600">总计</div>
            </div>
          </Col>
          <Col span={8}>
            <div className="text-center p-4 bg-green-50 rounded">
              <div className="text-2xl font-bold text-green-600">{success}</div>
              <div className="text-sm text-gray-600">成功</div>
            </div>
          </Col>
          <Col span={8}>
            <div className="text-center p-4 bg-red-50 rounded">
              <div className="text-2xl font-bold text-red-600">{failed}</div>
              <div className="text-sm text-gray-600">失败</div>
            </div>
          </Col>
        </Row>

        <div className="text-center mb-4">
          <Text strong>成功率: {successRate}%</Text>
        </div>

        {errors.length > 0 && (
          <div>
            <Paragraph className="mb-2">
              <WarningOutlined className="mr-1 text-orange-500" />
              <Text strong>错误详情:</Text>
            </Paragraph>
            <List
              size="small"
              bordered
              dataSource={errors}
              renderItem={(error) => (
                <List.Item className="text-red-600 text-sm">
                  <CloseCircleOutlined className="mr-2 text-red-500" />
                  <Text type="danger">{error}</Text>
                </List.Item>
              )}
              style={{ maxHeight: '200px', overflowY: 'auto' }}
            />
          </div>
        )}
      </div>

      {errors.length === 0 && (
        <Alert
          message="导入成功"
          description="所有数据已成功导入系统"
          type="success"
          showIcon
          icon={<CheckCircleOutlined />}
        />
      )}
    </Modal>
  )
}