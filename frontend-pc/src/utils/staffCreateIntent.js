// #2478：跨页「创建员工」一次性意图。
//
// 直属成员管理（/staff/direct）的「创建员工」跳转人员管理（/staff），后者默认
// 列表视图，只有页内按钮能切到创建视图 → 表现为「要点两次」。人员管理的查询参数
// 是本地 state（#2427），不能用 URL query 传意图，因此用 react-router 的
// location.state 携带一次性意图，落地页消费后立即 replace 清空。
//
// 纯函数收敛在此模块，供 node --test 覆盖（frontend-pc 无测试运行器）。

export const CREATE_INTENT_OPEN = 'openCreate'

// 严格布尔判定：字符串 'true'、1 等宽松真值一律不触发，避免外部构造 state 误开表单。
export function isCreateIntent(state) {
  return Boolean(state && state[CREATE_INTENT_OPEN] === true)
}

// 直属成员管理当前只创建商户直属员工（user_type 固定 merchant_direct）。
export function createIntentState() {
  return { [CREATE_INTENT_OPEN]: true, userType: 'merchant_direct' }
}

// 幂等判定：null/undefined/{}/false 均为 false；消费后 state 被清空即不再触发。
export function isConsumableIntent(state) {
  return isCreateIntent(state)
}
