#!/bin/bash
# OpenCode GitHub 工作流：基于状态机的异步处理脚本

# Helper function to check if a label exists on an issue
label_exists() {
  local issue_num=$1
  local label=$2
  gh issue view "$issue_num" --json labels -q ".labels[].name" 2>/dev/null | grep -q "^${label}$"
}

# Helper function to verify label was added
verify_label_added() {
  local issue_num=$1
  local label=$2
  local max_attempts=3
  local attempt=1
  
  while [ $attempt -le $max_attempts ]; do
    if label_exists "$issue_num" "$label"; then
      return 0
    fi
    sleep 1
    attempt=$((attempt + 1))
  done
  
  echo "ERROR: Failed to verify label '$label' was added to issue #$issue_num after $max_attempts attempts"
  return 1
}

# Helper function to verify label was removed
verify_label_removed() {
  local issue_num=$1
  local label=$2
  local max_attempts=3
  local attempt=1
  
  while [ $attempt -le $max_attempts ]; do
    if ! label_exists "$issue_num" "$label"; then
      return 0
    fi
    sleep 1
    attempt=$((attempt + 1))
  done
  
  echo "ERROR: Failed to verify label '$label' was removed from issue #$issue_num after $max_attempts attempts"
  return 1
}

# Helper function to safely add a label with verification
add_label() {
  local issue_num=$1
  local label=$2
  
  if gh issue edit "$issue_num" --add-label "$label"; then
    if verify_label_added "$issue_num" "$label"; then
      return 0
    else
      return 1
    fi
  else
    echo "ERROR: Failed to add label '$label' to issue #$issue_num"
    return 1
  fi
}

# Helper function to safely remove a label with verification
remove_label() {
  local issue_num=$1
  local label=$2
  
  if gh issue edit "$issue_num" --remove-label "$label" 2>/dev/null; then
    if verify_label_removed "$issue_num" "$label"; then
      return 0
    else
      return 1
    fi
  else
    # Label might not exist, which is OK for removal
    if ! label_exists "$issue_num" "$label"; then
      return 0
    fi
    echo "ERROR: Failed to remove label '$label' from issue #$issue_num"
    return 1
  fi
}

# Helper function to replace a label (remove old, add new)
replace_label() {
  local issue_num=$1
  local old_label=$2
  local new_label=$3
  
  local success=true
  
  # Remove old label if it exists
  if label_exists "$issue_num" "$old_label"; then
    if ! remove_label "$issue_num" "$old_label"; then
      success=false
    fi
  fi
  
  # Add new label
  if ! add_label "$issue_num" "$new_label"; then
    success=false
  fi
  
  if [ "$success" = true ]; then
    return 0
  else
    return 1
  fi
}

# Release the node ownership label from an issue (phase transition completion).
# Only removes the current node's own agent label; never touches other nodes.
release_agent_label() {
  local issue_num=$1
  local agent_label="agent:${OPENCODE_NODE_ID:-}"

  if [ -z "${OPENCODE_NODE_ID:-}" ] && [ -f "$HOME/.config/opencode/node-id" ]; then
    agent_label="agent:$(cat "$HOME/.config/opencode/node-id")"
  fi

  if [ "$agent_label" = "agent:" ]; then
    return 0
  fi

  if label_exists "$issue_num" "$agent_label"; then
    if ! remove_label "$issue_num" "$agent_label"; then
      echo "WARNING: Failed to remove agent label '$agent_label' from issue #$issue_num"
      return 1
    fi
  fi
  return 0
}

# Remove ALL agent:* ownership labels from an issue (terminal states only).
remove_all_agent_labels() {
  local issue_num=$1
  local labels
  labels=$(gh issue view "$issue_num" --json labels -q '.labels[].name' 2>/dev/null || true)
  local failed=""
  for lbl in $labels; do
    if echo "$lbl" | grep -q '^agent:'; then
      if ! remove_label "$issue_num" "$lbl"; then
        failed="$failed $lbl"
      fi
    fi
  done
  if [ -n "$failed" ]; then
    echo "WARNING: Failed to remove agent labels:$failed"
    return 1
  fi
  return 0
}

# Check if an issue is a testcase-class issue that must be excluded from the pipeline
# Returns 0 (true) on title prefix match (TC:, [TC], Testcase:, 测试用例) or type:testcase label
is_testcase() {
  local title=$1
  local labels=$2

  if echo "$title" | grep -qiE '^(TC|Testcase)[:：]|^\[TC\]|^测试用例'; then
    return 0
  fi

  if echo "$labels" | grep -qw 'type:testcase'; then
    return 0
  fi

  return 1
}

# List all status:todo issues, skipping testcase-class issues
# Prints issue numbers, one per line
list_todo_issues() {
  gh issue list --label "status:todo" --json number,title,labels -q '.[] | [.number, .title, ([.labels[].name] | join(","))] | @tsv' | \
    while IFS=$'\t' read -r num title labels; do
      [ -z "$num" ] && continue
      if ! is_testcase "$title" "$labels"; then
        echo "$num"
      fi
    done
}

COMMAND=$1
ISSUE_NUM=$2
EXTRA_MSG=$3

case $COMMAND in
  "init")
    # /todo: 创建任务
    # Usage: init --title "Title" <temp_file>
    if [ "$2" != "--title" ]; then
      echo "ERROR:--title is required. Usage: init --title \"Title\" <temp_file>"
      exit 1
    fi
    SHORT_TITLE="$3"
    TEMP_FILE="$4"
    if [ -z "$SHORT_TITLE" ] || [ -z "$TEMP_FILE" ]; then
      echo "ERROR:Both --title and <temp_file> must be provided"
      exit 1
    fi

    NEW_ID=$(gh issue create --title "[Task] $SHORT_TITLE" --label "status:todo" --body-file "$TEMP_FILE" | grep -oE "[0-9]+$")
    rm -f "$TEMP_FILE"
    
    if [ -n "$NEW_ID" ] && label_exists "$NEW_ID" "status:todo"; then
      echo "SUCCESS:ISSUE_ID:$NEW_ID"
    else
      echo "ERROR:Failed to create issue or add label"
      exit 1
    fi
    ;;

  "record")
    # /record: 将已完成的工作回溯记录为 Issue，直接标记 status:ready
    # Usage: record --title "Title" <temp_file>
    if [ "$2" != "--title" ]; then
      echo "ERROR:--title is required. Usage: record --title \"Title\" <temp_file>"
      exit 1
    fi
    SHORT_TITLE="$3"
    TEMP_FILE="$4"
    if [ -z "$SHORT_TITLE" ] || [ -z "$TEMP_FILE" ]; then
      echo "ERROR:Both --title and <temp_file> must be provided"
      exit 1
    fi

    NEW_ID=$(gh issue create --title "[Record] $SHORT_TITLE" --label "status:ready" --body-file "$TEMP_FILE" | grep -oE "[0-9]+$")
    rm -f "$TEMP_FILE"

    if [ -n "$NEW_ID" ] && label_exists "$NEW_ID" "status:ready"; then
      echo "SUCCESS:ISSUE_ID:$NEW_ID"
    else
      echo "ERROR:Failed to create issue or add label"
      exit 1
    fi
    ;;

  "analyze-start")
    # /analyze: DEPRECATED in multi-node protocol
    # Batch flipping all todos to analysis breaks node ownership.
    # find_issue.sh claims issues atomically per node now.
    echo "DEPRECATED: analyze-start batch flip is removed (multi-node ownership protocol)."
    echo "Use find_issue.sh analysis todo instead — it claims issues atomically per node."
    exit 0
    ;;

  "report-start")
    # /report: 获取所有待报表任务并切换状态，不创建分支
    ISSUES=$(list_todo_issues)
    for ID in $ISSUES; do
      if ! replace_label "$ID" "status:todo" "status:analysis"; then
        echo "ERROR:Failed to replace label for issue #$ID"
        exit 1
      fi
    done
    echo "SUCCESS:REPORT_STARTED:ON_MAIN_BRANCH"
    ;;

  "accept")
    # /analyze: Record plan and accept task
    # $3 contains the temp file path with plan content
    gh issue comment $ISSUE_NUM --body-file "$3"
    if ! replace_label "$ISSUE_NUM" "status:analysis" "status:accepted"; then
      echo "ERROR:Failed to replace label for issue #$ISSUE_NUM"
      rm -f "$3"
      exit 1
    fi
    # Release node ownership: analysis phase complete
    release_agent_label "$ISSUE_NUM"
    rm -f "$3"
    ;;

  "abort")
    # /analyze: 放弃任务 (Not Planned)
    # $3 contains the temp file path with abort reason
    gh issue comment $ISSUE_NUM --body-file "$3"
    if ! replace_label "$ISSUE_NUM" "status:analysis" "status:abort"; then
      echo "ERROR:Failed to replace label for issue #$ISSUE_NUM"
      rm -f "$3"
      exit 1
    fi
    # Terminal state: strip ALL ownership tokens (not just this node's)
    remove_all_agent_labels "$ISSUE_NUM" || true
    gh issue close $ISSUE_NUM --reason "not planned"
    rm -f "$3"
    ;;

  "work-start")
    # 检查是否已有 wip 任务，只取第一个
    WIP_ISSUE=$(gh issue list --label "status:wip" --json number -q '.[0].number')
    if [ -n "$WIP_ISSUE" ]; then
      echo "SUCCESS:WIP:$WIP_ISSUE"
    else
      # 如果没有 wip，找一个 accepted 任务并转换为 wip
      ACCEPTED_ISSUE=$(gh issue list --label "status:accepted" --json number -q '.[0].number')
      if [ -z "$ACCEPTED_ISSUE" ]; then
        echo "INFO:NO_ACCEPTED_ISSUES"
      else
        if ! replace_label "$ACCEPTED_ISSUE" "status:accepted" "status:wip"; then
          echo "ERROR:Failed to replace label for issue #$ACCEPTED_ISSUE"
          exit 1
        fi
        echo "SUCCESS:WIP:$ACCEPTED_ISSUE"
      fi
    fi
    ;;

  "work-finish")
    TEMP_FILE=$(mktemp)
    ISSUE=$(gh issue list --label "status:wip" --json number -q '.[0].number')
    if [ -z "$ISSUE" ]; then
      echo "INFO:NO_WIP_ISSUES"
    else
      HASH=$(git log -1 --format='%H')
      echo "Commit: $HASH" > "$TEMP_FILE"
      echo "Issue: #$ISSUE" >> "$TEMP_FILE"
      gh issue comment $ISSUE --body-file "$TEMP_FILE"
      if ! replace_label "$ISSUE" "status:wip" "status:ready"; then
        echo "ERROR:Failed to replace label for issue #$ISSUE"
        rm -f "$TEMP_FILE"
        exit 1
      fi
      # Release node ownership: wip phase complete
      release_agent_label "$ISSUE"
      rm -f "$TEMP_FILE"
      echo "SUCCESS:READY:$ISSUE"
    fi
    ;;

  "done")
    # /review pass: 标记为完成
    # $3 contains the temp file path with completion message
    gh issue comment $ISSUE_NUM --body-file "$3"
    
    # Add done label
    if ! add_label "$ISSUE_NUM" "status:done"; then
      echo "ERROR:Failed to add 'status:done' label to issue #$ISSUE_NUM"
      rm -f "$3"
      exit 1
    fi
    
    # Remove other status labels (continue on individual failures but report them)
    errors=""
    for label in "status:ready" "status:wip" "status:analysis" "status:accepted" "status:todo"; do
      if label_exists "$ISSUE_NUM" "$label" && ! remove_label "$ISSUE_NUM" "$label"; then
        errors="$errors $label"
      fi
    done
    
    if [ -n "$errors" ]; then
      echo "WARNING:Failed to remove some labels:$errors"
    fi
    
    # Terminal state: strip ALL ownership tokens (not just this node's)
    remove_all_agent_labels "$ISSUE_NUM" || true
    
    gh issue close $ISSUE_NUM --reason "completed"
    rm -f "$3"
    
    # 检查是否有父任务
    ISSUE_BODY=$(gh issue view $ISSUE_NUM --json body -q '.body')
    PARENT_MATCH=$(echo "$ISSUE_BODY" | grep -oE 'Parent: #[0-9]+' | grep -oE '[0-9]+$')
    
    if [ -n "$PARENT_MATCH" ]; then
      echo "INFO:FOUND_PARENT:$PARENT_MATCH"
      
      # 获取父任务的所有子任务
      CHILD_ISSUES=$(gh issue list --search "Parent: #$PARENT_MATCH" --json number,labels -q '.[] | select(.labels | any(.name == "status:done") | not) | .number')
      
      if [ -z "$CHILD_ISSUES" ]; then
        echo "INFO:ALL_CHILDREN_DONE:$PARENT_MATCH"
        
        # 所有子任务都已完成，自动完成父任务
        COMPLETION_TEMP=$(mktemp)
        echo "## Parent Task Auto-Completion" > "$COMPLETION_TEMP"
        echo "" >> "$COMPLETION_TEMP"
        echo "所有子任务已完成，自动关闭父任务。" >> "$COMPLETION_TEMP"
        echo "" >> "$COMPLETION_TEMP"
        echo "---" >> "$COMPLETION_TEMP"
        echo "*Model: auto-processor*" >> "$COMPLETION_TEMP"
        
        gh issue comment $PARENT_MATCH --body-file "$COMPLETION_TEMP"
        if add_label "$PARENT_MATCH" "status:done"; then
          remove_label "$PARENT_MATCH" "status:holdon" 2>/dev/null || true
        fi
        gh issue close $PARENT_MATCH --reason "completed"
        
        rm -f "$COMPLETION_TEMP"
        echo "SUCCESS:PARENT_DONE:$PARENT_MATCH"
      else
        echo "INFO:PENDING_CHILDREN:$PARENT_MATCH"
      fi
    fi
    ;;

  "final-submit")
    # /review end: Update blog on main branch
    # 完成消息来自 $2（review.md 约定 final-submit "Resolved Issues: #123"）
    extra_msg=$2
    if [ -z "$extra_msg" ]; then
      echo "ERROR: final-submit requires a completion message in \$2"
      echo "Usage: opencode_gh.sh final-submit \"Resolved Issues: #123, #125\""
      exit 1
    fi

    CURRENT_BRANCH=$(git branch --show-current)
    if [ "$CURRENT_BRANCH" != "main" ]; then
      echo "ERROR: final-submit must run on main branch (current: $CURRENT_BRANCH)"
      exit 1
    fi

    # 只读核对远端进度（失败静默不阻塞，不自动 merge、不重试）
    git fetch origin main >/dev/null 2>&1 || true

    # Update blog.md
    mkdir -p docs
    echo "- $(date +%F): 批量处理完成，包含任务: $extra_msg" >> docs/blog.md
    git add docs/blog.md
    git commit -m "docs: update blog for batch completion

Model: moonshotai-cn/kimi-k2-thinking"
    git push origin main
    
    echo "SUCCESS:FINAL_SUBMIT_ON_MAIN"
    ;;

  "reopen")
    # /reopen: Reopen closed Issue
    # $2 = Issue number, $3 = temp file with comment body
    ISSUE_NUM=$2
    TEMP_FILE=$3

    # Reopen the Issue
    gh issue reopen $ISSUE_NUM

    # Remove all status:xxx labels
    errors=""
    for label in "status:todo" "status:analysis" "status:accepted" "status:wip" "status:ready" "status:done" "status:abort"; do
      if label_exists "$ISSUE_NUM" "$label" && ! remove_label "$ISSUE_NUM" "$label"; then
        errors="$errors $label"
      fi
    done
    
    if [ -n "$errors" ]; then
      echo "WARNING:Failed to remove some labels:$errors"
    fi

    # Re-entering the queue: strip ALL ownership tokens
    remove_all_agent_labels "$ISSUE_NUM" || true

    # Add status:todo label
    if ! add_label "$ISSUE_NUM" "status:todo"; then
      echo "ERROR:Failed to add 'status:todo' label to issue #$ISSUE_NUM"
      rm -f "$TEMP_FILE"
      exit 1
    fi

    # Add description as comment
    gh issue comment $ISSUE_NUM --body-file "$TEMP_FILE"
    rm -f "$TEMP_FILE"

    echo "SUCCESS:REOPENED:ISSUE:$ISSUE_NUM"
    ;;

  "split")
    # /split: 自动扫描并分拆需要拆分的任务（无参数调用）
    
    TOTAL_PARENTS=0
    TOTAL_SUBTASKS=0
    
    while true; do
      # 查找所有可能包含分拆指令的任务（状态：wip, analysis, accepted）
      FOUND_TASKS=""
      
      for STATUS in "status:wip" "status:analysis" "status:accepted"; do
        TASKS=$(gh issue list --label "$STATUS" --json number -q '.[].number')
        if [ -n "$TASKS" ]; then
          FOUND_TASKS="$FOUND_TASKS $TASKS"
        fi
      done
      
      # 去除首尾空格
      FOUND_TASKS=$(echo "$FOUND_TASKS" | sed 's/^ *//' | sed 's/ *$//')
      
      # 如果没有任务，退出循环
      if [ -z "$FOUND_TASKS" ]; then
        break
      fi
      
      # 标记本轮是否有任务被分拆
      SPLIT_PERFORMED=false
      
      # 遍历每个任务
      for TASK_ID in $FOUND_TASKS; do
        # 获取最新评论（如果有评论）
        LATEST_COMMENT=$(gh issue view $TASK_ID --json comments -q '.comments[-1].body' 2>/dev/null || echo "")
        
        # 检查是否包含分拆指令（关键词匹配，不区分大小写）
        if [ -n "$LATEST_COMMENT" ] && echo "$LATEST_COMMENT" | grep -qiE "(建议拆分|任务过大|需要拆分|/split|拆分任务)"; then
          # 从评论中提取拆分方案（以 "- " 或 "1." 开头的行）
          SPLIT_PLAN=$(echo "$LATEST_COMMENT" | grep -E "^([\-\*]|([0-9]+\.))" | sed 's/^[[:space:]]*[\-\*0-9\.][[:space:]]*//')
          
          if [ -n "$SPLIT_PLAN" ]; then
            # 创建临时文件存储拆分方案
            SPLIT_TEMP=$(mktemp)
            echo "$SPLIT_PLAN" > "$SPLIT_TEMP"
            
            # 创建临时文件用于存储子任务列表（复用原有逻辑）
            SUMMARY_TEMP=$(mktemp)
            echo "## Split Summary" > "$SUMMARY_TEMP"
            echo "" >> "$SUMMARY_TEMP"
            echo "此任务已被拆分为以下子任务：" >> "$SUMMARY_TEMP"
            echo "" >> "$SUMMARY_TEMP"
            
            # 遍历拆分方案的每一行
            SUBTASK_NUM=1
            while IFS= read -r line; do
              # 跳过空行
              if [ -z "$line" ]; then
                continue
              fi
              
              # 创建子任务临时文件
              SUBTASK_TEMP=$(mktemp)
              echo "$line" > "$SUBTASK_TEMP"
              echo "" >> "$SUBTASK_TEMP"
              echo "Parent: #$TASK_ID" >> "$SUBTASK_TEMP"
              
              # 创建子任务
              SUBTASK_TITLE="[Sub-task of #$TASK_ID] $line"
              SUBTASK_ID=$(gh issue create --title "$SUBTASK_TITLE" --label "status:todo" --body-file "$SUBTASK_TEMP" | grep -oE "[0-9]+$")
              
              # Verify subtask label
              if [ -n "$SUBTASK_ID" ] && label_exists "$SUBTASK_ID" "status:todo"; then
                # 添加到汇总
                echo "- #$SUBTASK_ID: $line" >> "$SUMMARY_TEMP"
              else
                echo "ERROR:Failed to create subtask or verify label for task #$TASK_ID"
                rm -f "$SUBTASK_TEMP"
                rm -f "$SUMMARY_TEMP"
                rm -f "$SPLIT_TEMP"
                exit 1
              fi
              
              rm -f "$SUBTASK_TEMP"
              SUBTASK_NUM=$((SUBTASK_NUM + 1))
              TOTAL_SUBTASKS=$((TOTAL_SUBTASKS + 1))
            done < "$SPLIT_TEMP"
            
            # 在父任务中添加评论
            gh issue comment "$TASK_ID" --body-file "$SUMMARY_TEMP"
            
            # 将父任务状态改为 holdon（根据当前状态移除相应标签）
            status_changed=false
            for old_status in "status:wip" "status:analysis" "status:accepted"; do
              if label_exists "$TASK_ID" "$old_status" && replace_label "$TASK_ID" "$old_status" "status:holdon"; then
                status_changed=true
                break
              fi
            done
            
            if [ "$status_changed" = false ]; then
              # If no status label was replaced, just add holdon
              add_label "$TASK_ID" "status:holdon" 2>/dev/null || true
            fi

            # Parent is now holdon (this phase is terminal): release node ownership
            release_agent_label "$TASK_ID" || true
            
            rm -f "$SUMMARY_TEMP"
            rm -f "$SPLIT_TEMP"
            
            SPLIT_PERFORMED=true
            TOTAL_PARENTS=$((TOTAL_PARENTS + 1))
          fi
        fi
      done
      
      # 如果本轮没有任务被分拆，退出循环
      if [ "$SPLIT_PERFORMED" = false ]; then
        break
      fi
    done
    
    echo "SPLIT_COMPLETE: Parents=$TOTAL_PARENTS, Subtasks=$TOTAL_SUBTASKS"
    ;;

  *)
    echo "Unknown command: $COMMAND"
    exit 1
    ;;
esac
