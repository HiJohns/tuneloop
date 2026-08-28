// 后端错误消息统一解析器（#1675）
// 后端 message 是机器可读稳定标识（英文），前端在此集中翻译为用户友好中文。
// 三层回退：L1 message 精确匹配 → L2 code 家族映射 → L3 调用点 fallback。

// L2: code 家族 → 通用中文（兜底）
export const ERROR_CODE_MAP = {
  40000: '请求参数有误，请检查后重试',
  40001: '请求参数有误，请检查后重试',
  40002: '信息不完整，请补充必填项',
  40003: '操作不符合当前业务规则',
  40004: '数据已存在，请勿重复操作',
  40005: '当前状态不允许此操作',
  40100: '登录状态异常，请重新登录',
  40101: '登录已过期，请重新登录',
  40102: '登录已过期，请重新登录',
  40104: '登录状态异常，请重新登录',
  40300: '暂无权限执行此操作',
  40301: '暂无权限执行此操作',
  40400: '未找到相关数据',
  40900: '数据冲突，请刷新后重试',
  40901: '数据冲突，请刷新后重试',
  42900: '操作过于频繁，请稍后再试',
  50000: '服务器开小差了，请稍后再试',
  50001: '服务暂不可用，请稍后再试',
  50002: '数据保存失败，请稍后再试',
  50003: '文件上传失败，请重试',
  50004: '数据保存失败，请稍后再试',
}

// L1: 高频后端 message → 具体中文
export const ERROR_MESSAGE_MAP = {
  'instrument id is required': '缺少乐器信息',
  'instrument not found': '未找到乐器',
  'user not found': '未找到用户',
  'order not found': '未找到订单',
  'order_id is required': '缺少订单信息',
  'order id is required': '缺少订单信息',
  'site not found': '未找到网点',
  'tenant_id not found': '未找到租户信息',
  'Tenant ID is required': '缺少租户信息',
  'appeal not found': '未找到申诉记录',
  'ticket not found': '未找到维修工单',
  'ticket id required': '缺少工单信息',
  'forwarding session not found': '未找到流转记录',
  'property not found': '未找到属性',
  'setting key is required': '缺少配置项标识',
  'invalid side, must be front or back': '请选择正确的证件面',
  'sn is required': '请填写乐器序列号',
  'tracking number required': '请填写物流单号',
  'Merchant not found': '未找到商户',
  'merchant not found': '未找到商户',
  'Member not found': '未找到会员',
  'invalid parameters': '请求参数有误，请检查后重试',
  'invalid request': '请求参数有误，请检查后重试',
  'invalid request body': '请求参数有误，请检查后重试',
  'invalid id': '参数有误，请刷新后重试',
  'not found': '未找到相关数据',
  'failed to save payment record': '支付记录保存失败，请重试',
  'failed to create payment record': '支付创建失败，请重试',
  'failed to create payment': '支付创建失败，请重试',
  'failed to update order status': '订单状态更新失败，请重试',
  'failed to update instrument status': '乐器状态更新失败，请重试',
  'failed to update status': '状态更新失败，请重试',
  'failed to record status history': '状态记录保存失败，请重试',
  'failed to query pricing config': '价格配置读取失败，请重试',
  'user sync failed': '用户信息同步失败，请重试',
  'File upload failed': '文件上传失败，请重试',
  'invalid input': '输入有误，请检查后重试',
  '该手机号或邮箱已注册，请直接登录': '该手机号或邮箱已注册，请直接登录',
  'membership payment requires open_id': '暂无法发起支付，请重新登录后重试',
  'registration session not found or not pending': '注册会话已失效，请重新提交',
  'registration session expired, please resubmit': '注册会话已过期，请重新提交',
  'wx_user_not_found': '该微信尚未注册，请先注册会员',
  // #1798: instrument delete error messages (machine-readable English from backend)
  'instrument in use': '乐器正在使用中，无法删除',
  'instrument has linked orders': '乐器存在关联订单（历史交易），无法删除',
  'delete instrument failed': '删除乐器失败，请重试',
}

// 解析器（唯一出口）
export function resolveErrorMessage(result, fallback = '操作失败，请重试') {
  if (!result) return fallback
  const { code, message } = result
  if (message && ERROR_MESSAGE_MAP[message]) return ERROR_MESSAGE_MAP[message]
  if (code && ERROR_CODE_MAP[code]) return ERROR_CODE_MAP[code]
  return fallback
}
