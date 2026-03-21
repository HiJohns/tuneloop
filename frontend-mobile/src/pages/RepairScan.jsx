import { useState } from 'react';
import { Button, Input, TextArea, ImageUploader, Card, message } from 'antd-mobile';
import { CameraOutline, Scanline } from 'antd-mobile-icons';

const API_BASE = process.env.REACT_APP_API_BASE || 'http://localhost:5553';

export default function RepairScan() {
  const [snCode, setSnCode] = useState('');
  const [instrument, setInstrument] = useState(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [problem, setProblem] = useState('');
  const [images, setImages] = useState([]);

  const handleScan = () => {
    if (typeof wx !== 'undefined' && wx.scanQRCode) {
      wx.scanQRCode({
        needResult: 1,
        scanType: ['qrCode', 'barCode'],
        success: (res) => {
          const result = res.resultStr;
          setSnCode(result);
          fetchInstrument(result);
        },
        fail: () => {
          message.error('扫码失败');
        }
      });
    } else {
      message.info('请输入SN码手动查询');
    }
  };

  const fetchInstrument = async (sn) => {
    setLoading(true);
    try {
      const response = await fetch(`${API_BASE}/api/instruments?sn=${sn}`);
      const result = await response.json();
      
      if (result.code === 20000 && result.data.list?.length > 0) {
        setInstrument(result.data.list[0]);
        message.success('乐器信息已回显');
      } else {
        message.info('未找到乐器，请确认SN码');
      }
    } catch (error) {
      message.error('查询失败');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async () => {
    if (!instrument || !problem) {
      message.error('请填写完整信息');
      return;
    }

    setSubmitting(true);
    try {
      const response = await fetch(`${API_BASE}/api/maintenance`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          instrument_id: instrument.id,
          problem_description: problem,
          images: images,
          service_type: 'repair'
        })
      });

      const result = await response.json();
      if (result.code === 20000) {
        message.success('报修提交成功');
        setSnCode('');
        setInstrument(null);
        setProblem('');
        setImages([]);
      } else {
        message.error(result.message || '提交失败');
      }
    } catch (error) {
      message.error('提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 p-4 pb-20">
      <Card>
        <div className="mb-4">
          <label className="block text-gray-700 mb-2">SN码</label>
          <div className="flex gap-2">
            <Input
              className="flex-1"
              placeholder="扫描或输入SN码"
              value={snCode}
              onChange={setSnCode}
            />
            <Button onClick={handleScan} icon={<Scanline />}>
              扫码
            </Button>
          </div>
        </div>

        {instrument && (
          <Card className="bg-blue-50 mb-4">
            <p className="font-medium text-lg">{instrument.name}</p>
            <p className="text-gray-500 text-sm mt-1">品牌: {instrument.brand}</p>
            <p className="text-gray-500 text-sm">级别: {instrument.level_name}</p>
          </Card>
        )}

        <div className="mb-4">
          <label className="block text-gray-700 mb-2">故障描述</label>
          <textarea
            className="w-full p-3 border rounded-lg"
            rows={4}
            placeholder="请详细描述故障情况..."
            value={problem}
            onChange={(e) => setProblem(e.target.value)}
          />
        </div>

        <div className="mb-4">
          <label className="block text-gray-700 mb-2">故障部位照片</label>
          <div className="flex flex-wrap gap-2">
            {images.map((img, idx) => (
              <div key={idx} className="w-20 h-20 bg-gray-200 rounded">
                <img src={img} alt="" className="w-full h-full object-cover rounded" />
              </div>
            ))}
            <button className="w-20 h-20 border-2 border-dashed border-gray-300 rounded flex items-center justify-center text-gray-400">
              <CameraOutline size={24} />
            </button>
          </div>
        </div>

        <Button
          type="primary"
          block
          loading={submitting}
          disabled={!instrument || !problem}
          onClick={handleSubmit}
        >
          提交报修
        </Button>
      </Card>
    </div>
  );
}
