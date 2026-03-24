import { useEffect, useState } from 'react'
import { Table, Button, Input, Space, Tag, Image, Popconfirm, message } from 'antd'
import { SearchOutlined, PlusOutlined, EditOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons'

export default function CategoryList() {
  const [loading, setLoading] = useState(true)
  const [categories, setCategories] = useState([])
  const [searchText, setSearchText] = useState('')
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchCategories()
  }, [])

  const fetchCategories = async () => {
    try {
      setLoading(true)
      const response = await fetch(`${API_BASE_URL}/categories`)
      if (!response.ok) throw new Error('Failed to fetch categories')
      
      const data = await response.json()
      if (data.code === 20000) {
        setCategories(data.data || [])
      } else {
        throw new Error(data.message || 'API error')
      }
    } catch (err) {
      message.error('加载分类失败: ' + err.message)
      // Fallback demo data
      setCategories([
        {
          id: '1',
          name: '钢琴',
          icon: '🎹',
          level: 1,
          sort: 1,
          visible: true,
          sub_categories: [
            {
              id: '1-1',
              name: '雅马哈',
              icon: '',
              level: 2,
              sort: 1,
              visible: true,
            },
            {
              id: '1-2',
              name: '卡瓦依',
              icon: '',
              level: 2,
              sort: 2,
              visible: true,
            }
          ]
        },
        {
          id: '2',
          name: '吉他',
          icon: '🎸',
          level: 1,
          sort: 2,
          visible: true,
          sub_categories: []
        },
        {
          id: '3',
          name: '古筝',
          icon: '🎵',
          level: 1,
          sort: 3,
          visible: false,
          sub_categories: []
        }
      ])
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
            onClick={() => editCategory(record.id)}
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

  const editCategory = (id) => {
    message.info(`编辑分类: ${id}`)
  }

  const deleteCategory = (id) => {
    message.success(`已删除分类: ${id}`)
    fetchCategories()
  }

  const addCategory = () => {
    message.info('打开新增分类表单')
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
        dataSource={filteredCategories}
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
                  dataSource={record.sub_categories}
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
    </div>
  )
}
