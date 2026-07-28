import { useEffect, useState } from 'react'
import { Card, Select, List, Button, Modal, Form, Input, InputNumber, Switch, message, Spin, Empty, Space, Popconfirm, Tooltip } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, AppstoreOutlined, MenuOutlined, UpOutlined, DownOutlined, EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons'
import { DndContext, closestCenter, KeyboardSensor, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { api, categoriesApi } from '../../../services/api'

const { Option } = Select

function SortableItem({ category, onEdit, onDelete, onSortUp, onSortDown, onHide, list, idx, total }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: category.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    cursor: 'grab',
  }

  const hidden = (category.sort || 0) <= 0

  return (
    <div ref={setNodeRef} style={style} className="flex items-center justify-between p-3 bg-white border-b hover:bg-gray-50">
      <div className="flex items-center gap-2 flex-1">
        <MenuOutlined {...attributes} {...listeners} className="cursor-grab text-gray-400" />
        <span className={`font-medium ${hidden ? 'text-gray-300' : ''}`}>{category.name}</span>
        {category.icon && <span>{category.icon}</span>}
        {!category.visible && <span className="text-xs text-gray-400">(隐藏)</span>}
      </div>
      <Space size="small">
        <Tooltip title="首页上移"><Button size="small" icon={<UpOutlined />} disabled={idx <= 0 || hidden} onClick={(e) => { e.stopPropagation(); onSortUp(category, list) }} /></Tooltip>
        <Tooltip title="首页下移"><Button size="small" icon={<DownOutlined />} disabled={idx >= total - 1 || hidden} onClick={(e) => { e.stopPropagation(); onSortDown(category, list) }} /></Tooltip>
        {hidden
          ? <Tooltip title="首页显示"><Button size="small" icon={<EyeOutlined />} onClick={(e) => { e.stopPropagation(); onHide(category) }} /></Tooltip>
          : <Tooltip title="从首页隐藏"><Button size="small" icon={<EyeInvisibleOutlined />} onClick={(e) => { e.stopPropagation(); onHide(category) }} /></Tooltip>}
        <Button size="small" icon={<EditOutlined />} onClick={() => onEdit(category)} />
        <Popconfirm
          title="确定要删除此分类吗？"
          onConfirm={() => onDelete(category.id)}
          okText="确定"
          cancelText="取消"
        >
          <Button size="small" danger icon={<DeleteOutlined />} />
        </Popconfirm>
      </Space>
    </div>
  )
}

export default function CategoryList() {
  const [level1Categories, setLevel1Categories] = useState([])
  const [selectedParentId, setSelectedParentId] = useState(null)
  const [subCategories, setSubCategories] = useState([])
  const [loading, setLoading] = useState(true)
  const [savingSort, setSavingSort] = useState(false)
  const [editingCategory, setEditingCategory] = useState(null)
  const [modalVisible, setModalVisible] = useState(false)
  const [formMode, setFormMode] = useState('create')
  const [form] = Form.useForm()
  const [saving, setSaving] = useState(false)

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates })
  )

  useEffect(() => {
    fetchCategories()
  }, [])

  useEffect(() => {
    if (selectedParentId) {
      const parent = level1Categories.find(c => c.id === selectedParentId)
      if (parent && parent.sub_categories) {
        const sorted = [...parent.sub_categories].sort((a, b) => (a.sort || 0) - (b.sort || 0))
        setSubCategories(sorted)
      } else {
        setSubCategories([])
      }
    } else {
      setSubCategories([])
    }
  }, [selectedParentId, level1Categories])

  const fetchCategories = async () => {
    try {
      setLoading(true)
      const result = await api.get('/categories')
      const data = result?.data?.list || []
      setLevel1Categories(data)
      if (data.length > 0 && !selectedParentId) {
        setSelectedParentId(data[0].id)
      }
    } catch (err) {
      message.error('加载分类失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleDragEnd = async (event) => {
    const { active, over } = event
    if (!over || active.id === over.id) return

    const oldIndex = subCategories.findIndex(c => c.id === active.id)
    const newIndex = subCategories.findIndex(c => c.id === over.id)

    const newSubCategories = [...subCategories]
    const [removed] = newSubCategories.splice(oldIndex, 1)
    newSubCategories.splice(newIndex, 0, removed)
    setSubCategories(newSubCategories)

    const sortUpdates = newSubCategories.map((cat, index) => ({
      id: cat.id,
      sort: index + 1,
    }))

    setSavingSort(true)
    try {
      await api.put('/categories/sort', { items: sortUpdates })
      message.success('排序已更新')
      await fetchCategories()
    } catch (err) {
      message.error('Failed to update sort: ' + err.message)
      const parent = level1Categories.find(c => c.id === selectedParentId)
      if (parent && parent.sub_categories) {
        setSubCategories([...parent.sub_categories].sort((a, b) => (a.sort || 0) - (b.sort || 0)))
      }
    } finally {
      setSavingSort(false)
    }
  }

  const sortSingle = async (updates) => {
    const items = Array.isArray(updates) ? updates : [updates]
    try {
      await api.put('/categories/sort', { items: items.map(({ cat, sort }) => ({ id: cat.id, sort })) })
      items.forEach(({ cat, sort }) => { cat.sort = sort })
      const updated = [...level1Categories].sort((a, b) => (a.sort || 0) - (b.sort || 0))
      setLevel1Categories(updated)
      for (const p of updated) {
        for (const { cat } of items) {
          const idx = (p.sub_categories || []).findIndex(s => s.id === cat.id)
          if (idx >= 0) {
            p.sub_categories[idx].sort = cat.sort
            setSubCategories([...p.sub_categories].sort((a, b) => (a.sort || 0) - (b.sort || 0)))
            break
          }
        }
      }
      message.success('已更新')
    } catch (err) { message.error('更新失败: ' + err.message) }
  }

  const handleSortUp = (cat, list) => {
    const sorted = [...list].sort((a, b) => (a.sort || 0) - (b.sort || 0))
    const idx = sorted.findIndex(c => c.id === cat.id)
    if (idx <= 0) return
    const prev = sorted[idx - 1]
    const tmp = prev.sort || 1
    prev.sort = cat.sort || 1
    cat.sort = tmp
    sortSingle(cat, cat.sort)
    sortSingle(prev, prev.sort)
  }

  const handleSortDown = (cat, list) => {
    const sorted = [...list].sort((a, b) => (a.sort || 0) - (b.sort || 0))
    const idx = sorted.findIndex(c => c.id === cat.id)
    if (idx < 0 || idx >= sorted.length - 1) return
    const next = sorted[idx + 1]
    const tmp = next.sort || 1
    next.sort = cat.sort || 1
    cat.sort = tmp
    sortSingle(cat, cat.sort)
    sortSingle(next, next.sort)
  }

  const handleHide = (cat) => sortSingle({ cat, sort: (cat.sort || 0) <= 0 ? 1 : 0 })

  const handleSortUp = (cat, list) => {
    const sorted = [...list].sort((a, b) => (a.sort || 0) - (b.sort || 0))
    const idx = sorted.findIndex(c => c.id === cat.id)
    if (idx <= 0) return
    const prev = sorted[idx - 1]
    const tmp = prev.sort || 1
    prev.sort = cat.sort || 1
    cat.sort = tmp
    sortSingle([{ cat, sort: cat.sort }, { cat: prev, sort: prev.sort }])
  }

  const handleSortDown = (cat, list) => {
    const sorted = [...list].sort((a, b) => (a.sort || 0) - (b.sort || 0))
    const idx = sorted.findIndex(c => c.id === cat.id)
    if (idx < 0 || idx >= sorted.length - 1) return
    const next = sorted[idx + 1]
    const tmp = next.sort || 1
    next.sort = cat.sort || 1
    cat.sort = tmp
    sortSingle([{ cat, sort: cat.sort }, { cat: next, sort: next.sort }])
  }

  const handleCreateSubCategory = () => {
    if (!selectedParentId) {
      message.warning('Please select a parent category first')
      return
    }
    // Get parent category to inherit its icon
    const parentCategory = level1Categories.find(c => c.id === selectedParentId)
    const parentIcon = parentCategory?.icon || ''
    
    setEditingCategory({ parent_id: selectedParentId })
    setFormMode('create')
    form.resetFields()
    form.setFieldsValue({ 
      visible: true,
      sort: 1,
      icon: parentIcon
    })
    setModalVisible(true)
  }

  const handleEdit = (category) => {
    setEditingCategory({ ...category })
    setFormMode('edit')
    form.resetFields()
    form.setFieldsValue({
      ...category,
      visible: category.visible !== false,
    })
    setModalVisible(true)
  }

  const handleDelete = async (categoryId) => {
    try {
      await categoriesApi.delete(categoryId)
      message.success('删除成功')
      await fetchCategories()
    } catch (err) {
      message.error('Failed to delete: ' + err.message)
    }
  }

  const handleFormSubmit = async () => {
    setSaving(true)
    try {
      const values = await form.validateFields()

      let finalParentId = null
      if (formMode === 'create' && editingCategory?.parent_id) {
        finalParentId = editingCategory.parent_id
      } else if (formMode === 'edit') {
        finalParentId = editingCategory.parent_id || null
      }

      const formData = {
        name: values.name,
        icon: values.icon || '',
        visible: values.visible !== false,
        parent_id: finalParentId,
      }

      if (formMode === 'edit' && editingCategory?.id) {
        await categoriesApi.update(editingCategory.id, formData)
        message.success('更新成功')
      } else {
        const response = await categoriesApi.create(formData)
        if (response.code === 20000) {
          message.success('创建成功')
          if (response.data?.id) {
            setSelectedParentId(response.data.id)
          }
        }
      }

      setModalVisible(false)
      await fetchCategories()
    } catch (error) {
      if (!error.errorFields) {
        message.error(error.message || 'Failed to submit')
      }
    } finally {
      setSaving(false)
    }
  }

  const getParentCategoryName = () => {
    const parent = level1Categories.find(c => c.id === selectedParentId)
    return parent ? parent.name : ''
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div className="p-4 h-full">
      <div className="flex gap-4 h-full">
        <Card title="分类管理" className="w-1/3 flex flex-col" extra={<Button type="primary" size="small" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>新建顶级分类</Button>}>
          {level1Categories.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-64 text-gray-400">
              <AppstoreOutlined style={{ fontSize: 64, marginBottom: 16 }} />
              <p className="text-lg mb-4">暂无分类</p>
              <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTopLevel}>新建顶级分类</Button>
            </div>
          ) : (
            <div className="space-y-4 flex-1 flex flex-col">
              <List
                className="flex-1 overflow-auto border rounded"
                size="small"
                dataSource={level1Categories}
                renderItem={(cat) => {
                   const sorted = [...level1Categories].sort((a, b) => (a.sort || 0) - (b.sort || 0))
                   const idx = sorted.findIndex(c => c.id === cat.id)
                   const hidden = (cat.sort || 0) <= 0
                   return (
                   <List.Item
                     className={`cursor-pointer ${selectedParentId === cat.id ? 'bg-blue-50' : 'hover:bg-gray-50'}`}
                     onClick={() => setSelectedParentId(cat.id)}
                     extra={
                       <Space size="small">
                         <Tooltip title="首页上移"><Button size="small" icon={<UpOutlined />} disabled={idx <= 0 || hidden} onClick={(e) => { e.stopPropagation(); handleSortUp(cat, level1Categories) }} /></Tooltip>
                         <Tooltip title="首页下移"><Button size="small" icon={<DownOutlined />} disabled={idx >= sorted.length - 1 || hidden} onClick={(e) => { e.stopPropagation(); handleSortDown(cat, level1Categories) }} /></Tooltip>
                         {hidden
                          ? <Tooltip title="首页显示"><Button size="small" icon={<EyeOutlined />} onClick={(e) => { e.stopPropagation(); handleHide(cat) }} /></Tooltip>
                          : <Tooltip title="从首页隐藏"><Button size="small" icon={<EyeInvisibleOutlined />} onClick={(e) => { e.stopPropagation(); handleHide(cat) }} /></Tooltip>}
                       </Space>
                     }
                   >
                     <List.Item.Meta
                       avatar={<span className="text-lg">{cat.icon}</span>}
                       title={<span className={`font-medium ${hidden ? 'text-gray-300' : ''}`}>{cat.name}</span>}
                     />
                   </List.Item>
                   )
                 }}
              />
            </div>
          )}
        </Card>

        <Card className="w-2/3 flex flex-col" title={selectedParentId ? `"${getParentCategoryName()}" 的子分类列表` : '子分类列表'} extra={
          <Space>
            {savingSort && <Spin size="small" />}
            {selectedParentId && (
              <>
                <Button size="small" icon={<PlusOutlined />} onClick={handleCreateSubCategory}>创建子分类</Button>
                <Button 
                  size="small" 
                  icon={<EditOutlined />} 
                  onClick={() => handleEdit(level1Categories.find(c => c.id === selectedParentId))}
                >
                  编辑
                </Button>
              </>
            )}
          </Space>
        }>
          {!selectedParentId ? (
            <Empty description="Please select a parent category from the left" />
          ) : subCategories.length === 0 ? (
            <Empty description="暂无子分类，点击右上角「创建子分类」按钮添加" />
          ) : (
            <div className="flex-1 overflow-auto">
              <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
                <SortableContext items={subCategories.map(c => c.id)} strategy={verticalListSortingStrategy}>
                  <div className="border rounded">
                    {subCategories.map((category, idx) => (
                       <SortableItem key={category.id} category={category} onEdit={handleEdit} onDelete={handleDelete} onSortUp={handleSortUp} onSortDown={handleSortDown} onHide={handleHide} list={subCategories} idx={idx} total={subCategories.length} />
                     ))}
                  </div>
                </SortableContext>
              </DndContext>
            </div>
          )}
        </Card>
      </div>

      <Modal
        title={formMode === 'edit' ? 'Edit Category' : 'Create Category'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={handleFormSubmit}
        confirmLoading={saving}
        okText="提交"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="Category Name" rules={[{ required: true, message: 'Please enter category name' }, { min: 2, message: 'At least 2 characters' }, { max: 50, message: 'Max 50 characters' }]}>
            <Input placeholder="输入分类名称" />
          </Form.Item>

          {formMode === 'create' && editingCategory?.parent_id && (
            <Form.Item label="父分类">
              <div className="p-2 bg-gray-50 rounded">{getParentCategoryName()}</div>
            </Form.Item>
          )}

          <Form.Item name="icon" label="图标" extra="输入 emoji 或图标 URL">
            <Input placeholder="例如 🎹" />
          </Form.Item>

          <Form.Item name="sort" label="排序值" extra="数字越小越靠前；设为 0 或负数可隐藏该分类">
            <InputNumber min={0} max={999} style={{ width: 120 }} />
          </Form.Item>

          {formMode === 'edit' && (
            <Form.Item name="visible" label="可见性" valuePropName="checked">
              <Switch checkedChildren="可见" unCheckedChildren="隐藏" />
            </Form.Item>
          )}
        </Form>
      </Modal>

      <p className="mt-4 text-xs text-gray-400">↑↓ 调整首页菜单显示顺序，👁‍🗨 从首页隐藏该分类</p>
    </div>
  )
}
