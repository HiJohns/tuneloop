# TuneLoop UI Design Document

## 1. Overview

### 1.1 Purpose
This document defines the UI design specifications for the TuneLoop instrument rental management system, including page structure, features, interaction logic, and visual design principles.

### 1.2 Tech Stack
- **Frontend Framework**: React 18 + Vite
- **UI Component Library**: Ant Design 5.x
- **Routing**: React Router 6
- **State Management**: React Hooks
- **Styling**: Tailwind CSS + Ant Design overrides

### 1.3 Page Entry Points
- **PC Admin**: http://localhost:5554
- **Mobile**: http://localhost:5556

---

## 2. Navigation Structure

### 2.1 Sidebar Menu
```
├── Dashboard
├── Instrument Management
│   ├── Instrument List
│   ├── Add Instrument
│   ├── Instrument Categories
│   └── Property Management
├── Order Management
├── Maintenance Tickets
├── Inventory Transfer
├── Lease Ledger
├── Deposit Flow
├── Finance Config
├── Site Management
├── Client Management
├── Role Permission
└── System Settings
```

---

## 3. Page Designs

### 3.1 Dashboard

**Route**: `/dashboard`

**Features**:
- Key business metric cards
  - Total instruments
  - Available instruments
  - In-lease instruments
  - Pending maintenance tickets
- Lease trend chart
- Near-expiry lease list
- Recently added instruments

### 3.2 Instrument List

**Route**: `/instruments/list`

**Features**:
- Instrument table display
  - Image thumbnail
  - Serial number (SN)
  - Category name
  - Instrument level
  - Site
  - Status badge
- Toolbar
  - Import/Export
  - Search box
  - Category filter
  - Status filter
- Batch operations
  - Batch enable/disable
  - Batch price update
  - Batch delete
- Action buttons
  - Edit
  - Delete
  - View detail

### 3.3 Instrument Form

**Important Changes** (2026-04-16):
- Instruments no longer have a `name` field - identified solely by `sn` (serial number)
- Brand, model, etc. are now dynamic properties

**Routes**: 
- New: `/instruments/new/edit`
- Edit: `/instruments/:id/edit`

**Features**:
- Basic Information
  - Serial number (SN) - **unique identifier**
  - Category - tree select
  - Level - dropdown select
  - Site - tree select
- Dynamic Properties
  - Rendered dynamically based on system config
  - Examples: Brand, Model, Color, Year, etc.
  - Configurable in Property Management page
  - Optional
- Description
- Image Upload
  - Drag-to-sort
  - Upload progress
  - Retry on failure
- Video URL

**Form Validation**:
- Serial Number: Required, unique validation
- Category: Required
- Level: Required
- Site: Required
- Dynamic Properties: Optional

### 3.4 Instrument Detail

**Route**: `/instruments/detail/:id`

**Features**:
- Image gallery
- Basic info card
- Lease history
- Maintenance history
- Current status

### 3.5 Category Management

**Route**: `/instruments/categories`

**Features**:
- Two-column layout
  - Left: Level-1 category selector + Create button
  - Right: Level-2 category list (draggable)
- Create/edit/delete categories
- Drag-to-reorder level-2 categories

### 3.6 Property Management

**Route**: `/instruments/properties`

**Features**:
- Property list
- Create/edit properties
- Property option management
- Property alias merging

### 3.7 Order Management

**Route**: `/orders`

**Features**:
- Order table
- Status filter
- Status transition buttons
  - Pending → Paid
  - Paid → Pickup
  - Pickup → Return
  - Cancel available

### 3.8 Maintenance

**Route**: `/maintenance`

**Features**:
- Ticket list
- Ticket detail
- Merchant actions
- Technician actions

### 3.9 Inventory Transfer

**Route**: `/inventory/transfer`

**Features**:
- Inventory list
- Transfer request form
- Transfer history

### 3.10 Site Management

**Route**: `/sites`

**Features**:
- Site tree structure
- Site detail
- CRUD operations

---

## 4. Common Components

### 4.1 ProtectedRoute

**Function**: Verify login status, redirect to login if not authenticated

### 4.2 SortableImageUpload

**Features**:
- Drag-to-sort
- Upload progress
- Retry on failure
- Image preview

### 4.3 ImportResultModal

**Features**:
- Success count
- Failed count
- Error details

### 4.4 TreeSelect

**Features**:
- Dynamic loading of child nodes
- Used for categories and sites

---

## 5. Design Principles

### 5.1 Layout
- 24-column grid system
- Content padding: 24px
- Card spacing: 16px
- Form item spacing: 24px

### 5.2 Color Palette
- Primary: `#6366F1` (Indigo)
- Success: `#52C41A`
- Warning: `#FAAD14`
- Danger: `#F5222D`
- Info: `#1890FF`
- Text: `#262626` (primary), `#8C8C8C` (secondary)

### 5.3 Status Color Mapping
```
Instrument Status:
  - available: green
  - rented: orange
  - maintenance: red

Order Status:
  - pending: orange
  - paid: blue
  - in_lease: green
  - completed: gray
  - cancelled: gray
```

### 5.4 Responsive Design
- PC: >= 1200px
- Tablet: 768px - 1199px
- Mobile: < 768px

### 5.5 Interaction Feedback
- Button click: slight scale animation
- Loading: Spin component or skeleton
- Success: message.success
- Error: message.error

---

## Appendix A: Route List

| Page | Route | Permission |
|------|-------|------------|
| Login Callback | `/callback` | Public |
| Dashboard | `/dashboard` | Auth |
| Instrument List | `/instruments/list` | Auth |
| New Instrument | `/instruments/new/edit` | OWNER |
| Edit Instrument | `/instruments/:id/edit` | OWNER |
| Instrument Detail | `/instruments/detail/:id` | Auth |
| Categories | `/instruments/categories` | Auth |
| Properties | `/instruments/properties` | Auth |
| Orders | `/orders` | Auth |
| Maintenance | `/maintenance` | Auth |
| Inventory | `/inventory/transfer` | Auth |
| Sites | `/sites` | Auth |
| Clients | `/clients` | Auth |
| Permissions | `/permissions` | ADMIN |
| Tenants | `/tenants` | ADMIN |

---

*Model: glm-5*
