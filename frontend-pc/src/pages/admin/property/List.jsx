import { useState, useEffect } from 'react'
import { Card, Table, Button, Modal, Form, Input, Select, Tag, message, Space, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, CheckOutlined, MergeOutlined } from '@ant-design/icons'
import { api } from '../../../services/api'

const { Option } = Select

export default function PropertyList() {
  const [properties, setProperties] = useState([])
  const [selectedProperty, setSelectedProperty] = useState(null)
  const [loading, setLoading] = useState(true)
  const [propertyModalVisible, setPropertyModalVisible] = useState(false)
  const [mergeModalVisible, setMergeModalVisible] = useState(false)
  const [editingProperty, setEditingProperty] = useState(null)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()
  const [mergeForm] = Form.useForm()

  useEffect(() => {
    fetchProperties()
  }, [])

  const fetchProperties = async () => {
    try {
      setLoading(true)
      const result = await api.get('/properties')
      if (result.code === 20000) {
        setProperties(result.data || [])
      }
    } catch (err) {
      message.error('加载属性失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleCreateProperty = () => {
    setEditingProperty(null)
    form.resetFields()
    setPropertyModalVisible(true)
  }

  const handleEditProperty = (record) => {
    setEditingProperty(record)
    form.setFieldsValue(record)
    setPropertyModalVisible(true)
  }

  const handleSubmitProperty = async () => {
    try {
      const values = await form.validateFields()
      setSaving(true)

      const data = {
        name: values.name,
        property_type: values.property_type,
        is_required: values.is_required || false,
        unit: values.unit || '',
      }

      if (editingProperty?.id) {
        await api.put(`/property/${editingProperty.id}`, data)
        message.success('更新成功')
      } else {
        await api.post('/property', data)
        message.success('创建成功')
      }

      setPropertyModalVisible(false)
      fetchProperties()
    } catch (err) {
      if (err.errorFields) return
      message.error('操作失败: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleCreateOption = async (propertyId) => {
    const value = prompt('请输入属性值:')
    if (!value) return
    try {
      await api.post('/property/option', { property_id: propertyId, value })
      message.success('添加成功')
      fetchProperties()
    } catch (err) {
      message.error('添加失败: ' + err.message)
    }
  }

  const handleConfirm = async (propertyId, value) => {
    try {
      await api.put('/property/confirm', { property_id: propertyId, value })
      message.success('核定成功')
      fetchProperties()
    } catch (err) {
      message.error('核定失败: ' + err.message)
    }
  }

  const handleMerge = (option) => {
    setSelectedProperty({ ...option, propertyId: option.property_id })
    setMergeModalVisible(true)
  }

  const handleSubmitMerge = async () => {
    try {
      const values = await mergeForm.validateFields()
      await api.put('/property/merge', {
        property_id: selectedProperty.propertyId,
        source_value: selectedProperty.value,
        target_value: values.target_value,
      })
      message.success('归并成功')
      setMergeModalVisible(false)
      fetchProperties()
    } catch (err) {
      if (err.errorFields) return
      message.error('归并失败: ' + err.message)
    }
  }

  const propertyColumns = [
    {
      title: '属性名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '类型',
      dataIndex: 'property_type',
      key: 'property_type',
    },
    {
      title: '必填',
      dataIndex: 'is_required',
      key: 'is_required',
      render: (val) => val ? '是' : '否',
    },
    {
      title: '单位',
      dataIndex: 'unit',
      key: 'unit',
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space>
          <Button size="small" icon={<PlusOutlined />} onClick={() => handleCreateOption(record.id)}>
            添加值
          </Button>
          <Button size="small" icon={<EditOutlined />} onClick={() => handleEditProperty(record)}>
            编辑
          </Button>
        </Space>
      ),
    },
  ]

  const optionColumns = [
    {
      title: '属性值',
      dataIndex: 'value',
      key: 'value',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const config = {
          confirmed: { color: 'green', text: '已核定' },
          pending: { color: 'orange', text: '待核定' },
          abort: { color: 'red', text: '已废弃' },
        }
        const c = config[status] || config.pending
        return <Tag color={c.color}>{c.text}</Tag>
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => {
        if (record.status === 'confirmed') {
          return <span className="text-gray-400">-</span>
        }
        if (record.status === 'abort') {
          return <span className="text-gray-400">已归并</span>
        }
        return (
          <Space>
            <Button size="small" type="primary" icon={<CheckOutlined />} onClick={() => handleConfirm(record.property_id, record.value)}>
              核定
            </Button>
            <Button size="small" icon={<MergeOutlined />} onClick={() => handleMerge(record)}>
              归并
            </Button>
          </Space>
        )
      },
    },
  ]

  const selectedPropertyData = properties.find(p => p.id === selectedProperty?.id)
  const options = selectedPropertyData?.options || []

  const confirmedOptions = options.filter(o => o.status === 'confirmed')

  return (
    <div>
      <h2 className="text-xl font-bold mb-4">属性管理</h2>
      
      <div className="flex gap-4">
        <Card title="属性列表" className="w-1/3">
          <Table
            columns={propertyColumns}
            dataSource={properties}
            rowKey="id"
            loading={loading}
            size="small"
            onRow={(record) => ({
              onClick: () => setSelectedProperty(record),
              style: { cursor: 'pointer', background: selectedProperty?.id === record.id ? '#f0f5ff' : '' }
            })}
            pagination={false}
          />
          <Button type="dashed" block icon={<PlusOutlined />} onClick={handleCreateProperty} className="mt-2">
            添加属性
          </Button>
        </Card>

        <Card title="属性值矩阵" className="w-2/3">
          {selectedProperty ? (
            <>
              <div className="mb-2 font-medium">
                当前属性: {selectedProperty.name} ({selectedProperty.property_type})
              </div>
              <Table
                columns={optionColumns}
                dataSource={options}
                rowKey="id"
                pagination={false}
              />
            </>
          ) : (
            <div className="text-center text-gray-400 py-12">
              请在左侧选择属性查看其值
            </div>
          )}
        </Card>
      </div>

      <Modal
        title={editingProperty ? '编辑属性' : '添加属性'}
        open={propertyModalVisible}
        onCancel={() => setPropertyModalVisible(false)}
        onOk={handleSubmitProperty}
        confirmLoading={saving}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="name"
            label="属性名称"
            rules={[{ required: true, message: '请输入属性名称' }]}
          >
            <Input placeholder="如：产地" />
          </Form.Item>
          <Form.Item
            name="property_type"
            label="类型"
            rules={[{ required: true, message: '请选择类型' }]}
          >
            <Select placeholder="请选择类型">
              <Option value="string">字符串</Option>
              <Option value="int">整数</Option>
              <Option value="float">小数</Option>
              <Option value="date">日期</Option>
            </Select>
          </Form.Item>
          <Form.Item
            name="is_required"
            label="是否必填"
            valuePropName="checked"
          >
            <Select>
              <Option value={true}>是</Option>
              <Option value={false}>否</Option>
            </Select>
          </Form.Item>
          <Form.Item name="unit" label="单位">
            <Input placeholder="如：cm, kg" />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="归并属性值"
        open={mergeModalVisible}
        onCancel={() => setMergeModalVisible(false)}
        onOk={handleSubmitMerge}
      >
        <div className="mb-4">
          <p>将属性值 <strong>{selectedProperty?.value}</strong> 归并到:</p>
        </div>
        <Form form={mergeForm} layout="vertical">
          <Form.Item
            name="target_value"
            label="目标核定值"
            rules={[{ required: true, message: '请选择目标值' }]}
          >
            <Select placeholder="请选择已核定的属性值">
              {confirmedOptions.map(opt => (
                <Option key={opt.id} value={opt.value}>{opt.value}</Option>
              ))}
            </Select>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
