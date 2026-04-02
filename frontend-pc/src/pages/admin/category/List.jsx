import { useEffect, useState } from 'react'
import { Table, Button, Input, Space, Tag, Image, Popconfirm, message } from 'antd'
import CategoryForm from './Form'
import { SearchOutlined, PlusOutlined, EditOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons'
import { api } from '../../services/api'

export default function CategoryList() {
  const [loading, setLoading] = useState(true)
  const [categories, setCategories] = useState([])
  const [searchText, setSearchText] = useState('')
  const [formVisible, setFormVisible] = useState(false)
  const [editingCategory, setEditingCategory] = useState(null)
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchCategories()
  }, [])

  const fetchCategories = async () => {
    try {
      setLoading(true)
      const data = await api.get('/categories')
      setCategories(data || [])
    } catch (err) {
      console.error('Failed to fetch categories:', err)
      message.error('加载分类失败')
    } finally {
      setLoading(false)
    }
  }

  const handleSearch = (value) => {
    setSearchText(value.toLowerCase())
  }

  const filteredCategories = categories.filter(category => {
    const matchText = category.name.toLowerCase().includes(searchText) ||
                      category.icon?.toLowerCase().includes(searchText)
    return matchText
  })

  const columns = [
    {
      title: '图标',
      dataIndex: 'icon',
      key: 'icon',
      width: 80,
      render: (icon) => (
        <div className="text-2xl text-center">
          {icon || '🎵'}
        </div>
      )
    },
    {
      title: '分类名称',
      dataIndex: 'name',
      key: 'name',
      sorter: (a, b) => a.name.localeCompare(b.name),
      render: (text, record) => (
        <div className={record.level === 2 ? 'ml-6' : ''}>
          <span className={record.level === 2 ? 'text-gray-600' : 'font-semibold'}>
            {text}
          </span>
          {record.level === 1 && record.sub_categories?.length > 0 && (
            <span className="ml-2 text-xs text-gray-500">
              ({record.sub_categories.length} 子类)
            </span>
          )}
        </div>
      )
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      width: 100,
      render: (level) => (
        <Tag color={level === 1 ? 'blue' : 'green'}>
          {level === 1 ? '一级' : '二级'}
        </Tag>
      )
    },
    {
      title: '排序',
      dataIndex: 'sort',
      key: 'sort',
      width: 100,
      sorter: (a, b) => a.sort - b.sort
    },
    {
      title: '显示状态',
      dataIndex: 'visible',
      key: 'visible',
      width: 100,
      render: (visible) => (
        <Tag color={visible ? 'green' : 'red'}>
          {visible ? '显示' : '隐藏'}
        </Tag>
      )
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
            onClick={() => viewCategory(record.id)}
          >
            查看
          </Button>
          <Button
            type="link"
            icon={<EditOutlined />}
            onClick={() => editCategory(record)}
          >
            编辑
          </Button>
          <Popconfirm
            title="确定要删除这个分类吗？"
            onConfirm={() => deleteCategory(record.id)}
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

  const viewCategory = (id) => {
    message.info(`查看分类: ${id}`)
  }

  const deleteCategory = async (id) => {
    try {
      const response = await fetch(`${API_BASE_URL}/categories/${id}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json',
        }
      })
      
      if (!response.ok) throw new Error('删除失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success('删除成功')
        fetchCategories()
      } else {
        throw new Error(result.message || '删除失败')
      }
    } catch (error) {
      message.error(error.message || '删除失败')
    }
  }

  const addCategory = () => {
    setEditingCategory(null)
    setFormVisible(true)
  }

  const editCategory = (record) => {
    setEditingCategory(record)
    setFormVisible(true)
  }

  const handleFormSubmit = (data) => {
    message.success('分类保存成功')
    fetchCategories()
    setFormVisible(false)
    setEditingCategory(null)
  }

  return (
    <div className="p-6">
      {/* Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">乐器分类管理</h1>
        <p className="text-gray-600 mt-1">管理乐器的一级和二级分类</p>
      </div>

      {/* Toolbar */}
      <div className="mb-4 flex justify-between items-center">
        <Input
          placeholder="搜索分类名称..."
          prefix={<SearchOutlined className="text-gray-400" />}
          onChange={(e) => handleSearch(e.target.value)}
          style={{ width: 300 }}
          allowClear
        />
        
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={addCategory}
        >
          新增分类
        </Button>
      </div>

      {/* Table */}
      <Table
        columns={columns}
        dataSource={filteredCategories || []}
        rowKey="id"
        loading={loading}
        pagination={{
          pageSize: 10,
          showSizeChanger: true,
          showTotal: (total) => `共 ${total} 条`
        }}
        expandable={{
          expandedRowRender: (record) => {
            if (record.level === 1 && record.sub_categories && record.sub_categories.length > 0) {
              return (
                <Table
                  columns={columns}
                  dataSource={record.sub_categories || []}
                  rowKey="id"
                  pagination={false}
                  showHeader={false}
                />
              )
            }
            return null
          },
          rowExpandable: (record) => record.level === 1 && record.sub_categories && record.sub_categories.length > 0
        }}
      />
      
      <CategoryForm
        visible={formVisible}
        onCancel={() => {
          setFormVisible(false)
          setEditingCategory(null)
        }}
        onSubmit={handleFormSubmit}
        initialData={editingCategory}
      />
    </div>
  )
}