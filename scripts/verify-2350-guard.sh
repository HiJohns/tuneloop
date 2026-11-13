#!/usr/bin/env bash
# #2350: done 级联守卫验收（notgreen / green / noevidence）。
# 以合成父子 Issue + PATH 注入的 gh stub 驱动真实 opencode_gh.sh done 分支。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  notgreen|green|noevidence) ;;
  *) echo "usage: $0 notgreen|green|noevidence" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
GH_SCRIPT="${OPENCODE_GH_SCRIPT:-$HOME/.config/opencode/scripts/opencode_gh.sh}"
[ -f "$GH_SCRIPT" ] || { echo "ERROR: opencode_gh.sh not found: $GH_SCRIPT" >&2; exit 2; }

mkdir -p "$REPO_ROOT/docs/evidence/2350" "$REPO_ROOT/tmp"
PROVENANCE="build=done-cascade-guard commit=$(git -C "$REPO_ROOT" rev-parse HEAD) dirty=$(test -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no -- scripts)" && echo false || echo true)"

WORK="$(mktemp -d "$REPO_ROOT/tmp/2350-guard.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/bin" "$WORK/docs/evidence/999" "$WORK/tmp"

# ---------------------------------------------------------------- gh stub
cat > "$WORK/bin/gh" <<'STUB'
#!/usr/bin/env python3
import json, os, subprocess, sys

STATE = os.environ["GH_STUB_STATE"]
args = sys.argv[1:]
state = json.load(open(STATE))

def save():
    json.dump(state, open(STATE, "w"))

def issue(n):
    return state["issues"][str(n)]

def emit(obj, q=None):
    text = json.dumps(obj)
    if q:
        p = subprocess.run(["jq", "-r", q], input=text, text=True, capture_output=True)
        sys.stdout.write(p.stdout)
    else:
        sys.stdout.write(text + "\n")

def select(issue_obj, fields):
    out = {}
    for f in fields.split(","):
        f = f.strip()
        if f == "labels":
            out[f] = issue_obj.get("labels", [])
        elif f == "comments":
            out[f] = issue_obj.get("comments", [])
        else:
            out[f] = issue_obj.get(f)
    return out

cmd, sub = args[0], args[1] if len(args) > 1 else ""
rest = args[2:]

if cmd == "issue" and sub == "view":
    n = rest[0]
    q = None
    fields = "number,state,body,labels"
    i = 1
    while i < len(rest):
        if rest[i] == "--json":
            fields = rest[i + 1]; i += 2
        elif rest[i] == "-q":
            q = rest[i + 1]; i += 2
        else:
            i += 1
    emit(select(issue(n), fields), q)
elif cmd == "issue" and sub == "edit":
    n = rest[0]
    i = 1
    obj = issue(n)
    names = [l["name"] for l in obj.get("labels", [])]
    while i < len(rest):
        if rest[i] == "--add-label":
            if rest[i + 1] not in names:
                names.append(rest[i + 1])
            i += 2
        elif rest[i] == "--remove-label":
            if rest[i + 1] in names:
                names.remove(rest[i + 1])
            i += 2
        else:
            i += 1
    obj["labels"] = [{"name": x} for x in names]
    save()
elif cmd == "issue" and sub == "comment":
    n = rest[0]
    body = ""
    if "--body-file" in rest:
        body = open(rest[rest.index("--body-file") + 1]).read()
    issue(n).setdefault("comments", []).append({"body": body})
    save()
elif cmd == "issue" and sub == "close":
    issue(rest[0])["state"] = "CLOSED"
    save()
elif cmd == "issue" and sub == "list":
    items = []
    for n, obj in state["issues"].items():
        items.append({"number": obj.get("number", int(n)), "body": obj.get("body", ""),
                      "labels": obj.get("labels", []), "state": obj.get("state", "OPEN")})
    q = None
    if "-q" in rest:
        q = rest[rest.index("-q") + 1]
    emit(items, q)
else:
    pass
STUB
chmod +x "$WORK/bin/gh"

# ---------------------------------------------------------------- fixtures
case "$MODE" in
  notgreen)
    cat > "$WORK/docs/evidence/999/evidence.json" <<'JSON'
{"schema_version":1,"issue":999,"rows":[{"id":"R1","pass":true},{"id":"R13","pass":false}]}
JSON
    ;;
  green)
    cat > "$WORK/docs/evidence/999/evidence.json" <<'JSON'
{"schema_version":1,"issue":999,"rows":[{"id":"R1","pass":true},{"id":"R13","pass":true}]}
JSON
    ;;
  noevidence)
    ;;
esac

cat > "$WORK/state.json" <<'JSON'
{"issues":{
  "999":{"number":999,"state":"OPEN","body":"Synthetic parent for #2350","labels":[{"name":"status:accepted"}],"comments":[]},
  "1001":{"number":1001,"state":"OPEN","body":"Synthetic child\n\nParent: #999","labels":[{"name":"status:wip"},{"name":"agent:test-node"}],"comments":[]}
}}
JSON

printf '## done\n\nsynthetic completion\n' > "$WORK/done-body.md"

# ---------------------------------------------------------------- drive done
set +e
( cd "$WORK" && PATH="$WORK/bin:$PATH" GH_STUB_STATE="$WORK/state.json" GH_BIN="$WORK/bin/gh" \
    bash "$GH_SCRIPT" done 1001 "$WORK/done-body.md" ) > "$WORK/run.log" 2>&1
RUN_RC=$?
set -e

RESULT="$(python3 - "$WORK/state.json" <<'PY'
import json, sys
st = json.load(open(sys.argv[1]))
p, c = st["issues"]["999"], st["issues"]["1001"]
print("parent_state=%s" % p["state"])
print("parent_auto_comment=%s" % ("yes" if any("Parent Task Auto-Completion" in x.get("body", "") for x in p.get("comments", [])) else "no"))
print("child_state=%s" % c["state"])
print("child_labels=%s" % ",".join(l["name"] for l in c.get("labels", [])))
PY
)"

case "$MODE" in
  notgreen) OUT="docs/evidence/2350/R1-guard-notgreen.txt" ;;
  green)    OUT="docs/evidence/2350/R2-guard-green.txt" ;;
  noevidence) OUT="docs/evidence/2350/R3-guard-noevidence.txt" ;;
esac

{
  echo "PROVENANCE $PROVENANCE"
  echo "MODE $MODE"
  echo "CMD bash \"$GH_SCRIPT\" done 1001 <body>  (gh stub via PATH + GH_BIN)"
  echo "exit=$RUN_RC"
  echo "--- run log (key lines) ---"
  grep -E "INFO:|SUCCESS:|WARN:|ERROR:" "$WORK/run.log" || true
  echo "--- final state ---"
  printf '%s\n' "$RESULT"
} > "$REPO_ROOT/$OUT"

fail=0
case "$MODE" in
  notgreen)
    grep -q "INFO:PARENT_CONTRACT_NOT_GREEN:999" "$WORK/run.log" || { echo "R1 FAIL: missing INFO:PARENT_CONTRACT_NOT_GREEN" >&2; fail=1; }
    grep -q "parent_state=OPEN" <<<"$RESULT" || { echo "R1 FAIL: parent cascaded while contract not green" >&2; fail=1; }
    grep -q "parent_auto_comment=no" <<<"$RESULT" || { echo "R1 FAIL: auto-completion comment written" >&2; fail=1; }
    echo "RESULT: NOT-CASCADED" >> "$REPO_ROOT/$OUT"
    ;;
  green)
    grep -q "SUCCESS:PARENT_DONE:999" "$WORK/run.log" || { echo "R2 FAIL: parent not cascaded" >&2; fail=1; }
    grep -q "parent_state=CLOSED" <<<"$RESULT" || { echo "R2 FAIL: parent still open" >&2; fail=1; }
    echo "RESULT: CASCADED" >> "$REPO_ROOT/$OUT"
    ;;
  noevidence)
    grep -q "SUCCESS:PARENT_DONE:999" "$WORK/run.log" || { echo "R3 FAIL: legacy cascade changed" >&2; fail=1; }
    grep -q "parent_state=CLOSED" <<<"$RESULT" || { echo "R3 FAIL: parent still open" >&2; fail=1; }
    echo "RESULT: UNCHANGED" >> "$REPO_ROOT/$OUT"
    ;;
esac
grep -q "child_state=CLOSED" <<<"$RESULT" || { echo "FAIL: child was not closed" >&2; fail=1; }
[ "$fail" -eq 0 ] || exit 1
echo "$OUT: PASS"
