// #2133 二审返工：分类排序**调用链**断言脚本
//
// 审计 BLOCKER-1 指出：Work Summary 的 8 条断言均在「已排序输入」上直接单测
// assignPayload()，从未经过 List.jsx 的 moveVisible / handleDragEnd 调用链，
// 因此未能发现「载荷原样回写 → 排序静默失效」。
//
// 本脚本严格复刻 frontend-pc/src/pages/admin/category/List.jsx 的调用形态
// （moveVisible 与 handleDragEnd 的 arrayMove），再断言最终载荷能表达新顺序。
//
// 运行：node scripts/verify_category_order_2133.mjs
// 退出码 0 = 全部通过；1 = 有断言失败。

import {
  isHidden,
  bySort,
  visibleOrdered,
  assignPayload,
} from '../frontend-pc/src/pages/admin/category/categoryLogic.js'

let failed = 0
let passed = 0

function assert(name, actual, expected) {
  const a = JSON.stringify(actual)
  const e = JSON.stringify(expected)
  if (a === e) {
    passed++
    console.log(`  PASS  ${name}`)
  } else {
    failed++
    console.log(`  FAIL  ${name}\n          got=${a}\n          exp=${e}`)
  }
}

/** 复刻 List.jsx 的 moveVisible(cat, list, delta) */
function moveVisible(cat, list, delta) {
  const visible = visibleOrdered(list)
  const idx = visible.findIndex(c => c.id === cat.id)
  const target = idx + delta
  if (idx < 0 || target < 0 || target >= visible.length) return null
  const moved = [...visible]
  const [item] = moved.splice(idx, 1)
  moved.splice(target, 0, item)

  const hiddenItems = bySort(list.filter(c => isHidden(c)))
  return assignPayload([...moved, ...hiddenItems])
}

/** 复刻 List.jsx 的 handleDragEnd 中的 arrayMove */
function arrayMove(list, from, to) {
  const next = [...list]
  const [removed] = next.splice(from, 1)
  next.splice(to, 0, removed)
  return next
}

/** 载荷落库后按 sort 升序得到的可见顺序 */
function applyPayload(payload) {
  return [...payload].sort((a, b) => a.sort - b.sort).map(i => i.id)
}

console.log('=== #2133 BLOCKER-1 回归防护：排序调用链 ===\n')

console.log('[1] 一级分类上下移（moveVisible 调用链）')
{
  const l1 = [
    { id: 'A', name: '小提琴', visible: true, sort: 1 },
    { id: 'B', name: '中提琴', visible: true, sort: 2 },
    { id: 'C', name: '大提琴', visible: true, sort: 3 },
  ]
  assert('下移首项 A → B,A,C', applyPayload(moveVisible(l1[0], l1, 1)), ['B', 'A', 'C'])
  assert('上移末项 C → A,C,B', applyPayload(moveVisible(l1[2], l1, -1)), ['A', 'C', 'B'])
  assert('下移中间项 B → A,C,B', applyPayload(moveVisible(l1[1], l1, 1)), ['A', 'C', 'B'])
  assert('上移中间项 B → B,A,C', applyPayload(moveVisible(l1[1], l1, -1)), ['B', 'A', 'C'])
  assert('首项上移越界 → 无载荷', moveVisible(l1[0], l1, -1), null)
  assert('末项下移越界 → 无载荷', moveVisible(l1[2], l1, 1), null)
}

console.log('\n[2] 二级分类拖拽（handleDragEnd 调用链）')
{
  const subs = [
    { id: 's1', visible: true, sort: 1 },
    { id: 's2', visible: true, sort: 2 },
    { id: 's3', visible: true, sort: 3 },
    { id: 's4', visible: true, sort: 4 },
  ]
  assert('拖 s1 到末位 → s2,s3,s4,s1',
    applyPayload(assignPayload(arrayMove(subs, 0, 3))), ['s2', 's3', 's4', 's1'])
  assert('拖 s4 到首位 → s4,s1,s2,s3',
    applyPayload(assignPayload(arrayMove(subs, 3, 0))), ['s4', 's1', 's2', 's3'])
  assert('拖 s2 到 s3 之后 → s1,s3,s2,s4',
    applyPayload(assignPayload(arrayMove(subs, 1, 2))), ['s1', 's3', 's2', 's4'])
}

console.log('\n[3] 隐藏项不占可见序位，取消隐藏落在可见块末尾')
{
  const list = [
    { id: 'A', visible: true, sort: 1 },
    { id: 'B', visible: true, sort: 2 },
    { id: 'H', visible: false, sort: 0 },
  ]
  assert('含隐藏项下移 A → B,A,H',
    applyPayload(moveVisible(list[0], list, 1)), ['B', 'A', 'H'])

  // 一次排序操作后，隐藏项被排入隐藏块末尾（sort = k+1）
  const parked = assignPayload(bySort(list))
  assert('排序后隐藏项 H 落在隐藏块末尾（sort=3）',
    [...parked].sort((a, b) => a.sort - b.sort).map(i => [i.id, i.sort]),
    [['A', 1], ['B', 2], ['H', 3]])

  // 真实流程：handleHide 只翻 visible、不改 sort、不触发重编号。
  // 隐藏项已停在隐藏块末尾（sort=3），故取消隐藏后自然落在可见块末尾。
  const unhidden = parked.map(i => ({ id: i.id, visible: true, sort: i.sort }))
  assert('取消隐藏 H → A,B,H（落在可见块末尾）',
    applyPayload(assignPayload(visibleOrdered(unhidden))), ['A', 'B', 'H'])
}

console.log('\n[4] 存量 sort=0 归一（首次排序操作后）')
{
  const legacy = [
    { id: 'X', visible: true, sort: 0 },
    { id: 'Y', visible: true, sort: 0 },
    { id: 'Z', visible: true, sort: 5 },
  ]
  assert('重复 sort=0 归一为连续正数', assignPayload(bySort(legacy)).map(i => i.sort), [1, 2, 3])
}

console.log('\n[5] 可见性唯一由 visible 决定（sort 不参与判读）')
{
  assert('visible=true + sort=0 → 可见', isHidden({ visible: true, sort: 0 }), false)
  assert('visible=false + sort=9 → 隐藏', isHidden({ visible: false, sort: 9 }), true)
  assert('sort=-3 且 visible=true → 可见', isHidden({ visible: true, sort: -3 }), false)
  assert('visible=false + sort=0 → 隐藏', isHidden({ visible: false, sort: 0 }), true)
  assert('visible=null 按隐藏处理', isHidden({ sort: 1 }), true)
}

console.log('\n[6] sort 空值归一与排序语义')
{
  assert('null 视为 0', bySort([{ id: 'a', sort: 1 }, { id: 'b', sort: null }]).map(c => c.id),
    ['b', 'a'])
  assert('负数 < 0 < 正数',
    bySort([{ sort: 1 }, { sort: -5 }, { sort: 0 }]).map(c => c.sort), [-5, 0, 1])
}

console.log('\n[7] 边界与不可变性')
{
  assert('空列表 → 空载荷', assignPayload([]), [])
  assert('空列表不抛错', visibleOrdered([]), [])

  const src = [{ id: 'a', visible: true, sort: 2 }, { id: 'b', visible: true, sort: 1 }]
  const snapshot = JSON.stringify(src)
  bySort(src); visibleOrdered(src); assignPayload(src)
  assert('三个函数均不修改原数组', JSON.stringify(src), snapshot)
}

console.log(`\n=== 结果：通过 ${passed} 项，失败 ${failed} 项 ===`)
process.exit(failed === 0 ? 0 : 1)