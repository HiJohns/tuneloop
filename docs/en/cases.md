# Use Cases

## 0. Bootstrapping

**Goal**: Establish the first system admin and lock the initialization entry.

### 0.1 System Initialization Flow

1. **Access homepage**: User visits `/`
2. **System check**: Backend checks if `User` table is empty
3. **Route lock**:
   - If empty → redirect to `/setup`
   - If not empty and `/setup` accessed → return 403 or redirect to login
4. **Create system admin**:
   - Fields: email, password
   - Backend actions:
     a. Call IAM to create user (Project Admin role)
     b. Record UID in local `users` table
     c. Mark `is_system_admin = true`
5. **Login flow**: Redirect to IAM for first authentication

---

## 0.1 Merchant Management

**Terminology**: Merchant ↔ IAM Organization

### 0.1.1 Merchant List

**Permission**: Only users with `project_admin` claim in JWT

- Display: name, created_at, merchant code
- Delete logic: Block if merchant has active sites or incomplete instrument orders

### 0.1.2 Create Merchant

**Form fields**:
- Merchant name
- Merchant code (for URL/data isolation)
- Contact information
- Assign admin via User Selection Dialog

**Backend actions**:
1. Call IAM to create Organization
2. Assign admin user to org with "Organization Admin" role
3. Record merchant in local `merchants` table

---

## 0.2 User Selection Dialog

**Design principle**: Check first, then associate, create if not exists.

### 0.2.1 Search

Input: username, name, email, or phone

### 0.2.2 Result Handling

**Scenario A: User exists and belongs to current merchant**
- Display user info
- Return `User_ID` and `Name` on confirm

**Scenario B: User exists in platform but belongs to another merchant**
- Prompt: "User exists. Invite to join this merchant?"
- Backend: Call IAM API to associate UID to merchant org

**Scenario C: User does not exist**
- Prompt: "User not found. Create now?"
- Create form: name, email, phone, initial password
- Backend actions:
  1. Create user in IAM
  2. Associate to merchant org
  3. Return new UID

---

## 1. Instrument List

### 1.1 Add Instrument

Site staff logs in
Access instrument list
Click "Add Instrument"
Complete settings and submit
Instrument added to database

### 1.2 Batch Import

**Scenario**: Tenant admin or site manager needs to import multiple instruments at once (e.g., inventory check-in)

**Roles**: Tenant Admin, Site Manager

**Prerequisites**: CSV template and corresponding image/video files prepared

#### Workflow

1. **Download Template**
   - Navigate to instrument list page
   - Click "Batch Import" button
   - Download CSV template (fields: SN, category, level, description, dynamic property columns)
   - Fill in data following template instructions

2. **Upload CSV Validation**
   - Select filled CSV file for upload
   - System parses immediately and displays data in Grid table
   - Auto-highlight errors:
     - SN duplicates with database (red background)
     - SN duplicates within file (red background)
     - Required fields missing (red background)

3. **Inline Error Correction**
   - Double-click error cells to edit directly
   - No need to re-upload file
   - Auto-revalidates after modification

4. **Upload Media Package (Optional)**
   - Upload ZIP file (containing images/videos, naming format: SN_序号.jpg)
   - System auto-matches to corresponding instruments
   - Unmatched files shown in "Unmatched Zone", filename can be modified for rematching

5. **Confirm Import**
   - Click "Confirm Import"
   - System creates instruments transactionally
   - Results displayed: Success X items, Failure Y items with details

#### Error Handling

- File format error: Prompt correct CSV format
- SN duplicates: Block import, show conflicts
- Partial success: Show success/failure details, support retry for failed items

---

## 2. Rental Cycle

### 2.1 Inventory & Rent Setting

Site manager logs in
Access inventory management
View instrument list with rent prices
Filter by brand, model, category, level
Editable daily rent input box per instrument
Save button activates on changes
Batch save rent settings

### 2.2 Instrument Rental

User opens mini-program
Views instrument list with daily rent
Filter by category, site, level, status
Select instrument to view details:
- Latest images
- Brand, model, description
- Daily/weekly/monthly rent, deposit
- Order button to select rental period and address
- Complete payment → instrument booked
- System:
  - Generate shipping notification
  - Auto-generate PDF contract/receipt to user profile
During rental, user views "My":
- List of rental sessions (category, end date)
- Click to view order details
After rental period, user views "My":
- Click expired session
- Enter logistics info
- Return initiated

### 2.3 Warehouse Management

Staff logs in PC
View booked orders → arrange shipping
- After shipping, fill logistics info → status: shipped
Daily check shipped orders
- Delivered → status: in_lease (delivery time = start time)
Returned instrument arrives:
- Scan QR code → view info
- Take photos → upload
- If no damage → click "Return" → status: available
  - Auto-generate deposit refund
- If damaged:
  - Click "Assess Damage", enter comments, amount
  - Status: maintenance, create maintenance session

## 2.4 Appeals

User mini-program with damaged instrument:
- Receive damage report (photos, comments, amount)
- Click "Agree":
  - If deposit covers damage → deduct amount → refund deposit
  - If not enough → go to payment
  - If payment fails/timeout → record as appeal
- Click "Appeal", enter reason → submit
- Record appeal, instrument status: pending
Site manager views appeal list:
- Instrument info, rent, current photos
- User, staff info, damage report, appeal reason
- Rental history
Site manager can:
- Click "No Damage" → cancel claim, refund deposit, status: available
- Adjust claim amount
- Enter comments
- Click "Confirm" → status: maintenance
  - If deposit > claim → auto-generate deposit refund

---

## 3. Maintenance

### 3.1 Technician Management

Site manager enters site management
Create technician account (name, phone, etc.)
Delete technician account
View technician list with completion count
Click to view detailed orders

### 3.2 Maintenance Session

Technician logs in PC or WeChat
Views assigned maintenance orders (date, category, description, status)
Click to view details including photos
If pending: 'Accept' button → assigned, add to personal list
Technician clicks 'Start Work', scans QR code → status: in_progress
During maintenance: enter comments, upload photos
After completion: click 'Complete' → status: inspecting
Staff scans QR code, takes photos, clicks:
- 'Pass Inspection' → completed, status: available
- 'Fail Inspection' → add comments, status: pending

---

## 4. Organization Management

### 4.1 Site Management

#### 4.1.1 Site List

Site manager logs in PC
Access Organization → Site Management
Left side: lazy-loaded tree list
Click to expand sub-sites
Top-right: 'Create Top-level Site' button

#### 4.1.2 Create Site

Click 'Create Top-level Site'
URL: `/sites/new`
Right side: create site form
Fields: name, type, address, phone
Assign manager (system checks if user exists)
- If exists: fill username/email/phone
- If not exists: red error + link to create user dialog
Auto-fill manager after user creation
Check duplicate name before submit
After success: tree auto-updates and selects new site

#### 4.1.3 View Site Details

Click tree node
URL: `/sites/:id`
Right side: site details (name, type, address, phone, manager)
Manager clickable → `/staff/:id`

#### 4.1.4 Edit Site

Click 'Edit' on details page
URL: `/sites/:id/edit`
Reuse create form
Return to details after submit, tree syncs

#### 4.1.5 Site Member Management

**Permission**: Merchant admin or tenant global permission

**Member List Table**:
- Username (clickable)
- Role (Manager/Staff with colored tags)
- Join date
- Actions:
  - Switch role (Manager ↔ Staff)
  - Remove member

**Rules**:
- Last Manager: disable switch/remove buttons
- Add member: click 'Add Member' → open User Selection Dialog
- Default role: Staff

#### 4.1.6 Delete Site with Enhanced Validation

**Pre-checks**:
- Asset validation: Check instruments table
  - If available → Alert: "Transfer assets first"
  - If rented → Alert: "Process in-lease orders first"
- Member check: If members exist → Alert: "Remove all members first"

**Flow**:
- Click delete → Run validation
- All checks pass → Show confirm dialog
- Call DELETE API
- Success: Remove from tree, redirect to `/sites`

---

## 4.2 Staff Management

### 4.2.1 Staff List

Access Organization → Staff Management
URL: `/staff`
Table: name, email, phone, site, position, type, status
Search by name and site
Pagination support
Name clickable → user details

### 4.2.2 Create User

Click 'Create User'
Dialog: name, email, phone, site, position, type
Site dropdown: tree structure
Submit:
- Check email/phone uniqueness
- If conflict: show dialog with options
- "Continue" or "Select Existing"
Success: Close dialog, refresh list

### 4.2.3 Edit User

Click 'Edit' in list
Dialog: editable fields (email/phone disabled)
Submit → refresh list

---

*Model: zhipuai/glm-5*
