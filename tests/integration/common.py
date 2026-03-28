#!/usr/bin/env python3
"""
Tuneloop Integration Test Common Module

提供通用的测试基础设施：
- 配置加载
- Token 管理
- 账户管理
"""

import os
import sys
import argparse
import yaml
import requests
import time
from typing import Dict, List, Optional, Any
from dataclasses import dataclass
from pathlib import Path


@dataclass
class Account:
    """账户配置"""
    email: str
    password: str
    role: str
    name: str = ""
    tenant: str = ""
    token: Optional[str] = None


@dataclass
class TestConfig:
    """测试配置"""
    api_base_url: str
    iam_url: str
    client_id: str
    redirect_uri: str
    accounts: List[Account]
    log_level: str = "INFO"
    continue_on_error: bool = False
    cleanup_after_test: bool = True


class TokenManager:
    """Token 管理器"""
    
    def __init__(self, config: TestConfig):
        self.config = config
        self.tokens: Dict[str, str] = {}
    
    def get_token(self, account: Account) -> str:
        """获取账户的 token"""
        if account.token:
            return account.token
        
        print(f"  正在获取 token for {account.email}...")
        
        try:
            # 1. 先尝试直接调用 backend callback（如果 IAM 已经授权）
            callback_url = f"{self.config.redirect_uri}"
            
            # 简化：调用 IAM login endpoint（beaconiam uses /api/oauth/login not /oauth/token）
            token_url = f"{self.config.iam_url}/api/oauth/login"
            
            # 准备请求数据
            data = {
                "grant_type": "password",
                "username": account.email,
                "password": account.password,
                "client_id": self.config.client_id,
                "scope": "openid profile email"
            }
            
            headers = {
                "Content-Type": "application/json"
            }
            
            response = requests.post(token_url, json=data, headers=headers, timeout=30)
            
            if response.status_code == 200:
                token_data = response.json()
                account.token = token_data.get("access_token")
                print(f"    ✓ Token 获取成功")
            else:
                print(f"    ⚠ Token 获取失败: {response.status_code}")
                print(f"    响应: {response.text}")
                
        except Exception as e:
            print(f"    ⚠ 获取 token 异常: {e}")
        
        return account.token or ""
    
    def get_token_by_role(self, role: str) -> Optional[str]:
        """根据角色获取 token"""
        for account in self.config.accounts:
            if account.role == role:
                return self.get_token(account)
        return None


def load_config(config_path: str = "config.yaml") -> TestConfig:
    """加载测试配置"""
    
    base_dir = os.path.dirname(os.path.abspath(__file__))
    search_paths = [
        config_path,
        os.path.join(base_dir, config_path),
        os.path.join(base_dir, "config.yaml"),
        os.path.join(os.getcwd(), config_path),
        os.path.join(os.getcwd(), "config.yaml"),
    ]
    
    found_path = None
    for path in search_paths:
        if os.path.exists(path):
            found_path = path
            break
    
    if not found_path:
        print(f"❌ 错误: 找不到配置文件: {config_path}")
        print(f"搜索路径: {search_paths}")
        sys.exit(1)
    
    with open(found_path, 'r', encoding='utf-8') as f:
        data = yaml.safe_load(f)
    
    accounts = []
    for acc_data in data.get("accounts", []):
        accounts.append(Account(
            email=acc_data.get("email", ""),
            password=acc_data.get("password", ""),
            role=acc_data.get("role", "USER"),
            name=acc_data.get("name", ""),
            tenant=acc_data.get("tenant", "")
        ))
    
    config = TestConfig(
        api_base_url=data.get("api", {}).get("base_url", "http://localhost:5554/api"),
        iam_url=data.get("iam", {}).get("url", "http://localhost:5552"),
        client_id=data.get("iam", {}).get("client_id", "tuneloop"),
        redirect_uri=data.get("iam", {}).get("redirect_uri", "http://localhost:5554/callback"),
        accounts=accounts,
        log_level=data.get("settings", {}).get("log_level", "INFO"),
        continue_on_error=data.get("settings", {}).get("continue_on_error", False),
        cleanup_after_test=data.get("settings", {}).get("cleanup_after_test", True),
    )
    
    print(f"✓ 配置加载成功: {found_path}")
    print(f"  账户数: {len(accounts)}")
    for acc in config.accounts:
        print(f"    - {acc.name} ({acc.role})")
    
    return config


def create_api_client(config: TestConfig, account: Account) -> requests.Session:
    """创建带有认证的 API 客户端"""
    
    session = requests.Session()
    session.headers.update({
        "Content-Type": "application/json",
    })
    
    token = account.token or get_token_manager(config).get_token(account)
    
    if token:
        session.headers.update({
            "Authorization": f"Bearer {token}"
        })
    
    return session


def get_token_manager(config: TestConfig) -> TokenManager:
    """获取 TokenManager 实例"""
    if not hasattr(config, '_token_manager'):
        config._token_manager = TokenManager(config)
    return config._token_manager


def run_tests_for_all_accounts(config: TestConfig, test_func):
    """为所有账户运行测试"""
    
    results = {}
    token_manager = get_token_manager(config)
    
    for i, account in enumerate(config.accounts, 1):
        print(f"\n{'='*70}")
        print(f"账户 {i}/{len(config.accounts)}: {account.name} ({account.role})")
        print(f"{'='*70}\n")
        
        try:
            print(f"正在为账户 {account.email} 创建 API 客户端...")
            client = create_api_client(config, account)
            
            print(f"开始执行测试...")
            result = test_func(client, config, account)
            
            # FIX: Check actual result status
            status = result.get("status", "pass") if isinstance(result, dict) else "pass"
            
            results[account.email] = {
                "status": status.upper(),
                "account": account.name,
                "role": account.role,
                "result": result
            }
            
            if status == "pass":
                print(f"\n✓ 测试通过")
            else:
                print(f"\n❌ 测试失败: {result.get('error', 'Unknown error')}")
            
        except Exception as e:
            print(f"\n❌ 测试失败: {e}\n")
            import traceback
            traceback.print_exc()
            
            results[account.email] = {
                "status": "FAIL",
                "account": account.name,
                "role": account.role,
                "error": str(e)
            }
            
            if not config.continue_on_error:
                print("停止执行（continue_on_error = false）")
                break
    
    return results


def log(message):
    """日志输出"""
    timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] {message}")
