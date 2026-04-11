import { useEffect, useState } from 'react'
import { Card, Tree, Descriptions, Button, Modal, Form, Input, Select, Switch, TreeSelect, message, Spin, Empty, Space, Popconfirm } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, AppstoreOutlined } from '@ant-design/icons'
import { api, categoriesApi } from '../../../services/api'

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
  const [parentCategories, setParentCategories] = useState([])
  const [parentLoading, setParentLoading] = useState(false)
  const [parentCategoryName, setParentCategoryName] = useState('-')

  useEffect(() => {
    fetchCategoryTree()
    
    const path = window.location.pathname
    
    // Pattern 1: /categories/:id/edit - Edit mode
    const editMatch = path.match(/\/categories\/([^/]+)\/edit$/)
    if (editMatch) {
      const categoryId = editMatch[1]
      api.get(`/categories/${categoryId}`).then(result => {
        if (result.code === 20000 && result.data) {
          setSelectedCategory(result.data)
          setSelectedKeys([categoryId])
          setEditingCategory({ ...result.data })
          setFormMode('edit')
          form.resetFields()
          form.setFieldsValue({
            ...result.data,
            visible: result.data.visible !== false
          })
          fetchCategoryTreeForMove()
          setViewMode('form')
        }
      }).catch(err => console.error('Failed to load category:', err))
      return
    }
    
    // Pattern 2: /categories/new - Create top-level mode
    if (path.endsWith('/categories/new')) {
      setEditingCategory(null)
      setFormMode('create')
      form.resetFields()
      form.setFieldsValue({ visible: true, parent_id: null })
      fetchCategoryTreeForMove()
      setViewMode('form')
      return
    }
    
    // Pattern 3: /categories/:id - Detail view
    const detailMatch = path.match(/\/categories\/([^/]+)$/)
    if (detailMatch) {
      const categoryId = detailMatch[1]
      api.get(`/categories/${categoryId}`).then(result => {
        if (result.code === 20000 && result.data) {
          setSelectedCategory(result.data)
          setSelectedKeys([categoryId])
          setViewMode('detail')
        }
      }).catch(err => console.error('Failed to load category from URL:', err))
    }
  }, [])

  // Listen for browser back/forward navigation
  useEffect(() => {
    const handlePopState = () => {
      const path = window.location.pathname
      
      const editMatch = path.match(/\/categories\/([^/]+)\/edit$/)
      if (editMatch) {
        const categoryId = editMatch[1]
        api.get(`/categories/${categoryId}`).then(result => {
          if (result.code === 20000 && result.data) {
            setSelectedCategory(result.data)
            setSelectedKeys([categoryId])
            setEditingCategory({ ...result.data })
            setFormMode('edit')
            form.resetFields()
            form.setFieldsValue({
              ...result.data,
              visible: result.data.visible !== false
            })
            fetchCategoryTreeForMove()
            setViewMode('form')
          }
        })
        return
      }
      
      if (path.endsWith('/categories/new')) {
        setEditingCategory(null)
        setFormMode('create')
        form.resetFields()
        form.setFieldsValue({ visible: true, parent_id: null })
        fetchCategoryTreeForMove()
        setViewMode('form')
        return
      }
      
      const detailMatch = path.match(/\/categories\/([^/]+)$/)
      if (detailMatch) {
        const categoryId = detailMatch[1]
        api.get(`/categories/${categoryId}`).then(result => {
          if (result.code === 20000 && result.data) {
            setSelectedCategory(result.data)
            setSelectedKeys([categoryId])
            setViewMode('detail')
          }
        })
      } else {
        setSelectedCategory(null)
        setSelectedKeys([])
        setViewMode('detail')
      }
    }
    
    window.addEventListener('popstate', handlePopState)
    return () => {
      window.removeEventListener('popstate', handlePopState)
    }
  }, [])

  // Load parent category name when selectedCategory changes
  useEffect(() => {
    const loadParentCategoryName = async () => {
      if (selectedCategory?.parent_id) {
        try {
          const result = await api.get(`/categories/${selectedCategory.parent_id}`)
          if (result.code === 20000 && result.data) {
            setParentCategoryName(result.data.name)
          } else {
            setParentCategoryName('-')
          }
        } catch (err) {
          console.error('Failed to load parent category:', err)
          setParentCategoryName('-')
        }
      } else {
        setParentCategoryName('-')
      }
    }

    loadParentCategoryName()
  }, [selectedCategory])

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
      // Update URL
      window.history.pushState(null, '', `/instruments/categories/${category.id}`)
    } else {
      setSelectedCategory(null)
      window.history.pushState(null, '', '/instruments/categories')
    }
  }

  const fetchParentCategories = async () => {
    try {
      setParentLoading(true)
      const result = await api.get('/categories')
      if (result.code === 20000) {
        const level1Categories = (result.data?.list || [])
          .filter(cat => cat.level === 1)
        setParentCategories(level1Categories)
      }
    } catch (error) {
      message.error('加载父级分类失败: ' + error.message)
      setParentCategories([])
    } finally {
      setParentLoading(false)
    }
  }

  // Fetch all categories including sub_categories for move functionality
  const fetchCategoryTreeForMove = async () => {
    try {
      setParentLoading(true)
      const result = await api.get('/categories')
      if (result.code === 20000) {
        setParentCategories(result.data?.list || [])
      }
    } catch (error) {
      message.error('加载分类数据失败: ' + error.message)
      setParentCategories([])
    } finally {
      setParentLoading(false)
    }
  }

  // Get parent category name by ID
  const getParentCategoryName = (parentId) => {
    const parent = parentCategories.find(cat => cat.id === parentId)
    return parent?.name || '未知分类'
  }

  // Build category tree with virtual root for move functionality
  const buildCategoryTreeWithVirtualRoot = () => {
    const treeData = parentCategories.map(cat => {
      const hasChildren = cat.sub_categories && cat.sub_categories.length > 0
      return {
        key: cat.id,
        title: cat.name,
        value: cat.id,
        children: hasChildren ? buildSubTree(cat.sub_categories) : [],
        disabled: formMode === 'edit' && editingCategory?.id === cat.id ? true : false
      }
    })
    return [
      {
        key: 'root',
        title: '顶级分类',
        value: null,
        children: treeData
      }
    ]
  }

  const buildSubTree = (categories) => {
    return categories.map(cat => {
      const hasChildren = cat.sub_categories && cat.sub_categories.length > 0
      // Disable if this is the category being edited or any of its children
      let isDisabled = false
      if (formMode === 'edit' && editingCategory?.id) {
        isDisabled = isCategoryOrChild(cat.id, editingCategory.id)
      }
      return {
        key: cat.id,
        title: cat.name,
        value: cat.id,
        children: hasChildren ? buildSubTree(cat.sub_categories) : [],
        disabled: isDisabled
      }
    })
  }

  // Check if a category ID is the current editing category or any of its children
  const isCategoryOrChild = (categoryId, targetId) => {
    if (categoryId === targetId) return true
    const category = findCategoryById(treeData, targetId)
    if (category && category.sub_categories) {
      for (const child of category.sub_categories) {
        if (isCategoryOrChild(child.id, categoryId)) return true
      }
    }
    return false
  }

  const handleCreateTopLevel = () => {
    setEditingCategory(null)
    setFormMode('create')
    form.resetFields()
    form.setFieldsValue({ visible: true, parent_id: null })
    fetchParentCategories()
    setViewMode('form')
    // Update URL
    window.history.pushState(null, '', '/instruments/categories/new')
  }

  const handleCreateSubCategory = () => {
    if (!selectedCategory) {
      message.warning('请先选择一个分类')
      return
    }
    setEditingCategory({ parent_id: selectedCategory.id, name: selectedCategory.name })
    setFormMode('create')
    form.resetFields()
    form.setFieldsValue({ 
      visible: true,
      icon: selectedCategory.icon  // 继承父分类图标
    })
    fetchCategoryTreeForMove()
    setViewMode('form')
    // Update URL
    window.history.pushState(null, '', '/instruments/categories/new')
  }

  const handleEdit = () => {
    if (!selectedCategory) return
    setEditingCategory({ ...selectedCategory })
    setFormMode('edit')
    form.resetFields()
    form.setFieldsValue({
      ...selectedCategory,
      visible: selectedCategory.visible !== false
    })
    fetchCategoryTreeForMove()
    setViewMode('form')
    // Update URL
    if (selectedCategory?.id) {
      window.history.pushState(null, '', `/instruments/categories/${selectedCategory.id}/edit`)
    }
  }

  const handleFormCancel = () => {
    setViewMode('detail')
    form.resetFields()
    setEditingCategory(null)
    // Update URL based on selected category
    if (selectedCategory?.id) {
      window.history.pushState(null, '', `/instruments/categories/${selectedCategory.id}`)
    } else {
      window.history.pushState(null, '', '/instruments/categories')
    }
  }

  const handleDelete = async () => {
    if (!selectedCategory) return
    try {
      await categoriesApi.delete(selectedCategory.id)
      message.success('删除成功')
      setSelectedCategory(null)
      setSelectedKeys([])
      setViewMode('detail')
      fetchCategoryTree()
    } catch (err) {
      message.error('删除失败: ' + err.message)
    }
  }

  const handleFormSubmit = async () => {
    setSaving(true)
    try {
      const values = await form.validateFields()
      
      // Bug Fix: Handle parent_id for different scenarios
      // - Create sub-category: use editingCategory.parent_id
      // - Create top-level: parent_id should be null
      // - Edit mode: values.parent_id (handle 'root' virtual root as null)
      let finalParentId = null
      if (formMode === 'create' && editingCategory?.parent_id) {
        // Creating sub-category - use parent from editingCategory
        finalParentId = editingCategory.parent_id
      } else if (formMode === 'edit') {
        // Edit mode - handle virtual root
        finalParentId = values.parent_id === 'root' || values.parent_id === null ? null : values.parent_id
      } else {
        // Create top-level - parent_id is null
        finalParentId = null
      }
      
      // Prepare form data - exclude sort field per Issue #241
      const formData = {
        name: values.name,
        icon: values.icon || '',
        visible: values.visible !== false,
        parent_id: finalParentId,
      }
      
      let result
      if (formMode === 'edit' && editingCategory?.id) {
        result = await categoriesApi.update(editingCategory.id, formData)
        message.success('更新成功')
      } else {
        result = await categoriesApi.create(formData)
        message.success('创建成功')
      }
      
      // Refresh tree data
      await fetchCategoryTree()
      
      // Update selected category and URL
      const newCategoryId = formMode === 'edit' ? editingCategory.id : result.data?.id
      if (newCategoryId) {
        const updatedCategory = await api.get(`/categories/${newCategoryId}`)
        setSelectedCategory(updatedCategory?.data || null)
        setSelectedKeys(newCategoryId ? [newCategoryId] : [])
        
        // Bug Fix: Update URL after creating/editing category
        window.history.pushState(null, '', `/instruments/categories/${newCategoryId}`)
      }
      
      // Switch back to detail view
      setViewMode('detail')
      setEditingCategory(null)
    } catch (error) {
      if (error.errorFields) {
        // Form validation error
        console.error('Validation failed:', error)
      } else {
        message.error(error.message || '提交失败')
      }
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
                    {selectedCategory?.level === 1 && (
                      <Button icon={<PlusOutlined />} onClick={handleCreateSubCategory}>
                        创建下级分类
                      </Button>
                    )}
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
                  <Descriptions.Item label="父级分类">
                    {selectedCategory.parent_id ? parentCategoryName : '-'}
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
          
           {viewMode === 'form' && (
             <Card 
               title={formMode === 'edit' ? '编辑分类' : '创建分类'}
               extra={
                 <Space>
                   <Button onClick={handleFormCancel}>取消</Button>
                   <Button 
                     type="primary" 
                     onClick={handleFormSubmit}
                     loading={saving}
                   >
                     提交
                   </Button>
                 </Space>
               }
             >
              <Form form={form} layout="vertical">
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

                {/* Scenario 1: Create Top Level - Hide parent category field */}
                {formMode === 'create' && !editingCategory?.parent_id && (
                  <Form.Item name="parent_id" hidden>
                    <Input type="hidden" />
                  </Form.Item>
                )}

                {/* Scenario 2: Create Sub Category - Show parent as static text */}
                {formMode === 'create' && editingCategory?.parent_id && (
                  <Form.Item label="父级分类">
                    <div style={{ padding: '8px 0', color: '#666' }}>
                      {getParentCategoryName(editingCategory.parent_id)}
                    </div>
                  </Form.Item>
                )}

                {/* Scenario 3: Edit - TreeSelect with virtual root for move */}
                {formMode === 'edit' && (
                  <Form.Item
                    name="parent_id"
                    label="父级分类"
                    extra="不选择表示设为顶级分类"
                  >
                    <TreeSelect
                      treeData={buildCategoryTreeWithVirtualRoot()}
                      placeholder="请选择父级分类"
                      allowClear
                      treeDefaultExpandAll
                      fieldNames={{ title: 'title', value: 'value', children: 'children' }}
                    />
                  </Form.Item>
                )}

                <Form.Item
                  name="icon"
                  label="分类图标"
                  extra="输入emoji或图标URL，如：🎹 或 /images/icon.png"
                >
                  <Input placeholder="请输入图标，如：🎹" />
                </Form.Item>

                {/* Only show visible switch in edit mode */}
                {formMode === 'edit' && (
                  <Form.Item
                    name="visible"
                    label="显示状态"
                    valuePropName="checked"
                  >
                    <Switch checkedChildren="显示" unCheckedChildren="隐藏" />
                  </Form.Item>
                )}
              </Form>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
