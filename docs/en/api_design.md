# TuneLoop API Design Document

## 1. Overview

### 1.1 Purpose
This document defines all backend API endpoints for the TuneLoop instrument rental management system, including endpoint definitions, request parameters, response formats, and error code specifications.

### 1.2 Base Specifications
- **Base URL**: `/api`
- **Authentication**: Bearer Token (JWT)
- **Content Type**: `application/json`
- **Unified Response Format**:
```json
{
  "code": 20000,
  "message": "success",
  "data": { ... }
}
```

### 1.3 Error Code Specifications
| Code | Description |
|------|-------------|
| 20000 | Success |
| 20100 | Created Successfully |
| 40001 | Invalid Request Parameters |
| 40002 | Business Logic Error |
| 40100 | Unauthorized |
| 40101 | Token Expired |
| 40300 | Forbidden |
| 40400 | Resource Not Found |
| 50000 | Internal Server Error |

---

## 2. Authentication APIs

### 2.1 OAuth Callback
```
GET /api/auth/callback
POST /api/auth/callback
```

### 2.2 Login
```
POST /api/auth/login
```

### 2.3 Refresh Token
```
POST /api/auth/refresh
```

---

## 3. Instrument Management APIs

### 3.1 List Instruments
```
GET /api/instruments
```

### 3.2 Get Instrument
```
GET /api/instruments/:id
```

### 3.3 Create Instrument
```
POST /api/instruments
```

**Important Changes** (2026-04-16):
- Instruments no longer have `name`, `brand`, `model` fields
- Brand, model, etc. are passed via `properties` dynamic field

**Request Body**:
```json
{
  "sn": "SN123456",
  "category_id": "uuid",
  "site_id": "uuid",
  "level_id": "uuid",
  "description": "Description",
  "images": ["url1", "url2"],
  "video": "url",
  "properties": {
    "Brand": "Yamaha",
    "Model": "U1",
    "Color": ["Black", "White"],
    "Year": "2020"
  }
}
```

### 3.4 Update Instrument
```
PUT /api/instruments/:id
```

### 3.5 Check SN Availability
```
GET /api/instruments/check?sn=xxx
```

### 3.6 Get Instrument Levels
```
GET /api/instruments/levels
```

### 3.7 Get Instrument Pricing
```
GET /api/instruments/:id/pricing
```

### 3.8 Update Instrument Status
```
PUT /api/instruments/:id/status
```

---

## 4. Category Management APIs

### 4.1 List Categories
```
GET /api/categories
```

### 4.2 Get Category
```
GET /api/categories/:id
```

### 4.3 Create Category
```
POST /api/categories
```

### 4.4 Update Category
```
PUT /api/categories/:id
```

### 4.5 Delete Category
```
DELETE /api/categories/:id
```

### 4.6 Get Child Categories
```
GET /api/categories/:id/children
```

### 4.7 Batch Update Sort Order
```
PUT /api/categories/sort
```

---

## 5. Order/Lease APIs

### 5.1 Preview Order
```
POST /api/orders/preview
```

### 5.2 Create Order
```
POST /api/orders
```

### 5.3 List Orders
```
GET /api/orders
```

### 5.4 Get Order
```
GET /api/orders/:id
```

### 5.5 Pay Order
```
POST /api/orders/:id/pay
```

### 5.6 Confirm Pickup
```
POST /api/orders/:id/pickup
```

### 5.7 Confirm Return
```
POST /api/orders/:id/return
```

### 5.8 Cancel Order
```
POST /api/orders/:id/cancel
```

### 5.9 Get Overdue Leases
```
GET /api/overdue-leases
```

### 5.10 Transfer Ownership
```
POST /api/orders/:id/transfer-ownership
```

### 5.11 Terminate Order
```
PUT /api/orders/:id/terminate
```

---

## 6. Maintenance APIs

### 6.1 Submit Repair Request
```
POST /api/maintenance
```

### 6.2 Report Issue
```
POST /api/maintenance/report
```

### 6.3 Get Maintenance Detail
```
GET /api/maintenance/:id
```

### 6.4 Cancel Maintenance
```
PUT /api/maintenance/:id/cancel
```

### 6.5 List Merchant Maintenance
```
GET /api/merchant/maintenance
```

### 6.6 Merchant Accept
```
PUT /api/merchant/maintenance/:id/accept
```

### 6.7 Assign Technician
```
PUT /api/merchant/maintenance/:id/assign
```

### 6.8 Update Progress
```
PUT /api/merchant/maintenance/:id/update
```

### 6.9 Send Quote
```
POST /api/merchant/maintenance/:id/quote
```

### 6.10 List Technician Tickets
```
GET /api/technician/tickets
```

### 6.11 Technician Accept
```
PUT /api/technician/tickets/:id/accept
```

### 6.12 Technician Complete
```
POST /api/technician/tickets/:id/complete
```

---

## 7. Inventory Management APIs

### 7.1 List Inventory
```
GET /api/merchant/inventory
```

### 7.2 Transfer Request
```
POST /api/merchant/inventory/transfer
```

### 7.3 List Transfers
```
GET /api/merchant/inventory/transfers
```

---

## 8. Site Management APIs

### 8.1 List Sites
```
GET /api/common/sites
```

### 8.2 Get Nearby Sites
```
GET /api/common/sites/nearby?lat=xx&lng=xx
```

### 8.3 Get Site Detail
```
GET /api/common/sites/:id
```

### 8.4 Create Site
```
POST /api/merchant/sites
```

### 8.5 Update Site
```
PUT /api/merchant/sites/:id
```

### 8.6 Delete Site
```
DELETE /api/merchant/sites/:id
```

### 8.7 Get Site Tree
```
GET /api/sites/tree
```

---

## 9. Property Management APIs

### 9.1 List Properties
```
GET /api/properties
```

### 9.2 Create Property
```
POST /api/property
```

### 9.3 Update Property
```
PUT /api/property/:id
```

### 9.4 Create Property Option
```
POST /api/property/option
```

### 9.5 Confirm Property Value
```
PUT /api/property/confirm
```

### 9.6 Merge Property Values
```
PUT /api/property/merge
```

---

## 10. Permission Management APIs

### 10.1 Get Permissions
```
GET /api/admin/permissions
```

### 10.2 Get Roles
```
GET /api/admin/roles
```

### 10.3 Get Role Permissions
```
GET /api/admin/roles/:id/permissions
```

### 10.4 Update Role Permissions
```
PUT /api/admin/roles/:id/permissions
```

### 10.5 Create Role
```
POST /api/admin/roles
```

### 10.6 Delete Role
```
DELETE /api/admin/roles/:id
```

---

## Appendix A: Role Permissions

| Role | Description |
|------|-------------|
| ADMIN | Administrator |
| OWNER | Owner |
| USER | Regular User |

---

## Appendix B: Instrument Status

| Status | Description |
|--------|-------------|
| available | Available for Rent |
| rented | Rented Out |
| maintenance | Under Maintenance |

---

## Appendix C: Order Status

| Status | Description |
|--------|-------------|
| pending | Pending Payment |
| paid | Paid |
| in_lease | In Lease |
| completed | Completed |
| cancelled | Cancelled |

---

*Model: glm-5*
