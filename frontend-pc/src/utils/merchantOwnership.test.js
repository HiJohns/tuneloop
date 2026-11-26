// #2546 R8：商户所有人纯函数（node --test，PC 无组件测试运行器）。
import test from 'node:test';
import assert from 'node:assert/strict';

import {
  pickOwnerRow,
  transferCandidates,
  canRemoveMember,
  transferConfirmText,
} from './merchantOwnership.js';

const ownerRow = { user_id: 'u-owner', user_name: '所有人', role: 'merchant_admin', is_owner: true };
const adminRow = { user_id: 'u-admin', user_name: '管理员', role: 'merchant_admin', is_owner: false };
const staffRow = { user_id: 'u-staff', user_name: '直属员工', role: 'merchant_staff', is_owner: false };

test('pickOwnerRow 返回唯一的 is_owner 行', () => {
  assert.equal(pickOwnerRow([ownerRow, adminRow]), ownerRow);
});

test('pickOwnerRow 在未锚定所有人时返回 null（不得猜测）', () => {
  assert.equal(pickOwnerRow([adminRow, staffRow]), null);
  assert.equal(pickOwnerRow([]), null);
  assert.equal(pickOwnerRow(null), null);
});

test('transferCandidates 排除当前所有人（含 is_owner 与 owner_uid 两种标记）', () => {
  assert.deepEqual(transferCandidates([ownerRow, adminRow, staffRow], 'u-owner'), [adminRow, staffRow]);
  // owner_uid 有值但该行 is_owner 缺失（旧响应）时同样排除
  assert.deepEqual(transferCandidates([{ ...ownerRow, is_owner: undefined }, adminRow], 'u-owner'), [adminRow]);
});

test('transferCandidates 在只有所有人时为空（禁用转让入口）', () => {
  assert.deepEqual(transferCandidates([ownerRow], 'u-owner'), []);
  assert.deepEqual(transferCandidates(null, 'u-owner'), []);
});

test('canRemoveMember 禁止移除所有人，允许移除其他成员', () => {
  assert.equal(canRemoveMember(ownerRow, 'u-owner'), false);
  assert.equal(canRemoveMember({ ...ownerRow, is_owner: undefined }, 'u-owner'), false);
  assert.equal(canRemoveMember(adminRow, 'u-owner'), true);
  assert.equal(canRemoveMember(staffRow, 'u-owner'), true);
  assert.equal(canRemoveMember(null, 'u-owner'), false);
});

test('transferConfirmText 固定声明「转让后你将成为管理员」', () => {
  assert.match(transferConfirmText('管理员甲'), /管理员甲/);
  assert.match(transferConfirmText('管理员甲'), /转让后你将成为管理员/);
  assert.match(transferConfirmText(''), /所选成员/);
  assert.match(transferConfirmText(undefined), /转让后你将成为管理员/);
});
