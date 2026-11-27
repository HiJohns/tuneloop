import { useState, useEffect } from 'react';
import { Table, Card, Button, Space, Modal, Form, Input, Upload, Image, Popconfirm, message, ColorPicker } from 'antd';
import { PlusOutlined, UploadOutlined, EditOutlined, DeleteOutlined, ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import { bannerApi } from '../../services/api';
import { userFacingErrorMessage } from '../../services/apiError';
import { antdUploadProps } from '../../utils/upload';
import FormConflictAlert from '../../components/FormConflictAlert';
import { conflictText, isBusinessCorrectableConflict } from '../../utils/formConflict';
import MallImageManageCard from './MallImageManageCard';

export default function BannerManagePage() {
  const [banners, setBanners] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingBanner, setEditingBanner] = useState(null);
  const [form] = Form.useForm();
  // #2488（沿用 #2480 D8）：业务可纠正冲突用表单内持久提示，不用 3 秒 toast。
  const [formConflict, setFormConflict] = useState('');

  const fetchBanners = async () => {
    setLoading(true);
    try {
      const res = await bannerApi.list();
      if (res.code === 20000) {
        setBanners(res.data?.list || []);
      } else {
        message.error('获取轮播图列表失败');
      }
    } catch (e) {
      message.error('网络错误');
    }
    setLoading(false);
  };

  useEffect(() => { fetchBanners(); }, []);

  const handleCreate = () => {
    setFormConflict(''); // #2488 D8：打开表单即清空陈旧冲突提示
    setEditingBanner(null);
    form.resetFields();
    form.setFieldsValue({ status: 'active' });
    setModalOpen(true);
  };

  const handleEdit = (record) => {
    setFormConflict('');
    setEditingBanner(record);
    form.setFieldsValue(record);
    setModalOpen(true);
  };

  const handleDelete = async (id) => {
    try {
      const res = await bannerApi.delete(id);
      if (res.code === 20000) {
        message.success('删除成功');
        fetchBanners();
      } else {
        message.error('删除失败');
      }
    } catch (e) {
      message.error('网络错误');
    }
  };

  const handleMoveUp = (index) => {
    if (index === 0) return
    const sorted = [...banners]
    const prev = sorted[index - 1]
    const curr = sorted[index]
    const temp = prev.sort_order
    prev.sort_order = curr.sort_order
    curr.sort_order = temp
    Promise.all([
      bannerApi.update(prev.id, { sort_order: prev.sort_order }),
      bannerApi.update(curr.id, { sort_order: curr.sort_order }),
    ]).then(() => fetchBanners()).catch(() => message.error('排序更新失败'))
  }

  const handleMoveDown = (index) => {
    if (index === banners.length - 1) return
    const sorted = [...banners]
    const next = sorted[index + 1]
    const curr = sorted[index]
    const temp = next.sort_order
    next.sort_order = curr.sort_order
    curr.sort_order = temp
    Promise.all([
      bannerApi.update(next.id, { sort_order: next.sort_order }),
      bannerApi.update(curr.id, { sort_order: curr.sort_order }),
    ]).then(() => fetchBanners()).catch(() => message.error('排序更新失败'))
  }

  const handleSubmit = async () => {
    setFormConflict(''); // #2488 D8：提交开始即清空
    try {
      const values = await form.validateFields();
      if (editingBanner) {
        const res = await bannerApi.update(editingBanner.id, values);
        if (res.code === 20000) {
          message.success('更新成功');
        } else if (isBusinessCorrectableConflict(res)) {
          // #2488：轮播图业务冲突（重复/字段非法/状态冲突）改为表单内持久提示。
          setFormConflict(conflictText(res, '更新失败，请调整后重试'));
          return;
        } else {
          message.error(res.message || '更新失败');
          return;
        }
      } else {
        const maxSort = banners.reduce((max, b) => Math.max(max, b.sort_order || 0), 0);
        const res = await bannerApi.create({ ...values, sort_order: maxSort + 1 });
        if (res.code === 20000 || res.code === 20100) {
          message.success('创建成功');
        } else if (isBusinessCorrectableConflict(res)) {
          setFormConflict(conflictText(res, '创建失败，请调整后重试'));
          return;
        } else {
          message.error(res.message || '创建失败');
          return;
        }
      }
      setFormConflict('');
      setModalOpen(false);
      fetchBanners();
    } catch (e) {
      if (e.errorFields) return;
      // #2488 D7：传输层非 2xx 抛出的 ApiError 携带 `code`，业务可纠正冲突
      // （如 HTTP 400 + 40002）必须进持久提示，不得退化成瞬时 toast。
      if (isBusinessCorrectableConflict(e)) {
        setFormConflict(conflictText(e, editingBanner ? '更新失败，请调整后重试' : '创建失败，请调整后重试'));
        return;
      }
      // #2488 Rev.5（Bug B）+ #2609：仅 `ApiError`（后端中文，已由 resolveErrorMessage
      // 中文化）透传；网络/未知错误（浏览器 TypeError，ASCII 技术串）落中文兜底。
      message.error(userFacingErrorMessage(e, '操作失败'));
    }
  };

  const columns = [
    {
      title: '缩略图',
      dataIndex: 'image_url',
      key: 'image_url',
      width: 100,
      render: (url) => (
        url ? <Image src={url} width={60} height={60} style={{ objectFit: 'cover', borderRadius: 4 }} /> : '-'
      ),
    },
    { title: '排序', dataIndex: 'sort_order', key: 'sort_order', width: 80 },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status, record) => (
        <Button type="link" onClick={() => {
          const nextStatus = status === 'active' ? 'inactive' : 'active';
          bannerApi.update(record.id, { status: nextStatus }).then(res => {
            if (res.code === 20000) { message.success(nextStatus === 'active' ? '已启用' : '已停用'); fetchBanners(); }
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
          <Button type="link" size="small" icon={<ArrowDownOutlined />} disabled={index === banners.length - 1} onClick={() => handleMoveDown(index)} />
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)}>编辑</Button>
          <Popconfirm title="确定删除该轮播图？" onConfirm={() => handleDelete(record.id)} okText="确定" cancelText="取消">
            <Button type="link" danger icon={<DeleteOutlined />}>删除</Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <>
    <Card title="轮播图管理" extra={<Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>新增轮播图</Button>}>
      <Table
        dataSource={banners}
        columns={columns}
        rowKey="id"
        loading={loading}
        pagination={false}
      />
      <Modal
        title={editingBanner ? '编辑轮播图' : '新增轮播图'}
        open={modalOpen}
        onOk={handleSubmit}
        onCancel={() => { setFormConflict(''); setModalOpen(false); }}
        okText="保存"
        cancelText="取消"
      >
        <FormConflictAlert text={formConflict} testId="banner-form-conflict" />
        {/* #2488 Rev.5（CLEAR_FIELDS）：任一可编辑字段变更即清空陈旧冲突提示，
            一次覆盖 image_url/link_url/title/bg_color（含 ColorPicker）。 */}
        <Form form={form} layout="vertical" onValuesChange={() => { if (formConflict) setFormConflict(''); }}>
          <Form.Item label="图片" extra="支持上传或输入图片URL">
            <Upload
              showUploadList={false}
              {...antdUploadProps(
                (result) => {
                  form.setFieldsValue({ image_url: result.url });
                  // setFieldsValue 不触发 onValuesChange，上传成功需显式清空。
                  setFormConflict('');
                  message.success('上传成功');
                },
                { onError: (e) => message.error(userFacingErrorMessage(e, '上传失败')) }
              )}
            >
              <Button icon={<UploadOutlined />}>上传图片</Button>
            </Upload>
          </Form.Item>
          <Form.Item name="image_url" label="图片URL" rules={[{ required: true, message: '请上传或输入图片URL' }]}>
            <Input placeholder="图片URL" />
          </Form.Item>
          <Form.Item name="link_url" label="跳转链接">
            <Input placeholder="可选：点击轮播图跳转的链接" />
          </Form.Item>
          <Form.Item name="title" label="标题">
            <Input placeholder="可选：轮播图标题" />
          </Form.Item>
          <Form.Item name="bg_color" label="背景色" getValueFromEvent={(color) => typeof color === 'string' ? color : color?.toHexString()}>
            <ColorPicker format="hex" showText />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
    <MallImageManageCard />
    </>
  );
}
