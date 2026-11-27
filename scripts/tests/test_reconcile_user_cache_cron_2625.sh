#!/usr/bin/env bash
# #2625 证据/回归测试：例行任务安装器 + 运行器（无差异静默 / 有差异建单 / 连续 2 天升级 / 失败告警）。
#
# 全离线：fake gh（记录调用）、fake reconcile（按场景输出 token）、沙箱 HOME + XDG_STATE_HOME、
# fake systemctl / crontab。绝不写真实 ~/.config/systemd、真实 crontab，绝不发真实 Issue。
#
# 用法：
#   bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r1   # 安装器（print/apply/uninstall/幂等/unit 硬约束）
#   bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r2   # 运行器分支（无差异静默 / 有差异建单 / 同日去重）
#   bash scripts/tests/test_reconcile_user_cache_cron_2625.sh r3   # 连续 2 天升级 + 对账失败必须告警 + 缺 env
set -uo pipefail

REPO="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO" || exit 2

ISSUE=2625
INSTALLER="scripts/install-user-cache-reconcile-timer.sh"
RUNNER="scripts/reconcile-user-cache-cron.sh"
ROW="${1:-}"

# ---------------------------------------------------------------- provenance
HEAD_SHA="$(git rev-parse HEAD)"
FIX_REF="${FIX_COMMIT:-$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)}"
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}" 2>/dev/null || git rev-parse HEAD)"
if ! git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" 2>/dev/null; then
  echo "FATAL: FIX_COMMIT is not an ancestor of HEAD" >&2
  exit 3
fi
FIX_SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)docs/evidence/$ISSUE" ':(exclude)tmp')" ]; then
  DIRTY=true
fi
echo "# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
echo "GENERATED_AT_HEAD=$HEAD_SHA"

# ------------------------------------------------------------------- helpers
TMP="$(mktemp -d "${TMPDIR:-$REPO/tmp}/reconcile-cron-$ISSUE.XXXXXX")" || exit 2
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

FAILED=0
expect() { # name expected actual
  local name="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    printf '%s=%s\n' "$name" "$actual"
  else
    printf '%s=MISMATCH(expected=%s actual=%s)\n' "$name" "$expected" "$actual"
    FAILED=1
  fi
}
at_least() { # name expected_min actual
  local name="$1" min="$2" actual="$3"
  case "${actual:-x}" in
    ''|*[!0-9]*) printf '%s=MISMATCH(expected>=%s actual=%s)\n' "$name" "$min" "$actual"; FAILED=1; return 0 ;;
  esac
  if [ "$actual" -ge "$min" ]; then
    printf '%s=%s\n' "$name" "$actual"
  else
    printf '%s=MISMATCH(expected>=%s actual=%s)\n' "$name" "$min" "$actual"
    FAILED=1
  fi
}
tok() { # file name
  grep -E "^$2=" "$1" 2>/dev/null | head -n 1 | cut -d= -f2-
}
finish() {
  if [ "$FAILED" = "0" ]; then
    echo "TEST_EXIT=0"
    echo "RESULT: PASS"
    exit 0
  fi
  echo "TEST_EXIT=1"
  echo "RESULT: FAIL"
  exit 1
}

mkdir -p "$TMP/fakebin" "$TMP/home" "$TMP/state"
SANDBOX_HOME="$TMP/home"
SANDBOX_STATE="$TMP/state"
GH_LOG="$TMP/gh.log"
GH_BODY_DIR="$TMP/gh-bodies"
CRON_FILE="$TMP/crontab.txt"
mkdir -p "$GH_BODY_DIR"
: > "$GH_LOG"
: > "$CRON_FILE"

cat > "$TMP/fakebin/gh" <<'FAKEGH'
#!/usr/bin/env bash
# fake gh：记录每次调用；list 结果由 FAKE_GH_LIST_JSON 控制；create/comment 返回固定 URL。
# FAKE_GH_CREATE_RC / FAKE_GH_COMMENT_RC 非 0 时模拟调用失败（401/403），用于负例。
printf '%s\n' "$*" >> "$GH_LOG"
cmd="${1:-}"; sub="${2:-}"
case "$cmd $sub" in
  "issue list")
    printf '%s\n' "${FAKE_GH_LIST_JSON:-[]}"
    exit 0
    ;;
  "issue create")
    body=""
    prev=""
    for a in "$@"; do
      [ "$prev" = "--body-file" ] && body="$a"
      prev="$a"
    done
    if [ -n "$body" ] && [ -d "$GH_BODY_DIR" ]; then
      cp "$body" "$GH_BODY_DIR/create-$(date +%s%N).md" 2>/dev/null || true
    fi
    if [ "${FAKE_GH_CREATE_RC:-0}" != "0" ]; then
      echo "HTTP 401: Bad credentials (fake)" >&2
      exit "${FAKE_GH_CREATE_RC}"
    fi
    printf 'https://github.com/fake/repo/issues/%s\n' "${FAKE_GH_CREATE_NUMBER:-777}"
    exit 0
    ;;
  "issue comment")
    body=""
    prev=""
    for a in "$@"; do
      [ "$prev" = "--body-file" ] && body="$a"
      prev="$a"
    done
    if [ -n "$body" ] && [ -d "$GH_BODY_DIR" ]; then
      cp "$body" "$GH_BODY_DIR/comment-$(date +%s%N).md" 2>/dev/null || true
    fi
    if [ "${FAKE_GH_COMMENT_RC:-0}" != "0" ]; then
      echo "HTTP 403: Resource not accessible (fake)" >&2
      exit "${FAKE_GH_COMMENT_RC}"
    fi
    exit 0
    ;;
esac
exit 0
FAKEGH
chmod +x "$TMP/fakebin/gh"

cat > "$TMP/fakebin/systemctl" <<'FAKESYSTEMCTL'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${SYSTEMCTL_LOG:-/dev/null}"
for a in "$@"; do
  if [ "$a" = "enable" ]; then
    if [ "${FAKE_SYSTEMCTL_ENABLE_RC:-0}" != "0" ]; then
      echo "Failed to enable unit: fake failure" >&2
      exit "${FAKE_SYSTEMCTL_ENABLE_RC}"
    fi
    exit 0
  fi
done
case "$*" in
  *show-environment*) exit 0 ;;
  *--property=NextElapseUSecRealtime*) echo "Fri 2026-11-28 03:30:00 CST"; exit 0 ;;
  *) exit 0 ;;
esac
FAKESYSTEMCTL
chmod +x "$TMP/fakebin/systemctl"

cat > "$TMP/fakebin/crontab" <<'FAKECRON'
#!/usr/bin/env bash
if [ "${1:-}" = "-l" ]; then
  [ -f "$CRON_FILE" ] && cat "$CRON_FILE"
  exit 0
fi
if [ -n "${1:-}" ] && [ -f "$1" ]; then
  cp "$1" "$CRON_FILE"
  exit 0
fi
exit 2
FAKECRON
chmod +x "$TMP/fakebin/crontab"

cat > "$TMP/fake-reconcile.sh" <<'FAKERECON'
#!/usr/bin/env bash
# fake 对账：按 FAKE_RECONCILE_MODE 输出 JSON token 或失败。
mode="${FAKE_RECONCILE_MODE:-zero}"
case "$mode" in
  fail)
    echo "RESULT: FAIL (fake unreachable db)" >&2
    echo "RESULT: FAIL (fake unreachable db)"
    exit 2
    ;;
esac
zero_tokens='{"RECONCILE_DATE":"2026-11-28","RECONCILE_SOURCE":"fixture","USERS_LOCAL":"10","USERS_IAM_ACTIVE":"10","TENANT_MISMATCH":"0","ORG_MISMATCH":"0","NIL_UUID":"0","MISSING_LOCAL":"0","ORPHAN_LOCAL":"0","ROLE_MISMATCH":"0","ROLE_UNVERIFIED":"0","CONTEXT_AMBIGUOUS":"0","MEMBER_ONLY":"0","IAM_MISSING":"0","ORG_UNRESOLVED":"0","ROLE_REPAIR_SUPPORTED":"0","DB_WRITES":"0"}'
diff_tokens='{"RECONCILE_DATE":"2026-11-28","RECONCILE_SOURCE":"fixture","USERS_LOCAL":"10","USERS_IAM_ACTIVE":"10","TENANT_MISMATCH":"2","ORG_MISMATCH":"1","NIL_UUID":"0","MISSING_LOCAL":"0","ORPHAN_LOCAL":"0","ROLE_MISMATCH":"0","ROLE_UNVERIFIED":"0","CONTEXT_AMBIGUOUS":"0","MEMBER_ONLY":"0","IAM_MISSING":"0","ORG_UNRESOLVED":"0","ROLE_REPAIR_SUPPORTED":"0","DB_WRITES":"0"}'
case "$mode" in
  zero)
    jq -n --argjson t "$zero_tokens" '{tokens:$t, rows:[]}'
    ;;
  diff3|diff3_decline)
    jq -n --argjson t "$diff_tokens" '{tokens:$t, rows:[
      {iam_sub:"u-a",category:"TENANT_MISMATCH",local:"tenant=P",iam:"tenant=R",email:"a@x"},
      {iam_sub:"u-b",category:"TENANT_MISMATCH",local:"tenant=P",iam:"tenant=R",email:"b@x"},
      {iam_sub:"u-c",category:"ORG_MISMATCH",local:"org=X",iam:"org=Y",email:"c@x"}]}'
    ;;
  *)
    echo "unknown fake mode" >&2
    exit 2
    ;;
esac
FAKERECON
chmod +x "$TMP/fake-reconcile.sh"

run_installer() { # outfile args...
  local out="$1"; shift
  HOME="$SANDBOX_HOME" XDG_STATE_HOME="$SANDBOX_STATE" \
  PATH="$TMP/fakebin:$PATH" SYSTEMCTL_LOG="$TMP/systemctl.log" CRON_FILE="$CRON_FILE" \
  FAKE_SYSTEMCTL_ENABLE_RC="${FAKE_SYSTEMCTL_ENABLE_RC:-0}" \
  bash "$INSTALLER" "$@" > "$out" 2>&1
  printf '%s' "$?"
}

run_runner() { # outfile args...
  local out="$1"; shift
  HOME="$SANDBOX_HOME" XDG_STATE_HOME="$SANDBOX_STATE" \
  PATH="$TMP/fakebin:$PATH" GH_LOG="$GH_LOG" GH_BODY_DIR="$GH_BODY_DIR" CRON_FILE="$CRON_FILE" \
  GH_BIN="$TMP/fakebin/gh" RECONCILE_BIN="$TMP/fake-reconcile.sh" \
  BEACONIAM_DB_NAME="${RUN_BEACONIAM_DB_NAME-beaconiam_x}" \
  TUNELOOP_DB_NAME="${RUN_TUNELOOP_DB_NAME-tuneloop_x}" \
  FAKE_RECONCILE_MODE="${FAKE_RECONCILE_MODE:-zero}" \
  FAKE_GH_LIST_JSON="${FAKE_GH_LIST_JSON:-[]}" \
  FAKE_GH_CREATE_NUMBER="${FAKE_GH_CREATE_NUMBER:-777}" \
  FAKE_GH_CREATE_RC="${FAKE_GH_CREATE_RC:-0}" \
  FAKE_GH_COMMENT_RC="${FAKE_GH_COMMENT_RC:-0}" \
  bash "$RUNNER" "$@" > "$out" 2>&1
  printf '%s' "$?"
}

case "$ROW" in
r1)
  # 1) --print 不得写文件
  P_OUT="$TMP/r1-print.txt"
  P_RC="$(run_installer "$P_OUT" --print --backend systemd)"
  sed -n '1,40p' "$P_OUT"
  expect PRINT_EXIT 0 "$P_RC"
  expect PRINT_FILES_WRITTEN 0 "$(tok "$P_OUT" FILES_WRITTEN)"
  expect PRINT_WROTE_UNIT 0 "$(ls "$SANDBOX_HOME/.config/systemd/user" 2>/dev/null | wc -l | tr -d ' ')"
  expect PRINT_ONCALENDAR 1 "$(grep -c 'OnCalendar=\*-\*-\* 03:30:00' "$P_OUT")"

  # 2) --apply --backend systemd（fake systemctl）
  A_OUT="$TMP/r1-apply.txt"
  A_RC="$(run_installer "$A_OUT" --apply --backend systemd)"
  sed -n '1,20p' "$A_OUT"
  expect APPLY_EXIT 0 "$A_RC"
  at_least APPLY_FILES_WRITTEN 1 "$(tok "$A_OUT" FILES_WRITTEN)"
  SERVICE="$SANDBOX_HOME/.config/systemd/user/opencode-user-cache-reconcile.service"
  TIMER="$SANDBOX_HOME/.config/systemd/user/opencode-user-cache-reconcile.timer"
  expect UNIT_SERVICE_EXISTS 1 "$([ -f "$SERVICE" ] && echo 1 || echo 0)"
  expect UNIT_TIMER_EXISTS 1 "$([ -f "$TIMER" ] && echo 1 || echo 0)"
  expect UNIT_ONCALENDAR_0330 1 "$(grep -c 'OnCalendar=\*-\*-\* 03:30:00' "$TIMER")"
  expect UNIT_PERSISTENT 1 "$(grep -c '^Persistent=true$' "$TIMER")"
  expect UNIT_EXEC_PATH 1 "$(grep -c "ExecStart=$REPO/scripts/reconcile-user-cache-cron.sh" "$SERVICE")"
  expect UNIT_EXEC_READONLY 1 "$( [ "$(grep -c -- '--apply' "$SERVICE")" = "0" ] && echo 1 || echo 0)"
  expect UNIT_ENVFILE 1 "$(grep -c '^EnvironmentFile=%h/.config/tuneloop/reconcile.env$' "$SERVICE")"
  expect UNIT_NO_SECRET 1 "$( [ "$(grep -Eic 'PASSWORD|SECRET|TOKEN' "$SERVICE")" = "0" ] && echo 1 || echo 0)"
  expect UNIT_LOG_APPEND 1 "$(grep -c "^StandardOutput=append:$SANDBOX_STATE/tuneloop/user-cache-reconcile.log" "$SERVICE")"
  expect APPLY_TIMER_ENABLED 1 "$(tok "$A_OUT" TIMER_ENABLED)"

  # 3) 幂等：重复 --apply 内容不变
  SERVICE_SHA_BEFORE="$(sha256sum "$SERVICE" | awk '{print $1}')"
  I_OUT="$TMP/r1-idempotent.txt"
  I_RC="$(run_installer "$I_OUT" --apply --backend systemd)"
  expect IDEMPOTENT_EXIT 0 "$I_RC"
  expect IDEMPOTENT 1 "$(tok "$I_OUT" IDEMPOTENT)"
  expect UNIT_CONTENT_STABLE 1 "$([ "$(sha256sum "$SERVICE" | awk '{print $1}')" = "$SERVICE_SHA_BEFORE" ] && echo 1 || echo 0)"

  # 4) --uninstall 移除 unit
  U_OUT="$TMP/r1-uninstall.txt"
  U_RC="$(run_installer "$U_OUT" --uninstall)"
  expect UNINSTALL_EXIT 0 "$U_RC"
  at_least UNINSTALL_REMOVED 1 "$(tok "$U_OUT" REMOVED)"
  expect UNINSTALL_UNIT_GONE 1 "$([ ! -f "$SERVICE" ] && [ ! -f "$TIMER" ] && echo 1 || echo 0)"

  # 4b) 【Round2 非阻断建议落盘】`systemctl --user enable --now` 失败 → 不得报 PASS
  TE_OUT="$TMP/r1-timer-enable-fail.txt"
  TE_RC="$(FAKE_SYSTEMCTL_ENABLE_RC=1 run_installer "$TE_OUT" --apply --backend systemd)"
  sed -n '1,20p' "$TE_OUT"
  expect TIMER_ENABLE_FAIL_EXIT 2 "$TE_RC"
  expect TIMER_ENABLE_FAIL_TOKEN 0 "$(tok "$TE_OUT" TIMER_ENABLED)"
  expect TIMER_ENABLE_FAIL_NO_PASS 0 "$(grep -c '^RESULT: PASS' "$TE_OUT")"
  expect TIMER_ENABLE_FAIL_EXPLICIT 1 "$(grep -c '^RESULT: FAIL' "$TE_OUT")"

  # 4c) 同一失败在 --backend auto 下回退 cron：不报假 PASS、不留两份调度
  TF_OUT="$TMP/r1-timer-enable-fallback.txt"
  TF_RC="$(FAKE_SYSTEMCTL_ENABLE_RC=1 run_installer "$TF_OUT" --apply --backend auto)"
  sed -n '1,20p' "$TF_OUT"
  expect TIMER_FALLBACK_EXIT 0 "$TF_RC"
  expect TIMER_FALLBACK_WARN 1 "$(grep -c 'WARN:systemd-timer-not-enabled-fallback-cron' "$TF_OUT")"
  expect TIMER_FALLBACK_TIMER_ENABLED 0 "$(tok "$TF_OUT" TIMER_ENABLED)"
  expect TIMER_FALLBACK_UNIT_REMOVED 1 "$([ ! -f "$SERVICE" ] && [ ! -f "$TIMER" ] && echo 1 || echo 0)"
  expect TIMER_FALLBACK_CRON_LINE 1 "$(grep -Fxc '# opencode-user-cache-reconcile' "$CRON_FILE")"
  run_installer "$TMP/r1-cron-prewash.txt" --uninstall >/dev/null

  # 5) cron backend：marker 幂等
  C1_OUT="$TMP/r1-cron1.txt"
  C1_RC="$(run_installer "$C1_OUT" --apply --backend cron)"
  expect CRON_APPLY_EXIT 0 "$C1_RC"
  expect CRON_MARKER_PRESENT 1 "$([ "$(grep -Fxc '# opencode-user-cache-reconcile' "$CRON_FILE")" = "1" ] && echo 1 || echo 0)"
  expect CRON_SCHEDULE_0330 1 "$(grep -Fc '30 3 * * *' "$CRON_FILE")"
  C2_OUT="$TMP/r1-cron2.txt"
  run_installer "$C2_OUT" --apply --backend cron >/dev/null
  expect CRON_MARKER_SINGLE 1 "$([ "$(grep -Fxc '# opencode-user-cache-reconcile' "$CRON_FILE")" = "1" ] && echo 1 || echo 0)"
  expect CRON_IDEMPOTENT 1 "$(tok "$C2_OUT" IDEMPOTENT)"
  CU_OUT="$TMP/r1-cron-uninstall.txt"
  run_installer "$CU_OUT" --uninstall >/dev/null
  expect CRON_UNINSTALL_CLEAN 1 "$([ "$(grep -Fc '# opencode-user-cache-reconcile' "$CRON_FILE")" = "0" ] && echo 1 || echo 0)"

  # 6) 非法 --at → exit 2
  BAD_OUT="$TMP/r1-bad-at.txt"
  BAD_RC="$(run_installer "$BAD_OUT" --print --at 25:99)"
  expect BAD_AT_EXIT 2 "$BAD_RC"
  finish
  ;;

r2)
  ARCHIVE_DIR="$SANDBOX_STATE/tuneloop/reconcile"

  # 1) 无差异：静默（不建单、gh 零调用）
  : > "$GH_LOG"
  Z_OUT="$TMP/r2-zero.txt"
  Z_RC="$(FAKE_RECONCILE_MODE=zero run_runner "$Z_OUT" --date 2026-11-28)"
  sed -n '1,30p' "$Z_OUT"
  expect ZERO_EXIT 0 "$Z_RC"
  expect DIFFS0_DIFFS 0 "$(tok "$Z_OUT" DIFFS)"
  expect DIFFS0_GH_CALLS 0 "$(tok "$Z_OUT" GH_CALLS)"
  expect DIFFS0_ISSUE_CREATED 0 "$(grep -c 'issue create' "$GH_LOG")"
  expect DIFFS0_ISSUE_TOKEN "none" "$(tok "$Z_OUT" ISSUE)"
  expect ARCHIVE_WRITTEN 1 "$([ -f "$ARCHIVE_DIR/2026-11-28.json" ] && [ -f "$ARCHIVE_DIR/2026-11-28.txt" ] && echo 1 || echo 0)"

  # 2) 有差异：建单（标签仅 waitForConfirm + 正文含 token + 明细 ≤20）
  : > "$GH_LOG"
  rm -f "$GH_BODY_DIR"/*.md
  D_OUT="$TMP/r2-diff.txt"
  D_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$D_OUT" --date 2026-11-29)"
  sed -n '1,30p' "$D_OUT"
  expect DIFF_EXIT 0 "$D_RC"
  expect DIFFS_TOTAL 3 "$(tok "$D_OUT" DIFFS)"
  at_least DIFFS_GH_CALLS 1 "$(tok "$D_OUT" GH_CALLS)"
  expect DIFFS_ISSUE_CREATED 1 "$(grep -c 'issue create' "$GH_LOG")"
  expect DIFFS_ISSUE_NUMBER 777 "$(tok "$D_OUT" ISSUE)"
  CREATE_LINE="$(grep 'issue create' "$GH_LOG" | tail -n 1)"
  LABELS_ONLY=1
  printf '%s' "$CREATE_LINE" | grep -q -- '--label waitForConfirm' || LABELS_ONLY=0
  printf '%s' "$CREATE_LINE" | grep -q -- '--label status:' && LABELS_ONLY=0
  expect ISSUE_LABEL_WAITFORCONFIRM_ONLY 1 "$LABELS_ONLY"
  if [ "$LABELS_ONLY" = "1" ]; then echo "ISSUE_LABELS_EXACT=waitForConfirm"; else echo "ISSUE_LABELS_EXACT=MISMATCH"; FAILED=1; fi
  BODY_FILE="$(ls "$GH_BODY_DIR"/create-*.md 2>/dev/null | head -n 1)"
  expect ISSUE_BODY_PRESENT 1 "$([ -n "$BODY_FILE" ] && echo 1 || echo 0)"
  expect ISSUE_BODY_HAS_TOKENS 1 "$(grep -c 'TENANT_MISMATCH=2' "$BODY_FILE" 2>/dev/null | tr -d ' ')"
  DETAIL_LINES="$(grep -cE '^- iam_sub=' "$BODY_FILE" 2>/dev/null | tr -d ' ')"
  expect ISSUE_BODY_DETAIL_LIMIT 1 "$([ "${DETAIL_LINES:-0}" -le 20 ] && [ "${DETAIL_LINES:-0}" -ge 1 ] && echo 1 || echo 0)"
  expect ISSUE_BODY_NO_STATUS_LABEL_HINT 1 "$(grep -q 'waitForConfirm' "$BODY_FILE" && echo 1 || echo 0)"

  # 3) 同日第二次：复用既有单，不重复建单
  : > "$GH_LOG"
  S_OUT="$TMP/r2-same-day.txt"
  S_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_JSON='[{"number":777}]' run_runner "$S_OUT" --date 2026-11-29)"
  expect SAME_DAY_EXIT 0 "$S_RC"
  expect SAME_DAY_CREATES 0 "$(grep -c 'issue create' "$GH_LOG")"
  expect SAME_DAY_DUPLICATE 0 "$( [ "$(grep -c 'issue create' "$GH_LOG")" = "0" ] && echo 0 || echo 1)"
  expect SAME_DAY_ISSUE 777 "$(tok "$S_OUT" ISSUE)"

  # 4) --dry-run：不调用 gh、不写文件
  : > "$GH_LOG"
  rm -f "$ARCHIVE_DIR"/2026-11-30.*
  Y_OUT="$TMP/r2-dry-run.txt"
  Y_RC="$(FAKE_RECONCILE_MODE=diff3 run_runner "$Y_OUT" --date 2026-11-30 --dry-run)"
  expect DRY_RUN_EXIT 0 "$Y_RC"
  expect DRY_RUN_GH_CALLS 0 "$(tok "$Y_OUT" GH_CALLS)"
  expect DRY_RUN_NO_GH 0 "$(wc -l < "$GH_LOG" | tr -d ' ')"
  expect DRY_RUN_FILES_WRITTEN 0 "$(tok "$Y_OUT" FILES_WRITTEN)"

  # 5) 【审计 B1 负例】gh issue create 失败（401）→ 必须显式失败，不得 RESULT: PASS / exit 0
  : > "$GH_LOG"
  rm -f "$ARCHIVE_DIR"/2026-12-05.*
  CF_OUT="$TMP/r2-create-fail.txt"
  CF_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_CREATE_RC=1 run_runner "$CF_OUT" --date 2026-12-05)"
  sed -n '1,30p' "$CF_OUT"
  expect GH_CREATE_FAIL_EXIT 2 "$CF_RC"
  expect GH_CREATE_FAIL_NO_PASS 0 "$(grep -c '^RESULT: PASS' "$CF_OUT")"
  expect GH_CREATE_FAIL_ISSUE none "$(tok "$CF_OUT" ISSUE)"
  expect GH_CREATE_FAIL_TOKEN 1 "$(tok "$CF_OUT" ISSUE_CREATE_FAILED)"
  expect GH_CREATE_FAIL_NOT_SILENT 1 "$(grep -c '^RESULT: FAIL' "$CF_OUT")"
  expect GH_CREATE_FAIL_RC_SHOWN 1 "$(grep -c '^ISSUE_CREATE_RC=1$' "$CF_OUT")"
  expect GH_CREATE_FAIL_ARCHIVED 1 "$([ -f "$ARCHIVE_DIR/2026-12-05.json" ] && echo 1 || echo 0)"

  # 6) 【审计 B3 负例】--dry-run + 对账失败：零写入，不得覆盖同日既有归档，
  #    且被保护下来的归档必须让次日「连续 2 天不降」升级仍能触发。
  DF_DATE="2026-12-09"
  mkdir -p "$ARCHIVE_DIR"
  rm -f "$ARCHIVE_DIR/$DF_DATE".*
  printf '%s\n' '{"tokens":{"RECONCILE_DATE":"2026-12-09","TENANT_MISMATCH":"2","ORG_MISMATCH":"1"}}' > "$ARCHIVE_DIR/$DF_DATE.json"
  printf '%s\n' 'TENANT_MISMATCH=2' 'ORG_MISMATCH=1' > "$ARCHIVE_DIR/$DF_DATE.txt"
  DF_SHA_BEFORE="$(sha256sum "$ARCHIVE_DIR/$DF_DATE.json" | awk '{print $1}')"
  DF_LIST_BEFORE="$(ls -1 "$ARCHIVE_DIR" | LC_ALL=C sort | tr '\n' ' ')"
  : > "$GH_LOG"
  DF_OUT="$TMP/r2-dry-run-fail.txt"
  DF_RC="$(FAKE_RECONCILE_MODE=fail run_runner "$DF_OUT" --date "$DF_DATE" --dry-run)"
  sed -n '1,25p' "$DF_OUT"
  expect DRY_RUN_FAIL_EXIT 2 "$DF_RC"
  expect DRY_RUN_FAIL_NO_PASS 0 "$(grep -c '^RESULT: PASS' "$DF_OUT")"
  expect DRY_RUN_FAIL_FILES_WRITTEN 0 "$(tok "$DF_OUT" FILES_WRITTEN)"
  expect DRY_RUN_FAIL_GH_CALLS 0 "$(tok "$DF_OUT" GH_CALLS)"
  expect DRY_RUN_FAIL_DRY_RUN_TOKEN 1 "$(tok "$DF_OUT" DRY_RUN)"
  expect DRY_RUN_FAIL_NO_FILES 1 "$([ "$(ls -1 "$ARCHIVE_DIR" | LC_ALL=C sort | tr '\n' ' ')" = "$DF_LIST_BEFORE" ] && echo 1 || echo 0)"
  expect DRY_RUN_FAIL_ARCHIVE_PRESERVED 1 "$([ "$(sha256sum "$ARCHIVE_DIR/$DF_DATE.json" | awk '{print $1}')" = "$DF_SHA_BEFORE" ] && echo 1 || echo 0)"
  expect DRY_RUN_FAIL_ARCHIVE_TOKEN_DRYRUN "(dry-run)" "$(tok "$DF_OUT" ARCHIVE)"
  # 次日真实差异运行：归档完好 → 升级仍触发（缺陷版此处会被覆盖成 failed:true → ESCALATED=0）
  : > "$GH_LOG"
  DR_OUT="$TMP/r2-post-dry-run-escalate.txt"
  DR_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_JSON='[{"number":777}]' run_runner "$DR_OUT" --date 2026-12-10)"
  sed -n '1,25p' "$DR_OUT"
  expect POST_DRY_RUN_ESCALATE_EXIT 0 "$DR_RC"
  expect POST_DRY_RUN_ESCALATED 1 "$(tok "$DR_OUT" ESCALATED)"
  expect POST_DRY_RUN_ESCALATE_COMMENTS 1 "$(tok "$DR_OUT" ESCALATE_COMMENTS)"
  finish
  ;;

r3)
  ARCHIVE_DIR="$SANDBOX_STATE/tuneloop/reconcile"
  mkdir -p "$ARCHIVE_DIR"
  # 昨日归档：同类差异计数与今日相等（未下降）
  cat > "$ARCHIVE_DIR/2026-11-27.json" <<'PREV'
{"tokens":{"RECONCILE_DATE":"2026-11-27","RECONCILE_SOURCE":"fixture","USERS_LOCAL":"10","USERS_IAM_ACTIVE":"10","TENANT_MISMATCH":"2","ORG_MISMATCH":"1","NIL_UUID":"0","MISSING_LOCAL":"0","ORPHAN_LOCAL":"0","ROLE_MISMATCH":"0","ROLE_UNVERIFIED":"0","CONTEXT_AMBIGUOUS":"0","MEMBER_ONLY":"0","IAM_MISSING":"0","ORG_UNRESOLVED":"0","ROLE_REPAIR_SUPPORTED":"0","DB_WRITES":"0"},"rows":[]}
PREV
  : > "$GH_LOG"
  rm -f "$GH_BODY_DIR"/*.md
  E_OUT="$TMP/r3-escalate.txt"
  E_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_JSON='[{"number":777}]' run_runner "$E_OUT" --date 2026-11-28)"
  sed -n '1,30p' "$E_OUT"
  expect ESCALATE_EXIT 0 "$E_RC"
  expect ESCALATED 1 "$(tok "$E_OUT" ESCALATED)"
  expect ESCALATE_COMMENTS 1 "$(tok "$E_OUT" ESCALATE_COMMENTS)"
  expect ESCALATE_NEW_ISSUE 0 "$(grep -c 'issue create' "$GH_LOG")"
  expect ESCALATE_COMMENT_CALLS 1 "$(grep -c 'issue comment' "$GH_LOG")"
  expect ESCALATE_REASON_IN_BODY 1 "$(grep -l 'ESCALATE_REASON=' "$GH_BODY_DIR"/comment-*.md 2>/dev/null | wc -l | tr -d ' ')"

  # 对账失败：必须告警（建单 + exit 2），不得静默
  : > "$GH_LOG"
  F_OUT="$TMP/r3-failure.txt"
  F_RC="$(FAKE_RECONCILE_MODE=fail FAKE_GH_CREATE_NUMBER=888 run_runner "$F_OUT" --date 2026-12-01)"
  sed -n '1,20p' "$F_OUT"
  expect FAILURE_EXIT 2 "$F_RC"
  expect FAILURE_ISSUE_CREATED 1 "$(grep -c 'issue create' "$GH_LOG")"
  expect FAILURE_ISSUE 888 "$(tok "$F_OUT" FAILURE_ISSUE)"
  expect FAILURE_NOT_SILENT 1 "$(grep -c '^RESULT: FAIL' "$F_OUT")"
  expect FAILURE_NO_PASS 0 "$(grep -c '^RESULT: PASS' "$F_OUT")"
  FAIL_LINE="$(grep 'issue create' "$GH_LOG" | tail -n 1)"
  expect FAILURE_LABEL_EXACT 1 "$(printf '%s' "$FAIL_LINE" | grep -q -- '--label waitForConfirm' && ! printf '%s' "$FAIL_LINE" | grep -q -- '--label status:' && echo 1 || echo 0)"

  # 失败单也没建成（gh create 401）→ 仍 exit 2，且必须能区分「告警也未送达」
  : > "$GH_LOG"
  FC_OUT="$TMP/r3-failure-create-fail.txt"
  FC_RC="$(FAKE_RECONCILE_MODE=fail FAKE_GH_CREATE_RC=1 run_runner "$FC_OUT" --date 2026-12-03)"
  sed -n '1,20p' "$FC_OUT"
  expect FAILURE_CREATE_FAIL_EXIT 2 "$FC_RC"
  expect FAILURE_CREATE_FAIL_TOKEN 1 "$(tok "$FC_OUT" FAILURE_ISSUE_CREATE_FAILED)"
  expect FAILURE_CREATE_FAIL_ISSUE none "$(tok "$FC_OUT" FAILURE_ISSUE)"
  expect FAILURE_CREATE_FAIL_NO_PASS 0 "$(grep -c '^RESULT: PASS' "$FC_OUT")"

  # 【审计 B2 负例】升级评论失败（403）→ 必须显式失败，且不得声称已升级
  # 日期隔离说明（#2631 D7）：上一场景已在 2026-11-28 升级成功并写入当日 marker
  # （`.<date>.escalated`），同日重跑会被「当日只升级一次」跳过，故本场景改用 2026-12-31，
  # 仅隔离日期、不放宽任何断言。
  : > "$GH_LOG"
  rm -f "$GH_BODY_DIR"/*.md
  EC_OUT="$TMP/r3-escalate-comment-fail.txt"
  EC_RC="$(FAKE_RECONCILE_MODE=diff3 FAKE_GH_LIST_JSON='[{"number":777}]' FAKE_GH_COMMENT_RC=1 run_runner "$EC_OUT" --date 2026-12-31)"
  sed -n '1,30p' "$EC_OUT"
  expect GH_COMMENT_FAIL_EXIT 2 "$EC_RC"
  expect ESCALATE_CLAIMED_ON_FAILURE 0 "$(grep -c '^ESCALATED=1' "$EC_OUT")"
  expect ESCALATE_COMMENTS_ON_FAILURE 0 "$(grep -c '^ESCALATE_COMMENTS=1' "$EC_OUT")"
  expect ESCALATE_FAIL_TOKEN 1 "$(tok "$EC_OUT" ESCALATE_FAILED)"
  expect GH_COMMENT_FAIL_NO_PASS 0 "$(grep -c '^RESULT: PASS' "$EC_OUT")"
  expect GH_COMMENT_FAIL_RC_SHOWN 1 "$(grep -c '^ESCALATE_COMMENT_RC=1$' "$EC_OUT")"

  # env 缺失：exit 2 且打印缺失项
  : > "$GH_LOG"
  M_OUT="$TMP/r3-missing-env.txt"
  M_RC="$(RUN_BEACONIAM_DB_NAME='' FAKE_RECONCILE_MODE=zero run_runner "$M_OUT" --date 2026-12-02)"
  expect MISSING_ENV_EXIT 2 "$M_RC"
  expect MISSING_ENV_TOKEN "BEACONIAM_DB_NAME" "$(tok "$M_OUT" MISSING_ENV)"
  expect MISSING_ENV_NO_GH 0 "$(wc -l < "$GH_LOG" | tr -d ' ')"
  finish
  ;;

*)
  echo "usage: $0 <r1|r2|r3>" >&2
  exit 2
  ;;
esac
