import { useEffect, useState } from 'react'
import { Card, Tree, Descriptions, Button, Modal, Form, Input, Select, Switch, message, Spin, Empty, Space, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, AppstoreOutlined } from '@ant-design/icons'
import { api, categoriesApi } from '../../../services/api'
import CategoryForm from './Form'

const { Option } = Select

export default function CategoryList() {
  const [treeData, setTreeData] = useState([])
  const [selectedCategory, setSelectedCategory] = useState(null)
  const [editingCategory, setEditingCategory] = useState(null)
  const [loading, setLoading] = useState(true)
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)
  const [viewMode, setViewMode] = useState('detail') // 'detail' | 'form'
  const [formMode, setFormMode] = useState('create') // 'create' | 'edit'
  const [expandedKeys, setExpandedKeys] = useState([])
  const [selectedKeys, setSelectedKeys] = useState([])
  const [formVisible, setFormVisible] = useState(false)

  useEffect(() => {
    fetchCategoryTree()
  }, [])

  const fetchCategoryTree = async () => {
    try {
      setLoading(true)
      const result = await api.get('/categories')
      
      // Handle API response format: { code: 20000, data: { list: [...] } }
      const data = result?.data?.list || []
      
      // Convert to tree nodes format for Tree component
      const treeNodes = convertToTreeNodes(data)
      setTreeData(treeNodes)
      
      // Expand first level by default
      if (treeNodes.length > 0) {
        setExpandedKeys(treeNodes.map(node => node.key))
      }
    } catch (err) {
      message.error('加载分类数据失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  /*
   * Convert categories data to Tree component format
   * Input: [{id, name, icon, level, sort, visible, sub_categories: [...]}]
   * Output: [{key, title, data, children: [...]}]
   */
  const convertToTreeNodes = (categories) => {
    return categories.map(cat => {
      const hasChildren = cat.sub_categories && cat.sub_categories.length > 0
      return {
        key: cat.id,
        title: cat.name,
        data: cat,
        children: hasChildren ? convertToTreeNodes(cat.sub_categories) : []
      }
    })
  }

  const findCategoryById = (nodes, id) => {
    for (const node of nodes) {
      if (node.key === id) return node.data
      if (node.children) {
        const found = findCategoryById(node.children, id)
        if (found) return found
      }
    }
    return null
  }

  const onSelect = (selectedKeys) => {
    setSelectedKeys(selectedKeys)
    if (selectedKeys.length > 0) {
      const category = findCategoryById(treeData, selectedKeys[0])
      setSelectedCategory(category)
      setViewMode('detail')
    } else {
      setSelectedCategory(null)
    }
  }

  const handleCreateTopLevel = () => {
    setEditingCategory(null)
    setFormMode('create')
    setFormVisible(true)
  }

  const handleCreateSubCategory = () => {
    if (!selectedCategory) {
      message.warning('请先选择一个分类')
      return
    }
    setEditingCategory({ parent_id: selectedCategory.id })
    setFormMode('create')
    setFormVisible(true)
  }

  const handleEdit = () => {
    if (!selectedCategory) return
    setEditingCategory({ ...selectedCategory })
    setFormMode('edit')
    setFormVisible(true)
  }

  const handleDelete = async () => {
    if (!selectedCategory) return
    try {
      await categoriesApi.delete(selectedCategory.id)
      message.success('删除成功')
      setSelectedCategory(null)
      setSelectedKeys([])
      fetchCategoryTree()
    } catch (err) {
      message.error('删除失败: ' + err.message)
    }
  }

  const handleFormSubmit = async (data) => {
    setSaving(true)
    try {
      let result
      if (formMode === 'edit' && editingCategory?.id) {
        result = await categoriesApi.update(editingCategory.id, data)
        message.success('更新成功')
      } else {
        result = await categoriesApi.create(data)
        message.success('创建成功')
      }
      
      // Refresh tree data
      await fetchCategoryTree()
      
      // Update selected category if editing
      if (formMode === 'edit' && editingCategory?.id) {
        const updatedCategory = await api.get(`/categories/${editingCategory.id}`)
        setSelectedCategory(updatedCategory?.data || null)
      }
      
      // Close form
      setFormVisible(false)
      setEditingCategory(null)
      setViewMode('detail')
    } catch (error) {
      message.error(error.message || '操作失败')
    } finally {
      setSaving(false)
    }
  }

  const renderEmptyState = () => (
    <div className="flex flex-col items-center justify-center h-full text-gray-400">
      <AppstoreOutlined style={{ fontSize: 64, marginBottom: 16 }} />
      <p className="text-lg mb-4">暂无分类数据</p>
      <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>
        创建顶级分类
      </Button>
    </div>
  )

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="p-4">
      <div className="flex gap-4">
        {/* Left Panel - Tree */}
        <Card 
          title="分类结构" 
          className="w-1/3"
          extra={
            <Button type="primary" size="small" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>
              创建顶级分类
            </Button>
          }
        >
          {treeData.length === 0 ? (
            renderEmptyState()
          ) : (
            <Tree
              showIcon
              treeData={treeData}
              selectedKeys={selectedKeys}
              expandedKeys={expandedKeys}
              onSelect={onSelect}
              onExpand={setExpandedKeys}
              defaultExpandAll
            />
          )}
        </Card>

        {/* Right Panel - Detail or Form */}
        <div className="w-2/3">
          {viewMode === 'detail' && (
            <Card 
              title="分类详情" 
              extra={
                selectedCategory && (
                  <Space>
                    <Button icon={<PlusOutlined />} onClick={handleCreateSubCategory}>
                      创建下级分类
                    </Button>
                    <Button icon={<EditOutlined />} onClick={handleEdit}>
                      编辑
                    </Button>
                    <Popconfirm
                      title="确定要删除该分类吗？"
                      onConfirm={handleDelete}
                      okText="确定"
                      cancelText="取消"
                    >
                      <Button danger icon={<DeleteOutlined />}>
                        删除
                      </Button>
                    </Popconfirm>
                  </Space>
                )
              }
            >
              {selectedCategory ? (
                <Descriptions column={2} bordered>
                  <Descriptions.Item label="分类名称">{selectedCategory.name}</Descriptions.Item>
                  <Descriptions.Item label="分类图标">{selectedCategory.icon || '-'}</Descriptions.Item>
                  <Descriptions.Item label="级别" span={2}>
                    {selectedCategory.level === 1 ? '一级分类' : '二级分类'}
                  </Descriptions.Item>
                  <Descriptions.Item label="排序序号">{selectedCategory.sort || '-'}</Descriptions.Item>
                  <Descriptions.Item label="父级分类">
                    {selectedCategory.parent_id ? '有' : '顶级分类'}
                  </Descriptions.Item>
                  <Descriptions.Item label="显示状态">
                    {selectedCategory.visible ? '显示' : '隐藏'}
                  </Descriptions.Item>
                </Descriptions>
              ) : (
                <Empty description="请选择左侧分类查看详情" />
              )}
            </Card>
          )}
        </div>
      </div>

      {/* Form Modal */}
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
