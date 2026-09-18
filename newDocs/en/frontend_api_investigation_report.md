# Frontend API Integration Investigation Report

## 1. Overview

This report investigates all code points in the TuneLoop frontend project (frontend-pc) that send requests to the backend, analyzes the technologies used, and reviews whether the responses are uniformly processed.

### 1.1 Scope

- **Frontend Project Path**: `frontend-pc/src/`
- **Target**: Count all API request points and analyze request patterns

---

## 2. Request Technology Analysis

### 2.1 Technology Stack

The frontend project uses two main HTTP request technologies:

| Technology | Usage Scenario | File Location |
|------------|----------------|---------------|
| **Unified api.js module** | Recommended, modular API calls | `src/services/api.js` |
| **Native fetch** | Direct calls, some components bypass the unified module | Within various components |

### 2.2 Unified API Module (api.js)

**File Path**: `frontend-pc/src/services/api.js`

This is the core request wrapper module, providing the following features:

```javascript
// Core exports
export const api = {
  get: (endpoint) => request(endpoint),
  post: (endpoint, data) => request(endpoint, { method: 'POST', body: JSON.stringify(data) }),
  put: (endpoint, data) => request(endpoint, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (endpoint) => request(endpoint, { method: 'DELETE' }),
}
```

**Modular API wrappers** (lines 221-321):

| API Module | Purpose |
|------------|---------|
| `instrumentsApi` | Instrument management |
| `ordersApi` | Order management |
| `sitesApi` | Site management |
| `inventoryApi` | Inventory management |
| `maintenanceApi` | Maintenance tickets |
| `ownershipApi` | Ownership certificates |
| `permissionApi` | Permission management |
| `leaseApi` | Lease management |
| `depositApi` | Deposit management |
| `iamAdminApi` | IAM tenant/client management |
| `categoriesApi` | Category management |

### 2.3 Direct fetch Usage

Some components bypass the unified api.js module and use `fetch` directly:

| File | Line | Description |
|------|------|-------------|
| `App.jsx` | 267, 341 | OAuth callback, config fetch |
| `pages/admin/instrument/List.jsx` | 49, 335, 399 | Instrument list, import/export |
| `pages/admin/instrument/Edit.jsx` | 142, 175, 514 | Editor |
| `pages/admin/instrument/Detail.jsx` | 38, 197, 228, 244 | Detail page |
| `pages/admin/category/List.jsx` | 21, 194 | Category management |
| `pages/AuthCallback/index.tsx` | 38 | Auth callback |
| `pages/MaintenanceDispatch.jsx` | 21, 37, 53 | Maintenance dispatch |
| `components/AssetTimeline/index.jsx` | 25 | Timeline |
| `components/PricingMatrixEditor/index.jsx` | 32, 64 | Pricing matrix |
| `components/BrandProvider/index.tsx` | 39 | Brand config |

---

## 3. Unified Response Processing

### 3.1 Request Flow (api.js lines 118-212)

```
1. Token expiration check (sliding window renewal)
       ↓
2. Add Authorization Header
       ↓
3. Send fetch request
       ↓
4. Handle 401 status code (call handleAuthError)
       ↓
5. Handle 40101 business error code (token expired)
       ↓
6. Normalize response format (extract data field)
       ↓
7. Return processed data
```

### 3.2 Authentication & Token Handling

**Token Retrieval** (lines 3-14):
- Read `token` from cookie
- Fallback to localStorage/sessionStorage

**Token Storage** (lines 17-21):
```javascript
function storeTokens(accessToken, refreshToken) {
  localStorage.setItem('token', accessToken)
  localStorage.setItem('refresh_token', refreshToken)
  document.cookie = `token=${accessToken}; path=/; max-age=604800`
}
```

**Token Expiration Detection** (lines 55-68):
- Parse JWT payload's exp field
- Trigger renewal when remaining time is less than 30% of 30 days

**Token Refresh Mechanism** (lines 93-116):
- Use refresh_token to get new access_token
- Auto-store new token

**Auth Error Handling** (lines 70-91):
```javascript
async function handleAuthError(token, retryCount, endpoint, options) {
  if (retryCount < 1) {
    // Try to refresh token
    await refreshAccessToken()
    // Retry original request
    return await request(endpoint, options, retryCount + 1)
  }
  // Refresh failed, clear tokens and redirect to IAM
  clearTokens()
  redirectToIAM()
  return { __authFailed: true }
}
```

### 3.3 Response Normalization

api.js implements intelligent response extraction (lines 181-211):

```javascript
// Return array directly
if (Array.isArray(data)) return data

// Extract common wrapper fields
if (Array.isArray(data.data)) return data.data
if (Array.isArray(data.items)) return data.items
if (Array.isArray(data.result)) return data.result
if (Array.isArray(data.list)) return data.list

// Handle nested format
if (Array.isArray(data.data.instruments)) return data.data.instruments
if (Array.isArray(data.data.list)) return data.data.list

// Handle unified response format
if (data.success && Array.isArray(data.data)) return data.data
if (data.code === 0 && Array.isArray(data.data)) return data.data
if (data.code === 20000 && Array.isArray(data.data)) return data.data
```

---

## 4. Request Point Statistics

### 4.1 Requests via api.js Module

A total of **82** call points distributed across the following modules:

| Module | Requests | Main Endpoints |
|--------|----------|----------------|
| instrumentsApi | 3 | `/instruments` |
| ordersApi | 5 | `/orders/*` |
| sitesApi | 7 | `/common/sites`, `/merchant/sites`, `/sites/tree` |
| inventoryApi | 3 | `/merchant/inventory` |
| maintenanceApi | 8 | `/maintenance` |
| ownershipApi | 2 | `/user/ownership` |
| permissionApi | 5 | `/admin/*` |
| leaseApi | 4 | `/merchant/leases` |
| depositApi | 3 | `/merchant/deposits` |
| iamAdminApi | 8 | `/system/*` |
| categoriesApi | 5 | `/categories` |

### 4.2 Direct fetch Usage

A total of **25** direct calls, mainly distributed in:

- Instrument management: 11 locations
- Authentication: 3 locations
- Maintenance tickets: 3 locations
- Other components: 8 locations

---

## 5. Issues and Recommendations

### 5.1 Issues Found

1. **Direct fetch bypasses unified module**: Approximately 25 locations use fetch directly, bypassing api.js's unified authentication and response handling
2. **Inconsistent response handling**: Components using fetch directly need to handle response format and errors themselves
3. **Code duplication**: Multiple components repeat the same request logic

### 5.2 Recommendations

1. **Unified use of api.js module**: Migrate all direct fetch usage to api.js or corresponding modular APIs
2. **Component-level request interceptors**: Consider adding request/response interceptors at component level
3. **Request logging**: Add unified request logging for easier debugging

---

## 6. Appendix

### 6.1 API Endpoints Summary

| Endpoint Prefix | Purpose |
|-----------------|---------|
| `/instruments/*` | Instrument management |
| `/orders/*` | Order management |
| `/common/sites/*` | Public sites |
| `/merchant/sites/*` | Merchant site management |
| `/sites/tree` | Site tree structure |
| `/merchant/inventory/*` | Inventory management |
| `/maintenance/*` | Maintenance tickets |
| `/user/ownership/*` | Ownership certificates |
| `/admin/*` | System administration |
| `/merchant/leases/*` | Lease management |
| `/merchant/deposits/*` | Deposit management |
| `/system/*` | IAM system management |
| `/categories/*` | Category management |
| `/properties/*` | Property management |
| `/auth/*` | Authentication |

### 6.2 Key File Index

| File | Purpose |
|------|---------|
| `frontend-pc/src/services/api.js` | Unified request wrapper module |
| `frontend-pc/src/App.jsx` | App entry, includes OAuth callback |
| Various `pages/*` components | Business page requests |
| Various `components/*` components | Component-level requests |

---

*Model: moonshotai-cn/kimi-k2-thinking*
