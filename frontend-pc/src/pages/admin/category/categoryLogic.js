// #2133 分类可见性与排序解耦（PC 端共享业务逻辑）
//
// 契约：
// - 可见性唯一由 `category.visible` 决定。sort **不参与**显隐判读。
// - `sort` 是纯次序值，允许任意整数（含 0 与负数）。
//   DB 列默认值即为 0（`categories.sort integer DEFAULT 0`），
//   故 0 是合法序位，「sort=0 即隐藏」的旧哨兵约定已于 #2133 废除。
// - `assignPayload` 以**传入数组的顺序**为准重编号，不再按 `sort` 二次排序
//   （原因见该函数注释；#2133 二审 BLOCKER-1）。
// - 一级（antd List）与二级（dnd 拖拽）分类共用本模块的判定与排序逻辑；
//   渲染层差异留在各自组件内，不在此处处理。
//
// 后端对应：`GET /public/categories` 仅按 `visible = true` 过滤（handlers/public.go）。

/** 分类是否隐藏。唯一判读来源 —— 只看 visible，绝不看 sort。 */
export function isHidden(cat) {
  return !cat.visible
}

/** 排序值归一：null/undefined 视为 0，与 DB 默认值一致。 */
function sortValue(cat) {
  return cat.sort ?? 0
}

/** 按 sort 升序排序，返回新数组（不改原数组）。 */
export function bySort(list) {
  return [...list].sort((a, b) => sortValue(a) - sortValue(b))
}

/**
 * 可见分类的有序序列 —— 渲染顺序与上下移边界共用的唯一结果。
 * 抽出此函数是刻意的：此前 renderItem 内对每项重复计算，等价逻辑存在两份手写拷贝。
 */
export function visibleOrdered(list) {
  return bySort(list.filter(cat => !isHidden(cat)))
}

/**
 * 生成全量连续重编号载荷：可见项排 1..k，隐藏项排 k+1..k+m。
 *
 * ⚠️ **入参数组的顺序即目标顺序**，本函数刻意**不再**按 `sort` 重新排序。
 * 原因：调用方（上下移 moveVisible / 拖拽 handleDragEnd）只调整数组下标、
 * 并不写入新的 `sort` 值。若此处再按库中旧 `sort` 排序，就会把调用方刚做出的
 * 重排还原成原顺序，产出与现状逐字段相同的「原样回写」载荷 —— 服务端零变化、
 * 前端却提示「排序已更新」，使排序操作静默失效（#2133 二审 BLOCKER-1）。
 * 因此基准次序由调用方负责建立（如 `bySort(list)` 或 `visibleOrdered(list)`），
 * 本函数只负责 visible/hidden 分段与连续编号。
 *
 * 效果：
 * - 存量 sort=0 的重复值在首次排序操作后自动归一为正数
 * - 隐藏项不占用可见序位，取消隐藏后落在可见块末尾（而非跳到最前）
 * - 一二级共用同一算法，消除「上下移只排可见项 / 拖拽排全量」的分裂
 */
export function assignPayload(orderedList) {
  const visible = orderedList.filter(cat => !isHidden(cat))
  const hidden = orderedList.filter(cat => isHidden(cat))

  const payload = []
  visible.forEach((cat, i) => payload.push({ id: cat.id, sort: i + 1 }))
  hidden.forEach((cat, i) => payload.push({ id: cat.id, sort: visible.length + i + 1 }))

  return payload
}
