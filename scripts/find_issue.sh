#!/bin/bash

# ============================================================
# find_issue.sh — multi-node safe issue finder with ownership
#
# Usage: find_issue.sh <status1> [status2]
#        find_issue.sh --issue <num> <status1> [status2]
#
# Requires OPENCODE_NODE_ID (unique per node). Claims ownership
# of the returned issue by adding an agent:<node-id> label.
#
# Output:
#   <issue-number>  — claimed/resumed issue (exit 0)
#   0               — no candidates at all (exit 0)
#   BLOCKED         — candidates exist but all owned by others (exit 2)
#
# --issue <num>: directly target a specific issue instead of auto-discovery.
#   Validates: numeric, exists, has status:<status1> or status:<status2>,
#   no other agent:* label. Claims/resumes ownership before returning.
#   On validation failure prints an error to stderr and exits 1.
#
# Ownership protocol (verify-after-write CAS):
#   1. Resume: status:<status1> + agent:<own> → return it
#   2. Claim: first candidate with no agent:* label → add agent:<own>
#      (single edit) → verify no other agent appeared → success
#      If another agent appeared (race lost) → release + try next
#   3. Fallback: status:<status2> candidates → promote (status2→status1)
#      AND claim in one atomic edit
#   4. All blocked → BLOCKED + blocked list on stderr
# ============================================================

# Node identity — mandatory
NODE_ID="${OPENCODE_NODE_ID:-}"
if [ -z "$NODE_ID" ]; then
    echo "ERROR: OPENCODE_NODE_ID is not set. Export a unique node ID, e.g.: export OPENCODE_NODE_ID=node-1" >&2
    exit 1
fi
AGENT_LABEL="agent:${NODE_ID}"

PARAM1=$1
PARAM2=$2
ISSUE_SPEC=""

# Parse --issue <num> prefix: find_issue.sh --issue 123 ready
if [ "$PARAM1" = "--issue" ]; then
    ISSUE_SPEC=$2
    PARAM1=$3
    PARAM2=$4
fi

if [ -z "$PARAM1" ]; then
    echo "Usage: $0 <status1> [status2]" >&2
    echo "       $0 --issue <num> <status1> [status2]" >&2
    exit 1
fi

# Testcase filter: TC: / [TC] / Testcase: / 测试用例 titles or type:testcase label
is_testcase() {
    local title=$1
    local labels=$2

    # Case-insensitive title prefix match
    if echo "$title" | grep -qiE '^(TC|Testcase)[:：]|^\[TC\]|^测试用例'; then
        return 0
    fi

    # Label-based match (fallback defense)
    if echo "$labels" | grep -wq 'type:testcase'; then
        return 0
    fi

    return 1
}

# List candidates for a status label as TSV (number, title, labels),
# skipping testcase-class issues
list_candidates() {
    local label=$1
    gh issue list --label "$label" --json number,title,labels \
        -q '.[] | [.number, .title, ([.labels[].name] | join(","))] | @tsv' | \
        while IFS=$'\t' read -r num title labels; do
            [ -z "$num" ] && continue
            if ! is_testcase "$title" "$labels"; then
                printf '%s\t%s\t%s\n' "$num" "$title" "$labels"
            fi
        done
}

# Extract agent labels other than our own from comma-joined labels
other_agents() {
    local labels=$1
    echo "$labels" | tr ',' '\n' | grep '^agent:' | grep -v "^${AGENT_LABEL}$" || true
}

# Current comma-joined labels of an issue
issue_labels() {
    local num=$1
    gh issue view "$num" --json labels -q '[.labels[].name] | join(",")'
}

# Verify ownership after claim: our label present, no other agent label
claim_verified() {
    local num=$1
    local current
    current=$(issue_labels "$num")
    echo "$current" | grep -wq "$AGENT_LABEL" || return 1
    [ -z "$(other_agents "$current")" ]
}

# Ensure the agent label exists (create on demand)
ensure_agent_label() {
    if ! gh label list --json name -q '.[].name' | grep -qx "$AGENT_LABEL"; then
        gh label create "$AGENT_LABEL" --color "5319e7" --description "Node ownership token: ${NODE_ID}" --force >/dev/null 2>&1 || true
    fi
}

ensure_agent_label

BLOCKED_COUNT=0
HAD_CANDIDATES=0

# ============ Phase 0: Direct issue targeting (--issue <num>) ============
if [ -n "$ISSUE_SPEC" ]; then
    # 1. Numeric validation
    if ! echo "$ISSUE_SPEC" | grep -qE '^[0-9]+$'; then
        echo "ERROR: --issue expects a numeric issue number, got: '$ISSUE_SPEC'" >&2
        exit 1
    fi

    # 2. Existence check
    if ! gh issue view "$ISSUE_SPEC" --json number -q '.number' >/dev/null 2>&1; then
        echo "ERROR: Issue #$ISSUE_SPEC does not exist" >&2
        exit 1
    fi

    # 3. Status validation: must have status:PARAM1 or status:PARAM2
    CURRENT_LABELS=$(issue_labels "$ISSUE_SPEC")
    STATUS_OK=0
    for st in "$PARAM1" "$PARAM2"; do
        [ -z "$st" ] && continue
        if echo "$CURRENT_LABELS" | tr ',' '\n' | grep -qx "status:$st"; then
            STATUS_OK=1
            break
        fi
    done
    if [ "$STATUS_OK" != "1" ]; then
        echo "ERROR: Issue #$ISSUE_SPEC does not have required status (${PARAM1}${PARAM2:+ or ${PARAM2}}). Current: $(echo "$CURRENT_LABELS" | tr ',' '\n' | grep '^status:' | tr '\n' ' ')" >&2
        exit 1
    fi

    # 4. Ownership: other agent occupied → BLOCKED
    OTHER=$(other_agents "$CURRENT_LABELS")
    if [ -n "$OTHER" ]; then
        echo "  #$ISSUE_SPEC ($OTHER)" >&2
        echo "BLOCKED"
        exit 2
    fi

    # 5. Already ours → resume
    if echo "$CURRENT_LABELS" | tr ',' '\n' | grep -qx "$AGENT_LABEL"; then
        echo "$ISSUE_SPEC"
        exit 0
    fi

    # 6. Free → atomic claim + verify
    if gh issue edit "$ISSUE_SPEC" --add-label "$AGENT_LABEL" >/dev/null 2>&1; then
        if claim_verified "$ISSUE_SPEC"; then
            echo "$ISSUE_SPEC"
            exit 0
        fi
        # Race lost: another agent appeared
        echo "ERROR: Issue #$ISSUE_SPEC claim race lost (another agent appeared)" >&2
        exit 2
    fi

    echo "ERROR: Failed to claim issue #$ISSUE_SPEC" >&2
    exit 1
fi

# ============ Phase 1: Resume own issue (status1 + agent:own) ============
while IFS=$'\t' read -r num title labels; do
    [ -z "$num" ] && continue
    if echo "$labels" | grep -wq "$AGENT_LABEL"; then
        echo "$num"
        exit 0
    fi
done <<< "$(list_candidates "status:$PARAM1")"

# ============ Phase 2: Claim free issue in status1 ============
while IFS=$'\t' read -r num title labels; do
    [ -z "$num" ] && continue
    HAD_CANDIDATES=1
    OTHER=$(other_agents "$labels")
    if [ -n "$OTHER" ]; then
        echo "  #$num ($OTHER)" >&2
        BLOCKED_COUNT=$((BLOCKED_COUNT + 1))
        continue
    fi
    # Free candidate → atomic claim (single edit adds agent label)
    if gh issue edit "$num" --add-label "$AGENT_LABEL" >/dev/null 2>&1; then
        if claim_verified "$num"; then
            echo "$num"
            exit 0
        fi
        # Race lost: another agent appeared — release and continue
        gh issue edit "$num" --remove-label "$AGENT_LABEL" >/dev/null 2>&1 || true
    fi
done <<< "$(list_candidates "status:$PARAM1")"

# ============ Phase 3: Fallback — promote + claim from status2 ============
if [ -n "$PARAM2" ]; then
    while IFS=$'\t' read -r num title labels; do
        [ -z "$num" ] && continue
        HAD_CANDIDATES=1
        OTHER=$(other_agents "$labels")
        if [ -n "$OTHER" ]; then
            echo "  #$num ($OTHER)" >&2
            BLOCKED_COUNT=$((BLOCKED_COUNT + 1))
            continue
        fi
        # Atomic promote + claim in a single edit
        if gh issue edit "$num" --remove-label "status:$PARAM2" \
            --add-label "status:$PARAM1" --add-label "$AGENT_LABEL" >/dev/null 2>&1; then
            if claim_verified "$num"; then
                echo "$num"
                exit 0
            fi
            # Race lost — release agent label and continue
            gh issue edit "$num" --remove-label "$AGENT_LABEL" >/dev/null 2>&1 || true
        fi
    done <<< "$(list_candidates "status:$PARAM2")"
fi

# ============ Verdict ============
if [ "$HAD_CANDIDATES" = "1" ]; then
    echo "BLOCKED"
    exit 2
fi

echo "0"
