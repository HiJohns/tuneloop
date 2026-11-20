// #2427：人员管理查询参数统一构造。
// 背景：`site_id: values.siteId || null` 这类写法依赖下游（api.get/URLSearchParams）
// 兜底过滤空值；本模块把「空值不入参」收口到唯一入口，便于单测与跨页面复用。
// 规则：
//   - name 去首尾空白，空串省略；
//   - site_id：null / undefined / '' 一律省略（后端 site_id 为空 = 不过滤）；
//   - direct 仅在显式为 true（或 'true'）时输出 'direct=true'；
//   - page / page_size 原样保留（缺省则不输出，交给后端默认值）。

function isBlank(v) {
  return v === undefined || v === null || v === ''
}

export function buildPersonnelQuery({ name, siteId, direct, page, pageSize } = {}) {
  const params = {}

  const trimmedName = typeof name === 'string' ? name.trim() : name
  if (!isBlank(trimmedName)) params.name = trimmedName

  if (!isBlank(siteId)) params.site_id = siteId

  if (direct === true || direct === 'true') params.direct = 'true'

  if (!isBlank(page)) params.page = page
  if (!isBlank(pageSize)) params.page_size = pageSize

  return params
}

// 与 api.get 的序列化规则保持一致（同一过滤条件），供断言「查询串不含空值」使用。
export function personnelQueryString(params = {}) {
  return new URLSearchParams(
    Object.entries(params).filter(([, v]) => v !== undefined && v !== null && v !== '')
  ).toString()
}
