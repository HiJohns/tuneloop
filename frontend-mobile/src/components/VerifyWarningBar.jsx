// VerifyWarningBar — 实名核身警告条（#1792 T4，checkout/order-detail 共用）
// 仅警告 + 跳转编辑资料页，不阻断提交（核身不设支付/提交门槛）。
// Props:
//   status: id_verify_status（none/uploaded/pending_review/verified/rejected）
//   navigate: H5 路由 navigate 函数（weapp 用 Taro 跳转）
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { env } from '../platform'

const VerifyWarningBar = ({ status = '', navigate }) => {
  // verified / pending_review 无警告（pending_review 可轻提示，不强制）。
  if (status === 'verified' || status === 'pending_review') return null
  if (!status || status === 'none' || status === 'uploaded' || status === 'rejected') {
    const isRejected = status === 'rejected'
    const text = isRejected
      ? '实名核身审核未通过，请重新采集'
      : status === 'uploaded'
        ? '请完成自拍核身，以免影响发货'
        : '请先完成实名核身，以免影响发货'
    const goVerify = () => {
      if (env.isMiniProgram) {
        Taro.navigateTo({ url: '/pages-weapp/profile/edit/index' })
      } else if (navigate) {
        navigate('/profile/edit')
      }
    }
    return (
      <View style={{ backgroundColor: isRejected ? '#fef2f2' : '#fffbeb', borderWidth: 1, borderColor: isRejected ? '#fecaca' : '#fde68a', borderRadius: 8, padding: 10, marginBottom: 12, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <Text style={{ fontSize: 12, color: isRejected ? '#dc2626' : '#92400e', flex: 1 }}>
          {text}
        </Text>
        <View
          onClick={goVerify}
          style={{ marginLeft: 8, paddingHorizontal: 10, paddingVertical: 4, backgroundColor: isRejected ? '#dc2626' : '#f59e0b', borderRadius: 12 }}>
          <Text style={{ color: '#fff', fontSize: 11, fontWeight: '600' }}>去核身</Text>
        </View>
      </View>
    )
  }
  return null
}

export default VerifyWarningBar
