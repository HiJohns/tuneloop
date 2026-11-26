// #2546：商户「所有人（唯一 OWNER）」纯函数。
//
// 所有权在本地只有一份权威：merchants.admin_uid（后端 /admin/merchants/:id/members
// 以 owner_uid + 每行 is_owner 下发）。前端不缓存也不推断第二份所有权状态，
// 只按后端字段派生展示与可操作性，便于 node --test 直接覆盖。

/**
 * 选出当前所有人行（后端按 admin_uid 计算的 is_owner）。
 * @param {Array<object>} members
 * @returns {object|null}
 */
export function pickOwnerRow(members) {
  if (!Array.isArray(members)) return null;
  const owner = members.find((member) => member && member.is_owner === true);
  return owner || null;
}

/**
 * 转让候选：本商户 active 成员中排除当前所有人。
 * @param {Array<object>} members
 * @param {string} ownerUid
 * @returns {Array<object>}
 */
export function transferCandidates(members, ownerUid) {
  if (!Array.isArray(members)) return [];
  return members.filter((member) => {
    if (!member || !member.user_id) return false;
    if (member.is_owner === true) return false;
    if (ownerUid && member.user_id === ownerUid) return false;
    return true;
  });
}

/**
 * 该成员是否可被移除：所有人只能转让，不能移除。
 * @param {object|null} record
 * @param {string} ownerUid
 * @returns {boolean}
 */
export function canRemoveMember(record, ownerUid) {
  if (!record) return false;
  if (record.is_owner === true) return false;
  if (ownerUid && record.user_id === ownerUid) return false;
  return true;
}

/**
 * 转让二次确认文案（固定声明转让后当前所有人的新身份）。
 * @param {string} targetName
 * @returns {string}
 */
export function transferConfirmText(targetName) {
  const name = String(targetName || '').trim();
  const who = name || '所选成员';
  return `确定将商户所有权转让给 ${who} 吗？转让后你将成为管理员，且无法撤销。`;
}

/**
 * 「转让所有权」入口门控（#2546 D14）。
 *
 * 真值来自后端成员列表的 `viewer_is_owner`（merchants.admin_uid == 当前登录账号），
 * 不是平台级 `user_info.isOwner`（那是 users.is_owner 遗留标记，与组织 OWNER 无关）。
 * 四个条件全部满足才允许转让：后端判定「我是所有人」、本地已锚定所有人、
 * 锚定指针指向我本人、且存在除我之外的候选成员。
 *
 * @param {{viewerIsOwner?: boolean, ownerUid?: string, viewerUid?: string, candidates?: Array<object>}} input
 * @returns {boolean}
 */
export function canTransferOwnership({ viewerIsOwner, ownerUid, viewerUid, candidates } = {}) {
  if (viewerIsOwner !== true) return false;
  if (!ownerUid || !viewerUid) return false;
  if (ownerUid !== viewerUid) return false;
  return Array.isArray(candidates) && candidates.length > 0;
}
