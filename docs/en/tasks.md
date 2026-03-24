# TuneLoop Engineering Task List

**Version**: v2.1  
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

## 🔄 Phase 2: Rental Closed Loop (Lease-to-Return)

> Complete rental lifecycle from user ordering → payment → merchant accepting → delivery → user returning → deposit refund

### 2.1 User Flow (Mini Program)

#### M-005: Rental Order Placement【Core】
- **Module**: Rental Function
- **Task Description**:
  - Select specification, rental period (daily/weekly/monthly), quantity on instrument detail page
  - Select delivery method (home delivery or store pickup)
  - Select address (for delivery) or store (for pickup)
  - Auto calculate rent, deposit, delivery fee
  - Display cost breakdown: rent, deposit, delivery fee, total amount
  - Submit order after confirmation
- **Pages**: `pages/rental/order-confirm`
- **API Dependencies**: `[Backend-API]-R-001` Create Rental Order API

#### M-006: Order Payment【Core】
- **Module**: Rental Function
- **Task Description**:
  - Invoke WeChat Pay to pay rent + deposit (full amount)
  - Auto cancel order after 15 minutes payment timeout
  - Update order status to "pending pickup" after successful payment
  - Support retry payment (for failed or timeout payments)
- **Pages**: `pages/rental/payment`
- **API Dependencies**: `[Backend-API]-R-002` Payment API, `[Backend-API]-R-003` Order Status Query API

#### M-007: Order List & View【Core】
- **Module**: Order Center
- **Task Description**:
  - Display all rental orders
  - Filter by category: all, pending payment, pending pickup, in rental, pending return, completed, cancelled
  - Support filter by time (last 7 days/30 days/all), search by order number
  - Display core info: order number, instrument image & name, order status, rental period, amount
- **Pages**: `pages/order/rental-list`
- **API Dependencies**: `[Backend-API]-R-004` Rental Order List API

#### M-008: Order Detail & Operations【Core】
- **Module**: Order Center
- **Task Description**:
  - Display order details: instrument info, rental period, rent, deposit, delivery method, address
  - Display payment info: payment method, payment time, transaction number
  - Display pickup/return info (time, location, status)
  - Status-specific action buttons:
    - Pending payment: pay, cancel order
    - In rental: renewal request, return request
    - Completed: view deposit refund progress
- **Pages**: `pages/order/rental-detail`
- **API Dependencies**: `[Backend-API]-R-005` Order Detail API, `[Backend-API]-R-010` Cancel Order API

#### M-009: Renewal Request【Core】
- **Module**: Rental Function
- **Task Description**:
  - Initiate renewal request in "My Orders" (only for in-rental orders)
  - Select renewal period, auto calculate renewal rent
  - Display renewal details (no deposit required again)
  - Pay renewal fee
  - Sync order expiration time after renewal
  - Renewal request needs admin approval or auto-approval
- **Pages**: `pages/order/rental-renew`
- **API Dependencies**: `[Backend-API]-R-006` Renewal API, `[Backend-API]-R-007` Renewal Payment API

#### M-010: Return Request【Core】
- **Module**: Rental Function
- **Task Description**:
  - Initiate return request (support early or on-time return)
  - Select return method: home pickup (mark pickup fee) or store return
  - Fill return notes (instrument condition, usage feedback)
  - Wait for admin confirmation after submission
- **Pages**: `pages/order/rental-return-apply`
- **API Dependencies**: `[Backend-API]-R-008` Return Request API

#### M-011: Deposit Refund View【Core】
- **Module**: Personal Center - Deposit Management
- **Task Description**:
  - View deposit details in "Deposit Management"
  - Display pending/refunded deposit list
  - Show corresponding order number, amount, refund time
  - Display "Apply for Refund" button when rental completed without damage
  - Show deposit refund request review progress
- **Pages**: `pages/user/deposit`
- **API Dependencies**: `[Backend-API]-R-009` Deposit Query API, `[Backend-API]-R-012` Deposit Refund Request API

### 2.2 Admin Flow (Merchant Backend)

#### A-007: Rental Order Management List【Core】
- **Module**: Rental Order Management
- **Task Description**:
  - Display all rental orders (platform view) or merchant's own orders (merchant view)
  - Filter conditions: order number, user phone, order status, order time, instrument type
  - Display core fields: order number, user info, instrument info, rental period, amount, payment status, delivery method
  - Batch operations: batch export, batch status update, batch cancel
- **Pages**: `pages/order/rental-manage`
- **API Dependencies**: `[Backend-API]-R-004` Rental Order List API

#### A-008: Order Status Flow Operations【Core】
- **Module**: Rental Order Management
- **Task Description**:
  - **Pending Payment**: manually cancel order, send payment reminder (SMS/mini program message)
  - **Pending Pickup**: confirm pickup (record pickup time, pickup person), modify delivery/pickup info
  - **In Rental**: process renewal requests (approve/reject), process return requests, set overdue billing (manual/auto)
  - **Pending Return**: confirm return (record return time, instrument condition, any damage), deduct overdue fees (if applicable)
  - **Completed**: review deposit refund request, initiate refund (WeChat Pay back to source)
  - **Common**: add order notes, sync to mini program order detail
-  **Approval Flow Settings**  : Renewal requests can be configured as auto-approve or manual review
- **Pages**: `pages/order/rental-detail-manage`
- **API Dependencies**: `[Backend-API]-R-013` Order Status Update API, `[Backend-API]-R-014` Renewal Review API, `[Backend-API]-R-015` Return Confirmation API

#### A-009: Overdue Order Management【Core】
- **Module**: Rental Order Management
- **Task Description**:
  - Overdue order list: display overdue orders, overdue days, late fee amount
  - Auto late fee calculation: calculate automatically by configured rules (e.g., 1.5× daily rent), support manual adjustment
  - Collection reminder: manually send SMS/mini program message to remind user to return
  - Support one-click overdue fee report generation
- **Pages**: `pages/order/overdue-manage`
- **API Dependencies**: `[Backend-API]-R-016` Overdue Order Query API, `[Backend-API]-R-017` Late Fee Calculation API

#### A-010: Deposit Management【Core】
- **Module**: Finance & Settlement Management
- **Task Description**:
  - Deposit list: display pending/refunded deposits, associated order, user, amount
  - Deposit refund review: review user deposit refund request (check order status, instrument condition, overdue fees)
  - Initiate refund: call WeChat Pay refund API after approval, record refund amount, time, status, transaction number
  - Support batch refund (unified deposit refund for multiple orders)
  - Refund record query and export
- **Approval Flow**: Configurable multi-level review (e.g., require finance supervisor approval for amounts over threshold)
- **Pages**: `pages/finance/deposit-manage`
- **API Dependencies**: `[Backend-API]-R-018` Deposit Review API, `[Backend-API]-R-019` WeChat Pay Refund API

### 2.3 Rental Closed Loop Backend API

#### R-001: Create Rental Order API【Core】
- **Endpoint**: POST /api/rental-orders
- **Function Description**:
  - Receive order parameters: user ID, instrument ID, spec ID, rental type, rental length, quantity, delivery method, address ID
  - Calculate order amount: calculate rent by rental length and tiered pricing rules, calculate deposit by deposit rules
  - Generate unique order number (support date-based generation, e.g., R2026032400001)
  - Status initialization: "pending payment" after creation
  - Stock pre-occupation (optional, prevent overselling)
  - Return order detail and payment parameters

#### R-002: Rental Order Payment API【Core】
- **Endpoint**: POST /api/rental-orders/:orderId/pay
- **Function Description**:
  - Call WeChat Pay unified order interface
  - Receive payment result callback, update order status to "pending pickup"
  - Auto cancel order and release stock if payment timeout exceeds 15 minutes

#### R-003: Rental Order Query API【Core】
- **Endpoint**: GET /api/rental-orders
- **Function Description**:
  - Query current user's rental order list (mini program)
  - Support filter by status, time
  - Paginated return (10 per page)
  - Return fields: order number, instrument info, rental period, amount, status, order time

- **Endpoint**: GET /api/rental-orders/:orderId
- **Function Description**: Query single order detail

- **Endpoint**: GET /api/rental-orders/all (Admin)
- **Function Description**: Query all rental orders (platform/merchant view), support multi-dimensional filtering

#### R-004: Renewal API【Core】
- **Endpoint**: POST /api/rental-orders/:orderId/renew
- **Function Description**:
  - Receive renewal period parameter
  - Calculate new expiration time based on current expiration and renewal period
  - Calculate renewal rent (by tiered pricing, no deposit required)
  - Generate renewal record with "pending payment" status
  - Update main order expiration time after payment

#### R-005: Return Request API【Core】
- **Endpoint**: POST /api/rental-orders/:orderId/return
- **Function Description**:
  - Receive return method, return time, notes
  - Update order status to "pending return" (before merchant confirmation)
  - Generate return request record
  - Push message notification to merchant admin

- **Endpoint**: POST /api/rental-orders/:orderId/return/confirm (Admin)
- **Function Description**: Merchant confirms return, update status to "completed", trigger deposit refund flow

#### R-006: Order Status Flow API【Core】
- **Endpoint**: PUT /api/rental-orders/:orderId/status
- **Function Description**:
  - Update order status by different actions: pending payment→pending pickup→in rental→pending return→completed
  - Record operator, operation time, notes for each status flow

#### R-007: Late Fee Calculation API【Core】
- **Endpoint**: GET /api/rental-orders/:orderId/late-fee
- **Function Description**:
  - Calculate overdue days (based on current time and order expiration)
  - Calculate late fee amount by configured rules (e.g., 1.5× daily rent)
  - Support manual late fee adjustment (admin can modify)
  - Record late fee calculation log

#### R-008: Deposit Management API【Core】
- **Endpoint**: POST /api/deposit/refund-request
- **Function Description**:
  - Receive user deposit refund request
  - Validate order status (must be completed without overdue/damage)
  - Generate deposit refund record with "pending review" status

- **Endpoint**: POST /api/deposit/refund-requests/:requestId/review (Admin)
- **Function Description**: Admin reviews deposit refund, call WeChat refund API if approved

#### R-009: Order Statistics API【Utility】
- **Endpoint**: GET /api/rental-orders/stats
- **Function Description**:
  - Statistics on rental order data: today/yesterday/this month order count, transaction amount, total deposit
  - Statistics by instrument type, rental period, time range
  - Statistics by order status distribution

---

## 🔧 Phase 3: Maintenance Closed Loop (Maintenance-to-Finish)

> Complete maintenance lifecycle from user repair request → merchant accepting → quoting → user payment → repair completion → instrument return

### 3.1 User Flow (Mini Program)

#### M-012: Maintenance/Repair Package Display【Core】
- **Module**: Instrument Maintenance/Repair
- **Task Description**:
  - Category display: maintenance packages (piano tuning, guitar string replacement, etc.), repair packages (fault repair, parts replacement, etc.)
  - Display package info: name, price, service content, duration, applicable instrument types
  - Support search and filter (by instrument type, service type)
- **Pages**: `pages/maintenance/packages`
- **API Dependencies**: `[Backend-API]-M-001` Maintenance Package List API

#### M-013: Maintenance/Repair Appointment【Core】
- **Module**: Instrument Maintenance/Repair
- **Task Description**:
  - Select service type: maintenance / repair
  - Select instrument type, specific model (optional, select from order history)
  - Fill fault description: text description + upload images/video (max 5 images / 1 video)
  - Select service method: home pickup/delivery (fill address, mark pickup fee) or store drop-off (select store, fill arrival time)
  - Select appointment time: precise to date + time slot (e.g., 9:00-12:00), display available slots (from merchant calendar)
  - Submit appointment: generate maintenance order with "pending acceptance" status
- **Pages**: `pages/maintenance/appointment`
- **API Dependencies**: `[Backend-API]-M-002` Create Maintenance Order API

#### M-014: Maintenance Progress Tracking【Core】
- **Module**: Instrument Maintenance/Repair
- **Task Description**:
  - Display progress status flow: pending acceptance → pending pickup → repairing → pending quote → pending confirmation → repair complete → pending return
  - Real-time update: Auto sync after admin operations, display operation time, operator (configurable anonymous/real name)
  - View maintenance notes added by admin (e.g., "Need to replace strings, estimated 1 day to complete")
  - Support push message reminders (mini program subscription messages)
- **Pages**: `pages/maintenance/progress`
- **API Dependencies**: `[Backend-API]-M-003` Maintenance Order Detail API, `[Backend-API]-M-004` Progress Query API

#### M-015: Maintenance Order List【Core】
- **Module**: Order Center
- **Task Description**:
  - Display all maintenance orders
  - Filter by category: all, pending acceptance, pending pickup, repairing, pending quote, pending payment, completed, cancelled
  - Support filter by time (last 7 days/30 days/all), search by order number
  - Display core info: order number, service type, appointment time, order status, amount
- **Pages**: `pages/order/maintenance-list`
- **API Dependencies**: `[Backend-API]-M-005` Maintenance Order List API

#### M-016: Quote Confirmation & Payment【Core】
- **Module**: Instrument Maintenance/Repair
- **Task Description**:
  - Receive quote push notification (mini program template message)
  - View quote details: labor fee, parts fee, other fees, total amount
  - User actions:
    - **Confirm quote**: Proceed to payment, call WeChat Pay
    - **Reject quote**: Fill rejection reason, cancel appointment (order status becomes "cancelled")
  - After payment, order status auto flows to "repairing"
- **Pages**: `pages/maintenance/quote-confirm`
- **API Dependencies**: `[Backend-API]-M-006` Quote Confirmation API, `[Backend-API]-M-007` Maintenance Payment API

### 3.2 Admin Flow (Merchant Backend)

#### A-011: Maintenance Package Management【Foundation】
- **Module**: Maintenance Order Management
- **Task Description**:
  - Add/edit/delete maintenance/repair packages
  - Set package fields: name, price, service content, applicable instrument types, estimated duration
  - Shelf on/off control: non-display in mini program when off
  - Support upload service images, detailed description
  - Can set quote templates (preset common fault quote details)
- **Pages**: `pages/maintenance/package-manage`
- **API Dependencies**: `[Backend-API]-M-008` Maintenance Package CRUD API

#### A-012: Maintenance Order Management List【Core】
- **Module**: Maintenance Order Management
- **Task Description**:
  - Display all maintenance orders (platform view) or merchant's own orders (merchant view)
  - Filter conditions: order number, user phone, order status, appointment time, service type
  - Display fields: order number, user info, instrument info, service type, fault description, quote amount, appointment time
  - Batch operations: batch export, batch acceptance, batch cancellation
  - Support quick actions on list page (accept, update status, submit quote)
- **Pages**: `pages/maintenance/order-manage`
- **API Dependencies**: `[Backend-API]-M-005` Maintenance Order List API

#### A-013: Maintenance Order Status Flow Operations【Core】
- **Module**: Maintenance Order Management
- **Task Description**:
  - **Pending acceptance**: View order detail and user-submitted fault description/images, click "Accept" (assign technician) or "Reject" (fill reason)
  - **Pending pickup**: Confirm pickup (record pickup time, pickup person), arrange pickup logistics
  - **Repairing**: Update repair progress, submit quote (fill labor fee, parts fee, other fee details)
  - **Pending quote**: Wait for user confirmation (this status on mini program)
  - **Pending payment**: User confirmed quote, waiting for payment (can send payment reminder)
  - **Repair complete**: Confirm completion (record completion time), upload post-repair photos, arrange return (home/store)
  - **Pending return**: User confirmed repair completion, waiting for instrument return
  - **Order notes**: Add maintenance notes, sync to mini program progress page
- **Pages**: `pages/maintenance/order-detail-manage`
- **API Dependencies**: `[Backend-API]-M-009` Status Flow API, `[Backend-API]-M-010` Quote Submit API

#### A-014: Repair Technician/Tuner Management【Core】
- **Module**: Maintenance Order Management
- **Task Description**:
  - Upload/edit technician/tuner info: name, contact, professional skills, service area
  - Set technician acceptance rules: match by instrument type, service area, workload
  - Support auto-assignment (rule-based) or manual assignment (admin designated)
  - View technician workload and order statistics
- **Pages**: `pages/maintenance/technician-manage`
- **API Dependencies**: `[Backend-API]-M-011` Technician Management API

#### A-015: Quote Management【Core】
- **Module**: Maintenance Order Management
- **Task Description**:
  - Receive user custom repair appointments (when no matching package)
  - Submit quote: fill fee details (labor fee, parts fee, other fees), auto calculate total
  - Quote range validation: must be within reasonable range set by platform (avoid malicious high prices)
  - View user quote confirmation status (pending/confirmed/rejected)
  - Handle quote rejection: contact user to negotiate adjusted quote or cancel order
- **Pages**: `pages/maintenance/quote-manage`
- **API Dependencies**: `[Backend-API]-M-010` Quote Submit API

### 3.3 Maintenance Closed Loop Backend API

#### M-001: Maintenance Package API【Foundation】
- **Endpoint**: GET /api/maintenance-packages
- **Function Description**: Query available maintenance/repair package list, support filter by instrument type, service type

- **Endpoint**: POST /api/maintenance-packages (Admin)
- **Function Description**: Add, update, delete maintenance packages

#### M-002: Create Maintenance Order API【Core】
- **Endpoint**: POST /api/maintenance-orders
- **Function Description**:
  - Receive parameters: user ID, service type, instrument info, fault description, images/video, appointment time, service method
  - Generate unique maintenance order number (e.g., M2026032400001)
  - Initial status: pending acceptance
  - Push message notification to corresponding merchant admin

#### M-003: Maintenance Order Query API【Core】
- **Endpoint**: GET /api/maintenance-orders
- **Function Description**:
  - Mini program: query current user's maintenance order list
  - Support filter by status, time
  - Return fields: order number, service type, appointment time, order status, quote amount

- **Endpoint**: GET /api/maintenance-orders/all (Admin)
- **Function Description**: Query all maintenance orders (platform/merchant view), support multi-dimensional filtering

- **Endpoint**: GET /api/maintenance-orders/:orderId
- **Function Description**: Query single maintenance order detail, including progress flow, quote details

#### M-004: Maintenance Progress Update API【Core】
- **Endpoint**: PUT /api/maintenance-orders/:orderId/progress
- **Function Description**:
  - Admin updates repair progress status
  - Add progress notes
  - Trigger mini program real-time notification (subscription message)

#### M-005: Quote Submit & Confirmation API【Core】
- **Endpoint**: POST /api/maintenance-orders/:orderId/quote (Admin)
- **Function Description**:
  - Receive quote details: labor fee, parts fee, other fees
  - Auto calculate total amount
  - Validate quote is within reasonable range
  - Update order status to "pending quote"
  - Push mini program message to notify user confirmation

- **Endpoint**: POST /api/maintenance-orders/:orderId/quote/confirm (Mini Program)
- **Function Description**:
  - User confirms or rejects quote
  - Status flows to "pending payment" after confirmation
  - Status becomes "cancelled" after rejection

#### M-006: Maintenance Payment API【Core】
- **Endpoint**: POST /api/maintenance-orders/:orderId/pay
- **Function Description**:
  - Invoke WeChat Pay
  - Status auto flows to "repairing" after successful payment
  - Close order on payment timeout

#### M-007: Repair Complete & Return API【Core】
- **Endpoint**: POST /api/maintenance-orders/:orderId/complete (Admin)
- **Function Description**:
  - Submit repair completion info: completion time, maintenance notes, post-repair photos
  - Status flows to "repair complete"
  - Push notification to user

- **Endpoint**: POST /api/maintenance-orders/:orderId/return-confirm (Admin)
- **Function Description**: Confirm instrument returned, close order

#### M-008: Technician Management API【Foundation】
- **Endpoint**: POST /api/repair-technicians (Admin)
- **Function Description**:
  - Add, edit, delete repair technician info
  - Query technician list and order statistics
  - Auto-assignment rule configuration

#### M-009: Maintenance Statistics API【Utility】
- **Endpoint**: GET /api/maintenance-orders/stats
- **Function Description**:
  - Statistics on maintenance order data: today/this month order count, revenue
  - Statistics by service type, instrument type, time range
  - Statistics by order status distribution

---

## 📝 Phase 4: Other Core Feature Modules

### 4.1 Merchant Onboarding & Review Module (Merchant Onboarding)

#### A-016: Merchant Onboarding Application【Core】
- **Onboarding Type Selection**: Individual merchant (ID info) / Enterprise merchant (business license, legal representative info)
- **Basic Info Form**:
  - Merchant basic info: name (match qualification), contact person, contact phone (verified by SMS), contact email
  - Business info: main instrument types (multi-select), business model (rental/maintenance/both), store address (map location), business hours
  - Login info: set merchant backend login account (phone/custom), login password (strength validation)
- **Qualification Materials Upload**:
  - Individual merchant: ID front/back, holding ID photo, bank card photo
  - Enterprise merchant: business license, legal representative ID, corporate account info, industry qualification (optional)
  - Material requirements: JPG/PNG format, ≤5M per file, preview allowed, re-upload supported
- **Agreement Signing**: Display "Merchant Onboarding Agreement", etc., must check "I have read and agree"
- **Submission & Progress Tracking**: Generate application number after submission, can query review status (pending/reviewing/approved/rejected)
- **Notification**: Push mini program message when review status changes
- **Pages**: `pages/onboard/apply`, `pages/onboard/progress`
- **API Dependencies**: `[Backend-API]-O-001` Onboarding Application API, `[Backend-API]-O-002` Progress Query API

#### A-017: Merchant Onboarding Review【Core】
- **Review List**: Display all onboarding applications, support filter by application number, merchant name, type, status
- **View Details**: View all submitted merchant info, zoomable qualification materials
- **Approve**: Set merchant level (default regular), commission rate (customizable), deposit amount, auto-generate and push merchant backend account credentials
- **Reject**: Fill rejection reason (preset or custom), merchant receives notification to resubmit
- **Batch Operations**: Batch reject (need uniform reason), batch export application list
- **Pages**: `pages/onboard/review-list`, `pages/onboard/review-detail`
- **API Dependencies**: `[Backend-API]-O-003` Review API

#### A-018: Merchant Information Management【Core】
- **Merchant List**: Display all onboarded merchants, support multi-dimensional filtering
- **Merchant Details**: View complete info (basic info, qualification materials, business info, account info), support edit (commission rate, level, status)
- **Status Management**:
  - **Normal**: Merchant can operate normally
  - **Suspended**: Suspend operation permissions (cannot add instruments or accept orders), can process existing orders
  - **Disabled**: Completely disable all permissions, delete merchant backend account, must settle orders and commissions first
- **Qualification Update**: Review merchant qualification update applications, log updates
- **Pages**: `pages/merchant/list`, `pages/merchant/detail`
- **API Dependencies**: `[Backend-API]-O-004` Merchant Information Management API

### 4.2 Merchant Backend Management Module (Merchant Backend)

#### A-019: Merchant Login & Personal Info【Foundation】
- Login with assigned account password, support verification code login, password reset (bound phone)
- Record login logs (time, IP, device)
- Personal center displays merchant info (name, level, commission rate, deposit status)
- Support modify contact person, contact info, password
- View onboarding agreement, commission rules
- Receive platform notifications and user order messages
- **Pages**: `pages/merchant/login`, `pages/merchant/profile`

#### A-020: Merchant Permission Management【Core】
- **Role Assignment**: Merchant admin, order operator, finance operator, etc.
- **Permission Configuration**: Platform customizes merchant backend feature permissions (e.g., allow rent modification, allow self refund)
- **Account Management**: Reset merchant backend password, disable account, unbind phone
- **Pages**: `pages/merchant/permission`
- **API Dependencies**: `[Backend-API]-O-005` Merchant Permission Management API

### 4.3 Personal Center Module (Personal Center)

#### M-021: Personal Information Management【Foundation】
- Display avatar, nickname, phone number, membership level
- Support modify avatar, nickname
- **Pages**: `pages/user/profile`

#### M-022: Address Management【Foundation】
- Address list: display recipient, phone, detailed address, is default
- Operations: add, edit, delete address, set as default
- Address validation: auto-recognize province/city/district, support address search
- **Pages**: `pages/user/addresses`

#### M-023: Collections & Feedback【Utility】
- **My Collections**: Favorite instruments, maintenance packages
- **Feedback**: Fill feedback content + images, admin can reply after submission
- **Settings**: Notification switch, clear cache, about us
- **Invitation Code**: Generate exclusive invitation code, earn points when referrals make purchases
- **Pages**: `pages/user/collections`, `pages/user/feedback`, `pages/user/settings`

### 4.4 System Configuration & Management【Utility】

#### A-021: Finance & Data Statistics【Core】
- **Commission & Settlement Management**:
  - Commission rule setting: global commission, personalized commission, settlement cycle (weekly/monthly)
  - Commission details: view, filter, export all merchant commission details
  - Settlement operations: auto settlement, manual settlement, settlement review, exception handling
  - Deposit management: deposit collection, refund, deduction
- **Data Statistics**:
  - Business overview: today/yesterday/this month order count, transaction amount, total deposit, refund amount
  - Rental statistics: by instrument type, rental period, time range
  - Maintenance statistics: by service type, instrument type, time range
  - User statistics: new users, active users, repurchase rate
- **Order Export**: Support export to Excel by time, order type, status
- **Pages**: `pages/finance/commission`, `pages/data/stats`
- **API Dependencies**: `[Backend-API]-S-001` Statistics API, `[Backend-API]-S-002` Settlement API

#### A-022: Homepage & Announcement Configuration【Utility】
- **Homepage Configuration**:
  - Carousel management: upload images, set jump links, sorting, display status
  - Recommended instruments: manually set homepage recommendations
- **Announcement Management**:
  - Add/edit/delete announcements, set title, content, category, publish time, sticky
  - Auto push to mini program announcement center after publish
- **Basic Settings**:
  - Payment settings: configure WeChat Pay merchant ID, key, payment timeout
  - Fee settings: configure delivery fee, pickup fee, late fee rules, deposit rules
  - Message settings: configure mini program message, SMS templates
  - Store management: add/edit store address, business hours, contact phone
- **Pages**: `pages/system/home-config`, `pages/system/announcement`, `pages/system/settings`

---

## 🔧 Appendix: Task Priority & Version Planning

### P0 Priority (MVP Foundation, ~40 tasks) -【v1.0 Priority】
1. **User Authentication Module** (M-001, B-001)
2. **Instrument Display Closed Loop** (M-002, M-003, M-004, A-001-A-006, B-002, B-003, B-004)
3. **Basic Rental Flow** (M-005 order, M-006 payment, M-007 order list, A-007 order list, A-008 partial status operations)
4. **Order Management View** (M-008 order detail view, M-007 list)
5. **Stock Sync** (B-004 stock query, A-005 stock management)

### P1 Priority (Complete Business Features, ~60 tasks) -【v2.0-v3.0】
1. Complete rental closed loop (renewal M-009, return M-010, deposit management M-011, A-009 overdue management, A-010 deposit management)
2. Merchant Onboarding & Review (A-016, A-017, A-018)
3. Merchant Self Management (A-019, A-020)
4. Platform Admin Backend (A-008 complete order flow, A-011, A-012)
5. User Review System

### P2 Priority (Enhanced Features, ~70 tasks) -【v4.0】
1. Complete maintenance closed loop (M-012 to M-016, A-011 to A-015)
2. Technician Management & Assignment (A-014)
3. Complete Personal Center (M-021, M-022, M-023)
4. Commission & Settlement Management (A-021)
5. Data Statistics & Reporting (A-021)

### P3 Priority (Optimization & Auxiliary, ~68 tasks) -【v5.0 Optimization】
1. Invitation Code Feature (M-023)
2. Membership Level System (A-021)
3. Homepage Configuration & Announcements (A-022)
4. Excel Import/Export Optimization (A-006)
5. Cache Optimization & Performance Tuning
6. UI/UX Detail Optimization

---

## 📍 Version Iteration Planning Suggestion

### v1.0 MVP (Priority 4 weeks) - Core Rental Loop Foundation
- **Goal**: Achieve complete demonstrable flow from instrument display to rental order payment
- **Included Modules**:
  - User authentication & login
  - Instrument category, list, detail display
  - Admin backend: instrument, category, stock management
  - Mini program: rental order placement, payment, order viewing
  - Merchant backend: rental order list view, pickup confirmation
- **Deliverable**: End-to-end demonstrable rental flow

### v2.0 Merchant Onboarding (Week 5-6) - Multi-Merchant System
- **Goal**: Support merchant onboarding and self-management
- **New Modules**:
  - Merchant onboarding application and platform review
  - Merchant backend: self-manage instruments, orders, store info
  - Merchant roles and permission assignment
- **Deliverable**: Multi-merchant operation model support

### v3.0 Rental Closed Loop (Week 7-8) - Complete Lifecycle
- **Goal**: Achieve complete rental full-lifecycle management
- **New Modules**:
  - Renewal, return, overdue management
  - Deposit management and refund
  - Complete order status flow
  - Data statistics and reporting
- **Deliverable**: Complete rental lifecycle management

### v4.0 Maintenance Closed Loop (Week 9-10) - Value-Added Services
- **Goal**: Launch maintenance and repair services
- **New Modules**:
  - Maintenance package management and display
  - Full flow: appointment, quote confirmation, payment
  - Technician management and assignment system
- **Deliverable**: Maintenance value-added services online

### v5.0 Platform Operations (Week 11-12) - Full Operations Support
- **Goal**: Complete platform operation capabilities
- **New Modules**:
  - Complete platform admin features (user management, finance settlement, system configuration)
  - Membership system and points features
  - Homepage configuration, announcement push
  - Full data statistics and analysis
- **Deliverable**: Fully operation-ready SaaS platform

---

## 📊 Supplementary Notes

### Core Value of "Vertical Closed Loop" Thinking

Compared to traditional "horizontal feature module development" (develop all mini program features first, then all admin features), **Vertical Closed Loop** emphasizes:

1. **End-to-end delivery**: Each phase forms an independently runnable complete business flow (e.g., rental closed loop from order to return)
2. **Fast validation**: Prioritize MVP closed loop to let team and users quickly validate core value feasibility
3. **Risk reduction**: Avoid risk of "developed many features but can't form closed loop"
4. **Incremental expansion**: Each closed loop is incremental expansion on previous one (Display → Rental → Maintenance → Platform)

### Task Splitting Principles

- **Atomicity**: Each task development cycle controlled in 2-4 hours, completable in one workday
- **Independence**: Tasks as independent as possible to reduce dependency blocking
- **Testability**: Each task completion has clear acceptance criteria
- **Traceability**: All tasks linked to specific business requirements and user value

---

*Model: kimi-k2-thinking*
