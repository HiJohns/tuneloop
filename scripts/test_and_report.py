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

def run_test_script(script_path: str) -> tuple[str, int]:
    """执行测试脚本并返回输出"""
    if not os.path.exists(script_path):
        raise FileNotFoundError(f"Script not found: {script_path}")
    
    # 获取测试脚本目录
    test_dir = os.path.dirname(os.path.abspath(script_path))
    
    # 检查是否有 run_with_services.py
    wrapper_script = os.path.join(test_dir, "run_with_services.py")
    
    # 添加时间戳标记
    start_marker = f"=== TEST RUN START {time.strftime('%Y-%m-%d %H:%M:%S')} ==="
    end_marker = "=== TEST RUN END ==="
    
    if os.path.exists(wrapper_script):
        # 使用服务包装器并捕获所有输出到文件
        print("🚀 使用 run_with_services.py 管理服务生命周期...")
        
        # 创建 log.txt 文件
        log_file = os.path.join(test_dir, "log.txt")
        
        with open(log_file, 'w') as f:
            f.write(start_marker + "\n")
            result = subprocess.run(
                [sys.executable, wrapper_script, "-t", os.path.basename(script_path)],
                stdout=f,
                stderr=subprocess.STDOUT,
                text=True,
                timeout=600
            )
            f.write("\n" + end_marker + "\n")
        
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

def analyze_with_opencode(test_output: str) -> str:
    """调用 opencode 分析测试输出"""
    with tempfile.NamedTemporaryFile(mode='w', suffix='.txt', delete=False) as f:
        f.write(test_output)
        temp_file = f.name
    
    try:
        lines = test_output.split('\n')
        summary_lines = []
        
        for line in lines:
            if any(kw in line for kw in ['✅', '❌', 'PASS', 'FAIL', 'ERROR', '结果', '404', '401', '500']):
                summary_lines.append(line)
            if 'api' in line.lower() or 'http' in line.lower() or '/api/' in line:
                summary_lines.append(line)
        
        if summary_lines:
            return '\n'.join(summary_lines[:50])
        else:
            return "测试执行完成，请查看完整输出。"
    finally:
        os.unlink(temp_file)

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

def analyze_with_gemini(issue_content: str, test_script: str, test_output: str) -> str:
    """调用 Gemini API 分析测试结果"""
    from google import genai
    
    api_key = os.environ.get('GEMINI_API_KEY')
    if not api_key:
        raise ValueError("GEMINI_API_KEY environment variable not set")
    
    client = genai.Client(api_key=api_key)
    
    # 提取"本次运行"的输出（最后 3000 字符）
    recent_output = test_output[-3000:]
    
    prompt = f"""请分析以下测试场景（仅分析本次运行的输出）：

## Issue 描述
{issue_content}

## 测试脚本源码
```python
{test_script}
```

## 测试执行结果（本次运行）
```
{recent_output}
```

请提供：
1. 测试是否通过？
2. 如果失败，分析可能的原因
3. 建议的下一步操作

请用简洁的 Markdown 格式输出。"""
    
    response = client.models.generate_content(
        model="gemini-2.0-flash",
        contents=prompt
    )
    
    return response.text

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
    parser = argparse.ArgumentParser(
        description='执行测试脚本并报告结果到 GitHub Issue'
    )
    parser.add_argument('issue_url', help='GitHub Issue URL')
    parser.add_argument('test_script', help='测试脚本路径')
    
    args = parser.parse_args()
    
    issue_number = parse_issue_url(args.issue_url)
    print(f"📋 Issue 编号: #{issue_number}")
    print(f"🧪 测试脚本: {args.test_script}")
    
    api_key = os.environ.get('GEMINI_API_KEY')
    if not api_key:
        print("⚠️ 警告: GEMINI_API_KEY 未设置，将使用简化分析")
        use_gemini = False
    else:
        print("✅ GEMINI_API_KEY 已配置")
        use_gemini = True
    
    print("\n📖 正在获取 Issue 内容...")
    try:
        issue_content = get_issue_content(issue_number)
        print("✅ Issue 内容获取成功")
    except Exception as e:
        print(f"❌ 获取 Issue 内容失败: {e}")
        sys.exit(1)
    
    print("\n📄 正在读取测试脚本...")
    try:
        test_script_content = read_test_script(args.test_script)
        print("✅ 测试脚本读取成功")
    except Exception as e:
        print(f"❌ 读取测试脚本失败: {e}")
        sys.exit(1)
    
    print("\n🚀 开始执行测试...")
    try:
        output, exit_code = run_test_script(args.test_script)
    except Exception as e:
        print(f"❌ 测试执行失败: {e}")
        sys.exit(1)
    
    print(f"✅ 测试执行完成 (exit code: {exit_code})")
    
    print("\n📊 正在分析输出...")
    try:
        if use_gemini:
            analysis = analyze_with_gemini(issue_content, test_script_content, output)
            print("✅ Gemini 分析完成")
        else:
            analysis = analyze_with_opencode(output)
            print("✅ 简化分析完成")
    except Exception as e:
        print(f"⚠️ 分析失败，使用回退方案: {e}")
        analysis = analyze_with_opencode(output)
    
    # 准备调试信息（前100行测试输出）
    debug_lines = output.split('\n')[:100]
    debug_output = '\n'.join(debug_lines)
    
    comment_body = f"""## 测试执行报告

**测试脚本**: `{args.test_script}`
**执行状态**: {'✅ 成功' if exit_code == 0 else '❌ 失败'} (exit code: {exit_code})

### 分析结果

{analysis}

### 调试信息

<details>
<summary>点击查看完整测试输出（前100行）</summary>

```
{debug_output}
```
</details>

---
**API Endpoint 信息**: 请查看上方调试信息中的 URL 和 HTTP 方法
**状态码**: 请查看调试信息中的 HTTP 状态码（如 401, 404, 500 等）

*自动生成于 test_and_report.py* (Enhanced with Gemini API & Debug Info)
"""
    
    print("\n💬 正在添加评论到 Issue...")
    try:
        add_comment_to_issue(issue_number, comment_body)
        print(f"✅ 评论已添加到 Issue #{issue_number}")
    except Exception as e:
        print(f"❌ 添加评论失败: {e}")
        sys.exit(1)
    
    print("\n🏷️ 正在更新 Issue 标签...")
    try:
        update_issue_labels(issue_number, "status:todo")
    except Exception as e:
        print(f"⚠️ 警告: 更新标签失败: {e}")
        print("   继续执行，不影响评论发布")

if __name__ == '__main__':
    main()
