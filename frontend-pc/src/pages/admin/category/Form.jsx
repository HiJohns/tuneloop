import { useState, useEffect } from 'react'
import { Modal, Form, Input, Select, Upload, Switch, message, Button } from 'antd'
import { UploadOutlined } from '@ant-design/icons'

export default function CategoryForm({ visible, onCancel, onSubmit, initialData = null }) {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [categories, setCategories] = useState([])
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    if (visible) {
      fetchParentCategories()
      if (initialData) {
        form.setFieldsValue(initialData)
      } else {
        form.resetFields()
      }
    }
  }, [visible, initialData])

  const fetchParentCategories = async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/categories`)
      if (!response.ok) throw new Error('Failed to fetch categories')
      
      const data = await response.json()
      if (data.code === 20000) {
        // Filter only level 1 categories as parent options
        const level1Categories = (data.data || [])
          .filter(cat => cat.level === 1)
          .map(cat => ({ id: cat.id, name: cat.name }))
        setCategories(level1Categories)
      }
    } catch (error) {
      message.error('加载父级分类失败: ' + error.message)
      // Fallback demo data
      setCategories([
        { id: '1', name: '钢琴' },
        { id: '2', name: '吉他' },
        { id: '3', name: '古筝' }
      ])
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      setLoading(true)
      
      // Check name uniqueness (mock)
      if (!initialData && values.name === '钢琴') {
        message.error('分类名称 "钢琴" 已存在')
        setLoading(false)
        return
      }
      
      // Prepare form data
      const formData = {
        name: values.name,
        icon: values.icon || '',
        sort: values.sort || 99,
        visible: values.visible !== false,
        parent_id: values.parent_id || null,
      }
      
      // Submit to API
      const url = initialData ? `${API_BASE_URL}/categories/${initialData.id}` : `${API_BASE_URL}/categories`
      const method = initialData ? 'PUT' : 'POST'
      
      const response = await fetch(url, {
        method,
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(formData)
      })
      
      if (!response.ok) throw new Error('提交失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success(initialData ? '更新成功' : '创建成功')
        onSubmit(result.data)
        form.resetFields()
      } else {
        throw new Error(result.message || '提交失败')
      }
    } catch (error) {
      if (error.errorFields) {
        // Form validation error
        console.error('Validation failed:', error)
      } else {
        message.error(error.message || '提交失败')
      }
    } finally {
      setLoading(false)
    }
  }

  const title = initialData ? '编辑分类' : '新增分类'

  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={onCancel}
      onOk={handleSubmit}
      confirmLoading={loading}
      width={600}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          visible: true,
          sort: 99
        }}
      >
        <Form.Item
          name="name"
          label="分类名称"
          rules={[
            { required: true, message: '请输入分类名称' },
            { min: 2, message: '分类名称至少需要2个字符' },
            { max: 50, message: '分类名称不能超过50个字符' }
          ]}
        >
          <Input placeholder="请输入分类名称，如：钢琴" />
        </Form.Item>

        <Form.Item
          name="parent_id"
          label="父级分类"
          extra="不选择表示创建一级分类"
        >
          <Select
            placeholder="请选择父级分类（可选）"
            allowClear
            disabled={initialData?.level === 2} // Can't change parent for level 2
          >
            {categories.map(cat => (
              <Select.Option key={cat.id} value={cat.id}>
                {cat.name}
              </Select.Option>
            ))}
          </Select>
        </Form.Item>

        <Form.Item
          name="sort"
          label="排序序号"
          rules={[{ required: true, message: '请输入排序序号' }]}
        >
          <Input type="number" placeholder="请输入排序序号，数字越小越靠前" />
        </Form.Item>

        <Form.Item
          name="icon"
          label="分类图标"
          extra="输入emoji或图标URL，如：🎹 或 /images/icon.png"
        >
          <Input placeholder="请输入图标，如：🎹" />
        </Form.Item>

        <Form.Item
          name="visible"
          label="显示状态"
          valuePropName="checked"
        >
          <Switch checkedChildren="显示" unCheckedChildren="隐藏" defaultChecked />
        </Form.Item>
      </Form>
    </Modal>
  )
}
