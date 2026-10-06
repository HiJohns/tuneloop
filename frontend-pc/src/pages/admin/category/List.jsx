import { useEffect, useState } from 'react'
import { Card, Select, List, Button, Modal, Form, Input, Switch, message, Spin, Empty, Space, Popconfirm, Tooltip } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined, AppstoreOutlined, MenuOutlined, UpOutlined, DownOutlined, EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons'
import { DndContext, closestCenter, KeyboardSensor, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { api, categoriesApi } from '../../../services/api'
import { isHidden, bySort, visibleOrdered, assignPayload } from './categoryLogic'

const { Option } = Select

function SortableItem({ category, onEdit, onDelete, onSortUp, onSortDown, onHide, list, idx, total }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: category.id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    cursor: 'grab',
  }

  // #2133: 可见性只看 visible，不看 sort。
  const hidden = isHidden(category)

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
        setSubCategories(bySort(parent.sub_categories))
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

    // #2133: 拖拽后用共享的连续重编号算法，隐藏项排到可见块之后
    const sortUpdates = assignPayload(newSubCategories)

    setSavingSort(true)
    try {
      await api.put('/categories/sort', { items: sortUpdates })
      message.success('排序已更新')
      await fetchCategories()
    } catch (err) {
      message.error('Failed to update sort: ' + err.message)
      const parent = level1Categories.find(c => c.id === selectedParentId)
      if (parent && parent.sub_categories) {
        setSubCategories(bySort(parent.sub_categories))
      }
    } finally {
      setSavingSort(false)
    }
  }

  // 提交一次全量重编号载荷并刷新，避免本地乐观更新与服务端结果不一致
  const sortSingle = async (payload) => {
    if (payload.length === 0) return
    setSavingSort(true)
    try {
      await api.put('/categories/sort', { items: payload })
      message.success('排序已更新')
      await fetchCategories()
    } catch (err) {
      message.error('更新失败: ' + err.message)
    } finally {
      setSavingSort(false)
    }
  }

// #2133: 隐藏是纯 visible 切换，绝不改写 sort。
  // 若继续用 sort=0 作隐藏哨兵，sort 就又被迫承载两种语义。
  const handleHide = async (cat) => {
    const willShow = isHidden(cat)
    try {
      await categoriesApi.update(cat.id, { visible: willShow })
      message.success(willShow ? '已在首页显示' : '已从首页隐藏')
      await fetchCategories()
    } catch (err) {
      message.error('更新失败: ' + err.message)
    }
  }

  // 上移/下移：先在「可见序列」内换位，再交由 assignPayload 全量重编号。
  // 与拖拽路径共用同一算法 —— 此前拖拽排全量、上下移只排可见项，两条路径
  // 对隐藏项的处理互相矛盾。
  const moveVisible = (cat, list, delta) => {
    const visible = visibleOrdered(list)
    const idx = visible.findIndex(c => c.id === cat.id)
    const target = idx + delta
    if (idx < 0 || target < 0 || target >= visible.length) return
    const moved = [...visible]
    const [item] = moved.splice(idx, 1)
    moved.splice(target, 0, item)

    // 隐藏项保持相对次序，由 assignPayload 追加到可见块之后
    const hiddenItems = bySort(list.filter(c => isHidden(c)))
    sortSingle(assignPayload([...moved, ...hiddenItems]))
  }

  const handleSortUp = (cat, list) => moveVisible(cat, list, -1)

  const handleSortDown = (cat, list) => moveVisible(cat, list, 1)

  const handleCreateTopLevel = () => {
    setEditingCategory(null)
    setFormMode('create')
    form.resetFields()
    form.setFieldsValue({ visible: true })
    setModalVisible(true)
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

  // #2133: 可见序列与下标在回调外一次算好。
  // 此前 renderItem 内对每一项都重新 filter+sort 全列表，等价逻辑存在两份。
  const visibleLevel1 = visibleOrdered(level1Categories)
  const visibleSubs = visibleOrdered(subCategories)

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
                   const idx = visibleLevel1.findIndex(c => c.id === cat.id)
                   const hidden = isHidden(cat)
                   return (
                   <List.Item
                     className={`cursor-pointer ${selectedParentId === cat.id ? 'bg-blue-50' : 'hover:bg-gray-50'}`}
                     onClick={() => setSelectedParentId(cat.id)}
                     extra={
                       <Space size="small">
                         <Tooltip title="首页上移"><Button size="small" icon={<UpOutlined />} disabled={idx <= 0 || hidden} onClick={(e) => { e.stopPropagation(); handleSortUp(cat, level1Categories) }} /></Tooltip>
                         <Tooltip title="首页下移"><Button size="small" icon={<DownOutlined />} disabled={idx >= visibleLevel1.length - 1 || hidden} onClick={(e) => { e.stopPropagation(); handleSortDown(cat, level1Categories) }} /></Tooltip>
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
                    {subCategories.map((category) => {
                       const visIdx = visibleSubs.findIndex(c => c.id === category.id)
                       return (
                       <SortableItem key={category.id} category={category} onEdit={handleEdit} onDelete={handleDelete} onSortUp={handleSortUp} onSortDown={handleSortDown} onHide={handleHide} list={subCategories} idx={visIdx} total={visibleSubs.length} />
                     )})}
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

          {/* #2133: sort 输入框已移除 —— 新建时后端按同级 max+1 自动分配，
              调整次序统一用 ↑↓ / 拖拽；sort 不再兼任「隐藏」开关。 */}
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
