// #2214: 集中拦截——三层回退出口统一经 t()（覆盖全部 resolveErrorMessage 调用点）
import { t } from '../i18n'
// #2480: 解析层序收敛到纯核心（无 i18n/Taro 依赖，可被 node --test 覆盖）。
import { resolveErrorCore } from './errorMessageResolution.mjs'

// 后端错误消息统一解析器（#1675）
// 后端 message 是机器可读稳定标识（英文），前端在此集中翻译为用户友好中文。
// 层序 owner = errorMessageResolution.mjs（#2480 / #2504）：
//   L1 message 精确匹配 → L2 CJK 可读文案 → L3 code 家族映射 → L4 CJK 兜底透传 → L5 调用点 fallback。
// #2504：ASCII 机器串不外溢（如 `access denied` 由 40303 → 「暂无权限访问该页面」）。

// L2: code 家族 → 通用中文（兜底）
export const ERROR_CODE_MAP = {
  40000: '请求参数有误，请检查后重试',
  40001: '请求参数有误，请检查后重试',
  40002: '信息不完整，请补充必填项',
  40003: '操作不符合当前业务规则',
  40004: '数据已存在，请勿重复操作',
  40005: '当前状态不允许此操作',
  40006: '用户名或密码错误', // #2080
  40007: '账号未激活，请联系管理员', // #2080
  40008: '服务已停用，请联系管理员', // #2080
  40100: '登录状态异常，请重新登录',
  40101: '登录已过期，请重新登录',
  40102: '登录已过期，请重新登录',
  40104: '登录状态异常，请重新登录',
  40300: '暂无权限执行此操作',
  40301: '暂无权限执行此操作',
  40303: '暂无权限访问该页面', // #2504：personnel/权限中间件「权限范围不匹配」
  40400: '未找到相关数据',
  40900: '数据冲突，请刷新后重试',
  40901: '数据冲突，请刷新后重试',
  42900: '操作过于频繁，请稍后再试',
  50000: '服务器开小差了，请稍后再试',
  50001: '服务暂不可用，请稍后再试',
  50002: '数据保存失败，请稍后再试',
  50003: '文件上传失败，请重试',
  50004: '数据保存失败，请稍后再试',
  // #2598：上游 IAM 401/403/404（配置/权限/数据不一致）——持久失败，不可重试。
  // 文案必须与后端 `backend/handlers/iam_failure.go` 的 iamConfigErrorMessage 逐字一致。
  50015: '身份服务配置异常，请联系平台管理员处理',
}

// L1: 高频后端 message → 具体中文
export const ERROR_MESSAGE_MAP = {
  'invalid credentials': '用户名或密码错误', // #2080
  'account not active': '账号未激活，请联系管理员', // #2080
  'user id verification required': '用户未完成实名核身，请联系平台运营完成审核', // #1792/#1796
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
  // #1861: staff refund / settlement isolation messages
  'order does not belong to your site': '订单不属于您所在的网点',
  'no permission to refund orders': '无退款权限',
  'order is not in refunding status': '订单当前状态不允许退款',
  // #2601 Rev.2 D6b：扫码绑定/合并的「过期/失效」分支仍是机器串（后端保留 ASCII，
  // 文案 owner 在前端码表）。不登记会落到 ERROR_CODE_MAP 的 40004/40002 通用语义
  // （「数据已存在」「信息不完整」），把「二维码过期」误陈述成别的失败。
  'invalid or expired token': '绑定二维码已失效，请让管理员重新生成二维码后重试',
  'invalid or expired merge ticket': '绑定二维码已失效，请让管理员重新生成二维码后重试',
  'merge ticket does not match binding token': '绑定二维码已失效，请让管理员重新生成二维码后重试',
}

// 解析器（唯一出口）
// #2480：层序 owner = 纯核心 errorMessageResolution.mjs（PC 同序，跨包漂移由平价脚本兜住）。
// 仅 map 命中层与 fallback 走 t()（源中文串即 key，#2214）；后端原文（human_message /
// raw_message）原样返回——对用户文案做字典查找没有意义，也会掩盖真实内容。
export function resolveErrorMessage(result, fallback = '操作失败，请重试') {
  const core = resolveErrorCore(result, {
    codeMap: ERROR_CODE_MAP,
    messageMap: ERROR_MESSAGE_MAP,
    fallback,
  })
  if (core.layer === 'human_message' || core.layer === 'raw_message') {
    return core.text
  }
  return t(core.text)
}
