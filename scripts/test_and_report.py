#!/usr/bin/env python3
"""
test_and_report.py - 执行测试脚本并自动报告结果到 GitHub Issue

Usage:
    python test_and_report.py <issue_url> <test_script>
    
Example:
    python test_and_report.py https://github.com/HiJohns/tuneloop/issues/127 tests/integration/test_visibility_stock_loop.py
"""

import argparse
import subprocess
import sys
import os
import re
import tempfile
from pathlib import Path

def parse_issue_url(url: str) -> int:
    """从 Issue URL 提取编号"""
    match = re.search(r'/issues/(\d+)', url)
    if not match:
        raise ValueError(f"Invalid issue URL: {url}")
    return int(match.group(1))

def run_test_script(script_path: str) -> tuple[str, int]:
    """执行测试脚本并返回输出"""
    if not os.path.exists(script_path):
        raise FileNotFoundError(f"Script not found: {script_path}")
    
    result = subprocess.run(
        [sys.executable, script_path],
        capture_output=True,
        text=True,
        timeout=300
    )
    
    output = result.stdout + result.stderr
    return output, result.returncode

def analyze_with_opencode(test_output: str) -> str:
    """调用 opencode 分析测试输出"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False) as f:
        f.write(test_output)
        temp_file = f.name
    
    try:
        lines = test_output.split('\n')
        summary_lines = []
        
        for line in lines:
            if any(kw in line for kw in ['✅', '❌', 'PASS', 'FAIL', 'ERROR', '结果']):
                summary_lines.append(line)
        
        if summary_lines:
            return '\n'.join(summary_lines[:20])
        else:
            return "测试执行完成，请查看完整输出。"
    finally:
        os.unlink(temp_file)

def add_comment_to_issue(issue_number: int, comment: str):
    """使用 gh CLI 添加评论"""
    cmd = ['gh', 'issue', 'comment', str(issue_number), '-b', comment]
    result = subprocess.run(cmd, capture_output=True, text=True)
    
    if result.returncode != 0:
        raise RuntimeError(f"Failed to add comment: {result.stderr}")

def main():
    parser = argparse.ArgumentParser(
        description='执行测试脚本并报告结果到 GitHub Issue'
    )
    parser.add_argument('issue_url', help='GitHub Issue URL')
    parser.add_argument('test_script', help='测试脚本路径')
    
    args = parser.parse_args()
    
    issue_number = parse_issue_url(args.issue_url)
    print(f"📋 Issue 编号: #{issue_number}")
    print(f"🧪 测试脚本: {args.test_script}")
    
    print("\n🚀 开始执行测试...")
    try:
        output, exit_code = run_test_script(args.test_script)
    except Exception as e:
        print(f"❌ 测试执行失败: {e}")
        sys.exit(1)
    
    print(f"✅ 测试执行完成 (exit code: {exit_code})")
    
    print("\n📊 正在分析输出...")
    analysis = analyze_with_opencode(output)
    
    comment_body = f"""## 测试执行报告

**测试脚本**: `{args.test_script}`
**执行状态**: {'✅ 成功' if exit_code == 0 else '❌ 失败'} (exit code: {exit_code})

### 结果摘要

{analysis}

---
*自动生成于 test_and_report.py*"""
    
    print("\n💬 正在添加评论到 Issue...")
    try:
        add_comment_to_issue(issue_number, comment_body)
        print(f"✅ 评论已添加到 Issue #{issue_number}")
    except Exception as e:
        print(f"❌ 添加评论失败: {e}")
        sys.exit(1)

if __name__ == '__main__':
    main()
