import { useState, useEffect } from 'react';
import { Table, Card, Button, Space, Modal, Form, Input, Upload, Image, Popconfirm, message } from 'antd';
import { PlusOutlined, UploadOutlined, EditOutlined, DeleteOutlined, ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import { mallImageApi } from '../../services/api';
import { antdUploadProps } from '../../utils/upload';

// #2210: 商城页图片管理（平台级全站配置，与租户无关）——复用轮播图管理模式。
export default function MallImageManageCard() {
  const [images, setImages] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingImage, setEditingImage] = useState(null);
  const [form] = Form.useForm();

  const fetchImages = async () => {
    setLoading(true);
    try {
      const res = await mallImageApi.list();
      if (res.code === 20000) {
        setImages(res.data?.list || []);
      } else {
        message.error('获取商城图片列表失败');
      }
    } catch (e) {
      message.error('网络错误');
    }
    setLoading(false);
  };

  useEffect(() => { fetchImages(); }, []);

  const handleCreate = () => {
    setEditingImage(null);
    form.resetFields();
    form.setFieldsValue({ status: 'active' });
    setModalOpen(true);
  };

  const handleEdit = (record) => {
    setEditingImage(record);
    form.setFieldsValue(record);
    setModalOpen(true);
  };

  const handleDelete = async (id) => {
    try {
      const res = await mallImageApi.delete(id);
      if (res.code === 20000) {
        message.success('删除成功');
        fetchImages();
      } else {
        message.error('删除失败');
      }
    } catch (e) {
      message.error('网络错误');
    }
  };

  const handleMoveUp = (index) => {
    if (index === 0) return
    const sorted = [...images]
    const prev = sorted[index - 1]
    const curr = sorted[index]
    const temp = prev.sort_order
    prev.sort_order = curr.sort_order
    curr.sort_order = temp
    Promise.all([
      mallImageApi.update(prev.id, { sort_order: prev.sort_order }),
      mallImageApi.update(curr.id, { sort_order: curr.sort_order }),
    ]).then(() => fetchImages()).catch(() => message.error('排序更新失败'))
  }

  const handleMoveDown = (index) => {
    if (index === images.length - 1) return
    const sorted = [...images]
    const next = sorted[index + 1]
    const curr = sorted[index]
    const temp = next.sort_order
    next.sort_order = curr.sort_order
    curr.sort_order = temp
    Promise.all([
      mallImageApi.update(next.id, { sort_order: next.sort_order }),
      mallImageApi.update(curr.id, { sort_order: curr.sort_order }),
    ]).then(() => fetchImages()).catch(() => message.error('排序更新失败'))
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      if (editingImage) {
        const res = await mallImageApi.update(editingImage.id, values);
        if (res.code === 20000) {
          message.success('更新成功');
        } else {
          message.error('更新失败');
          return;
        }
      } else {
        const maxSort = images.reduce((max, b) => Math.max(max, b.sort_order || 0), 0);
        const res = await mallImageApi.create({ ...values, sort_order: maxSort + 1 });
        if (res.code === 20000 || res.code === 20100) {
          message.success('创建成功');
        } else {
          message.error('创建失败');
          return;
        }
      }
      setModalOpen(false);
      fetchImages();
    } catch (e) {
      if (e.errorFields) return;
      message.error('操作失败');
    }
  };

  const columns = [
    {
      title: '图片',
      dataIndex: 'image_url',
      key: 'image_url',
      width: 120,
      render: (url) => (
        url ? <Image src={url} width={80} height={48} style={{ objectFit: 'cover', borderRadius: 4 }} /> : '-'
      ),
    },
    { title: '标题', dataIndex: 'title', key: 'title', ellipsis: true },
    { title: '排序', dataIndex: 'sort_order', key: 'sort_order', width: 80 },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status, record) => (
        <Button type="link" onClick={() => {
          const nextStatus = status === 'active' ? 'inactive' : 'active';
          mallImageApi.update(record.id, { status: nextStatus }).then(res => {
            if (res.code === 20000) { message.success(nextStatus === 'active' ? '已启用' : '已停用'); fetchImages(); }
          }).catch(() => message.error('操作失败'));
        }}
        style={{ fontWeight: status === 'active' ? 700 : 400, color: status === 'active' ? '#52c41a' : '#999', cursor: 'pointer', border: 'none', background: 'none', padding: 0 }}>
          {status === 'active' ? '启用' : '停用'}
        </Button>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 150,
      render: (_, record, index) => (
        <Space>
          <Button type="link" size="small" icon={<ArrowUpOutlined />} disabled={index === 0} onClick={() => handleMoveUp(index)} />
          <Button type="link" size="small" icon={<ArrowDownOutlined />} disabled={index === images.length - 1} onClick={() => handleMoveDown(index)} />
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)}>编辑</Button>
          <Popconfirm title="确定删除该商城图片？" onConfirm={() => handleDelete(record.id)} okText="确定" cancelText="取消">
            <Button type="link" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card title="商城页图片管理" extra={<Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>新增图片</Button>} style={{ marginTop: 16 }}>
      <Table
        dataSource={images}
        columns={columns}
        rowKey="id"
        loading={loading}
        pagination={false}
      />
      <Modal
        title={editingImage ? '编辑商城图片' : '新增商城图片'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => setModalOpen(false)}
        okText="保存"
        cancelText="取消"
      >
        <Form form={form} layout="vertical">
          <Form.Item label="图片" extra="支持上传或输入图片URL，建议 750×400px">
            <Upload
              showUploadList={false}
              {...antdUploadProps(
                (result) => {
                  form.setFieldsValue({ image_url: result.url });
                  message.success('上传成功');
                },
                { onError: (e) => message.error(e.message || '上传失败') }
              )}
            >
              <Button icon={<UploadOutlined />}>上传图片</Button>
            </Upload>
          </Form.Item>
          <Form.Item name="image_url" label="图片URL" rules={[{ required: true, message: '请上传或输入图片URL' }]}>
            <Input placeholder="图片URL" />
          </Form.Item>
          <Form.Item name="link_url" label="跳转链接">
            <Input placeholder="可选：点击图片跳转的链接（留空则点击不跳转）" />
          </Form.Item>
          <Form.Item name="title" label="标题">
            <Input placeholder="可选：图片标题" />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
