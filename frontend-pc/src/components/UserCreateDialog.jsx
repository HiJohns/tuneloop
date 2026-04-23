import { Form, Input, Select, Button, Space } from 'antd'

const { Option } = Select

export default function UserCreateDialog({ form, onSubmit, onCancel, siteOptions, positionOptions }) {
  return (
    <Form
      form={form}
      layout="vertical"
      onFinish={onSubmit}
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

      <Form.Item
        name="position"
        label="职位"
        rules={[{ required: true, message: '请选择职位' }]}
      >
        <Select placeholder="请选择职位">
          {positionOptions.map(pos => (
            <Option key={pos} value={pos}>
              {pos}
            </Option>
          ))}
        </Select>
      </Form.Item>

      <Form.Item
        name="user_type"
        label="用户类型"
        initialValue="staff"
        rules={[{ required: true, message: '请选择用户类型' }]}
      >
        <Select placeholder="请选择用户类型">
          <Option value="staff">员工</Option>
          <Option value="admin">管理员</Option>
          <Option value="manager">网点经理</Option>
        </Select>
      </Form.Item>

      <Form.Item>
        <Space>
          <Button type="primary" htmlType="submit">
            创建
          </Button>
          <Button onClick={onCancel}>
            取消
          </Button>
        </Space>
      </Form.Item>
    </Form>
  )
}