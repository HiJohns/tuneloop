import { useState, useEffect } from 'react'
import { Table, InputNumber, Button, Space, Form, message, Card, Input, Select } from 'antd'
import { inventoryApi } from '../../../services/api'

const { Search } = Input
const { Option } = Select

export default function InventoryRentSetting() {
  const [loading, setLoading] = useState(false)
  const [instruments, setInstruments] = useState([])
  const [editedIds, setEditedIds] = useState(new Set())
  const [pagination, setPagination] = useState({
    current: 1,
    pageSize: 20,
    total: 0,
  })
  const [filters, setFilters] = useState({
    brand: '',
    model: '',
    category_id: '',
    level_id: '',
  })

  // Table data includes current edits
  const [tableData, setTableData] = useState([])

  useEffect(() => {
    loadData()
  }, [pagination.current, pagination.pageSize, filters])

  useEffect(() => {
    // Update table data when instruments change
    setTableData(instruments)
  }, [instruments])

  const loadData = async () => {
    setLoading(true)
    try {
      const response = await inventoryApi.getRentSetting({
        page: pagination.current,
        pageSize: pagination.pageSize,
        ...filters,
      })
      
      if (response.code === 20000) {
        setInstruments(response.data.list || [])
        setPagination({
          ...pagination,
          total: response.data.total,
        })
      } else {
        message.error('加载数据失败: ' + response.message)
      }
    } catch (error) {
      message.error('加载数据失败: ' + error.message)
      console.error('Failed to load inventory rent settings:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleRentChange = (id, value) => {
    // Mark as edited
    setEditedIds(prev => new Set(prev).add(id))
    
    // Update table data
    setTableData(prev => prev.map(item => 
      item.id === id ? { ...item, daily_rent: value } : item
    ))
  }

  const handleSave = async () => {
    if (editedIds.size === 0) {
      message.info("没有需要保存的修改")
      return
    }

    // Collect edited items
    const itemsToUpdate = tableData
      .filter(item => editedIds.has(item.id))
      .map(item => ({
        id: item.id,
        daily_rent: item.daily_rent,
      }))

    try {
      setLoading(true)
      const response = await inventoryApi.batchUpdateRent({ items: itemsToUpdate })
      
      if (response.code === 20000) {
        message.success(`成功更新 ${response.data.updated} 条记录`)
        setEditedIds(new Set()) // Clear edits
        await loadData() // Reload to confirm changes
      } else {
        message.error('保存失败: ' + response.message)
      }
    } catch (error) {
      message.error('保存失败: ' + error.message)
      console.error('Failed to batch update rent:', error)
    } finally {
      setLoading(false)
    }
  }

  const columns = [
    {
      title: '识别码',
      dataIndex: 'sn',
      key: 'sn',
      width: 120,
    },
    {
      title: '分类',
      dataIndex: 'category_name',
      key: 'category_name',
      width: 100,
    },
    {
      title: '级别',
      dataIndex: 'level_name',
      key: 'level_name',
      width: 80,
    },
    {
      title: '品牌',
      dataIndex: 'brand',
      key: 'brand',
      width: 100,
      render: (text) => text || '-',
    },
    {
      title: '型号',
      dataIndex: 'model',
      key: 'model',
      width: 100,
      render: (text) => text || '-',
    },
    {
      title: '网点',
      dataIndex: 'site_name',
      key: 'site_name',
      width: 120,
    },
    {
      title: '日租金',
      dataIndex: 'daily_rent',
      key: 'daily_rent',
      width: 120,
      render: (value, record) => (
        <InputNumber
          min={0}
          precision={2}
          value={value}
          onChange={(val) => handleRentChange(record.id, val)}
          style={{ width: '100%' }}
          formatter={(val) => `¥ ${val}`}
          parser={(val) => val.replace(/\¥\s?/g, '')}
        />
      ),
    },
  ]

  return (
    <div className="p-6">
      <Card title="库存管理&租金设定" extra={
        <Space>
          <Button
            type="primary"
            onClick={handleSave}
            disabled={editedIds.size === 0}
            loading={loading}
          >
            保存修改 ({editedIds.size})
          </Button>
          <Button onClick={loadData} loading={loading}>刷新</Button>
        </Space>
      }>
        <div className="mb-4">
          <Space wrap>
            <Search
              placeholder="品牌"
              value={filters.brand}
              onChange={(e) => setFilters({ ...filters, brand: e.target.value })}
              style={{ width: 150 }}
              onSearch={() => setPagination({ ...pagination, current: 1 })}
            />
            <Search
              placeholder="型号"
              value={filters.model}
              onChange={(e) => setFilters({ ...filters, model: e.target.value })}
              style={{ width: 150 }}
              onSearch={() => setPagination({ ...pagination, current: 1 })}
            />
            <Search
              placeholder="分类"
              value={filters.category_id}
              onChange={(e) => setFilters({ ...filters, category_id: e.target.value })}
              style={{ width: 150 }}
              onSearch={() => setPagination({ ...pagination, current: 1 })}
            />
            <Search
              placeholder="等级"
              value={filters.level_id}
              onChange={(e) => setFilters({ ...filters, level_id: e.target.value })}
              style={{ width: 150 }}
              onSearch={() => setPagination({ ...pagination, current: 1 })}
            />
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={tableData}
          rowKey="id"
          pagination={{
            ...pagination,
            showSizeChanger: true,
            showQuickJumper: true,
            onChange: (page, pageSize) => {
              setPagination({ ...pagination, current: page, pageSize })
            },
          }}
          loading={loading}
          rowClassName={(record) => editedIds.has(record.id) ? 'bg-blue-50' : ''}
          scroll={{ x: true }}
        />
      </Card>
    </div>
  )
}
