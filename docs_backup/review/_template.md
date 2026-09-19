# TC-{NNNN} Review — {业务名称}

> TC Issue: #{NNNN} | 生成日期: {YYYY-MM-DD} | 类型: 后端测试 / 前端静态 / 混合

## 1. 目标

{该 TC 验证的业务行为，来自 TC Issue body}

## 2. 验证方式

- [ ] 后端测试: `go test -run {TestName} ./handlers/ -v`
- [ ] 静态验证: `bash scripts/static-verify.sh {flow}`
- [ ] 真机验证: {仅前端 UI 类，#1555 特例}

## 3. 流程步骤与证据

### Step {n}: {动作描述}

**代码证据**: `backend/handlers/{file}.go:{line}` — {函数/控件说明}

**测试证据**:
```
{go test 实际输出，含 PASS 行 + 关键断言行}
```

**静态证据**（如适用）:
```
{static-verify.sh 实际输出，绿色 ✅ 行}
```

### Step {n+1}: ...

## 4. 结论

| 检查项 | 结果 | 证据位置 |
|--------|:---:|---------|
| {业务点 1} | ✅ | Step {n} 测试证据 |
| {业务点 2} | ✅ | ... |
| ... | | |

**总判定**: ✅ 通过 / ⚠️ 部分通过 / ❌ 未通过

## 5. 已知局限

- {无法自动验证的部分}
- {预存失败 / 依赖问题}

---

*Model: deepseek/deepseek-v4-flash*
