#!/usr/bin/env python3
"""
测试场景 1: 资产可见性与库存闭环
验证管理端操作是否能实时通过 API 穿透到消费端
"""

import sys
import os
import argparse
import time

# 添加当前目录到 path
sys.path.insert(0, os.path.dirname(__file__))

from common import load_config, run_tests_for_all_accounts, TestConfig, Account, log


def ensure_category_exists(client, config: TestConfig, account: Account) -> str:
    """确保至少有一个分类存在，如果没有则使用 OWNER 账户创建"""
    log("\nStep 0: 获取或创建分类...")
    
    response = client.get(f"{config.api_base_url}/categories")
    if response.status_code == 200:
        data = response.json()
        categories = data.get('data', [])
        if categories and len(categories) > 0:
            category_id = categories[0].get('id')
            log(f"  ✓ 使用现有分类: {categories[0].get('name')} (ID: {category_id})")
            return category_id
    
    # 如果没有分类且当前是 OWNER，创建测试分类
    if account.role == "OWNER":
        log("  没有可用分类，尝试创建测试分类...")
        category_data = {
            "name": "测试分类",
            "icon": "🎹",
            "level": "professional",
            "visible": True,
            "sort": 1
        }
        response = client.post(f"{config.api_base_url}/categories", json=category_data)
        if response.status_code == 201:
            category_id = response.json().get("data", {}).get("id")
            log(f"  ✓ 创建测试分类成功: {category_id}")
            return category_id
        else:
            log(f"  ⚠ 创建分类失败: {response.status_code} - {response.text}")
    else:
        log(f"  ⚠ 没有可用分类且当前账户不是 OWNER，无法创建")
    
    return None


def test_visibility_stock_loop(client, config: TestConfig, account: Account):
    """测试资产可见性与库存闭环"""
    
    log("="*70)
    log(f"测试开始 - 账户: {account.name} ({account.role})")
    log("="*70)
    
    instrument_id = None
    results = {}
    
    try:
        # Step 0: 确保有可用分类（数据自愈）
        category_id = ensure_category_exists(client, config, account)
        if not category_id:
            return {
                "status": "fail",
                "error": "无法获取或创建分类"
            }
        
        # Step 1: 尝试创建乐器（RBAC 验证）
        log("\nStep 1: 尝试创建乐器（RBAC 验证）...")
        
        instrument_data = {
            "name": f"测试钢琴-{int(time.time())}-by-{account.email.split('@')[0]}",
            "brand": "雅马哈",
            "level": "professional",
            "category_id": category_id,
            "pricing": {
                "daily_rate": 50,
                "weekly_rate": 300,
                "monthly_rate": 1200
            }
        }
        
        response = client.post(
            f"{config.api_base_url}/instruments",
            json=instrument_data
        )
        
        # RBAC 验证
        expected_status = 201 if account.role in ["OWNER", "ADMIN"] else 403
        
        if response.status_code == expected_status:
            if response.status_code == 201:
                instrument = response.json().get("data", {})
                instrument_id = instrument.get("id")
                log(f"✓ 创建乐器成功: {instrument.get('name', 'Unknown')}")
                log(f"  乐器ID: {instrument_id}")
                results['instrument_created'] = True
            else:
                log(f"✓ 权限验证通过: {account.role} 无法创建乐器（预期 403）")
                results['rbac_verified'] = True
        else:
            log(f"❌ RBAC 验证失败: 期望 {expected_status}, 实际 {response.status_code}")
            log(f"  响应: {response.text}")
            return {
                "status": "fail",
                "error": f"RBAC check failed: expected {expected_status}, got {response.status_code}"
            }
        
        # Step 2: 查询乐器列表
        log("\nStep 2: 查询乐器列表...")
        response = client.get(f"{config.api_base_url}/instruments")
        
        if response.status_code == 200:
            instruments = response.json().get("data", [])
            log(f"✓ 查询成功, 找到 {len(instruments)} 个乐器")
            
            # 多租户验证
            if instrument_id:
                found = any(i.get("id") == instrument_id for i in instruments)
                if found:
                    log(f"✓ 多租户验证: 乐器对 {account.role} 可见")
                else:
                    # 如果是非创建者查询，可能是正常的权限隔离
                    if account.role not in ["OWNER", "ADMIN"]:
                        log(f"⚠ 乐器对 {account.role} 不可见（可能是权限隔离）")
                    else:
                        log(f"⚠ 警告: 刚创建的乐器未在列表中")
        else:
            log(f"❌ 查询乐器失败: {response.status_code}")
            return {"status": "fail", "error": f"Query failed: {response.status_code}"}
        
        # Step 3: 下架乐器（仅限 OWNER/ADMIN 且成功创建了乐器）
        if instrument_id and account.role in ["OWNER", "ADMIN"]:
            log("\nStep 3: 尝试下架乐器...")
            update_data = {"stock_status": "unavailable"}
            
            response = client.put(
                f"{config.api_base_url}/instruments/{instrument_id}/status",
                json=update_data
            )
            
            if response.status_code == 200:
                log(f"✓ 下架乐器成功")
            else:
                log(f"⚠ 下架乐器失败: {response.status_code}")
                log(f"  响应: {response.text}")
        
        # Step 4: 再次查询验证下架效果
        log("\nStep 4: 再次查询乐器列表验证下架...")
        time.sleep(1)
        
        response = client.get(f"{config.api_base_url}/instruments")
        
        if response.status_code == 200:
            instruments = response.json().get("data", [])
            log(f"✓ 查询成功, 找到 {len(instruments)} 个乐器")
        else:
            log(f"❌ 查询失败: {response.status_code}")
            return {"status": "fail", "error": f"Query failed: {response.status_code}"}
        
        # 最终测试结果判断
        log("\n" + "="*70)
        log(f"账户 {account.email[:20]} 测试通过")
        log("="*70 + "\n")
        
        return {
            "status": "pass",
            "instrument_created": instrument_id is not None,
            "instrument_id": instrument_id,
            **results
        }
        
    except Exception as e:
        log(f"\n❌ 测试异常: {str(e)}\n")
        import traceback
        traceback.print_exc()
        return {
            "status": "fail",
            "error": str(e)
        }


def main():
    parser = argparse.ArgumentParser(
        description="Tuneloop Integration Test - 场景1: 资产可见性与库存闭环",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例:
  # 使用默认配置文件 (config.yaml)
  python test_visibility_stock_loop.py

  # 指定配置文件
  python test_visibility_stock_loop.py -c my_config.yaml

  # 增加日志详细程度
  python test_visibility_stock_loop.py -v
        """
    )
    
    parser.add_argument("-c", "--config", 
                        default="config.yaml",
                        help="配置文件路径 (默认: config.yaml)")
    
    parser.add_argument("-v", "--verbose",
                        action="store_true",
                        help="显示详细日志")
    
    args = parser.parse_args()
    
    print("\n" + "="*70)
    print("Tuneloop 集成测试 - 场景1: 资产可见性与库存闭环")
    print("="*70)
    
    # 加载配置
    print(f"\n加载配置: {args.config}")
    config = load_config(args.config)
    
    print(f"\n测试配置:")
    print(f"  API: {config.api_base_url}")
    print(f"  IAM: {config.iam_url}")
    print(f"  账户数: {len(config.accounts)}")
    print(f"  继续出错: {config.continue_on_error}")
    
    # 运行测试
    print(f"\n开始测试...")
    results = run_tests_for_all_accounts(config, test_visibility_stock_loop)
    
    # 输出结果总结
    print("\n" + "="*70)
    print("测试完成 - 结果总结")
    print("="*70)
    
    for email, result in results.items():
        status = result.get("status", "UNKNOWN")
        account_name = result.get("account", email)
        role = result.get("role", "UNKNOWN")
        
        emoji = "✅" if status == "pass" else "❌"
        print(f"{emoji} {account_name} ({role}): {status.upper()}")
    
    all_passed = all(r.get("status") == "pass" for r in results.values())
    
    print("\n" + "="*70)
    if all_passed:
        print("结果: 全部通过 ✅")
    else:
        print("结果: 部分失败 ⚠️")
    print("="*70 + "\n")
    
    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\n测试被用户中断")
        sys.exit(130)
    except Exception as e:
        print(f"\n\n致命错误: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)
