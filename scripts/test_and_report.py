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
import json
import tempfile
from pathlib import Path

def parse_issue_url(url: str) -> int:
    """从 Issue URL 提取编号"""
    match = re.search(r'/issues/(\d+)', url)
    if not match:
        raise ValueError(f"Invalid issue URL: {url}")
    return int(match.group(1))

def run_test_script(script_path: str, issue_url: str) -> tuple[str, int]:
    """执行测试脚本并返回输出"""
    if not os.path.exists(script_path):
        raise FileNotFoundError(f"Script not found: {script_path}")
    
    # 获取测试脚本目录
    test_dir = os.path.dirname(os.path.abspath(script_path))
    
    # 检查是否有 run_with_services.py
    wrapper_script = os.path.join(test_dir, "run_with_services.py")
    
    if os.path.exists(wrapper_script):
        # 使用服务包装器并捕获所有输出到文件
        print("🚀 使用 run_with_services.py 管理服务生命周期...")
        
        log_file = os.path.join(test_dir, "log.txt")
        
        # FIX: 删除旧的日志文件（如果存在）
        if os.path.exists(log_file):
            print(f"🧹 删除旧的日志文件: {log_file}")
            os.unlink(log_file)
        
        # 捕获输出到日志文件
        with open(log_file, 'w') as f:
            result = subprocess.run(
                [sys.executable, wrapper_script, "-t", os.path.basename(script_path)],
                stdout=f,
                stderr=subprocess.STDOUT,
                text=True,
                timeout=600
            )
        
        # 读取日志文件内容
        with open(log_file, 'r') as f:
            output = f.read()
        
        # 删除日志文件（清理）
        os.unlink(log_file)
        
        return output, result.returncode
    else:
        # 回退: 直接运行测试脚本
        print("⚠️  未找到 run_with_services.py，直接运行测试脚本...")
        result = subprocess.run(
            [sys.executable, script_path],
            capture_output=True,
            text=True,
            timeout=300
        )
        output = result.stdout + result.stderr
        return output, result.returncode

def analyze_with_opencode(issue_url: str, test_script_path: str, test_output: str) -> str:
    """使用 opencode 分析测试结果"""
    
    # 限制输出长度（避免过大）
    max_output_length = 50000  # 50KB limit
    truncated_output = test_output[:max_output_length]
    
    if len(test_output) > max_output_length:
        truncated_output += f"\n\n... (output truncated, {len(test_output) - max_output_length} chars omitted)"
    
    # 创建分析请求
    analysis_request = f"""# Test Analysis Request

## Issue URL
{issue_url}

## Test Script
Path: {test_script_path}

## Test Output Summary
```
{truncated_output}
```

## Analysis Task
Please analyze this test output and identify:
1. Most relevant error messages
2. Root causes
3. Suggested fixes
4. Whether the test passed or failed and why
"""
    
    with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
        f.write(analysis_request)
        request_file = f.name
    
    try:
        # 调用 opencode 命令行工具
        result = subprocess.run(
            ['opencode', 'analyze', request_file],
            capture_output=True,
            text=True,
            timeout=60
        )
        
        if result.returncode == 0:
            return result.stdout.strip()
        else:
            return f"Analysis failed: {result.stderr}"
    finally:
        os.unlink(request_file)

def get_issue_content(issue_number: int) -> str:
    """使用 gh CLI 获取 Issue 内容"""
    cmd = ['gh', 'issue', 'view', str(issue_number), '--json', 'title,body']
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"Failed to get issue: {result.stderr}")
    data = json.loads(result.stdout)
    return f"Title: {data.get('title', '')}\n\nBody: {data.get('body', '')}"

def read_test_script(script_path: str) -> str:
    """读取测试脚本源码"""
    with open(script_path, 'r') as f:
        return f.read()

def add_comment_to_issue(issue_number: int, comment: str):
    """使用 gh CLI 添加评论"""
    cmd = ['gh', 'issue', 'comment', str(issue_number), '-b', comment]
    result = subprocess.run(cmd, capture_output=True, text=True)
    
    if result.returncode != 0:
        raise RuntimeError(f"Failed to add comment: {result.stderr}")

def update_issue_labels(issue_number: int, add_label: str = "status:todo"):
    """更新 Issue 标签：移除所有标签，仅添加指定标签"""
    # 1. 先获取当前所有标签
    cmd = ['gh', 'issue', 'view', str(issue_number), '--json', 'labels']
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"Failed to get issue labels: {result.stderr}")
    
    data = json.loads(result.stdout)
    current_labels = [label['name'] for label in data.get('labels', [])]
    
    # 2. 移除所有现有标签
    if current_labels:
        for label in current_labels:
            cmd = ['gh', 'issue', 'edit', str(issue_number), '--remove-label', label]
            result = subprocess.run(cmd, capture_output=True, text=True)
            if result.returncode != 0:
                print(f"⚠️ 警告: 移除标签 {label} 失败: {result.stderr}")
    
    # 3. 添加 status:todo 标签
    cmd = ['gh', 'issue', 'edit', str(issue_number), '--add-label', add_label]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"Failed to add label: {result.stderr}")
    
    print(f"✅ 标签已更新: {add_label}")

def main():

def main():
    parser = argparse.ArgumentParser(
        description="Execute test script and generate analysis report"
    )
    parser.add_argument("issue_url", help="GitHub Issue URL")
    parser.add_argument("test_script", help="Test script path")
    
    args = parser.parse_args()
    
    issue_number = parse_issue_url(args.issue_url)
    print(f"📋 Issue number: #{issue_number}")
    print(f"🧪 Test script: {args.test_script}")
    
    print("\n📖 Getting Issue content...")
    try:
        issue_content = get_issue_content(issue_number)
        print("✅ Issue content retrieved")
    except Exception as e:
        print(f"❌ Failed to get Issue: {e}")
        sys.exit(1)
    
    print("\n📄 Reading test script...")
    try:
        test_script_content = read_test_script(args.test_script)
        print("✅ Test script read")
    except Exception as e:
        print(f"❌ Failed to read test script: {e}")
        sys.exit(1)
    
    print("\n🚀 Starting test...")
    try:
        output, exit_code = run_test_script(args.test_script, args.issue_url)
    except Exception as e:
        print(f"❌ Test execution failed: {e}")
        sys.exit(1)
    
    print(f"✅ Test execution completed (exit code: {exit_code})")
    
    print("\n📊 Analyzing results...")
    try:
        analysis = analyze_with_opencode(args.issue_url, args.test_script, output)
        print("✅ Analysis completed")
    except Exception as e:
        print(f"⚠️ Analysis failed: {e}")
        analysis = "Opencode analysis failed. Please check logs manually."
    
    comment_body = f"""## Test Execution Report

**Test script**: `{args.test_script}`
**Status**: {✅ PASSED if exit_code == 0 else ❌ FAILED} (exit code: {exit_code})

### Opencode Analysis

{analysis}

---
*Auto-generated by test_and_report.py* (powered by opencode)
"""
    
    print("\n💬 Adding comment to Issue...")
    try:
        add_comment_to_issue(issue_number, comment_body)
        print(f"✅ Comment added to Issue #{issue_number}")
    except Exception as e:
        print(f"❌ Failed to add comment: {e}")
        sys.exit(1)
    
    print("\n🏷️ Updating Issue labels...")
    try:
        update_issue_labels(issue_number, "status:todo")
    except Exception as e:
        print(f"⚠️ Warning: Failed to update label: {e}")
        print("   Continuing...")

if __name__ == __main__:
    main()

