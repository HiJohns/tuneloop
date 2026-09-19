# Tuneloop Integration Test Documentation

## Overview

本文档描述 Tuneloop 集成测试框架的架构和使用方法。

## Quick Start

### 1. Install Dependencies

```bash
cd tests/integration
pip install -r requirements.txt
```

Requirements file content:
```txt
PyYAML>=6.0
requests>=2.28.0
```

### 2. Prepare Accounts

A sample config.yaml is already provided in `tests/integration/`.
You can modify it with your actual test accounts:

```yaml
api:
  base_url: "http://localhost:5554/api"
  timeout: 30

iam:
  url: "http://opencode.linxdeep.com:5552"
  client_id: "tuneloop"
  redirect_uri: "http://localhost:5554/callback"

accounts:
  - email: "admin_debug@tuneloop.com"
    password: "Debug@2026"
    role: "OWNER"
    name: "Admin Debug (Owner)"
    tenant: "多伦多音悦琴行"

  - email: "tech_zhang@tuneloop.com"
    password: "Debug@2026"
    role: "TECHNICIAN"
    name: "Tech Zhang (Technician)"
    tenant: "多伦多音悦琴行"

  - email: "customer_lee@tuneloop.com"
    password: "Debug@2026"
    role: "USER"
    name: "Customer Lee (User)"
    tenant: "多伦多音悦琴行"

settings:
  log_level: "INFO"
  continue_on_error: false
  cleanup_after_test: true
```

### 3. Verify Config

```bash
cd tests/integration
python -c "
from common import load_config
c = load_config()
print(f'✓ Config loaded: {len(c.accounts)} accounts')
for acc in c.accounts:
    print(f'  - {acc.name} ({acc.role})')
"
```

### 4. Run Tests

```bash
cd tests/integration
python test_visibility_stock_loop.py
```

Or specify a different config file:

```bash
python test_visibility_stock_loop.py -c my_config.yaml
```

## Architecture

### Directory Structure

```
tests/integration/
├── config.yaml              # 配置文件
├── common.py               # 公共模块
├── requirements.txt      # Python 依赖
├── test_*.py             # 测试脚本
└── README.md             # 本文档
```

### Core Modules

#### common.py

Provides shared infrastructure for all tests:

- `Account` - Account data structure
- `TestConfig` - Test configuration data
- `TokenManager` - Token retrieval and caching
- `load_config()` - Load YAML configuration
- `create_api_client()` - Create authenticated API client
- `run_tests_for_all_accounts()` - Run tests for all accounts
- `log()` - Unified logging

#### Token Management

Token is fetched automatically for each account:

1. Check if token already exists in account.token
2. Call IAM `/oauth/token` endpoint with password grant
3. Cache token in account.token for reuse
4. Set Authorization header in requests.Session

## Test Scenarios

### Scenario 1: Asset Visibility & Stock Loop

Verifies that management operations propagate to consumer side in real-time.

**Run:**
```bash
python test_visibility_stock_loop.py
```

**Test Flow:**
1. Each account attempts to create instrument (only Owner/Admin succeed)
2. All accounts query instruments list
3. If instrument created, attempt to update its status
4. Verify changes are visible to all accounts

### Scenario 2: SKU Price Engine Loop (TBD)

### Scenario 3: Inventory Locking Loop (TBD)

### Scenario 4: Maintenance Flow Loop (TBD)

### Scenario 5: Multi-Tenant Isolation Loop (TBD)

## Preparation Checklist

Before running tests, ensure:

- [ ] Backend service is running (`go run main.go`)
- [ ] IAM service is accessible
- [ ] Test accounts exist in IAM
- [ ] Config file has correct credentials
- [ ] Python dependencies installed

## Troubleshooting

### Token Fetch Failure

**Symptoms:** 401 errors, "Failed to get token"

**Check:**
1. IAM URL is correct in config
2. Account credentials are valid
3. IAM service is running
4. Network connectivity

### Test Failures

**Symptoms:** Assertion errors

**Check:**
1. Backend is running and accessible
2. Database has required data (categories, etc.)
3. Use `-v` flag for verbose output

## Adding New Tests

1. Create `test_new_scenario.py`
2. Import common module
3. Implement test function with signature:
   ```python
   def test_new_scenario(client, config: TestConfig, account: Account):
       # test logic
       pass
   ```
4. Use `run_tests_for_all_accounts()` in main()

Example:
```python
if __name__ == "__main__":
    config = load_config()
    run_tests_for_all_accounts(config, test_new_scenario)
```

## Environment Variables

For CI/CD or local development:

```bash
export TUNELOOP_API_URL="http://localhost:5554/api"
export IAM_URL="http://opencode.linxdeep.com:5552"
```

## Continuous Integration

Example GitHub Actions workflow:

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.9'
      
      - name: Install dependencies
        run: |
          cd tests/integration
          pip install -r requirements.txt
      
      - name: Run tests
        run: |
          cd tests/integration
          python test_visibility_stock_loop.py
        env:
          IAM_URL: ${{ secrets.IAM_URL }}
```

## Impact

- **New Files**:
  - `tests/integration/config.yaml` - Configuration
  - `tests/integration/common.py` - Shared module
  - `docs/autotest.md` - This documentation
- **Modified Files**:
  - `tests/integration/test_visibility_stock_loop.py` - Refactored to use common module

## Risk Assessment

- **Low Risk**: Only affects test infrastructure
- **Reversible**: All changes in tests/ directory
- **No Production Impact**: Test-only code

---

*Model: moonshotai-cn/kimi-k2-thinking*
