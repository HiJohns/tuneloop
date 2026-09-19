#!/bin/bash

set -e

echo "🔍 开始环境完整性检查..."

# ============ 1. 核心文件检查与修复 ============

# 确保当前项目有 scripts 目录
mkdir -p scripts
mkdir -p prompts
mkdir -p docs/en

#!/bin/bash

# 1. 处理目录：不存在则创建
if [ ! -d "tmp" ]; then
    mkdir -p tmp
    echo "Directory 'tmp' created."
fi

# 2. 处理 .gitignore：确保包含 tmp/
GITIGNORE=".gitignore"
touch "$GITIGNORE" # 确保文件存在

if ! grep -Fxq "tmp/" "$GITIGNORE"; then
    # 补齐换行符，防止追加在已有内容末尾
    [ -s "$GITIGNORE" ] && [ "$(tail -c1 "$GITIGNORE" | wc -l)" -eq 0 ] && echo "" >> "$GITIGNORE"

    echo "tmp/" >> "$GITIGNORE"
    echo "Added 'tmp/' to .gitignore"

    # 3. Git 流程：仅当 .gitignore 有变动时执行
    if [ -d .git ]; then
        git add "$GITIGNORE"
        git commit -m "chore: add tmp directory to .gitignore"

        # 获取当前分支名并推送
        CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
        git push origin "$CURRENT_BRANCH"
        echo "Changes pushed to branch: $CURRENT_BRANCH"
    else
        echo "Warning: Not a git repository, skipping commit/push."
    fi
fi

# 如果当前在 ~/.config/opencode 目录，完全跳过所有软链接创建（避免自我引用和覆盖）
if [ "$(pwd)" == "$HOME/.config/opencode" ]; then
  echo "⚠️  检测到在 ~/.config/opencode 主配置目录，跳过所有软链接和文件操作"
else
  # ===== 软链接创建区（仅在非 ~/.config/opencode 目录执行）=====
  
  REAL_SCRIPT_PATH="$HOME/.config/opencode/scripts/opencode_gh.sh"
  PROJECT_SCRIPT="scripts/opencode_gh.sh"
  PREPARE_SCRIPT="scripts/prepare.sh"

  # 检查真实脚本是否存在
  if [ ! -f "$REAL_SCRIPT_PATH" ]; then
    echo "❌ Error: $REAL_SCRIPT_PATH 不存在，请检查项目结构"
    exit 1
  fi

  # 检查并修复 opencode_gh.sh 的软链接
  if [ ! -e "$PROJECT_SCRIPT" ]; then
    echo "📝 scripts/opencode_gh.sh 不存在，创建软链接..."
    ln -s "$REAL_SCRIPT_PATH" "$PROJECT_SCRIPT"
  elif [ ! -L "$PROJECT_SCRIPT" ]; then
    echo "📝 scripts/opencode_gh.sh 不是软链接，替换为正确的软链接..."
    rm -f "$PROJECT_SCRIPT"
    ln -s "$REAL_SCRIPT_PATH" "$PROJECT_SCRIPT"
  elif [ "$(readlink "$PROJECT_SCRIPT")" != "$REAL_SCRIPT_PATH" ]; then
    echo "📝 scripts/opencode_gh.sh 指向错误位置，修正软链接..."
    rm -f "$PROJECT_SCRIPT"
    ln -s "$REAL_SCRIPT_PATH" "$PROJECT_SCRIPT"
  fi

  # 检查并修复 find_issue.sh 的软链接
  REAL_FIND_ISSUE_PATH="$HOME/.config/opencode/scripts/find_issue.sh"
  PROJECT_FIND_ISSUE="scripts/find_issue.sh"
  if [ ! -e "$PROJECT_FIND_ISSUE" ]; then
    echo "📝 scripts/find_issue.sh 不存在，创建软链接..."
    ln -s "$REAL_FIND_ISSUE_PATH" "$PROJECT_FIND_ISSUE"
  elif [ ! -L "$PROJECT_FIND_ISSUE" ]; then
    echo "📝 scripts/find_issue.sh 不是软链接，替换为正确的软链接..."
    rm -f "$PROJECT_FIND_ISSUE"
    ln -s "$REAL_FIND_ISSUE_PATH" "$PROJECT_FIND_ISSUE"
  elif [ "$(readlink "$PROJECT_FIND_ISSUE")" != "$REAL_FIND_ISSUE_PATH" ]; then
    echo "📝 scripts/find_issue.sh 指向错误位置，修正软链接..."
    rm -f "$PROJECT_FIND_ISSUE"
    ln -s "$REAL_FIND_ISSUE_PATH" "$PROJECT_FIND_ISSUE"
  fi

  # 检查并修复 prepare.sh 的软链接
  REAL_PREPARE_PATH="$HOME/.config/opencode/scripts/prepare.sh"
  if [ ! -e "$PREPARE_SCRIPT" ]; then
    echo "📝 scripts/prepare.sh 不存在，创建软链接..."
    ln -s "$REAL_PREPARE_PATH" "$PREPARE_SCRIPT"
  elif [ ! -L "$PREPARE_SCRIPT" ]; then
    echo "📝 scripts/prepare.sh 不是软链接，替换为正确的软链接..."
    rm -f "$PREPARE_SCRIPT"
    ln -s "$REAL_PREPARE_PATH" "$PREPARE_SCRIPT"
  elif [ "$(readlink "$PREPARE_SCRIPT")" != "$REAL_PREPARE_PATH" ]; then
    echo "📝 scripts/prepare.sh 指向错误位置，修正软链接..."
    rm -f "$PREPARE_SCRIPT"
    ln -s "$REAL_PREPARE_PATH" "$PREPARE_SCRIPT"
  fi

  # 检查并修复 prompts/instructions.md 的软链接
  INSTRUCTIONS_TARGET="$HOME/.config/opencode/prompts/instructions.md"
  INSTRUCTIONS_LINK="prompts/instructions.md"
  if [ ! -e "$INSTRUCTIONS_LINK" ] || [ ! -L "$INSTRUCTIONS_LINK" ] || [ "$(readlink "$INSTRUCTIONS_LINK")" != "$INSTRUCTIONS_TARGET" ]; then
    echo "📝 prompts/instructions.md 检查/修复中..."
    rm -f "$INSTRUCTIONS_LINK"
    ln -s "$INSTRUCTIONS_TARGET" "$INSTRUCTIONS_LINK"
  fi

  # 检查并修复 prompts/task-profiles.md 的软链接
  TASK_PROFILES_TARGET="$HOME/.config/opencode/prompts/task-profiles.md"
  TASK_PROFILES_LINK="prompts/task-profiles.md"
  if [ ! -e "$TASK_PROFILES_LINK" ] || [ ! -L "$TASK_PROFILES_LINK" ] || [ "$(readlink "$TASK_PROFILES_LINK")" != "$TASK_PROFILES_TARGET" ]; then
    echo "📝 prompts/task-profiles.md 检查/修复中..."
    rm -f "$TASK_PROFILES_LINK"
    ln -s "$TASK_PROFILES_TARGET" "$TASK_PROFILES_LINK"
  fi
fi

# 检查 prompts/project.md（此文件可以安全地在任何目录创建）
if [ ! -f "prompts/project.md" ]; then
  echo "📝 prompts/project.md 不存在，创建空文件..."
  touch prompts/project.md
fi

# ============ 2. GitHub 标签检查与创建 ============

# 定义标签颜色和描述
declare -A LABEL_COLORS=(
  ["status:todo"]="ededed"
  ["status:analysis"]="fef2c0"
  ["status:accepted"]="b6e3ff"
  ["status:wip"]="d4c5f9"
  ["status:ready"]="c2e0c6"
  ["status:rejected"]="d93f0b"
  # ["status:holdon"]="fbca04"
  ["status:done"]="0e8a16"
  ["status:abort"]="666666"
  ["auto-merge"]="fbca04"
)

declare -A LABEL_DESCRIPTIONS=(
  ["status:todo"]="待处理任务，初始进入状态"
  ["status:analysis"]="正在分析中，AI 正在评估逻辑或调查 Bug"
  ["status:accepted"]="分析已完成，修改计划已被认可"
  ["status:wip"]="正在执行代码修改或修复中"
  ["status:ready"]="任务已完成，等待审核"
  ["status:rejected"]="代码审查未通过，需要修复"
  # ["status:holdon"]="任务已拆分，等待子任务完成"
  ["status:done"]="任务已完成并合并"
  ["status:abort"]="任务已取消或判定为误报"
  ["auto-merge"]="标记此 PR 为自动合并"
)

echo "🏷️  检查 GitHub 工作流标签..."

# 检查并创建每个标签
for label in "${!LABEL_COLORS[@]}"; do
  if ! gh label list --json name --jq ".[] | select(.name == \"$label\")" | grep -q "$label"; then
    echo "  📌 创建标签: $label"
    gh label create "$label" \
      --color "${LABEL_COLORS[$label]}" \
      --description "${LABEL_DESCRIPTIONS[$label]}" \
      --force 2>/dev/null || echo "    ⚠️  标签 $label 创建失败或已存在"
  fi
done

# 节点身份标签（多节点并发所有权令牌）
# OPENCODE_NODE_ID 存在则创建对应 agent:<node-id>，否则创建默认 agent:default
NODE_ID="${OPENCODE_NODE_ID:-default}"
AGENT_LABEL="agent:${NODE_ID}"
if ! gh label list --json name --jq ".[] | select(.name == \"$AGENT_LABEL\")" | grep -q "$AGENT_LABEL"; then
  echo "  📌 创建节点身份标签: $AGENT_LABEL"
  gh label create "$AGENT_LABEL" \
    --color "5319e7" \
    --description "节点所有权令牌：$NODE_ID 正在处理的 Issue" \
    --force 2>/dev/null || echo "    ⚠️  标签 $AGENT_LABEL 创建失败或已存在"
fi
if [ -z "${OPENCODE_NODE_ID:-}" ]; then
  echo "  ⚠️  OPENCODE_NODE_ID 未设置，已创建默认标签 agent:default。多节点并发时必须为每个节点设置唯一 ID（如 export OPENCODE_NODE_ID=node-1）。"
fi

# ============ 3. 文档结构检查 ============

# 确保 docs/en/blog.md 存在
if [ ! -f "docs/en/blog.md" ]; then
  echo "📝 docs/en/blog.md 不存在，创建空文件..."
  touch docs/en/blog.md
fi

echo "✅ 环境完整性检查完成！"
