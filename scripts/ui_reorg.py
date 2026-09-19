#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""ui.md 追加区三块并入规章节（Issue #1970 C2 阶段）。

用法：
  python scripts/ui_reorg.py --dry-run   # 仅输出变更映射 + 校验结果
  python scripts/ui_reorg.py --apply     # 执行重组 + 生成 .bak 备份

规则表：见下方 RULES（按 Issue #1970 C2 定稿，2026-09-19 用户确认 B 方案）。
"""
import argparse
import re
import shutil
import sys
from pathlib import Path

UI_PATH = Path("newDocs/spec/ui/ui.md")

# ---------------------------------------------------------------- 规则表 ---
# 移动端节：归入二章（接 2.10 起）
# 处置: keep=完整并入 | dup->§x=去重留指针 | deprecated=并入+废弃标注
MOBILE = {
    "3.18": ("2.10", "keep"),
    "3.20": ("2.11", "keep"),
    "3.21": ("2.12", "dup->§2.2 首页（乐器浏览）"),
    "3.22": ("2.13", "dup->§2.3 乐器详情页"),
    "3.23": ("2.14", "keep"),
    "3.24": ("2.15", "keep"),
    "3.25": ("2.16", "keep"),
    "3.28": ("2.17", "deprecated"),
    "3.29": ("2.18", "keep"),
    "3.30": ("2.19", "keep"),
    "3.32": ("2.20", "keep"),
    "3.36.1": ("2.21", "keep"),
    "3.37": ("2.22", "keep"),
    "3.40": ("2.23", "keep"),
}
# PC 端节：归入三章（接 3.11 起）
PC = {
    "3.17": ("3.11", "keep"),
    "3.19": ("3.12", "keep"),
    "3.27": ("3.13", "keep"),
    "3.31": ("3.14", "keep"),
    "3.33": ("3.15", "keep"),
    "3.34": ("3.16", "deprecated"),
    "3.35": ("3.17", "deprecated"),
    "3.35.1": ("3.18", "keep"),
    "3.35.2": ("3.19", "keep"),
    "3.38": ("3.20", "keep"),
    "3.39": ("3.21", "keep"),
}
# 双端/通用节：归入三章尾部（接 3.22 起）
GENERAL = {
    "3.26": ("3.22", "keep"),
    "3.36": ("3.23", "keep"),
    "3.99": ("3.24", "keep"),
    "3.41": ("3.25", "keep"),
    "3.42": ("3.26", "keep"),
}

# 追加区三块边界（标题行）
BLOCK1_HEAD = "## PC端侧边栏菜单"          # ① PC 侧边栏
BLOCK2_HEAD = "## 补充章节"                # ② 补充章节（含 3.0 冷启动）
BLOCK3_HEAD = "## 附录 A"                  # ③ 附录 A（路由表 + 3.17-3.42）
COLD_START_HEAD = "### 3.0 冷启动向导 (Setup)"
COLD_START_NEW = "### 3.2.1 冷启动向导 (Setup)"

# ① 内丢弃的开发记录节
BLOCK1_DROP_HEADS = {"### 构建与部署", "### 最后更新记录", "### 关键代码位置"}
BLOCK1_660_HEAD = "### 权限管理页面设计（#660）"
BLOCK1_PWD_HEAD = "### 3.10.3 密码重置（独立页面）"

HEAD_RE = re.compile(r"^(#{2,4})\s+")
SECTION_RE = re.compile(r"^###\s+(\d+(?:\.\d+)*)\s+")


def find_lines(text):
    lines = text.split("\n")
    heads = {}
    for i, ln in enumerate(lines):
        if HEAD_RE.match(ln):
            heads[i] = ln
    return lines, heads


def section_boundary(lines, heads, start):
    """[start, end)：从 start 标题行到下一个同级或更高级标题前。"""
    level = len(re.match(r"^(#{2,4})", heads[start]).group(1))
    end = start + 1
    while end < len(lines):
        m = HEAD_RE.match(lines[end])
        if m and len(m.group(1)) <= level:
            break
        end += 1
    return start, end


def extract_section(lines, heads, start):
    s, e = section_boundary(lines, heads, start)
    return s, e, lines[s:e]


def find_head_row(heads, prefix):
    for i, ln in heads.items():
        if ln.startswith(prefix):
            return i
    return None


def build_report():
    text = UI_PATH.read_text(encoding="utf-8")
    lines, heads = find_lines(text)
    out = []
    for head, name in [(BLOCK1_HEAD, "① PC侧边栏"), (BLOCK2_HEAD, "② 补充章节"),
                       (BLOCK3_HEAD, "③ 附录A")]:
        row = find_head_row(heads, head)
        out.append(f"块 {name}: 行 {row + 1 if row is not None else '?'}")
    for old, (new, action) in {**MOBILE, **PC, **GENERAL}.items():
        row = find_head_row(heads, f"### {old} ")
        out.append(f"  节 {old} -> {new} [{action}] @行{row + 1 if row is not None else '?'}")
    return "\n".join(out)


def reorg():
    text = UI_PATH.read_text(encoding="utf-8")
    lines, heads = find_lines(text)

    # ---- 收集 26 节 ----
    sections = []  # (旧编号, 新编号, 处置, 新标题, 内容行)
    for old, (new, action) in {**MOBILE, **PC, **GENERAL}.items():
        row = find_head_row(heads, f"### {old} ")
        if row is None:
            raise SystemExit(f"找不到标题: ### {old}")
        s, e, body = extract_section(lines, heads, row)
        title = body[0]
        m = re.search(r"^### \d+(?:\.\d+)*\s+(.+)$", title)
        name = m.group(1) if m else title
        new_title = f"### {new} {name}"
        sections.append((old, new, action, new_title, body[1:]))

    # ---- ① PC 侧边栏拆解 ----
    block1_row = find_head_row(heads, BLOCK1_HEAD)
    s1, e1, block1_body = extract_section(lines, heads, block1_row)
    b1_sections = []
    cur = None
    for ln in block1_body[1:]:
        if re.match(r"^###\s+", ln):
            if cur and cur[0]:
                b1_sections.append(cur)
            cur = [ln, []]
        else:
            if cur is None:
                cur = ["", []]
            cur[1].append(ln)
    if cur and cur[0]:
        b1_sections.append(cur)

    block1_keep = []
    block1_660 = []
    for title, body in b1_sections:
        if title in BLOCK1_DROP_HEADS:
            continue
        if title == BLOCK1_660_HEAD:
            block1_660 = body
            continue
        if title == BLOCK1_PWD_HEAD:
            title = "### 密码重置（独立页面）"
        block1_keep.append([title] + body)

    # ---- ② 冷启动 ----
    cold_row = find_head_row(heads, COLD_START_HEAD)
    if cold_row is None:
        raise SystemExit("找不到 3.0 冷启动向导")
    s, e, cold_body = extract_section(lines, heads, cold_row)
    cold_title = cold_body[0].replace(COLD_START_HEAD, COLD_START_NEW, 1)

    # ---- ③ 附录 A 路由表 ----
    block3_row = find_head_row(heads, BLOCK3_HEAD)
    s3, e3, block3_body = extract_section(lines, heads, block3_row)
    appendix_body = []
    for ln in block3_body[1:]:
        if re.match(r"^###\s+3\.", ln):
            break
        appendix_body.append(ln)

    # ---- 定位正文接收点 ----
    chap3_row = find_head_row(heads, "## 三、PC 端")
    chap4_row = find_head_row(heads, "## 四、原子组件设计")
    if chap3_row is None or chap4_row is None:
        raise SystemExit("找不到 ## 三、PC 端 或 ## 四、原子组件设计")

    prefix = lines[:chap3_row]                     # 一、二章
    chap3_body = lines[chap3_row:chap4_row]        # 三章
    tail = lines[chap4_row:block1_row]             # 四~七章（含版本记录）

    # 移动端节按物理序
    def phys(order_map):
        return sorted(order_map.keys(), key=lambda x: tuple(int(p) for p in x.split(".")))

    # ---- 组装移动端块（二章末尾 2.10 起）----
    mobile_block = []
    for old in phys(MOBILE):
        new, action = MOBILE[old]
        sec = next(x for x in sections if x[0] == old)
        if action.startswith("dup"):
            ptr = action.split("->", 1)[1].strip()
            content = ["", f"> 内容已归入正文 {ptr}，本节保留编号作占位。", ""]
        elif action == "deprecated":
            content = ["", "> ⚠️ **已废弃**：本节保留编号作占位，功能已由替代页面承担。", ""] + sec[4]
        else:
            content = sec[4]
        mobile_block += [sec[3]] + content + ["", "---", ""]

    # ---- 组装 PC 端块（三章 3.11 起）----
    pc_block = []
    for old in phys(PC):
        new, action = PC[old]
        sec = next(x for x in sections if x[0] == old)
        if action == "deprecated":
            content = ["", "> ⚠️ **已废弃**：本节保留编号作占位，功能已由替代页面承担。", ""] + sec[4]
        else:
            content = sec[4]
        pc_block += [sec[3]] + content + ["", "---", ""]

    # ---- 组装双端块（三章 3.22 起，按新编号排序）----
    gen_block = []
    gen_sorted = sorted(GENERAL.items(), key=lambda kv: tuple(int(p) for p in kv[1][0].split(".")))
    for old, _ in gen_sorted:
        sec = next(x for x in sections if x[0] == old)
        gen_block += [sec[3]] + sec[4] + ["", "---", ""]

    # ---- 组装①剩余（三章 3.27 起，子节降为 ####）----
    b1_block = ["### 3.27 PC 端侧边栏菜单与权限（Imported from AGENTS.md, 2026-04-17）"]
    for sec_body in block1_keep:
        if sec_body and sec_body[0].startswith("### "):
            sec_body = ["#### " + sec_body[0][4:]] + sec_body[1:]
        b1_block += [""] + sec_body
    b1_block += ["", "---", ""]

    # ---- #660 → 3.10.3（插到三章 3.10.2 节后）----
    sec102 = None
    for i, ln in enumerate(chap3_body):
        if ln.startswith("### 3.10.2"):
            sec102 = i
            break
    if sec102 is not None:
        j = sec102 + 1
        while j < len(chap3_body):
            if re.match(r"^#{2,3}\s+", chap3_body[j]):
                break
            j += 1
        block660 = ["### 3.10.3 权限管理页面设计（#660）"] + [""] + block1_660 + ["", "---", ""]
        chap3_body = chap3_body[:j] + block660 + chap3_body[j:]

    # ---- 冷启动 → 3.2.1（插到 3.3 前）----
    sec33 = None
    for i, ln in enumerate(chap3_body):
        if ln.startswith("### 3.3 Dashboard"):
            sec33 = i
            break
    if sec33 is None:
        raise SystemExit("找不到 ### 3.3 Dashboard")
    cold_block = [cold_title] + cold_body[1:] + ["", "---", ""]
    chap3_body = chap3_body[:sec33] + cold_block + chap3_body[sec33:]

    # ---- 最终拼装 ----
    appendix_block = ["## 附录 A: 页面路由清单"] + appendix_body
    result_lines = (prefix + mobile_block + chap3_body + pc_block + gen_block
                    + b1_block + tail + appendix_block)
    return "\n".join(result_lines)


def validate(text):
    lines, heads = find_lines(text)
    issues = []
    seqs = {"二章": [], "三章": []}
    for i, ln in heads.items():
        m = SECTION_RE.match(ln)
        if not m:
            continue
        n = m.group(1)
        if n.startswith("2."):
            seqs["二章"].append(n)
        elif n.startswith("3."):
            seqs["三章"].append(n)
    for name, nums in seqs.items():
        seen = {}
        for n in nums:
            seen[n] = seen.get(n, 0) + 1
        for n, c in seen.items():
            if c > 1:
                issues.append(f"{name}: 编号 {n} 重复 {c} 次")
    for bad in [BLOCK1_HEAD, BLOCK2_HEAD,
                "### 3.17 申诉处理", "### 3.99 本地化规范", "### 3.42 乐器丢失",
                COLD_START_HEAD]:
        for i, ln in heads.items():
            if ln.startswith(bad):
                issues.append(f"追加区残留: {ln}")
    # 附录 A 只允许出现一次（新文末附录）
    app_count = sum(1 for ln in heads.values() if ln.startswith(BLOCK3_HEAD))
    if app_count != 1:
        issues.append(f"附录 A 出现 {app_count} 次（应为 1）")
    return issues


def main():
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except AttributeError:
        pass
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true", help="只输出报告，不写入")
    ap.add_argument("--apply", action="store_true", help="执行重组")
    args = ap.parse_args()

    if args.dry_run:
        print(build_report())
        return

    if args.apply:
        bak = UI_PATH.with_suffix(".md.bak")
        shutil.copy2(UI_PATH, bak)
        new_text = reorg()
        issues = validate(new_text)
        if issues:
            print("❌ 校验发现问题，不写入：")
            for x in issues:
                print("  -", x)
            sys.exit(1)
        UI_PATH.write_text(new_text, encoding="utf-8")
        print(f"✅ 已写入 {UI_PATH}（备份: {bak.name}）")
        print("校验通过：二章/三章编号无重复，追加区无残留。")
    else:
        ap.print_help()


if __name__ == "__main__":
    main()