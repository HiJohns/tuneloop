import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { contractsApi } from '../services/api'
import { ArrowLeft, FileText, ChevronRight, ExternalLink, Calendar } from 'lucide-react'

export default function MyContracts() {
  const navigate = useNavigate()
  const [contracts, setContracts] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchContracts = async () => {
      try {
        const resp = await contractsApi.list()
        if (resp.code === 20000) {
          setContracts(resp.data?.list || [])
        }
      } catch (err) {
        console.error('Failed to fetch contracts:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchContracts()
  }, [])

  if (loading) {
    return <div className="min-h-screen bg-brand-bg flex items-center justify-center">
      <div className="text-gray-500">加载中...</div>
    </div>
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">我的合同</h1>
      </div>

      <div className="p-4 space-y-3">
        {contracts.length === 0 ? (
          <div className="bg-white rounded-xl p-8 text-center text-gray-400">
            <FileText size={48} className="mx-auto mb-3 opacity-50" />
            <p>暂无合同</p>
          </div>
        ) : (
          contracts.map(contract => (
            <div
              key={contract.id}
              className="bg-white rounded-xl p-4 cursor-pointer"
              onClick={() => {
                if (contract.contract_url) {
                  window.open(contract.contract_url, '_blank')
                }
              }}
            >
              <div className="flex justify-between items-start">
                <span className="text-sm font-medium">{contract.contract_number || '合同'}</span>
                <span className="text-xs px-2 py-1 rounded-full bg-green-100 text-green-700">
                  {contract.status === 'active' ? '有效' : contract.status}
                </span>
              </div>
              <div className="mt-2 text-xs text-gray-500 space-y-1">
                <p className="flex items-center gap-1">
                  <Calendar size={12} />
                  {contract.generated_at ? new Date(contract.generated_at).toLocaleDateString() : '-'}
                </p>
              </div>
              {contract.contract_url ? (
                <div className="mt-2 flex items-center gap-1 text-xs text-brand-primary">
                  <ExternalLink size={12} /> 查看/下载 PDF
                </div>
              ) : (
                <p className="mt-2 text-xs text-yellow-600">PDF 待生成</p>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  )
}