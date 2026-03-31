import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Table, Button, Input, Space, Tag, Image, message, Popconfirm, Select, Modal, Form, InputNumber, Upload, Checkbox } from 'antd'
import { Row, Col } from 'antd'
import { PlusOutlined, SearchOutlined, EditOutlined, DeleteOutlined, EyeOutlined, ArrowUpOutlined, ArrowDownOutlined, DollarOutlined, UploadOutlined, DownloadOutlined, ExportOutlined } from '@ant-design/icons'
import { api } from '../../../services/api'
import InstrumentForm from './Form'
import ImportResultModal from '../../../components/ImportResultModal'

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
  const [expandedRowKeys, setExpandedRowKeys] = useState([])
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'
  
  const [pagination, setPagination] = useState({
    page: 1,
    pageSize: 20,
    total: 0
  })
  
  // Import/Export state
  const [importResultModalVisible, setImportResultModalVisible] = useState(false)
  const [importResult, setImportResult] = useState(null)
  const [exportFieldModalVisible, setExportFieldModalVisible] = useState(false)
  const [selectedExportFields, setSelectedExportFields] = useState([])

  useEffect(() => {
    fetchInstruments(pagination.page, pagination.pageSize)
    fetchCategories()
    setSelectedExportFields(['name', 'brand', 'model', 'category_name', 'stock', 'status'])
  }, [])

  const fetchInstruments = async (page = 1, pageSize = 20) => {
    setLoading(true)
    try {
      const response = await fetch(`${API_BASE_URL}/instruments?page=${page}&pageSize=${pageSize}`)
      const result = await response.json()
      
      if (result.code === 20000) {
        setInstruments(Array.isArray(result.data) ? result.data : [])
        setPagination({
          page: result.pagination?.page || page,
          pageSize: result.pagination?.pageSize || pageSize,
          total: result.pagination?.total || 0
        })
      } else {
        setInstruments([])
      }
    } catch (error) {
      message.error('加载乐器失败: ' + error.message)
      setInstruments([])
    } finally {
      setLoading(false)
    }
  }

  const fetchCategories = async () => {
    try {
      const data = await api.get('/categories')
      setCategories(data || [])
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
      width: 120,
      render: (_, record) => (
        <Space>
          <Button
            type="link"
            icon={<EditOutlined />}
            onClick={() => navigate(`/instruments/${record.id}/edit`)}
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
      const result = await api.delete(`/instruments/${id}`)
      if (result.code === 20000) {
        message.success('删除成功')
        fetchInstruments(pagination.page, pagination.pageSize)
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
    fetchInstruments(pagination.page, pagination.pageSize)
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
      const result = await api.put('/instruments/batch/status', {
        ids: selectedRowKeys,
        status: status
      })
      
      if (result.code === 20000) {
        message.success(`已成功更新 ${selectedRowKeys.length} 个乐器状态`)
        setSelectedRowKeys([])
        fetchInstruments(pagination.page, pagination.pageSize)
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
          const result = await api.delete('/instruments/batch', {
            ids: selectedRowKeys
          })
          
          if (result.code === 20000) {
            message.success(`已成功删除 ${selectedRowKeys.length} 个乐器`)
            setSelectedRowKeys([])
            fetchInstruments(pagination.page, pagination.pageSize)
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
      
      const result = await api.put('/instruments/batch/price', {
        ids: selectedRowKeys,
        price_type: values.price_type,
        amount: values.amount,
        operator: values.operator
      })
      
      if (result.code === 20000) {
        message.success(`已成功修改 ${selectedRowKeys.length} 个乐器价格`)
        setBatchPriceModalVisible(false)
        setSelectedRowKeys([])
        fetchInstruments(pagination.page, pagination.pageSize)
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

  // Import/Export handlers
  const handleImport = async (file) => {
    try {
      const formData = new FormData()
      formData.append('file', file)
      
      const response = await fetch(`${API_BASE_URL}/instruments/import`, {
        method: 'POST',
        body: formData
      })
      
      const result = await response.json()
      if (result.code === 20000) {
        setImportResult({
          success: result.data?.success || 0,
          failed: result.data?.failed || 0,
          total: result.data?.total || 0,
          errors: result.data?.errors || []
        })
        setImportResultModalVisible(true)
        fetchInstruments(pagination.page, pagination.pageSize)
        message.success('导入完成')
      } else {
        throw new Error(result.message || '导入失败')
      }
    } catch (error) {
      message.error(error.message || '导入失败')
      setImportResult({
        success: 0,
        failed: 1,
        total: 1,
        errors: [error.message]
      })
      setImportResultModalVisible(true)
    }
    
    return false // Prevent automatic upload
  }

  const downloadTemplate = () => {
    const template = [
      ['name', 'brand', 'model', 'category_name', 'stock', 'description'],
      ['雅马哈立式钢琴', 'Yamaha', 'U1', '钢琴', '5', '日本原装进口钢琴'],
      ['马丁D-28吉他', 'Martin', 'D-28', '吉他', '8', '美国产经典民谣吉他']
    ]
    
    const csvContent = template.map(row => row.join(',')).join('\n')
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = 'instruments_template.csv'
    link.click()
    message.success('模板下载成功')
  }

  const showExportFieldModal = () => {
    setExportFieldModalVisible(true)
  }

  const handleExport = async () => {
    if (selectedExportFields.length === 0) {
      message.warning('请至少选择一个导出字段')
      return
    }
    
    try {
      const params = new URLSearchParams({
        fields: selectedExportFields.join(',')
      })
      
      const response = await fetch(`${API_BASE_URL}/instruments/export?${params}`, {
        method: 'GET'
      })
      
      if (!response.ok) throw new Error('导出失败')
      
      const blob = await response.blob()
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `instruments_export_${new Date().toISOString().split('T')[0]}.csv`
      link.click()
      window.URL.revokeObjectURL(url)
      
      setExportFieldModalVisible(false)
      message.success('导出成功')
    } catch (error) {
      message.error(error.message || '导出失败')
    }
  }

  const fieldOptions = [
    { label: '乐器名称', value: 'name' },
    { label: '品牌', value: 'brand' },
    { label: '型号', value: 'model' },
    { label: '分类', value: 'category_name' },
    { label: '库存', value: 'stock' },
    { label: '状态', value: 'status' },
    { label: '描述', value: 'description' }
  ]

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
          <Upload
            accept=".csv,.xlsx,.xls"
            showUploadList={false}
            beforeUpload={handleImport}
          >
            <Button
              icon={<UploadOutlined />}
            >
              导入
            </Button>
          </Upload>
          <Button
            icon={<DownloadOutlined />}
            onClick={downloadTemplate}
          >
            模板下载
          </Button>
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
        
        <Space>
          <Button
            icon={<ExportOutlined />}
            onClick={showExportFieldModal}
          >
            导出
          </Button>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            onClick={addInstrument}
          >
            新增乐器
          </Button>
        </Space>
      </div>

      {/* Note: Batch price modification removed as prices are now in expandable specifications */}

      {/* Table */}
      <Table
        columns={columns}
        dataSource={filteredInstruments || []}
        rowKey="id"
        loading={loading}
        rowSelection={handleRowSelection}
        expandedRowKeys={expandedRowKeys}
        onExpand={(expanded, record) => {
          if (expanded) {
            setExpandedRowKeys([...expandedRowKeys, record.id])
          } else {
            setExpandedRowKeys(expandedRowKeys.filter(key => key !== record.id))
          }
        }}
        expandIcon={({ expanded, onExpand, record }) => (
          <div
            onClick={e => onExpand(record, e)}
            style={{ cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          >
            {expanded ? (
              <span style={{ fontSize: '16px', fontWeight: 'bold' }}>-</span>
            ) : (
              <span style={{ fontSize: '16px', fontWeight: 'bold' }}>+</span>
            )}
          </div>
        )}
        expandedRowRender={(record) => {
          const specs = record.specifications || []
          if (specs.length === 0) {
            return <div style={{ padding: '16px', color: '#999' }}>暂无规格信息</div>
          }
          
          return (
            <div style={{ margin: '-16px', padding: '16px', background: '#fafafa' }}>
              <Table
                columns={[
                  { title: '规格名称', key: 'name', render: (text, spec) => spec.name },
                  { title: '日租金', key: 'daily', render: (text, spec) => `¥${spec.daily_rent || 0}` },
                  { title: '周租金', key: 'weekly', render: (text, spec) => `¥${spec.weekly_rent || 0}` },
                  { title: '月租金', key: 'monthly', render: (text, spec) => `¥${spec.monthly_rent || 0}` },
                  { 
                    title: '押金', 
                    key: 'deposit',
                    render: (text, spec) => `¥${spec.deposit || 0}`
                  },
                  { 
                    title: '库存', 
                    key: 'stock',
                    render: (text, spec) => spec.stock || 0
                  }
                ]}
                pagination={false}
                rowKey="name"
              />
            </div>
          )
        }}
        pagination={{
          current: pagination.page,
          pageSize: pagination.pageSize,
          total: pagination.total,
          showSizeChanger: true,
          showTotal: (total) => `共 ${total} 条`,
          onChange: (page, pageSize) => fetchInstruments(page, pageSize),
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
      
      {/* Export Field Selection Modal */}
      <Modal
        title="选择导出字段"
        visible={exportFieldModalVisible}
        onOk={handleExport}
        onCancel={() => setExportFieldModalVisible(false)}
        width={500}
      >
        <Checkbox.Group
          value={selectedExportFields}
          onChange={setSelectedExportFields}
          style={{ width: '100%' }}
        >
          <Row gutter={[8, 8]}>
            {fieldOptions.map(option => (
              <Col span={12} key={option.value}>
                <Checkbox value={option.value}>{option.label}</Checkbox>
              </Col>
            ))}
          </Row>
        </Checkbox.Group>
      </Modal>

      {/* Import Result Modal */}
      <ImportResultModal
        visible={importResultModalVisible}
        onClose={() => {
          setImportResultModalVisible(false)
          setImportResult(null)
        }}
        importResult={importResult}
      />

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