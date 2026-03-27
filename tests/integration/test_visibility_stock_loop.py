#!/usr/bin/env python3
"""
测试场景 1: 资产可见性与库存闭环
验证管理端操作是否能实时通过 API 穿透到消费端
"""

import requests
import time
import os
import sys
from datetime import datetime

# 配置
PC_API_BASE = "http://localhost:5554/api"
WECHAT_API_BASE = "http://localhost:5554/api"

# 从环境变量获取 token
PC_TOKEN = os.getenv("PC_TOKEN", "")
WECHAT_TOKEN = os.getenv("WECHAT_TOKEN", "")

def log(message):
    """日志输出"""
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{timestamp}] {message}")

def test_visibility_stock_loop():
    """测试资产可见性与库存闭环"""
    
    log("\n" + "="*60)
    log("测试场景 1: 资产可见性与库存闭环")
    log("="*60 + "\n")
    
    # 检查 token
    if not PC_TOKEN or not WECHAT_TOKEN:
        log("❌ 错误: 需要设置环境变量 PC_TOKEN 和 WECHAT_TOKEN")
        log("示例: export PC_TOKEN='your_owner_token' WECHAT_TOKEN='your_customer_token'")
        return False
    
    instrument_id = None
    
    try:
        # Step 1: PC端创建乐器 A，库存设为 1，状态设为 available
        log("Step 1: PC端创建乐器 A...")
        instrument_data = {
            "name": f"测试钢琴 - {int(time.time())}",
            "brand": "雅马哈",
            "level": "professional",
            "category_id": "fc53b5cc-3778-4980-8db1-499ac08c8bf2",  # 需要替换为实际category_id
            "stock_status": "available",
            "pricing": {
                "daily_rate": 50,
                "weekly_rate": 300,
                "monthly_rate": 1200
            }
        }
        
        response = requests.post(
            f"{PC_API_BASE}/instruments",
            headers={"Authorization": f"Bearer {PC_TOKEN}"},
            json=instrument_data,
            timeout=10
        )
        
        if response.status_code != 201:
            log(f"❌ 创建乐器失败: {response.status_code} - {response.text}")
            return False
        
        instrument = response.json().get("data", {})
        instrument_id = instrument.get("id")
        
        if not instrument_id:
            log("❌ 无法获取创建的乐器ID")
            return False
        
        log(f"✓ 创建乐器成功: {instrument.get('name', 'Unknown')} (ID: {instrument_id})")
        
        # Step 2: 微信端查询乐器列表，断言乐器 A 出现在列表中
        log("\nStep 2: 微信端查询乐器列表...")
        time.sleep(1)  # 等待数据同步
        
        response = requests.get(
            f"{WECHAT_API_BASE}/instruments",
            headers={"Authorization": f"Bearer {WECHAT_TOKEN}"},
            timeout=10
        )
        
        if response.status_code != 200:
            log(f"❌ 查询乐器失败: {response.status_code} - {response.text}")
            return False
        
        instruments = response.json().get("data", [])
        
        found = any(i.get("id") == instrument_id for i in instruments)
        if not found:
            log(f"❌ 微信端未找到乐器 A (ID: {instrument_id})")
            log(f"返回的乐器列表: {[i.get('id') for i in instruments]}")
            return False
        
        log(f"✓ 微信端找到乐器 A")
        
        # Step 3: PC端下架乐器
        log("\nStep 3: PC端下架乐器...")
        update_data = {
            "stock_status": "unavailable"
        }
        
        response = requests.put(
            f"{PC_API_BASE}/instruments/{instrument_id}/status",
            headers={"Authorization": f"Bearer {PC_TOKEN}"},
            json=update_data,
            timeout=10
        )
        
        if response.status_code != 200:
            log(f"❌ 下架乐器失败: {response.status_code} - {response.text}")
            return False
        
        log(f"✓ 下架乐器成功")
        
        # Step 4: 微信端再次查询，断言乐器 A 消失
        log("\nStep 4: 微信端再次查询乐器列表...")
        time.sleep(1)  # 等待数据同步
        
        response = requests.get(
            f"{WECHAT_API_BASE}/instruments",
            headers={"Authorization": f"Bearer {WECHAT_TOKEN}"},
            timeout=10
        )
        
        if response.status_code != 200:
            log(f"❌ 查询乐器失败: {response.status_code} - {response.text}")
            return False
        
        instruments = response.json().get("data", [])
        
        found = any(i.get("id") == instrument_id for i in instruments)
        if found:
            log(f"❌ 微信端仍然显示已下架的乐器 A (ID: {instrument_id})")
            return False
        
        log(f"✓ 微信端已看不到下架的乐器 A")
        
        # 清理
        log("\nStep 5: 清理测试数据...")
        response = requests.delete(
            f"{PC_API_BASE}/instruments/{instrument_id}",
            headers={"Authorization": f"Bearer {PC_TOKEN}"},
            timeout=10
        )
        
        if response.status_code == 200:
            log(f"✓ 清理测试数据成功")
        else:
            log(f"⚠ 清理测试数据失败: {response.status_code}")
        
        log("\n" + "="*60)
        log("场景 1 测试通过: ✅")
        log("="*60 + "\n")
        
        return True
        
    except Exception as e:
        log(f"❌ 测试异常: {str(e)}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == "__main__":
    success = test_visibility_stock_loop()
    sys.exit(0 if success else 1)
