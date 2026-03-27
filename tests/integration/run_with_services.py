#!/usr/bin/env python3
"""
测试执行包装器 - 自动启动服务、运行测试、关闭服务

用法:
    python run_with_services.py [-t TEST_SCRIPT] [-c CONFIG]
    
示例:
    # 运行默认测试
    python run_with_services.py
    
    # 指定测试脚本
    python run_with_services.py -t test_visibility_stock_loop.py
"""

import subprocess
import sys
import time
import signal
import os
import argparse

def start_services():
    """启动后端服务"""
    print("🚀 启动后端服务...")
    proc = subprocess.Popen(
        ["make", "run"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        cwd=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    )
    
    # 等待服务启动
    print("⏳ 等待服务就绪 (10秒)...")
    time.sleep(10)
    
    return proc

def stop_services(proc):
    """停止后端服务"""
    print("\n🛑 停止后端服务...")
    proc.send_signal(signal.SIGTERM)
    proc.wait()
    print("✓ 服务已停止")

def run_tests(test_script, config_file):
    """运行测试脚本"""
    print(f"🧪 运行集成测试: {test_script}...")
    cmd = [sys.executable, test_script]
    if config_file:
        cmd.extend(["-c", config_file])
    
    result = subprocess.run(
        cmd,
        cwd=os.path.dirname(os.path.abspath(__file__))
    )
    return result.returncode

def main():
    parser = argparse.ArgumentParser(
        description="测试执行包装器 - 自动管理服务生命周期"
    )
    
    parser.add_argument("-t", "--test",
                        default="test_visibility_stock_loop.py",
                        help="测试脚本 (默认: test_visibility_stock_loop.py)")
    
    parser.add_argument("-c", "--config",
                        default="config.yaml",
                        help="配置文件 (默认: config.yaml)")
    
    args = parser.parse_args()
    
    proc = None
    try:
        # 启动服务
        proc = start_services()
        
        # 运行测试
        exit_code = run_tests(args.test, args.config)
        
        # 停止服务
        stop_services(proc)
        
        sys.exit(exit_code)
        
    except KeyboardInterrupt:
        print("\n\n收到中断信号...")
        if proc:
            stop_services(proc)
        sys.exit(130)
    except Exception as e:
        print(f"\n\n错误: {e}")
        if proc:
            stop_services(proc)
        import traceback
        traceback.print_exc()
        sys.exit(1)

if __name__ == "__main__":
    main()
