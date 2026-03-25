import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Table, Button, Input, Space, Tag, Image, message, Popconfirm, Select, Modal, Form, InputNumber } from 'antd'
import { PlusOutlined, SearchOutlined, EditOutlined, DeleteOutlined, EyeOutlined, ArrowUpOutlined, ArrowDownOutlined, DollarOutlined } from '@ant-design/icons'
import InstrumentForm from './Form'

const { Option } = Select

export default function InstrumentList() {
  const navigate = useNavigate()
  const [instruments, setInstruments] = useState([])
  const [loading, setLoading] = useState(false)
  const [searchText, setSearchText] = useState('')
  const [categoryFilter, setCategoryFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [formVisible, setFormVisible] = useState(false)
  const [editingInstrument, setEditingInstrument] = useState(null)
  const [categories, setCategories] = useState([])
  const [selectedRowKeys, setSelectedRowKeys] = useState([])
  const [batchPriceModalVisible, setBatchPriceModalVisible] = useState(false)
  const [batchPriceForm] = Form.useForm()
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchInstruments()
    fetchCategories()
  }, [])

  const fetchInstruments = async () => {
    setLoading(true)
    try {
      const response = await fetch(`${API_BASE_URL}/instruments`)
      if (!response.ok) throw new Error('Failed to fetch instruments')
      
      const data = await response.json()
      if (data.code === 20000) {
        setInstruments(data.data || [])
      } else {
        // Fallback demo data
        setInstruments([
          {
            id: '1',
            name: '雅马哈立式钢琴 U1',
            category_name: '钢琴',
            brand: 'Yamaha',
            model: 'U1',
            daily_rate: 50,
            monthly_rate: 1200,
            deposit: 5000,
            stock: 5,
            images: ['/images/piano1.jpg'],
            status: 'available'
          },
          {
            id: '2',
            name: '马丁 D-28 吉他',
            category_name: '吉他',
            brand: 'Martin',
            model: 'D-28',
            daily_rate: 30,
            monthly_rate: 600,
            deposit: 2000,
            stock: 8,
            images: ['/images/guitar1.jpg'],
            status: 'available'
          }
        ])
      }
    } catch (error) {
      message.error('加载乐器失败: ' + error.message)
      // Fallback demo data
      setInstruments([
        {
          id: '1',
          name: '雅马哈立式钢琴 U1',
          category_name: '钢琴',
          brand: 'Yamaha',
          model: 'U1',
          daily_rate: 50,
          monthly_rate: 1200,
          deposit: 5000,
          stock: 5,
          images: ['/images/piano1.jpg'],
          status: 'available'
        }
      ])
    } finally {
      setLoading(false)
    }
  }

  const fetchCategories = async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/categories`)
      if (!response.ok) throw new Error('Failed to fetch categories')
      
      const data = await response.json()
      if (data.code === 20000) {
        setCategories(data.data || [])
      }
    } catch (error) {
      console.error('Load categories failed:', error)
    }
  }

  const filteredInstruments = instruments.filter(instrument => {
    const matchText = instrument.name.toLowerCase().includes(searchText.toLowerCase()) ||
                      instrument.brand?.toLowerCase().includes(searchText.toLowerCase()) ||
                      instrument.model?.toLowerCase().includes(searchText.toLowerCase())
    const matchCategory = !categoryFilter || instrument.category_name === categoryFilter
    const matchStatus = !statusFilter || instrument.status === statusFilter
    return matchText && matchCategory && matchStatus
  })

  const columns = [
    {
      title: '图片',
      dataIndex: 'images',
      key: 'images',
      width: 80,
      render: (images) => (
        <Image
          src={images && images.length > 0 ? images[0] : '/images/default-instrument.jpg'}
          alt="instrument"
          width={60}
          height={60}
          className="object-cover rounded"
        />
      )
    },
    {
      title: '乐器名称',
      dataIndex: 'name',
      key: 'name',
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (text, record) => (
        <div>
          <div className="font-medium">{text}</div>
          <div className="text-xs text-gray-500">{record.brand} {record.model}</div>
        </div>
      )
    },
    {
      title: '分类',
      dataIndex: 'category_name',
      key: 'category_name',
      width: 120,
      filters: [...new Set(instruments.map(i => i.category_name))].map(cat => ({
        text: cat,
        value: cat
      })),
      onFilter: (value, record) => record.category_name === value
    },
    {
      title: '日租金',
      dataIndex: 'daily_rate',
      key: 'daily_rate',
      width: 100,
      sorter: (a, b) => a.daily_rate - b.daily_rate,
      render: (rate) => `¥${rate}`
    },
    {
      title: '月租金',
      dataIndex: 'monthly_rate',
      key: 'monthly_rate',
      width: 100,
      sorter: (a, b) => a.monthly_rate - b.monthly_rate,
      render: (rate) => `¥${rate}`
    },
    {
      title: '押金',
      dataIndex: 'deposit',
      key: 'deposit',
      width: 100,
      render: (deposit) => `¥${deposit}`
    },
    {
      title: '库存',
      dataIndex: 'stock',
      key: 'stock',
      width: 80,
      sorter: (a, b) => a.stock - b.stock,
      render: (stock) => (
        <Tag color={stock > 0 ? 'green' : 'red'}>{stock}</Tag>
      )
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => {
        const statusMap = {
          available: { color: 'green', text: '可租' },
          rented: { color: 'orange', text: '已租出' },
          maintenance: { color: 'red', text: '维修中' }
        }
        const config = statusMap[status] || { color: 'default', text: '未知' }
        return <Tag color={config.color}>{config.text}</Tag>
      }
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space>
          <Button
            type="link"
            icon={<EyeOutlined />}
            onClick={() => viewInstrument(record.id)}
          >
            查看
          </Button>
          <Button
            type="link"
            icon={<EditOutlined />}
            onClick={() => editInstrument(record)}
          >
            编辑
          </Button>
          <Popconfirm
            title="确定要删除这个乐器吗？"
            onConfirm={() => deleteInstrument(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="link" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      )
    }
  ]

  const viewInstrument = (id) => {
    navigate(`/instruments/detail/${id}`)
  }

  const editInstrument = (instrument) => {
    setEditingInstrument(instrument)
    setFormVisible(true)
  }

  const deleteInstrument = async (id) => {
    try {
      const response = await fetch(`${API_BASE_URL}/instruments/${id}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json',
        }
      })
      
      if (!response.ok) throw new Error('删除失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success('删除成功')
        fetchInstruments()
      } else {
        throw new Error(result.message || '删除失败')
      }
    } catch (error) {
      message.error(error.message || '删除失败')
    }
  }

  const addInstrument = () => {
    setEditingInstrument(null)
    setFormVisible(true)
  }

  const handleFormSubmit = (data) => {
    message.success('乐器保存成功')
    fetchInstruments()
    setFormVisible(false)
    setEditingInstrument(null)
  }

  // Batch operations
  const handleRowSelection = {
    selectedRowKeys,
    onChange: (selectedKeys) => setSelectedRowKeys(selectedKeys)
  }

  const batchChangeStatus = async (status) => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要操作的乐器')
      return
    }
    
    try {
      const response = await fetch(`${API_BASE_URL}/instruments/batch/status`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ids: selectedRowKeys,
          status: status
        })
      })
      
      if (!response.ok) throw new Error('批量操作失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success(`已成功更新 ${selectedRowKeys.length} 个乐器状态`)
        setSelectedRowKeys([])
        fetchInstruments()
      } else {
        throw new Error(result.message || '批量操作失败')
      }
    } catch (error) {
      message.error(error.message || '批量操作失败')
    }
  }

  const batchDelete = async () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要删除的乐器')
      return
    }
    
    Modal.confirm({
      title: '批量删除确认',
      content: `确定要删除选中的 ${selectedRowKeys.length} 个乐器吗？此操作不可恢复！`,
      okText: '确定删除',
      cancelText: '取消',
      okButtonProps: { danger: true },
      onOk: async () => {
        try {
          const response = await fetch(`${API_BASE_URL}/instruments/batch`, {
            method: 'DELETE',
            headers: {
              'Content-Type': 'application/json',
            },
            body: JSON.stringify({ ids: selectedRowKeys })
          })
          
          if (!response.ok) throw new Error('批量删除失败')
          
          const result = await response.json()
          if (result.code === 20000) {
            message.success(`已成功删除 ${selectedRowKeys.length} 个乐器`)
            setSelectedRowKeys([])
            fetchInstruments()
          } else {
            throw new Error(result.message || '批量删除失败')
          }
        } catch (error) {
          message.error(error.message || '批量删除失败')
        }
      }
    })
  }

  const showBatchPriceModal = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请先选择要修改价格的乐器')
      return
    }
    batchPriceForm.resetFields()
    setBatchPriceModalVisible(true)
  }

  const handleBatchPriceSubmit = async () => {
    try {
      const values = await batchPriceForm.validateFields()
      
      const response = await fetch(`${API_BASE_URL}/instruments/batch/price`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ids: selectedRowKeys,
          price_type: values.price_type,
          amount: values.amount,
          operator: values.operator
        })
      })
      
      if (!response.ok) throw new Error('批量价格修改失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success(`已成功修改 ${selectedRowKeys.length} 个乐器价格`)
        setBatchPriceModalVisible(false)
        setSelectedRowKeys([])
        fetchInstruments()
      } else {
        throw new Error(result.message || '批量价格修改失败')
      }
    } catch (error) {
      if (error.errorFields) {
        // Form validation error
        console.error('Validation failed:', error)
      } else {
        message.error(error.message || '批量价格修改失败')
      }
    }
  }

  return (
    <div className="p-6">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">乐器管理</h1>
        <p className="text-gray-600 mt-1">管理可租赁的乐器信息、价格和库存</p>
      </div>

      {/* Batch Operations Toolbar */}
      {selectedRowKeys.length > 0 && (
        <div className="mb-4 p-3 bg-blue-50 border border-blue-200 rounded flex justify-between items-center">
          <div className="flex items-center">
            <span className="mr-4">已选择 <span className="font-bold">{selectedRowKeys.length}</span> 项</span>
            <Space>
              <Button
                size="small"
                icon={<ArrowUpOutlined />}
                onClick={() => batchChangeStatus('active')}
              >
                批量上架
              </Button>
              <Button
                size="small"
                icon={<ArrowDownOutlined />}
                onClick={() => batchChangeStatus('inactive')}
              >
                批量下架
              </Button>
              <Button
                size="small"
                icon={<DollarOutlined />}
                onClick={showBatchPriceModal}
              >
                批量改价
              </Button>
              <Button
                size="small"
                danger
                onClick={batchDelete}
              >
                批量删除
              </Button>
            </Space>
          </div>
          <Button size="small" onClick={() => setSelectedRowKeys([])}>取消选择</Button>
        </div>
      )}

      {/* Toolbar */}
      <div className="mb-4 flex justify-between items-center">
        <Space>
          <Input
            placeholder="搜索乐器名称..."
            prefix={<SearchOutlined className="text-gray-400" />}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 250 }}
            allowClear
          />
          <Select
            placeholder="选择分类"
            style={{ width: 150 }}
            allowClear
            onChange={setCategoryFilter}
          >
            {categories.map(cat => (
              <Option key={cat.id} value={cat.name}>{cat.name}</Option>
            ))}
          </Select>
          <Select
            placeholder="选择状态"
            style={{ width: 120 }}
            allowClear
            onChange={setStatusFilter}
          >
            <Option value="available">可租</Option>
            <Option value="rented">已租出</Option>
            <Option value="maintenance">维修中</Option>
          </Select>
        </Space>
        
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={addInstrument}
        >
          新增乐器
        </Button>
      </div>

      {/* Table */}
      <Table
        columns={columns}
        dataSource={filteredInstruments}
        rowKey="id"
        loading={loading}
        rowSelection={handleRowSelection}
        pagination={{
          pageSize: 10,
          showSizeChanger: true,
          showTotal: (total) => `共 ${total} 条`
        }}
      />
      
      {/* Batch Price Modal */}
      <Modal
        title="批量修改价格"
        visible={batchPriceModalVisible}
        onOk={handleBatchPriceSubmit}
        onCancel={() => setBatchPriceModalVisible(false)}
        width={500}
      >
        <Form
          form={batchPriceForm}
          layout="vertical"
        >
          <Form.Item
            name="price_type"
            label="价格类型"
            rules={[{ required: true, message: '请选择价格类型' }]}
          >
            <Select placeholder="请选择价格类型">
              <Option value="daily_rate">日租金</Option>
              <Option value="weekly_rate">周租金</Option>
              <Option value="monthly_rate">月租金</Option>
              <Option value="deposit">押金</Option>
            </Select>
          </Form.Item>
          
          <Form.Item
            name="operator"
            label="操作"
            rules={[{ required: true, message: '请选择操作' }]}
          >
            <Select placeholder="请选择操作">
              <Option value="set">设置为</Option>
              <Option value="increase">增加</Option>
              <Option value="decrease">减少</Option>
              <Option value="multiply">乘以</Option>
            </Select>
          </Form.Item>
          
          <Form.Item
            name="amount"
            label="金额/比例"
            rules={[{ required: true, message: '请输入金额或比例' }]}
          >
            <InputNumber min={0} style={{ width: '100%' }} placeholder="输入数值" />
          </Form.Item>
        </Form>
      </Modal>
      
      <InstrumentForm
        visible={formVisible}
        onCancel={() => {
          setFormVisible(false)
          setEditingInstrument(null)
        }}
        onSubmit={handleFormSubmit}
        initialData={editingInstrument}
        categories={categories}
      />
    </div>
  )
}