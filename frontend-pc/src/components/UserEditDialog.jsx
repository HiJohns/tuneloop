import { Form, Input, Select, Button, Space, Alert } from 'antd'

const { Option } = Select

export default function UserEditDialog({ form, onSubmit, onCancel, siteOptions, initialValues }) {
  return (
    <Form
      form={form}
      layout="vertical"
      onFinish={onSubmit}
      initialValues={initialValues}
    >
      <Form.Item
        name="name"
        label="姓名"
        rules={[{ required: true, message: '请输入姓名' }]}
      >
        <Input placeholder="请输入姓名" />
      </Form.Item>

      <Form.Item
        name="email"
        label="邮箱"
        rules={[
          { type: 'email', message: '请输入有效的邮箱地址' },
          { required: true, message: '请输入邮箱' }
        ]}
      >
        <Input placeholder="请输入邮箱" />
      </Form.Item>

      <Form.Item
        name="phone"
        label="手机号"
        rules={[{ required: true, message: '请输入手机号' }]}
      >
        <Input placeholder="请输入手机号" />
      </Form.Item>

      {initialValues?.email && (
        <Alert
          message="修改邮箱后，系统将发送确认邮件到新邮箱地址，需确认后方可生效。"
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      <Form.Item
        name="site_id"
        label="归属网点"
        rules={[{ required: true, message: '请选择归属网点' }]}
      >
        <Select 
          placeholder="请选择归属网点"
          style={{ width: '100%' }}
          dropdownStyle={{ maxHeight: 300, overflow: 'auto' }}
        >
          {siteOptions.map(option => (
            <Option key={option.key} value={option.value}>
              {option.label}
            </Option>
          ))}
        </Select>
      </Form.Item>

      <Form.Item>
        <Space>
          <Button type="primary" htmlType="submit">
            保存
          </Button>
          <Button onClick={onCancel}>
            取消
          </Button>
        </Space>
      </Form.Item>
    </Form>
  )
}