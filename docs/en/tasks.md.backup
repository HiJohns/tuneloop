# TuneLoop Engineering Task List

**Version**: v2.0  
**Generated Date**: 2026-03-24  
**Source**: `docs/features.csv` (188 feature points)  
**Model**: kimi-k2-thinking

---

## 📊 Task Statistics Overview

| Endpoint | Module Count | Estimated Tasks |
|----------|--------------|----------------|
| [Mobile-5553] WeChat Mini Program | 7 main modules | ~88 atomic tasks |
| [Admin-5554] Admin Dashboard PC | 9 main modules | ~100 atomic tasks |
| [Backend-API] Backend Service | System-wide support | ~50 API tasks |
| **Total** | 16 main modules | **~238 atomic tasks** |

---

## 🎯 Phase 1: Instrument Display Closed Loop (Priority Implementation)

### 1.1 [Mobile-5553] WeChat Mini Program

#### M-001: User Authentication & Authorization【Foundation】
- **Module**: Registration/Login
- **Task Description**:
  - Implement WeChat authorization for quick phone number login (using WeChat Mini Program phone component)
  - Login session persistence: local storage of JWT token, 30-day auto-login, automatic logout on expiration
  - Auto-registration on first login, automatically create user profile (avatar, nickname, open_id, union_id)
- **API Dependencies**: `[Backend-API]-A-001` User Authentication API
- **Pages**: `pages/auth/login`

#### M-002: Instrument Category Page【Core】
- **Module**: Instrument Display
- **Task Description**:
  - Display primary categories (Piano, Guitar, Guzheng, Violin, etc.)
  - Secondary category filtering (by brand, by type)
  - Support custom sorting for categories, ability to hide/show specific categories (backend-controlled)
  - Category icon display (dynamically loaded from backend)
- **API Dependencies**: `[Backend-API]-C-001` Category List API
- **Pages**: `pages/category/index`

#### M-003: Instrument List Page【Core】
- **Module**: Instrument Display
- **Task Description**:
  - Display core information: instrument name, main image, rental price (daily/monthly), deposit, real-time stock status
  - Filter functions: by price range, brand, rental type, stock status (in stock/out of stock)
  - Sort functions: by price ascending/descending, by newest addition
  - Infinite scroll pagination
- **API Dependencies**: `[Backend-API]-C-002` Instrument List API, `[Backend-API]-C-003` Stock Query API
- **Pages**: `pages/instrument/list`

#### M-004: Instrument Detail Page【Core】
- **Module**: Instrument Display
- **Task Description**:
  - Display basic info: name, brand, model, material, size, suitable user group
  - Multimedia: multi-image carousel (supports zoom), video introduction (optional)
  - Specification selection: different specs correspond to different rent/deposit (tiered pricing)
  - Clear display of daily/weekly/monthly unit price (weekly = daily ×6, monthly = daily ×25, configurable)
  - Real-time stock display: show available rental quantity in real-time, mark "temporarily unavailable" when out of stock
  - Delivery service selection: store pickup/offline express delivery
  - User review display: rating, text, images
- **API Dependencies**: `[Backend-API]-C-004` Instrument Detail API, `[Backend-API]-C-005` Review List API
- **Pages**: `pages/instrument/detail`

### 1.2 [Admin-5554] Admin Dashboard PC

#### A-001: Instrument Category Management List Page【Foundation】
- **Module**: Instrument Management
- **Task Description**:
  - List display of all primary/secondary categories
  - Display fields: category name, sorting, icon, display status
  - Search support by category name
- **API Dependencies**: `[Backend-API]-C-001` Category List API

#### A-002: Create/Edit Instrument Category【Foundation】
- **Module**: Instrument Management
- **Task Description**:
  - Form fields: category name (required), parent category (optional, for secondary), sorting number, icon upload, display status
  - Form validation: category name uniqueness check
  - Sync to Mini Program in real-time after submission
- **API Dependencies**: `[Backend-API]-C-006` Category Create/Edit API

#### A-003: Instrument Information Management List Page【Core】
- **Module**: Instrument Management
- **Task Description**:
  - List display of all instruments
  - Display fields: instrument name, brand, model, stock, rent range, shelf status
  - Filter conditions: by category, brand, stock status, shelf status
  - Batch operations: batch shelf on/off, batch delete, batch modify rent
- **API Dependencies**: `[Backend-API]-C-002` Instrument List API

#### A-004: Create/Edit Instrument Form【Core】
- **Module**: Instrument Management
- **Task Description**:
  - Basic info: name, brand, model, material, size, suitable audience, category
  - Multimedia: multi-image upload (supports drag-and-drop sorting), video upload (optional)
  - Specification management: support multiple specs (e.g., piano sizes 120cm/125cm), each spec independently sets rent, deposit, stock
  - Price settings: daily/weekly/monthly unit price, support tiered pricing rule configuration
  - Shelf control: manual shelf on/off, non-display in Mini Program when off
  - Form validation: required field completeness check, price range rationality check
- **API Dependencies**: `[Backend-API]-C-007` Instrument Create/Edit API, `[Backend-API]-C-008` Image Upload API

#### A-005: Inventory Management Module【Core】
- **Module**: Instrument Management
- **Task Description**:
  - Real-time inventory monitoring: in-stock/in-rental/maintenance three-state quantities
  - Inventory adjustment: manually adjust available rental quantity (operation log required)
  - Inventory alert: set inventory threshold, highlight when below threshold
- **API Dependencies**: `[Backend-API]-C-009` Inventory Management API

#### A-006: Excel Import/Export Function【Utility】
- **Module**: Instrument Management
- **Task Description**:
  - Import: Support bulk import instrument information from Excel, support field mapping configuration, return error row prompt on import failure
  - Export: Support export instrument list by filter conditions, customizable export fields
- **API Dependencies**: `[Backend-API]-C-010` Import API, `[Backend-API]-C-011` Export API

### 1.3 [Backend-API] Backend Service

#### B-001: User Authentication API【Foundation】
- **Endpoint**: POST /api/auth/wx-login
- **Function Description**:
  - Receive WeChat Mini Program code, call WeChat API to get open_id/union_id
  - Query user table, auto-register if not exists (create user profile)
  - Generate JWT token (contains user_id, role), 30-day validity
  - Return token and user info

#### B-002: Category Management API Family【Foundation】
- **API List**:
  - GET /api/categories - Category list (shared by Mini Program and admin)
  - POST /api/categories - Create category (admin)
  - PUT /api/categories/:id - Update category (admin)
  - DELETE /api/categories/:id - Delete category (admin, need to check references)
- **Function Description**: Category CRUD, support multi-level categories, sorting, display status control

#### B-003: Instrument Management API Family【Core】
- **API List**:
  - GET /api/instruments - Instrument list (supports filter, sort, pagination)
  - GET /api/instruments/:id - Instrument detail
  - POST /api/instruments - Create instrument (admin)
  - PUT /api/instruments/:id - Update instrument (admin)
  - PUT /api/instruments/:id/shelf-status - Shelf on/off control
- **Function Description**: Instrument CRUD, support multi-spec management, price strategy, media file management

#### B-004: Inventory Management API Family【Core】
- **API List**:
  - GET /api/instruments/:id/stock - Real-time stock query
  - PUT /api/instruments/:id/stock - Stock adjustment (admin)
  - GET /api/stock/transaction-log - Stock change log
- **Function Description**: Real-time stock query, stock adjustment, operation log recording

#### B-005: Review Management API Family【Utility】
- **API List**:
  - GET /api/instruments/:id/reviews - Review list
- **Function Description**: Read review data (comments + rating + images)

#### B-006: File Upload API【Common】
- **Endpoint**: POST /api/upload
- **Function Description**: Support image and video upload, return URL, integrate OSS or local storage

---

## 📱 Phase 2: Complete User Features

### 2.1 [Mobile-5553] WeChat Mini Program

#### Rental Function Module
- **M-005**: Rental Order Process【Core】
  - Rental period selection: daily (1-30 days), weekly (1-4 weeks), monthly (1-12 months), supports custom days
  - Rent calculation: auto calculate total rent = period × unit price, display discounts (long-term rental discounts)
  - Deposit rules: charge by fixed ratio or fixed amount, clearly mark return conditions
  - Delivery method selection: home delivery (select address, mark delivery fee) or store pickup (display store address, hours)
  - Order confirmation: display rent, deposit, delivery fee, total amount, submit after confirmation
  - Payment: integrate WeChat Pay, pay rent + deposit (full amount), auto cancel if payment timeout exceeds 15 minutes

- **M-006**: Renewal Function【Core】
  - Initiate renewal request before order expires
  - Select renewal period, auto calculate renewal rent
  - Pay renewal fee (deposit not required again)
  - Renewal request needs admin approval or auto-approval

- **M-007**: Return Request【Core】
  - Initiate return request (support early or on-time return)
  - Select return method: home pickup (mark pickup fee) or store return
  - Fill return note (instrument condition)
  - Wait for admin confirmation, generate return order after confirmation

- **M-008**: Expiration Reminder【Utility】
  - Push reminder 3 days and 1 day before expiration
  - Overdue reminder: charge 1.5× daily rent for each overdue day
  - Trigger risk control alert if overdue exceeds 7 days

#### Maintenance & Repair Module
- **M-009**: Maintenance/Repair Package Display【Core】
  - Display package categories: maintenance packages, repair packages
  - Show package name, price, service content, duration, applicable instrument types
  - Support custom quote (when no matching package exists)

- **M-010**: Repair Appointment【Core】
  - Select service type (maintenance/repair), instrument type, specific model
  - Fill fault description: text + images/video (max 5 images/1 video)
  - Select service method: home pickup/delivery or store drop-off
  - Select appointment time: precise to date + time slot (e.g., 9:00-12:00), display available slots
  - Submit appointment, generate appointment order

- **M-011**: Repair Progress Tracking【Core】
  - Display progress status: pending→pending pickup→repairing→pending quote→pending confirmation→repair complete→pending return
  - Real-time update: sync after admin operations, display operation time, operator
  - Progress notes: view maintenance notes from admin

- **M-012**: Quote Confirmation【Core】
  - Receive quote push notification
  - Display quote details: labor fee, parts fee, other fees
  - User actions: confirm (proceed to payment) or reject (fill reason)
  - Pay maintenance fee (WeChat Pay support)

#### Order Center Module
- **M-013**: Order List【Core】
  - Display by category: rental orders, repair orders
  - Support filter by time (last 7 days/30 days/all), search by order number
  - Status filter: all, pending payment, pending pickup, in progress, pending return, completed, cancelled, etc.

- **M-014**: Order Detail【Core】
  - Display basic info: order number, order time, order status
  - Rental order detail: instrument info, rental period, rent, deposit, delivery method, payment info
  - Repair order detail: instrument info, service type, fault description, quote, repair progress
  - Payment/refund info display
  - Action buttons: display based on status (pending payment→pay, in rental→renew/return request)

- **M-015**: Payment & Refund【Core】
  - Support replay payment for pending payment orders
  - Refund request: rental orders (full refund if not picked up, refund deposit minus rent after pickup), repair orders (full refund if not started)
  - Display refund progress and status

#### Personal Center Module
- **M-016**: Personal Info Management【Foundation】
  - Display avatar, nickname, phone number, membership level
  - Support modifying avatar and nickname

- **M-017**: Address Management【Foundation】
  - Address list: display recipient, phone, detailed address, is default
  - Actions: add, edit, delete address, set as default
  - Address validation: auto-recognize province/city/district, support address search

- **M-018**: Deposit Management【Core】
  - Deposit details: display pending/refunded deposits, associated order, amount, refund time
  - Deposit refund request: apply for deposit refund after rental completion without damage

- **M-019**: Customer Service【Utility】
  - Online customer service: redirect to WeChat customer service (enterprise/special account)
  - Phone customer service: display merchant contact phone, one-click dial
  - FAQ list

- **M-020**: Announcement Center【Utility】
  - Display merchant announcements
  - Categories: all, rental-related, maintenance-related, system notifications
  - Read/unread marking, support deleting read announcements

- **M-021**: Other Features【Utility】
  - My collections: favorite instruments, repair packages
  - Feedback: fill feedback content + images, admin can reply after submission
  - Settings: notification switch, clear cache, about us
  - Invitation code: generate exclusive invitation code, earn points when referrals make purchases

---

## 💻 Phase 3: Complete Admin Features

### 3.1 [Admin-5554] Admin Dashboard PC

#### Merchant Onboarding Review Module
- **A-007**: Merchant Onboarding Application Review【Core】
  - Review list: display all onboarding applications, support filtering (application number, merchant name, type, status)
  - View details: view all submitted merchant info, qualification materials (zoomable)
  - Approve: set merchant level, commission rate, deposit amount, auto-generate and push merchant backend account credentials
  - Reject: fill rejection reason (preset or custom), merchant receives notification to resubmit
  - Batch operations: batch rejection (need uniform reason), batch export application list

#### Merchant Backend Management Module
- **A-008**: Merchant Login & Personal Center【Foundation】
  - Merchant login: account password or verification code login, record login logs
  - Personal center: display merchant info, account info, support modifying contact, phone, password
  - Notifications: receive platform notifications and user order notifications

- **A-009**: Merchant's Own Instrument Management【Core】
  - Instrument categories: can only manage own primary categories (platform approved)
  - Instrument info management: add, edit, delete own instruments (must be within platform range, need approval if exceeding)
  - Instrument review management: view and reply to user reviews, handle negative reviews

- **A-010**: Merchant Order Management【Core】
  - Rental order management: view own orders, handle pickup confirmation, renewal/return requests, calculate overdue fees, refund deposits
  - Repair order management: accept orders, update progress, submit quotes, confirm completion, arrange return
  - Order statistics: view own order data, generate simple reports

- **A-011**: Merchant Repair Package Management【Core】
  - Package management: add, edit, delete own repair/maintenance packages
  - Quote management: receive user custom repair appointments, submit quotes (must be within platform range)

- **A-012**: Merchant Finance & Settlement Management【Core】
  - Commission details: view own commission details, export details
  - Settlement management: view settlement sheets, confirm amounts, submit early settlement applications
  - Deposit management: view deposit amount, status, submit refund applications

- **A-013**: Store Configuration【Utility】
  - Store info: complete store address, hours, contact, introduction, upload images
  - Message settings: set order message push methods
  - Violations & appeals: view violation records, submit appeals

#### Platform Admin Module
- **A-014**: Merchant Information Management【Core】
  - Merchant list: display all onboarded merchants, support various filtering
  - Merchant details: view, edit merchant info (commission rate, level, status)
  - Status management: normal/suspended/disabled (different permission levels)
  - Qualification update: review merchant qualification update applications

- **A-015**: Merchant Permission Management【Core】
  - Role assignment: merchant admin, order operator, finance operator, etc.
  - Permission configuration: customize merchant backend feature permissions (differentiated by merchant level)
  - Account management: reset password, disable account, unbind phone number

- **A-016**: Commission & Settlement Management【Core】
  - Commission rule setting: global commission, personalized commission, settlement cycle configuration
  - Commission details: view, filter, export all merchant commission details
  - Settlement operations: auto settlement, manual settlement, settlement review, exception handling
  - Deposit management: deposit collection, refund, deduction operations

- **A-017**: Rental Order Management【Core】
  - Order list: display all rental orders, support filtering and batch operations
  - Order operations: pending payment, pending pickup, in rental, pending return, completed status operations
  - Overdue management: overdue order list, automatic overdue fee calculation, collection reminder

- **A-018**: Repair Order Management【Core】
  - Order list: display all repair orders, support filtering and batch operations
  - Order operations: pending acceptance, pending pickup, repairing, pending quote, pending payment, repair completion status operations
  - Repair package management: add, edit, delete platform-maintained repair packages
  - Technician management: maintain repair technician/tuner info, support auto/manual assignment

- **A-019**: User Management【Core】
  - User list: display all users, support filter by phone, registration time, membership level
  - User details: view user orders, addresses, deposit info
  - Membership management: set membership levels and benefits (deposit reduction, rent discount)
  - Address management: view, modify, delete user saved addresses

- **A-020**: Finance & Data Statistics【Core】
  - Deposit management: deposit list, refund operations, refund records
  - Order export: export orders by conditions to Excel
  - Data statistics: business overview, rental statistics, repair statistics, user statistics

- **A-021**: System Configuration【Utility】
  - Home page config: carousel image management, recommended instrument settings
  - Announcement management: add, edit, delete announcements, auto-push to Mini Program after publish
  - Basic settings: payment settings, fee settings, message settings, store management

---

## 🔧 Appendix: Task Priority Suggestion

### High Priority (MVP Foundation, ~40 tasks)
1. **User Authentication Module** (M-001, B-001)
2. **Instrument Display Closed Loop** (M-002, M-003, M-004, A-001-A-006, B-002, B-003, B-004)
3. **Basic Rental Process** (Core part of M-005: order + payment)
4. **Order Management** (M-013 list view, A-017 basic operations)
5. **Stock Sync** (B-004 stock query)

### High+ Priority (Complete Business Features, ~60 tasks)
1. Complete rental flow (renewal, return, reminder)
2. Maintenance & repair flow
3. Merchant onboarding & review
4. Merchant backend management
5. User review system

### Medium Priority (Enhanced Features, ~70 tasks)
1. Invitation code feature
2. Membership level system
3. Ad carousel management
4. Complex data statistics and reporting

### Low Priority (Optimization & Auxiliary, ~68 tasks)
1. Excel import/export optimization
2. Cache optimization
3. Message push optimization
4. UI beautification

---

## 📍 Version Iteration Planning Suggestion

### v1.0 MVP (Priority 4 weeks)
- Complete user authentication, instrument display, basic rental ordering, order viewing
- Admin backend complete instrument management, basic order operations
- Goal: End-to-end demonstrable flow

### v2.0 Merchant Onboarding (Week 5-6)
- Complete merchant onboarding application, review, merchant backend foundation
- Goal: Support merchant self-management

### v3.0 Complete Rental (Week 7-8)
- Complete renewal, return, overdue, deposit management
- Goal: Complete rental lifecycle closed loop

### v4.0 Maintenance & Repair (Week 9-10)
- Complete maintenance & repair full flow
- Goal: Value-added service launch

### v5.0 Platform Operations (Week 11-12)
- Complete platform admin all features
- Goal: Full operation launch

---

*Model: kimi-k2-thinking*
