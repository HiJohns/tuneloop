#!/bin/bash

# ============================================================================
# 调试环境创建脚本
# 任务: 通过 API 自动化构建全角色调试环境
# Issue: #72
# ============================================================================

set -e

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 配置参数
TENANT_NAME="多伦多音悦琴行 (Debug_Store_01)"
TENANT_ID="00000000-0000-0000-0000-000000000000"  # 默认租户ID，需要替换为真实值
ORG_ID="00000000-0000-0000-0000-000000000000"    # 默认组织ID

# 账户配置
DEBUG_PASSWORD="Debug@2026"
SYSADMIN_EMAIL="super_god@beaconiam.com"
SYSADMIN_PASSWORD="${SYSADMIN_PASSWORD:-}"  # 需要从环境变量获取

# 服务地址
BACKEND_URL="http://localhost:5554"
IAM_URL="${BEACONIAM_INTERNAL_URL:-http://localhost:8080}"

# 账户信息
OWNER_EMAIL="admin_debug@tuneloop.com"
OWNER_USERNAME="admin_debug"
OWNER_NAME="调试管理员"

TECH_EMAIL="tech_zhang@tuneloop.com"
TECH_USERNAME="tech_zhang"
TECH_NAME="张师傅"
TECH_PHONE="13800138001"

USER_EMAIL="customer_lee@tuneloop.com"
USER_USERNAME="customer_lee"
USER_NAME="李先生"

# 全局变量
SYSADMIN_TOKEN=""
TENANT_ID_ACTUAL=""
OWNER_USER_ID=""
TECH_USER_ID=""
CUST_USER_ID=""
TECHNICIAN_ID=""

# ============================================================================
# 函数定义
# ============================================================================

print_header() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}$1${NC}"
    echo -e "${GREEN}========================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# 检查环境
check_environment() {
    print_header "环境检查"
    
    # 检查后端服务
    if curl -s -f "${BACKEND_URL}/health" > /dev/null 2>&1; then
        print_success "Backend service is running at ${BACKEND_URL}"
    else
        print_error "Backend service is not accessible at ${BACKEND_URL}"
        print_warning "Please start the backend service first: make run-backend"
        return 1
    fi
    
    # 检查 IAM 服务
    if curl -s -f "${IAM_URL}/health" > /dev/null 2>&1; then
        print_success "IAM service is running at ${IAM_URL}"
    else
        print_warning "IAM service is not accessible at ${IAM_URL}"
        print_warning "Some features may not work without IAM"
    fi
    
    # 检查必需的环境变量
    if [ -z "$SYSADMIN_PASSWORD" ]; then
        print_error "SYSADMIN_PASSWORD environment variable is not set"
        print_warning "Please set it: export SYSADMIN_PASSWORD='your_password'"
        return 1
    fi
    
    print_success "Environment check passed"
}

# 获取 IAM Token
get_iam_token() {
    local username="$1"
    local password="$2"
    
    print_header "获取 IAM Token: $username"
    
    # 注意：实际 IAM API 端点可能不同，需要根据实际情况调整
    local token_response
    token_response=$(curl -s -X POST "${IAM_URL}/api/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$username\",\"password\":\"$password\"}" 2>/dev/null || echo "")
    
    if [ -z "$token_response" ]; then
        print_error "Failed to get token from IAM"
        return 1
    fi
    
    # 提取 token（假设响应格式为：{"data":{"access_token":"xxx"}}）
    local token
    token=$(echo "$token_response" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
    
    if [ -z "$token" ]; then
        print_error "Token not found in IAM response"
        print_warning "Response: $token_response"
        return 1
    fi
    
    print_success "Token obtained successfully"
    echo "$token"
}

# 创建租户
create_tenant() {
    print_header "创建租户: $TENANT_NAME"
    
    # TODO: 检查 POST /api/system/tenants 端点是否存在
    # 目前系统中可能没有独立的 tenants 表
    # 租户信息可能存储在 IAM 系统中
    
    print_warning "Tenant creation API endpoint not confirmed"
    print_warning "Assuming default tenant ID: $TENANT_ID"
    
    # 备选方案：直接插入数据库（如果表存在）
    # psql "postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${TUNELOOP_DB}" -c "INSERT INTO tenants (id, name) VALUES (gen_random_uuid(), '$TENANT_NAME') RETURNING id;"
    
    TENANT_ID_ACTUAL="$TENANT_ID"
    print_success "Using tenant ID: $TENANT_ID_ACTUAL"
}

# 创建用户账户
create_user() {
    local email="$1"
    local username="$2"
    local name="$3"
    local role="$4"
    local password="$5"
    
    print_header "创建用户: $email ($role)"
    
    # 检查用户注册端点
    local register_response
    register_response=$(curl -s -X POST "${BACKEND_URL}/api/auth/register" \
        -H "Content-Type: application/json" \
        -d "{
            \"email\":\"$email\",
            \"username\":\"$username\",
            \"name\":\"$name\",
            \"role\":\"$role\",
            \"password\":\"$password\",
            \"tenant_id\":\"$TENANT_ID_ACTUAL\",
            \"org_id\":\"$ORG_ID\"
        }" 2>/dev/null || echo "")
    
    if [ -z "$register_response" ]; then
        print_error "Failed to create user via API"
        # 备选方案：直接插入数据库
        print_warning "Falling back to direct database insertion"
        
        # 生成密码哈希（需要知道哈希算法，这里使用占位符）
        local hashed_password="$password"  # TODO: 实际使用中需要哈希
        
        # 插入 users 表
        local user_id=$(psql "postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${TUNELOOP_DB}" -t -c "
            INSERT INTO users (iam_sub, tenant_id, org_id, name, email, phone) 
            VALUES ('iam_$username', '$TENANT_ID_ACTUAL', '$ORG_ID', '$name', '$email', '') 
            RETURNING id;
        " 2>/dev/null | tr -d ' ')
        
        if [ -z "$user_id" ]; then
            print_error "Failed to create user in database"
            return 1
        fi
        
        echo "$user_id"
    else
        # 提取 user_id 从响应
        local user_id
        user_id=$(echo "$register_response" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
        
        if [ -z "$user_id" ]; then
            print_error "User ID not found in response"
            print_warning "Response: $register_response"
            return 1
        fi
        
        print_success "User created: $user_id"
        echo "$user_id"
    fi
}

# 创建技术员
create_technician_record() {
    local user_id="$1"
    local name="$2"
    local phone="$3"
    
    print_header "创建技术员记录: $name"
    
    # 检查是否有创建技术员的 API
    # 如果没有，直接插入数据库
    
    local technician_id=$(psql "postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${TUNELOOP_DB}" -t -c "
        INSERT INTO technicians (tenant_id, org_id, site_id, name, phone) 
        VALUES ('$TENANT_ID_ACTUAL', '$ORG_ID', NULL, '$name', '$phone') 
        RETURNING id;
    " 2>/dev/null | tr -d ' ')
    
    if [ -z "$technician_id" ]; then
        print_error "Failed to create technician record"
        return 1
    fi
    
    print_success "Technician record created: $technician_id"
    echo "$technician_id"
}

# 生成调试报告
generate_debug_report() {
    print_header "生成调试报告"
    
    local report_file="$HOME/debug_accounts.md"
    
    cat > "$report_file" << EOF
# 调试环境账户清单

**租户**: $TENANT_NAME  
**租户ID**: $TENANT_ID_ACTUAL  
**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')

## 账户列表

| 账户 (Email/Username) | 角色 (Role) | 关联租户 (Tenant Name) | 初始密码 | 备注 | 用户ID |
| :--- | :--- | :--- | :--- | :--- | :--- |
| $OWNER_EMAIL | OWNER | $TENANT_NAME | $DEBUG_PASSWORD | 租户总管 | $OWNER_USER_ID |
| $TECH_EMAIL | TECHNICIAN | $TENANT_NAME | $DEBUG_PASSWORD | 维保师傅 | $TECH_USER_ID |
| $USER_EMAIL | USER | $TENANT_NAME | $DEBUG_PASSWORD | 普通客户 | $CUST_USER_ID |
| $SYSADMIN_EMAIL | SYS_ADMIN | 全局/系统 | (现有密码) | 系统最高管理 | - |

## 快速开始

### 登录方式

1. **PC端**: http://localhost:5554
2. **移动端**: http://localhost:5553

### 测试建议

1. 使用 OWNER 账户登录，验证租户管理功能
2. 使用 TECHNICIAN 账户登录，验证工单处理流程
3. 使用 USER 账户登录，验证租赁流程
4. 使用 SYS_ADMIN 账户登录，验证系统管理功能

### 注意事项

⚠️ 本环境为调试环境，仅供开发和测试使用
⚠️ 请勿在生产环境使用相同的密码
⚠️ 调试完成后，请执行清理脚本删除测试数据

EOF

    print_success "报告已生成: $report_file"
    cat "$report_file"
}

# ============================================================================
# 主流程
# ============================================================================

main() {
    print_header "开始创建调试环境"
    
    # 检查环境
    if ! check_environment; then
        print_error "环境检查失败，请解决上述问题后重试"
        exit 1
    fi
    
    # 获取 SysAdmin Token
    SYSADMIN_TOKEN=$(get_iam_token "$SYSADMIN_EMAIL" "$SYSADMIN_PASSWORD")
    if [ -z "$SYSADMIN_TOKEN" ]; then
        print_error "无法获取 SysAdmin Token"
        print_warning "跳过需要 IAM Token 的操作"
    fi
    
    # 创建租户
    if ! create_tenant; then
        print_error "租户创建失败"
        exit 1
    fi
    
    # 创建 Owner
    OWNER_USER_ID=$(create_user "$OWNER_EMAIL" "$OWNER_USERNAME" "$OWNER_NAME" "OWNER" "$DEBUG_PASSWORD")
    if [ -z "$OWNER_USER_ID" ]; then
        print_error "Owner 创建失败"
        exit 1
    fi
    
    # 创建 Technician
    TECH_USER_ID=$(create_user "$TECH_EMAIL" "$TECH_USERNAME" "$TECH_NAME" "TECHNICIAN" "$DEBUG_PASSWORD")
    if [ -z "$TECH_USER_ID" ]; then
        print_error "Technician 创建失败"
        exit 1
    fi
    
    # 创建 Technician 记录
    TECHNICIAN_ID=$(create_technician_record "$TECH_USER_ID" "$TECH_NAME" "$TECH_PHONE")
    if [ -z "$TECHNICIAN_ID" ]; then
        print_error "Technician 记录创建失败"
        exit 1
    fi
    
    # 创建 User
    CUST_USER_ID=$(create_user "$USER_EMAIL" "$USER_USERNAME" "$USER_NAME" "USER" "$DEBUG_PASSWORD")
    if [ -z "$CUST_USER_ID" ]; then
        print_error "User 创建失败"
        exit 1
    fi
    
    # 生成报告
    generate_debug_report
    
    print_header "调试环境创建完成"
    print_success "✅ 所有账户创建成功"
    print_success "📄 报告已保存到 ~/debug_accounts.md"
}

# 检查必需的环境变量
check_required_env() {
    local required_vars=("POSTGRES_HOST" "POSTGRES_PORT" "POSTGRES_USER" "POSTGRES_PASSWORD" "TUNELOOP_DB")
    for var in "${required_vars[@]}"; do
        if [ -z "${!var}" ]; then
            print_error "Required environment variable $var is not set"
            return 1
        fi
    done
    return 0
}

# 脚本入口
if ! check_required_env; then
    print_error "请设置必需的环境变量后再运行此脚本"
    print_warning "Example: export POSTGRES_HOST=localhost POSTGRES_USER=tuneloop ..."
    exit 1
fi

# 执行主函数
main "$@"