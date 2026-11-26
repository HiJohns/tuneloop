// #2546 R16：checkPermission 的 businessRoles 分支（node --test，PC 无组件测试运行器）。
//
// 商户管理员（businessRole=merchant_admin，sys_perm=1073740800 = bit 10-29）不含
// tenant_list/view/update 位，必须靠 businessRoles 才能进入 /merchants；平台专属
// 路由（/merchants/new）不得带 businessRoles。
import test from 'node:test';
import assert from 'node:assert/strict';

import { checkPermission } from './menuPermissions.js';

const MERCHANT_ADMIN_SYS_PERM = 1073740800; // bit 10-29
const PLATFORM_SYS_PERM = 0b11 | (1 << 5) | (1 << 6) | (1 << 8); // namespace_admin 位图
const merchantAdmin = { businessRole: 'merchant_admin' };
const platformAdmin = { businessRole: 'system_admin' };

const merchantMenuPermission = { sysPermBits: [6], businessRoles: ['merchant_admin'] };
const merchantDetailPermission = { sysPermBits: [5], businessRoles: ['merchant_admin'] };
const newMerchantPermission = { sysPermBits: [5] };

test('CHECKPERM_MA：商户管理员命中 businessRoles → 商户菜单/详情可进入', () => {
  assert.equal(
    checkPermission(merchantMenuPermission, MERCHANT_ADMIN_SYS_PERM, 0, {}, merchantAdmin),
    true,
  );
  assert.equal(
    checkPermission(merchantDetailPermission, MERCHANT_ADMIN_SYS_PERM, 0, {}, merchantAdmin),
    true,
  );
});

test('CHECKPERM_PLATFORM：平台管理员仍有 sys_perm 位 → 商户菜单/详情可进入', () => {
  assert.equal(
    checkPermission(merchantMenuPermission, PLATFORM_SYS_PERM, 0, {}, platformAdmin),
    true,
  );
  assert.equal(
    checkPermission(merchantDetailPermission, PLATFORM_SYS_PERM, 0, {}, platformAdmin),
    true,
  );
});

test('NEW_ROUTE_PLATFORM_ONLY：/merchants/new 对商户管理员仍拒绝', () => {
  assert.equal(
    checkPermission(newMerchantPermission, MERCHANT_ADMIN_SYS_PERM, 0, {}, merchantAdmin),
    false,
  );
  assert.equal(
    checkPermission(newMerchantPermission, PLATFORM_SYS_PERM, 0, {}, platformAdmin),
    true,
  );
});

test('无 businessRole 命中且无 sys/cus 位 → 拒绝（不因新增字段而放行所有人）', () => {
  assert.equal(
    checkPermission({ businessRoles: ['merchant_admin'] }, MERCHANT_ADMIN_SYS_PERM, 0, {}, { businessRole: 'site_admin' }),
    false,
  );
  assert.equal(checkPermission({ businessRoles: ['merchant_admin'] }, 0, 0, {}, {}), false);
});

test('既无 sys/cus 也无 businessRoles → 放行（既有语义不变）', () => {
  assert.equal(checkPermission({}, 0, 0, {}, {}), true);
  assert.equal(checkPermission(null, 0, 0, {}, {}), true);
});

test('requireAll 与 businessRoles 并存时，businessRoles 命中即放行（OR）', () => {
  assert.equal(
    checkPermission(
      { sysPermBits: [10], cusPermCodes: ['instrument:read'], requireAll: true, businessRoles: ['merchant_admin'] },
      MERCHANT_ADMIN_SYS_PERM,
      0,
      { 'instrument:read': 0 },
      merchantAdmin,
    ),
    true,
  );
});
