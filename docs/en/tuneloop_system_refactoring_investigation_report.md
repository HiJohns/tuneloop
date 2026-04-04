# TuneLoop System Refactoring and Feature Evolution Investigation Report

## 1. Overview

This report provides an in-depth investigation of the "TuneLoop System Refactoring and Feature Evolution Plan" proposed in Issue #199, analyzing the gap between the current system architecture and the planned features, and proposing feasible implementation paths.

### 1.1 Goals and Scope

The plan covers four core areas:
- **Infrastructure & Auth**: Unified API calls, login state optimization, UUID validation
- **Data Schema Refactoring**: JSONB non-standard attributes, media matrix, label normalization
- **UI/UX Experience Upgrade**: List page flattening, edit mode conversion, visual optimization
- **Rental Security Loop**: Outbound confirmation, return audit, legal verification

### 1.2 Core Principles

System logic is transitioning from "industrial standard products" to "high-value non-standard asset" management, following:
- One item, one code
- Asset centralization
- Process closed-loop

---

## 2. Current System Architecture Analysis

### 2.1 Frontend API Service Layer (`frontend-pc/src/services/api.js`)

#### 2.1.1 Implemented Features

| Feature Module | Status | Description |
|----------------|--------|-------------|
| JWT Auto-injection | ✅ Implemented | Lines 124-126, auto-add `Authorization: Bearer` header |
| 401 Global Interceptor | ✅ Implemented | Lines 135-145, handle HTTP 401 status code |
| 40101 Error Code Handling | ✅ Implemented | Lines 154-166, handle business layer token expired error |
| Sliding Window Renewal | ✅ Implemented | Lines 108-118, check and auto-refresh token before request |
| Token Refresh Mechanism | ✅ Implemented | Lines 80-103, refreshAccessToken function |
| Basic API Wrapper | ✅ Implemented | api.get/post/put/delete methods |
| Domain API Modules | ✅ Implemented | instrumentsApi, ordersApi, sitesApi, etc. |

#### 2.1.2 Features to Enhance

1. **WeChat Webview Environment Adaptation**: Current redirectToIAM function does not detect WeChat environment
2. **30-day Long-term Login**: Current token validity period needs confirmation

### 2.2 Backend Data Model (`backend/models/models.go`)

#### 2.2.1 JSONB Field Usage

| Field | Table | Type | Description |
|-------|-------|------|-------------|
| Images | Instrument | jsonb | In use |
| Specifications | Instrument | jsonb | In use |
| Pricing | Instrument | jsonb | In use |
| Images | MaintenanceTicket | jsonb | In use |
| RepairPhotos | MaintenanceTicket | jsonb | In use |
| CompletionPhotos | MaintenanceTicket | jsonb | In use |
| RedirectURIs | Client | jsonb | In use |

#### 2.2.2 Fields to Add

1. **metadata field**: For storing maker, origin, material and other non-standard attributes (currently missing)
2. **Label System**: Lacks independent label table and normalization mapping table

---

## 3. Phase Implementation Analysis

### 3.1 Phase 1: System Core

#### Task 1.1: Refactor api.js and Interceptor Logic

**Current Status**: api.js already has a solid foundation:
- Unified request interceptor
- JWT auto-injection
- 401/40101 error handling
- Sliding window renewal

**Recommendations**:
- Evaluate current token validity configuration
- Add WeChat Webview environment detection
- Consider adding WeChat environment detection logic to redirectToIAM

#### Task 1.2: Database Schema Adjustment

**Current Status**: Some fields already use JSONB, but metadata field is missing

**Recommendations**:
- Add metadata JSONB field to Instrument model
- Consider creating independent label system table (label_normalization)

#### Task 1.3: Global CSS Optimization

**Current Status**: Need to evaluate existing index.css and component styles

**Recommendations**:
- Review existing scrollbar styles
- Evaluate implementing scheme three (floating auto-hide)

### 3.2 Phase 2: Asset Management

#### Task 2.1: Full-screen Edit Page

**Current Status**:
- InstrumentEdit.jsx currently uses Modal dialog
- Specification and price linkage calculation logic needs implementation

**Recommendations**:
- Develop full-screen edit route page
- Implement two-row specification layout
- Add automatic price ratio calculation logic

#### Task 2.2: Upload Lock Logic

**Current Status**: Need to evaluate existing upload components

**Recommendations**:
- Ensure image/video upload completes before API submission
- Add upload state management

#### Task 2.3: Label Audit Center

**Current Status**: Currently lacks independent label management module

**Recommendations**:
- Create label database table
- Implement normalization mapping logic
- Develop backend label audit interface

### 3.3 Phase 3: Rental Workflow

#### Task 3.1: Mobile Outbound Confirmation

**Current Status**: Need to develop new mini-program page

**Recommendations**:
- Develop mini-program "outbound confirmation" page
- Implement user preview inbound photo and check confirmation

#### Task 3.2: Comparison Damage Assessment UI

**Current Status**: Need to develop new admin page

**Recommendations**:
- Develop "outbound vs return" side-by-side comparison UI
- Integrate electronic signature component

#### Task 3.3: PDF Generation Service

**Current Status**: Backend currently has no PDF generation capability

**Recommendations**:
- Integrate PDF generation library (e.g., gofpdf)
- Develop "return assessment report" generation endpoint

---

## 4. Supplementary Recommendations Analysis

### 4.1 Asset ID (QR Code)

**Feasibility**: High
- Backend already has unique UUID
- Can use UUID to generate QR code image
- Frontend already has image display component

### 4.2 Branch Inventory Dashboard

**Feasibility**: Medium
- Need to add map view component
- Need to implement multi-branch inventory aggregation query

### 4.3 Caching Strategy

**Feasibility**: Medium
- Need to evaluate Redis integration cost
- Dashboard statistics need backend cache support

---

## 5. Implementation Dependencies

```
Task 1.1 ──┬──> Task 1.2 ──> Task 2.1 ──> Task 3.1
           │              │
           │              └─> Task 2.2 ──> Task 3.2
           │                         │
           │                         └─> Task 3.3
           │
           └─> Task 1.3 ──> Task 2.3
```

---

## 6. Conclusions and Recommendations

### 6.1 Overall Assessment

- **Infrastructure**: Basic features 80% complete, fine-tuning needed
- **Data Schema**: Need to expand metadata field and label system
- **UI/UX**: Need significant changes, involving multiple pages
- **Process Loop**: Need to develop new features from scratch

### 6.2 Priority Implementation Recommendations

1. **Phase 1 Priority**: API WeChat adaptation + metadata field + CSS optimization
2. **Phase 2 Priority**: Full-screen edit page + label system
3. **Phase 3 Priority**: Mini-program and PDF features

---

## Appendix

### A. Related File Index

| File | Purpose |
|------|---------|
| `frontend-pc/src/services/api.js` | Frontend API service layer |
| `backend/models/models.go` | Backend data model |
| `backend/handlers/api.go` | Backend API handlers |
| `frontend-pc/src/pages/admin/instrument/` | Instrument management pages |

### B. Database Table List

| Table Name | Description |
|------------|--------------|
| instruments | Instrument main table |
| categories | Category table |
| orders | Order table |
| leases | Lease table |
| maintenance_tickets | Maintenance ticket table |
| sites | Branch/site table |

---

*Model: google/gemini-3-pro-preview*
