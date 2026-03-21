import { useEffect, useState } from 'react';
import { Button } from 'antd';

export default function TransferProgress({ accumulatedMonths, targetMonths = 12, onViewCertificate }) {
  const [progress, setProgress] = useState(0);
  const [color, setColor] = useState('#3B82F6');
  
  useEffect(() => {
    const percentage = Math.min((accumulatedMonths / targetMonths) * 100, 100);
    setProgress(percentage);
    
    if (accumulatedMonths >= targetMonths) {
      setColor('#22C55E');
    } else if (accumulatedMonths >= 6) {
      setColor('#F59E0B');
    } else {
      setColor('#3B82F6');
    }
  }, [accumulatedMonths, targetMonths]);
  
  const isComplete = accumulatedMonths >= targetMonths;
  const remaining = Math.max(targetMonths - accumulatedMonths, 0);
  
  return (
    <div className="flex flex-col items-center">
      <div className="relative w-32 h-32">
        <svg className="w-full h-full transform -rotate-90">
          <circle
            cx="64"
            cy="64"
            r="56"
            stroke="#E5E7EB"
            strokeWidth="12"
            fill="none"
          />
          <circle
            cx="64"
            cy="64"
            r="56"
            stroke={color}
            strokeWidth="12"
            fill="none"
            strokeDasharray={`${(progress / 100) * 352} 352`}
            strokeLinecap="round"
            className="transition-all duration-500"
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          {isComplete ? (
            <span className="text-3xl">🎉</span>
          ) : (
            <>
              <span className="text-2xl font-bold text-gray-800">{accumulatedMonths}</span>
              <span className="text-sm text-gray-500">/ {targetMonths}个月</span>
            </>
          )}
        </div>
      </div>
      
      <div className="mt-4 text-center">
        {isComplete ? (
          <div className="space-y-2">
            <p className="text-green-600 font-medium">恭喜！租期已满</p>
            <Button type="primary" onClick={onViewCertificate}>
              查看电子证书
            </Button>
          </div>
        ) : (
          <p className="text-gray-600">
            还需 <span className="font-bold text-orange-500">{remaining}</span> 个月
          </p>
        )}
      </div>
    </div>
  );
}
