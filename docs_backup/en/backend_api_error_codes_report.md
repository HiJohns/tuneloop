# Backend API Error Codes Investigation Report

## 1. Overview

This report investigates all API error codes used in the TuneLoop backend project (backend), analyzing the error code classification system and usage scenarios.

### 1.1 Scope

- **Backend Project Path**: `backend/handlers/`
- **Target**: Catalog all API error codes

---

## 2. Error Code System

The backend uses a standardized error code system, grouped by category:

| Error Code | Meaning | HTTP Status | Usage Scenario |
|------------|---------|-------------|----------------|
| **20000** | Success | 200 | Normal success response |
| **20100** | Created | 201 | Resource created successfully |
| **40000** | General Request Error | 400 | Parameter validation failed |
| **40001** | Request Format Error | 400 | Request body parsing failed |
| **40002** | Missing/Invalid Parameters | 400 | Required parameter missing |
| **40003** | Business Validation Failed | 400 | Business rule validation failed |
| **40004** | Duplicate Resource | 400 | Resource already exists |
| **40005** | Resource Conflict | 400 | Resource state conflict |
| **40006** | Condition Not Met | 400 | Precondition not met |
| **40100** | Authentication Failed | 401 | Missing tenant_id |
| **40101** | Not Authenticated | 401 | User not logged in / token invalid |
| **40102** | Token Expired | 401 | Token has expired |
| **40300** | Insufficient Permissions | 403 | Role permission insufficient |
| **40301** | No Operation Permission | 403 | Not authorized to perform operation |
| **40400** | Resource Not Found | 404 | Record not found |
| **40401** | Resource Deleted | 404 | Resource has been soft deleted |
| **50000** | Internal Server Error | 500 | Database/service exception |
| **50001** | Service Unavailable | 500 | External dependency failed |
| **50002** | Database Error | 500 | Database operation failed |

---

## 3. Error Code Details

### 3.1 Success (2xx)

| Error Code | Description | Example |
|------------|-------------|---------|
| **20000** | Operation successful | GET/POST/PUT success return |
| **20100** | Resource created | POST creates new resource |

### 3.2 Client Error (4xx)

#### 40000-40099 Request Error

| Error Code | Description | Typical Scenario |
|------------|--------------|------------------|
| **40000** | General request error | Parameter binding failed, JSON parsing error |
| **40001** | Request format error | ShouldBind failed |
| **40002** | Parameter missing | Required field empty, ID not provided |
| **40003** | Business validation failed | State mismatch, insufficient amount |
| **40004** | Duplicate resource | Unique index conflict |
| **40005** | Resource conflict | State doesn't allow operation |
| **40006** | Condition not met | Precondition not met |

#### 40100-40199 Authentication Error

| Error Code | Description | Typical Scenario |
|------------|--------------|------------------|
| **40100** | Missing tenant info | tenant_id empty or invalid |
| **40101** | Not authenticated / Token invalid | User not logged in or Token invalid |
| **40102** | Token expired | JWT Token has expired |

#### 40300-40399 Permission Error

| Error Code | Description | Typical Scenario |
|------------|--------------|------------------|
| **40300** | Insufficient permissions | User role cannot access |
| **40301** | No operation permission | Can only operate on own data |

#### 40400-40499 Resource Error

| Error Code | Description | Typical Scenario |
|------------|--------------|------------------|
| **40400** | Resource not found | Record not found |
| **40401** | Resource deleted | After soft delete query |

### 3.3 Server Error (5xx)

| Error Code | Description | Typical Scenario |
|------------|--------------|------------------|
| **50000** | Internal server error | General exception |
| **50001** | Service unavailable | External dependency failed |
| **50002** | Database error | SQL execution failed |

---

## 4. Error Code Usage Statistics by Module

### 4.1 instrument.go (Instrument Management)

| Error Code | Count | Description |
|------------|-------|-------------|
| 40002 | 6 | Parameter missing |
| 40003 | 4 | Business validation failed |
| 40004 | 2 | Duplicate resource |
| 40005 | 2 | Resource conflict |
| 40100 | 1 | Authentication failed |
| 40101 | 1 | Token invalid |
| 40401 | 1 | Resource deleted |
| 50000 | 4 | Server error |
| 50001 | 1 | Service unavailable |

### 4.2 property.go (Property Management)

| Error Code | Count | Description |
|------------|-------|-------------|
| 40002 | 5 | Parameter missing |
| 40003 | 1 | Business validation failed |
| 40400 | 3 | Resource not found |
| 50000 | 4 | Server error |

### 4.3 site.go (Site Management)

| Error Code | Count | Description |
|------------|-------|-------------|
| 40002 | 5 | Parameter missing |
| 40400 | 3 | Resource not found |
| 50000 | 4 | Server error |

### 4.4 lease.go (Lease Management)

| Error Code | Count | Description |
|------------|-------|-------------|
| 40000 | 4 | Request error |
| 40100 | 4 | Missing tenant_id |
| 50000 | 2 | Server error |

### 4.5 maintenance.go (Maintenance Tickets)

| Error Code | Count | Description |
|------------|-------|-------------|
| 40101 | 5 | Not authenticated |
| 40301 | 2 | No operation permission |
| 40400 | 2 | Resource not found |
| 50000 | 2 | Server error |

### 4.6 permission.go (Permission Management)

| Error Code | Count | Description |
|------------|-------|-------------|
| 40000 | 4 | Request error |
| 40300 | 1 | Insufficient permissions |
| 40400 | 2 | Resource not found |
| 50000 | 1 | Server error |

---

## 5. Error Handling Patterns

### 5.1 Standard Response Format

```json
{
  "code": <error_code>,
  "message": "<error_description>"
}
```

### 5.2 Success Response Format

```json
{
  "code": 20000,
  "data": {
    // Business data
  }
}
```

### 5.3 Common Handling Patterns

**Parameter Validation**:
```go
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{
        "code":    40001,
        "message": "invalid parameters: " + err.Error(),
    })
    return
}
```

**Resource Not Found**:
```go
if err == gorm.ErrRecordNotFound {
    c.JSON(http.StatusNotFound, gin.H{
        "code":    40400,
        "message": "resource not found",
    })
    return
}
```

**Server Error**:
```go
if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{
        "code":    50000,
        "message": "failed to process: " + err.Error(),
    })
    return
}
```

---

## 6. Recommendations

### 6.1 Existing Issues

1. **Scattered error codes**: Error codes defined in individual handlers, lack of unified management
2. **Some error codes unused**: 40006, 40102, 50002 not found in usage
3. **Inconsistent message text**: Error message format not unified

### 6.2 Recommendations

1. **Define error code constants**: Recommend defining unified error code constants in the project
2. **Supplement frontend handling**: Confirm frontend handles all error code cases
3. **Unify error messages**: Establish error message specifications

---

## 7. Appendix

### 7.1 Complete Error Code List

```
20000, 20100, 40000, 40001, 40002, 40003, 40004, 40005, 40006,
40100, 40101, 40102, 40300, 40301, 40400, 40401,
50000, 50001, 50002
```

### 7.2 File Index

| File | Responsibility |
|------|----------------|
| `handlers/instrument.go` | Instrument CRUD |
| `handlers/property.go` | Property management |
| `handlers/site.go` | Site management |
| `handlers/lease.go` | Lease management |
| `handlers/maintenance.go` | Maintenance tickets |
| `handlers/permission.go` | Permission management |
| `handlers/label.go` | Label management |
| `handlers/order.go` | Order management |

---

*Model: moonshotai-cn/kimi-k2-thinking*
