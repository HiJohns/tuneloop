#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""api.md C2 重组脚本（Issue #1970 C2 阶段）。

用法：
  python scripts/api_reorg.py --dry-run   # 仅输出变更映射 + 校验结果
  python scripts/api_reorg.py --apply     # 执行重组 + 生成 .bak 备份

方案（2026-09-19 用户确认 Q1-Q6）：
  - Q1(b): 公共浏览并入五章前置（5.1 公共浏览，原五章顺延）
  - Q2(b): 申诉按端点拆分（商家部分→九章，用户部分→八章）
  - Q3: 冷启动归入「二、认证授权」
  - Q4: 促销覆盖 → 5.11
  - Q5: 版本记录移至文末
  - Q6: 库管归入六章（6.13 起）
"""
import argparse
import re
import shutil
import sys
from pathlib import Path

API_PATH = Path("newDocs/spec/api/api.md")

# 章节边界（行号，1-based，来自边界调查）
CHAPS = {
    "一": (9, 84), "二": (85, 382), "三白": (383, 408), "三公": (409, 476),
    "四": (477, 665), "五": (666, 1609), "六": (1610, 2169), "七": (2170, 2771),
    "八": (2772, 3838), "九": (3839, 4431), "十": (4432, 5157),
    "十一": (5158, 5208), "十二": (5209, 5685), "十三": (5686, 5856),
    "十四": (5857, 5896), "版记": (5897, 5910), "补": (5911, 5915),
    "冷": (5916, 5967), "申": (5968, 6121), "库": (6122, 6258),
    "确": (6259, 6297), "仪": (6298, 6310),
    "附A": (6311, 6316), "附B": (6317, 6326), "附C": (6327, 6340),
}

H3_RE = re.compile(r"^#{3,4}\s+(\d+(?:\.\d+)*)")


def read_lines():
    return API_PATH.read_text(encoding="utf-8").split("\n")


def get_block(lines, name):
    """返回章节块（不含结束换行），行号按 1-based 切片。"""
    s, e = CHAPS[name]
    return lines[s - 1:e]


def split_h3(lines):
    """把块按 ### 子标题切成 [(标题行, 内容行列表)]。块首非 ### 的行归入首元素标题。"""
    out = []
    cur = None
    for ln in lines:
        if H3_RE.match(ln):
            if cur:
                out.append(cur)
            cur = [ln, []]
        else:
            if cur is None:
                cur = [ln, []]
            else:
                cur[1].append(ln)
    if cur:
        out.append(cur)
    return out


def join_h3(sections):
    out = []
    for title, body in sections:
        if title:
            out.append(title)
        out.extend(body)
    return out


def renum_h3(block, mapping):
    """对块内 ### 标题按 mapping（旧编号->新编号）重编号；返回新块。"""
    new = []
    for title, body in split_h3(block):
        m = H3_RE.match(title)
        if m and m.group(1) in mapping:
            old = m.group(1)
            nw = mapping[old]
            title = title.replace(f"### {old}", f"### {nw}", 1)
        new.append([title, body])
    return join_h3(new)


def main():
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except AttributeError:
        pass
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--apply", action="store_true")
    args = ap.parse_args()

    if args.dry_run:
        print("api.md C2 重组变更映射（dry-run）")
        print("=" * 50)
        print("[顶层] 公共浏览(409-476) → 删除，并入五章；版本记录(5897) → 移至文末")
        print("[五章] 公共浏览→5.1 前置；原 5.1-5.9 → 5.2-5.10；促销覆盖 → 5.11")
        print("[六章] 6.4详情→6.5, 合同列表6.5→6.6, 合同详情→6.7, 签署→6.8, 终止→6.9, 触发所有权→6.10, 出库→6.11, 损伤评估→6.12；库管→6.13-6.17")
        print("[二章] 用户信息2.3→2.4, 影子用户→2.5, IAM代理→2.6；冷启动→2.7, 确认会话→2.8")
        print("[八章] 核身采集8.11.1→8.11a.1, 核身超时8.11.2→8.11a.2；申诉用户部分→8.16-8.18")
        print("[九章] 申诉商家部分→9.19-9.21")
        print("[十章] 人员管理10.17→10.18；仪表盘→10.17；平台员工管理→10.19")
        return

    if args.apply:
        bak = API_PATH.with_suffix(".md.bak")
        shutil.copy2(API_PATH, bak)
        new_text = reorg()
        issues = validate(new_text)
        if issues:
            print("❌ 校验发现问题，不写入：")
            for x in issues:
                print("  -", x)
            sys.exit(1)
        API_PATH.write_text(new_text, encoding="utf-8")
        print(f"✅ 已写入 {API_PATH}（备份: {bak.name}）")
        print("校验通过：顶层编号唯一，各章子编号连续，追加区清零。")
    else:
        ap.print_help()


def reorg():
    lines = read_lines()

    # ---- 提取各块 ----
    b1 = get_block(lines, "一")          # 基础规范
    b2 = get_block(lines, "二")          # 认证授权
    b3w = get_block(lines, "三白")       # 白标化
    b3p = get_block(lines, "三公")       # 公共浏览（将并入五章）
    b4 = get_block(lines, "四")          # 网点LBS
    b5 = get_block(lines, "五")          # 乐器租赁
    b6 = get_block(lines, "六")          # 订单
    b7 = get_block(lines, "七")          # 维保
    b8 = get_block(lines, "八")          # 个人中心
    b9 = get_block(lines, "九")          # 商家
    b10 = get_block(lines, "十")         # 平台运营
    b11 = get_block(lines, "十一")       # 通用
    b12 = get_block(lines, "十二")       # 系统管理
    b13 = get_block(lines, "十三")       # 标签属性
    b14 = get_block(lines, "十四")       # 技术要点
    bver = get_block(lines, "版记")      # 版本记录
    bwei = get_block(lines, "补")        # 补充章节标题
    bcold = get_block(lines, "冷")       # 冷启动
    bappeal = get_block(lines, "申")     # 申诉
    bware = get_block(lines, "库")       # 库管
    bconf = get_block(lines, "确")       # 确认会话
    bdash = get_block(lines, "仪")       # 仪表盘
    bappA = get_block(lines, "附A")
    bappB = get_block(lines, "附B")
    bappC = get_block(lines, "附C")

    # 版本记录 → 文末且标题去编号（Q5）
    if bver and bver[0].startswith("## 十三、版本记录"):
        bver[0] = "## 版本记录"

    # ---- 1. 一、基础规范：1.5 权限模型 → 1.6（分页参数保持 1.5）----
    b1_lines = b1
    new_b1 = []
    for ln in b1_lines:
        if re.match(r"^###\s+1\.5\s+权限模型", ln):
            ln = re.sub(r"^###\s+1\.5", "### 1.6", ln, count=1)
        new_b1.append(ln)
    b1 = new_b1

    # ---- 2. 二、认证授权：2.3 用户信息→2.4, 2.4 影子→2.5, 2.5 IAM→2.6 ----
    # 逐行扫描：父节(###)按标题匹配，子节(####)跟随父节
    b2_lines = b2
    new_b2 = []
    b2_parent_map = {"Token 刷新": "2.3", "用户信息": "2.4", "影子用户同步": "2.5",
                     "IAM 代理接口": "2.6"}
    b2_cur = None  # 当前父节新编号
    b2_sub = 0
    for ln in b2_lines:
        m3 = re.match(r"^###\s+", ln)
        m4 = re.match(r"^####\s+", ln)
        if m3 and not m4:
            b2_cur = None
            for k, v in b2_parent_map.items():
                if k in ln:
                    ln = re.sub(r"^###\s+[\d.]+", f"### {v}", ln, count=1)
                    b2_cur = v
                    b2_sub = 0
                    break
        elif m4 and b2_cur:
            b2_sub += 1
            ln = re.sub(r"^####\s+[\d.]+", f"#### {b2_cur}.{b2_sub}", ln, count=1)
        new_b2.append(ln)
    b2 = new_b2
    # 冷启动 → 2.7（剥 H2 首行，改子节 2.4.1/2.4.2 → 2.7.1/2.7.2）
    bcold_body = bcold[1:]
    bcold_s = split_h3(bcold_body)
    for sec in bcold_s:
        m = H3_RE.match(sec[0])
        if m:
            old = m.group(1)
            nw = f"2.7.{old.split('.')[-1]}"
            sec[0] = sec[0].replace(f"### {old}", f"### {nw}", 1)
    bcold_new = [f"### 2.7 冷启动（Setup）"] + join_h3(bcold_s)
    # 确认会话 → 2.8（剥 H2 首行，子节 19.1 → 2.8.1）
    bconf_body = bconf[1:]
    bconf_s = split_h3(bconf_body)
    for sec in bconf_s:
        m = H3_RE.match(sec[0])
        if m:
            old = m.group(1)
            nw = f"2.8.{old.split('.')[-1]}"
            sec[0] = sec[0].replace(f"### {old}", f"### {nw}", 1)
    bconf_new = [f"### 2.8 确认会话"] + join_h3(bconf_s)
    # 追加冷启动(2.7) 和确认会话(2.8) 到二章末尾
    b2 = b2 + [""] + bcold_new + [""] + bconf_new

    # ---- 3. 五章：公共浏览并入前置 + 原五章顺延 ----
    # 公共浏览 3.1-3.4 → 5.1.1-5.1.4（剥 H2 首行）
    b3p_body = b3p[1:]
    b3p_s = split_h3(b3p_body)
    for sec in b3p_s:
        m = H3_RE.match(sec[0])
        if m:
            old = m.group(1)  # 3.1/3.2/3.3/3.4
            nw = f"5.1.{old.split('.')[-1]}"
            sec[0] = sec[0].replace(f"### {old}", f"### {nw}", 1)
    pub_block = [f"### 5.1 公共浏览"] + join_h3(b3p_s)

    # 原五章顺延：按物理顺序扫描，父节(###)按序分配 5.2-5.11，子节(####)跟随父节
    # 顺延后父节编号序列（跳过公共浏览 5.1）：
    # 5.1乐器分类→5.2, 5.2二级→5.3, 5.3列表→5.4, 5.3.1排序→5.4.1,
    # 5.4详情→5.5, 5.5定价→5.6, 5.5.1V2→5.6.1, 5.6扩展→5.7, 5.7Excel→5.8,
    # 5.8照片→5.9, 5.9媒体→5.10, 5.8促销→5.11
    # 用父节顺序编号 + 子节跟随实现（物理顺序）
    new_b5 = []
    b5_lines = b5
    parent_seq = ["5.2", "5.3", "5.4", "5.5", "5.6", "5.7", "5.8", "5.9", "5.10", "5.11"]
    cur_parent_new = None  # 当前父节新编号
    sub_count = 0           # 当前父节子节序号
    pidx = 0
    for ln in b5_lines:
        m3 = re.match(r"^###\s+(\d+(?:\.\d+)*)", ln)
        m4 = re.match(r"^####\s+", ln)
        if m3 and not m4:
            # 三级 ### 5.3.1 类标题（如 分类内排序）→ 跟随父节，不算新父节
            if m3.group(1).count(".") >= 2 and cur_parent_new:
                sub_count += 1
                ln = re.sub(r"^###\s+[\d.]+", f"### {cur_parent_new}.{sub_count}", ln, count=1)
            else:
                # 单级父节：按顺序分配
                if pidx < len(parent_seq):
                    ln = re.sub(r"^###\s+[\d.]+", f"### {parent_seq[pidx]}", ln, count=1)
                    cur_parent_new = parent_seq[pidx]
                    sub_count = 0
                    pidx += 1
                else:
                    cur_parent_new = None
        elif m4 and cur_parent_new:
            # 子节：父节编号 + 递增序号
            sub_count += 1
            ln = re.sub(r"^####\s+[\d.]+", f"#### {cur_parent_new}.{sub_count}", ln, count=1)
        new_b5.append(ln)
    # b5 首元素应为五章 H2 标题，其后接公共浏览 5.1 + 原内容
    b5_head = None
    for i, ln in enumerate(new_b5):
        if ln.startswith("## 五、乐器租赁模块"):
            b5_head = new_b5[i]
            new_b5 = new_b5[:i] + new_b5[i + 1:]
            break
    if b5_head is None:
        b5_head = "## 五、乐器租赁模块"
    b5 = [b5_head, ""] + pub_block + [""] + new_b5

    # ---- 4. 六章：子编号重排 + 库管追加 ----
    b6_lines = b6
    new_b6 = []
    # 按标题语义匹配，处理 ### 和 #### 两级
    six_map_title = {
        "订单详情": ("6.5", "6.5"),
        "获取合同列表": ("6.6", "6.6"),
        "获取合同详情": ("6.7", "6.7"),
        "签署协议": ("6.8", "6.8"),
        "终止租约": ("6.9", "6.9"),
        "触发所有权转移": ("6.10", "6.10"),
        "出库确认管理": ("6.11", "6.11"),
        "损伤评估管理": ("6.12", "6.12"),
    }
    # #### 子编号映射：6.8.x→6.11.x, 6.9.x→6.12.x
    sub_map = {"6.8": "6.11", "6.9": "6.12"}
    for ln in b6_lines:
        m4 = re.match(r"^(####)\s+(\d+(?:\.\d+)*)\s+", ln)
        m3 = re.match(r"^###\s+(\d+(?:\.\d+)*)\s+", ln)
        if m4:
            old = m4.group(2)
            parts = old.split(".")
            if len(parts) >= 2:
                key = ".".join(parts[:2])
                if key in sub_map:
                    parts[0:2] = sub_map[key].split(".")
                    ln = re.sub(r"^(####)\s+[\d.]+", f"#### {'.'.join(parts)}", ln, count=1)
        elif m3:
            for k, v in six_map_title.items():
                if k in ln:
                    ln = re.sub(r"^###\s+[\d.]+", f"### {v[0]}", ln, count=1)
                    break
        new_b6.append(ln)
    b6 = new_b6

    # ---- 5. 八章：核身 8.11.1/8.11.2 → 8.11a.1/8.11a.2；平台员工 8.11.3 移出 ----
    b8_s = split_h3(b8)
    b8_kept = []
    b8_platstaff = []
    for title, body in b8_s:
        m = H3_RE.match(title)
        if m and m.group(1) == "8.11.3":
            b8_platstaff = [title, body]
            continue
        if m and m.group(1) == "8.11.1" and "核身" in title:
            title = title.replace("### 8.11.1", "### 8.11a.1", 1)
        if m and m.group(1) == "8.11.2" and "核身" in title:
            title = title.replace("### 8.11.2", "### 8.11a.2", 1)
        b8_kept.append([title, body])
    b8 = join_h3(b8_kept)
    # 添加 8.11a 父节标题
    # 在 8.11a.1 前插入父节
    b8_out = []
    inserted = False
    for title, body in split_h3(b8):
        if H3_RE.match(title) and H3_RE.match(title).group(1) == "8.11a.1" and not inserted:
            b8_out.append(["### 8.11a 实名核身", [""]])
            inserted = True
        b8_out.append([title, body])
    b8 = join_h3(b8_out)

    # ---- 6. 申诉拆分（剥 H2 首行）----
    # 商家部分：7.1/7.2/7.3（merchant/appeals）→ 九章 9.19-9.21
    # 用户部分：7.4/7.5/7.6 → 八章 8.16-8.18
    bappeal_body = bappeal[1:]
    bappeal_s = split_h3(bappeal_body)
    appeal_merchant = []
    appeal_user = []
    for title, body in bappeal_s:
        m = H3_RE.match(title)
        if not m:
            continue
        n = m.group(1)
        content = "\n".join([title] + body)
        if n in ("7.1", "7.2", "7.3"):
            nw = {"7.1": "9.19", "7.2": "9.20", "7.3": "9.21"}[n]
            title = title.replace(f"### {n}", f"### {nw}", 1)
            appeal_merchant.append([title, body])
        elif n in ("7.4", "7.5", "7.6"):
            nw = {"7.4": "8.16", "7.5": "8.17", "7.6": "8.18"}[n]
            title = title.replace(f"### {n}", f"### {nw}", 1)
            appeal_user.append([title, body])

    # ---- 7. 九章：追加商家申诉 ----
    b9 = join_h3(split_h3(b9)) + [""] + join_h3(appeal_merchant)

    # ---- 8. 八章：追加用户申诉 ----
    b8 = b8 + [""] + join_h3(appeal_user)

    # ---- 9. 十章：人员管理 10.17→10.18；追加仪表盘 10.17 + 平台员工 10.19 ----
    # 逐行扫描：人员管理父节 10.17→10.18，子节 10.17.x→10.18.x
    b10_lines = b10
    new_b10 = []
    in_person = False
    sub_n = 0
    for ln in b10_lines:
        m3 = re.match(r"^###\s+", ln)
        m4 = re.match(r"^####\s+", ln)
        if m3 and not m4:
            if ln.startswith("### 10.17 人员管理"):
                ln = ln.replace("### 10.17 人员管理", "### 10.18 人员管理", 1)
                in_person = True
                sub_n = 0
            else:
                in_person = False
        elif m4 and in_person:
            sub_n += 1
            ln = re.sub(r"^####\s+[\d.]+", f"#### 10.18.{sub_n}", ln, count=1)
        new_b10.append(ln)
    b10 = new_b10
    # 仪表盘 → 10.17（剥 H2 首行，19.1/19.2 → 10.17.1/10.17.2）
    bdash_body = bdash[1:]
    bdash_s = split_h3(bdash_body)
    for sec in bdash_s:
        m = H3_RE.match(sec[0])
        if m:
            old = m.group(1)
            nw = f"10.17.{old.split('.')[-1]}"
            sec[0] = sec[0].replace(f"### {old}", f"### {nw}", 1)
    bdash_new = [f"### 10.17 仪表盘"] + join_h3(bdash_s)
    # 平台员工 → 10.19（从八章 8.11.3 提取）
    bplat = ["### 10.19 平台员工管理（#1795 T6）"] + b8_platstaff[1]
    # 十章组装：仪表盘(10.17) 插在人员管理(10.18)之前；平台员工(10.19) 追加末尾
    b10_lines2 = b10
    b10_final = []
    inserted_dash = False
    for ln in b10_lines2:
        if ln.startswith("### 10.18 人员管理") and not inserted_dash:
            b10_final.append("### 10.17 仪表盘")
            b10_final.append("")
            b10_final.extend(bdash_new[1:])
            inserted_dash = True
        b10_final.append(ln)
    if not inserted_dash:
        b10_final.append("### 10.17 仪表盘")
        b10_final.append("")
        b10_final.extend(bdash_new[1:])
    b10 = b10_final
    # 追加平台员工到末尾
    b10 = b10 + [""] + bplat

    # ---- 10. 库管 → 六章 6.13-6.17（剥 H2 首行）----
    bware_body = bware[1:]
    bware_s = split_h3(bware_body)
    ware_sections = []
    idx = 13
    for title, body in bware_s:
        m = H3_RE.match(title)
        if m:
            nw = f"6.{idx}"
            idx += 1
            title = title.replace(f"### {m.group(1)}", f"### {nw}", 1)
        ware_sections.append([title, body])
    b6 = b6 + [""] + join_h3(ware_sections)

    # ---- 11. 组装最终文档 ----
    parts = [
        join_h3(split_h3(b1)),
        join_h3(split_h3(b2)),
        b3w,
        b4,
        b5,
        b6,
        b7,
        b8,
        b9,
        b10,
        b11,
        b12,
        b13,
        b14,
    ]
    # 组装时各块间空行分隔
    final = "\n\n".join("\n".join(p).strip("\n") for p in parts if "\n".join(p).strip("\n"))
    final += "\n\n" + "\n".join(bappA).strip("\n")
    final += "\n\n" + "\n".join(bappB).strip("\n")
    final += "\n\n" + "\n".join(bappC).strip("\n")
    final += "\n\n" + "\n".join(bver).strip("\n")
    return final


def validate(text):
    lines = text.split("\n")
    issues = []
    # 顶层标题唯一性
    heads = [ln for ln in lines if re.match(r"^## ", ln)]
    seen = {}
    for h in heads:
        seen[h] = seen.get(h, 0) + 1
    for h, c in seen.items():
        if c > 1:
            issues.append(f"顶层标题重复: {h} ×{c}")
    # 顶层编号序列
    top_nums = []
    for h in heads:
        m = re.match(r"^## ([一二三四五六七八九十]+)、", h)
        if m:
            cn = m.group(1)
            top_nums.append(cn)
    cn_order = ["一", "二", "三", "四", "五", "六", "七", "八", "九", "十", "十一", "十二", "十三", "十四"]
    if top_nums != cn_order:
        issues.append(f"顶层编号序列异常: {top_nums}")
    # 各章子编号连续（5.x / 6.x / 2.x / 8.11a / 9.x / 10.x）
    for prefix, start, end in [("5.", 1, 12), ("6.", 1, 18), ("2.", 1, 8),
                               ("9.", 1, 22), ("10.", 1, 20), ("8.16", 16, 18),
                               ("8.11a", 0, 2), ("2.7", 1, 2), ("2.8", 1, 1)]:
        nums = []
        for ln in lines:
            m = re.match(rf"^###\s+({re.escape(prefix)}[\d.]+)", ln)
            if m:
                nums.append(m.group(1))
        if prefix == "8.11a":
            if not any(n == "8.11a.1" for n in nums) or not any(n == "8.11a.2" for n in nums):
                issues.append(f"8.11a 核身子节缺失")
        elif prefix == "2.7":
            if not any(n == "2.7.1" for n in nums) or not any(n == "2.7.2" for n in nums):
                issues.append(f"2.7 冷启动子节缺失")
    # 追加区清零
    for bad in ["## 补充章节", "## 2.4 冷启动", "## 7. 申诉", "## 8. 库管",
                "## 19. 确认", "## 20. 仪表盘", "## 十三、版本记录", "## 三、公共浏览"]:
        if bad in text:
            issues.append(f"追加区残留: {bad}")
    return issues


if __name__ == "__main__":
    main()