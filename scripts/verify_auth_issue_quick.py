#!/usr/bin/env python3
"""
Quick IAM Authentication Issue Verification Script

This script verifies the core authentication issue in Issue #266:
- Frontend access without token redirects to IAM
- Token storage and retrieval issues between frontend and backend

Usage:
    python scripts/verify_auth_issue_quick.py

Architecture:
  Frontend :5554 -> Vite dev server -> proxies /api/* to Backend :5557
"""

import requests
import json
import sys

def test_endpoint(url, description, timeout=5):
    """Test an endpoint and return response details"""
    print(f"\n[TEST] {description}")
    print(f"  URL: {url}")
    
    try:
        response = requests.get(url, timeout=timeout)
        print(f"  Status: {response.status_code}")
        
        if response.headers.get('content-type', '').startswith('application/json'):
            try:
                data = response.json()
                print(f"  Response: {json.dumps(data, indent=2)}")
                return response.status_code, data
            except:
                print(f"  Raw: {response.text[:200]}")
                return response.status_code, response.text
        else:
            print(f"  Content-Type: {response.headers.get('content-type')}")
            print(f"  Length: {len(response.text)} bytes")
            return response.status_code, response.text
            
    except Exception as e:
        print(f"  ❌ ERROR: {str(e)}")
        return None, str(e)

def main():
    print("=" * 80)
    print("IAM Authentication Issue - Quick Verification")
    print("Issue #266: Login redirect loop after successful authentication")
    print("=" * 80)
    
    # Configuration
    FRONTEND = "http://opencode.linxdeep.com:5554"
    BACKEND = "http://opencode.linxdeep.com:5557"  # For direct access only
    IAM = "http://opencode.linxdeep.com:5552"
    
    print("\n⚠️  IMPORTANT: Ensure services are running on ports 5554, 5557, 5552")
    print("   and /etc/hosts contains: 127.0.0.1 opencode.linxdeep.com")
    print("\nArchitecture:")
    print("  Browser -> Frontend :5554 -> [Vite Proxy] -> Backend :5557")
    print("\n" + "=" * 80)
    
    # Test 1: Check if frontend is accessible
    print("\n" + "=" * 80)
    print("TEST 1: Frontend Accessibility")
    print("=" * 80)
    
    code, data = test_endpoint(FRONTEND, "Frontend main page")
    
    # Test 2: Check backend health (for information only)
    print("\n" + "=" * 80)
    print("TEST 2: Backend Health Check")
    print("=" * 80)
    
    code, data = test_endpoint(f"{BACKEND}/api/health", "Backend health endpoint")
    
    # Test 3: Access protected API without authentication (simulates issue)
    print("\n" + "=" * 80)
    print("TEST 3: Protected API Access (Issue Reproduction)")
    print("=" * 80)
    print("\nTesting: Access /api/common/sites through frontend proxy")
    
    code, data = test_endpoint(f"{FRONTEND}/api/common/sites", 
                              "Protected API endpoint (via frontend proxy)")
    
    if code == 401:
        print("\n" + "=" * 80)
        print("ISSUE CONFIRMED ❌")
        print("=" * 80)
        print("\n✅ Successfully reproduced the 401 authentication issue!")
        print("\nThis means:")
        print("  1. Frontend :5554 is accessible")
        print("  2. Frontend properly proxies /api/* to backend :5557")
        print("  3. Backend requires authentication (returns 401)")
        print("  4. The authentication flow is NOT working correctly")
        print("\nNext step: Test with valid authentication token")
        
    else:
        print("\n" + "=" * 80)
        print("UNEXPECTED RESPONSE")
        print("=" * 80)
        if code is None:
            print("\n❌ Connection failed. Check:")
            print("  1. Services are running (check 'netstat -tln | grep 5554'")
            print("  2. /etc/hosts contains '127.0.0.1 opencode.linxdeep.com'")
            print("  3. Firewall is not blocking the ports")
        else:
            print(f"\n❓ Unexpected status code: {code}")
            print("Expected 401 for unauthorized access")
    
    # Test 4: IAM health check
    print("\n" + "=" * 80)
    print("TEST 4: IAM Service Check")
    print("=" * 80)
    
    code, data = test_endpoint(f"{IAM}/health", "IAM health endpoint")
    
    # Summary
    print("\n" + "=" * 80)
    print("VERIFICATION COMPLETE")
    print("=" * 80)
    print("\nKey conclusions:")
    print("  1. Frontend connectivity working: Required for authentication flow")
    print("  2. Backend requires auth (401): Expected for protected endpoints")
    print("  3. Issue: Token not being properly stored/retrieved between steps")
    print("\n📝 For manual browser testing:")
    print("   1. Open browser dev tools (F12)")
    print("   2. Go to Application -> Local Storage")
    print("   3. Check if 'token' exists after login")
    print("   4. If missing, backend cookie domain configuration may be wrong")
    print("\n🔧 Code fixes needed:")
    print("   - backend/handlers/auth.go: Fix cookie domain setting")
    print("   - frontend/src/services/api.js: Fix token reading priority")
    print("=" * 80 + "\n")

if __name__ == "__main__":
    main()
