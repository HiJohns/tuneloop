import { useState } from 'react';
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components';
import { apiFetch, api } from '../services/api';
import { env } from '../platform';
import { ArrowLeft, Camera, Scan } from 'lucide-react';
import { message } from 'antd';

export default function RepairScan() {
  const navigate = useNavigate()
  const [snCode, setSnCode] = useState('');
  const [instrument, setInstrument] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [problem, setProblem] = useState('');
  const [images, setImages] = useState([]);
  const baseUrl = env.apiBaseUrl;

  const handleScan = async () => {
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
    try {
      const response = await apiFetch(`${baseUrl}/instruments?sn=${sn}`);
      const result = await response.json();
      
      if (result.code === 20000 && result.data.list?.length > 0) {
        setInstrument(result.data.list[0]);
        message.success('乐器信息已回显');
      } else {
        message.info('未找到乐器，请确认SN码');
      }
    } catch {
      message.error('查询失败');
    }
  };

  const handleSubmit = async () => {
    if (!instrument || !problem) {
      message.error('请填写完整信息');
      return;
    }

    setSubmitting(true);
    try {
      const response = await apiFetch(`${baseUrl}/maintenance`, {
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
    } catch {
      message.error('提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">报修扫码</Text>
      </View>

      <View className="p-4 space-y-4">
        {/* SN Code Input */}
        <View className="bg-white rounded-xl p-4">
          <label className="block text-sm font-medium text-gray-700 mb-2">SN码</label>
          <View className="flex gap-2">
            <input
              className="flex-1 border rounded-lg px-3 py-2 text-sm"
              placeholder="扫描或输入SN码"
              value={snCode}
              onChange={(e) => setSnCode(e.target.value)}
            />
            <Button
              onClick={handleScan}
              className="px-4 py-2 bg-brand-primary text-white rounded-lg text-sm flex items-center gap-1"
            >
              <Scan size={16} /> 扫码
            </Button>
          </View>
        </View>

        {/* Instrument Info */}
        {instrument && (
          <View className="bg-white rounded-xl p-4">
            <Text className="text-sm font-medium text-gray-900 mb-2">乐器信息</Text>
            <Text className="font-medium">{instrument.name}</Text>
            {instrument.brand && <Text className="text-sm text-gray-500 mt-1">品牌: {instrument.brand}</Text>}
            {instrument.level_name && <Text className="text-sm text-gray-500">级别: {instrument.level_name}</Text>}
            <Text className="text-sm text-gray-500">SN: {instrument.sn}</Text>
          </View>
        )}

        {/* Problem Description */}
        <View className="bg-white rounded-xl p-4">
          <label className="block text-sm font-medium text-gray-700 mb-2">故障描述</label>
          <textarea
            className="w-full p-3 border rounded-lg text-sm"
            rows={4}
            placeholder="请详细描述故障情况..."
            value={problem}
            onChange={(e) => setProblem(e.target.value)}
          />
        </View>

        {/* Photo Upload */}
        <View className="bg-white rounded-xl p-4">
          <label className="block text-sm font-medium text-gray-700 mb-2">故障部位照片</label>
          <View className="flex flex-wrap gap-2">
            {images.map((img, idx) => (
              <View key={idx} className="w-20 h-20 bg-gray-200 rounded overflow-hidden">
                <Image src={img} alt="" className="w-full h-full object-cover" />
              </View>
            ))}
            <View className="w-20 h-20 border-2 border-dashed border-gray-300 rounded flex items-center justify-center text-gray-400">
              <Camera size={24} />
            </View>
          </View>
        </View>

        {/* Submit Button */}
        <Button
          onClick={handleSubmit}
          disabled={!instrument || !problem || submitting}
          className="w-full py-3 bg-brand-primary text-white rounded-lg font-medium disabled:opacity-50"
        >
          {submitting ? '提交中...' : '提交报修'}
        </Button>
      </View>
    </View>
  );
}
