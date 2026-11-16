# R3 人工回执 —— #2370 一次性初始密码 Modal

> 本文件为**待 Owner 填写的回执模板**。核对完成后，把文末 `RESULT: PENDING` 改为 `RESULT: PASS`（或 `FAIL` + 原因）。
> 契约：`grep -q 'RESULT: PASS' docs/evidence/2370/R3-modal-manual.md`（见 #2370 Accepted Plan 的 R3 行）。

## 被测对象

- 组件：`frontend-pc/src/components/InitialPasswordModal.jsx`
- 调用约定：`initialPassword = [{ identifier, password }, ...]`（1..N 条）
- 引入点：创建用户/成员的「创建成功」回调（#2372 / #2373 合入后），或临时页面传 mock 数据验证

## 前置

- [ ] 工作区在包含 `f6d988544` 的提交上
- [ ] 准备两组 mock：1 条（`[{identifier:"tech_a", password:"Abcd1234Wxyz"}]`）与 3 条

## 核对步骤与期望

| # | 操作 | 期望 | 实测 |
|---|---|---|---|
| 1 | 以 1 条数据打开 Modal | 显示 `identifier` 与对应 12 位密码；无多余空行 | ☐ |
| 2 | 点击密码旁的复制按钮 | 剪贴板内容 === 该密码（粘贴到记事本核对） | ☐ |
| 3 | 以 3 条数据打开 Modal | 逐条展示 `identifier: password`，顺序与入参一致 | ☐ |
| 4 | 关闭 Modal 后再次触发同一批数据 | **不可再查看**（父组件已清空；列表/详情中不出现明文密码） | ☐ |
| 5 | 刷新页面 | 明文密码不复现（未落 localStorage / sessionStorage / URL） | ☐ |
| 6 | DevTools → Network / Application | 无密码写入 storage；响应体仅创建接口返回一次 | ☐ |

## 证据要求

- [ ] 步骤 1 截图（1 条）
- [ ] 步骤 3 截图（3 条，注意遮挡中间密码）
- [ ] 步骤 5/6 的 Application → Storage 截图（证明无落盘）
- [ ] 剪贴板核对的说明（可文字描述）

## 结论

- 核对人（executor）：user
- 核对时间：
- 环境：本地 dev / 预生产（删除其一）
- 备注：

RESULT: PASS
