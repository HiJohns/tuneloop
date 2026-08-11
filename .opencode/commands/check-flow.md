# /check-flow — 全链路静态检查（结构层 + 行为层）

## 用途
对指定的业务流程做前端全链路静态校验（结构完整性 + 控件门控 + 数据刷新），代替部分手工测试工作。

## 用法
```
/check-flow <参数>
```

## 参数
- **用例文件路径**：`docs/cases/lease.md`（检查指定文件的所有用例）
- **流程关键词**：`damage-refund` / `下单发货` / `L-04`（自动匹配对应用例）
- **默认（无参数）**：检查 `docs/cases/lease.md` 全部用例

## 执行
```bash
# 根据参数解析目标文件
CASES_DIR="docs/cases"
TARGET=""

# 若参数是文件路径（含 / 或 .md）
if echo "$1" | grep -qE "/|\.md$"; then
    TARGET="$CASES_DIR/$(basename "$1" .md).md"
    [ ! -f "$TARGET" ] && TARGET="$1"
# 若参数是关键词（如 L-04 / damage-refund / 下单）
elif [ -n "$1" ]; then
    # 搜索匹配的用例
    MATCH=$(grep -rl "$1" "$CASES_DIR"/*.md 2>/dev/null | head -1)
    if [ -z "$MATCH" ]; then
        echo "未找到匹配 '$1' 的用例文件"
        echo "可用用例：$(ls $CASES_DIR/*.md | grep -v README | grep -v _template | xargs -I{} basename {} .md | tr '\n' ' ')"
        exit 1
    fi
    TARGET="$MATCH"
else
    TARGET="$CASES_DIR/lease.md"
fi

echo "=== /check-flow: $(basename "$TARGET" .md) ==="
echo ""

# 结构层检查
echo "--- 结构层：页面注册 + 控件存在 + API 路由 ---"
python3 scripts/checklist-verify.py "$TARGET" --verbose 2>&1 | grep -E "❌|✅|结果|跨端死链" | head -60

echo ""
echo "--- 行为层：控件门控 + 数据刷新 ---"
python3 scripts/checklist-verify.py "$TARGET" --behavioral --verbose 2>&1 | grep "⚠️" | head -30

echo ""
echo "结构层使用 --verbose 查看全部结果"
echo "行为层使用 --behavioral 启用（控件门控 / 数据刷新检查）"
```

## 行为层检查说明

| 检查项 | 检测范围 | 对应 Bug 类型 |
|--------|---------|--------------|
| **控件门控** | YAML `gate` 字段 vs JSX 条件变量 | 按钮在错误状态下显示/缺失（#1623） |
| **数据刷新** | weapp 页缺 `useDidShow` / H5 useEffect 缺 id 依赖 | 页面重进拿缓存旧数据（#1625） |

## 已知限制
- 控件门控依赖 YAML `controls` 文本与 JSX 源码精确匹配（含 emoji 的控件名需在 YAML 中同步）
- 数据刷新检查依赖 `_page_jsx` 路径映射表（新增页面需补充映射）
- 不覆盖后端计算逻辑（#1621 类）——需 Go test 覆盖
