# TuneLoop Database Design Document

## 1. Overview

### 1.1 Purpose
This document defines the database table structure for the TuneLoop instrument rental management system.

### 1.2 Database Type
- **PostgreSQL** (Recommended)
- **ORM**: GORM

### 1.3 Naming Conventions
- Table names: snake_case
- Primary key: `id` (UUID type)
- Tenant isolation: All business tables include `tenant_id`
- Timestamps: `created_at`, `updated_at`

---

## 2. Table Structures

### 2.1 users

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| iam_sub | VARCHAR(255) | UNIQUE, NOT NULL | IAM identity |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| org_id | UUID | INDEX, NOT NULL | Organization ID |
| name | VARCHAR(255) | | User name |
| phone | VARCHAR(50) | | Phone number |
| email | VARCHAR(255) | | Email |
| credit_score | INT | DEFAULT 600 | Credit score |
| deposit_mode | VARCHAR(20) | DEFAULT 'standard' | Deposit mode |
| is_shadow | BOOLEAN | DEFAULT true | Shadow user flag |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

### 2.2 categories

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| name | VARCHAR(100) | NOT NULL | Category name |
| icon | VARCHAR | | Icon (emoji or URL) |
| parent_id | UUID | | Parent category ID |
| level | INT | DEFAULT 1 | Level (1=first, 2=second) |
| sort | INT | DEFAULT 0 | Sort order |
| visible | BOOLEAN | DEFAULT true | Visibility |
| created_at | TIMESTAMP | | Created timestamp |

### 2.3 instruments

**Important Changes** (2026-04-16):
- Instruments no longer have a `name` field - identified solely by `sn` (serial number)
- Brand, model, and other attributes are now dynamic properties in `instrument_properties` table
- Basic instrument info: SN, category, site, level only

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| org_id | UUID | INDEX | Organization ID |
| category_id | UUID | INDEX | Category ID |
| category_name | VARCHAR(100) | | Category name (redundant) |
| sn | VARCHAR(100) | | **Serial number (unique identifier)** |
| level_id | UUID | INDEX | Level ID (references instrument_levels) |
| level_name | VARCHAR(50) | | Level name (redundant) |
| site_id | UUID | INDEX | Site ID |
| current_site_id | UUID | INDEX | Current site ID |
| description | TEXT | | Description |
| images | JSONB | DEFAULT '[]' | Image URLs |
| video | VARCHAR(500) | | Video URL |
| specifications | JSONB | DEFAULT '{}' | Specifications |
| pricing | JSONB | DEFAULT '{}' | Pricing info |
| stock_status | VARCHAR(20) | DEFAULT 'available' | Stock status |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

**Removed Fields**:
- `name`: Instrument name (removed, use `sn` as identifier)
- `brand`: Brand (moved to dynamic properties)
- `model`: Model (moved to dynamic properties)
- `level`: Level string (deprecated, use `level_id`)
- `site`: Site name (deprecated, use `site_id`)

**Indexes**:
- `idx_instruments_tenant_category` ON (tenant_id, category_id)
- `idx_instruments_tenant_status` ON (tenant_id, stock_status)

### 2.4 instrument_levels

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| caption | VARCHAR(50) | UNIQUE, NOT NULL | Display name |
| code | VARCHAR(20) | UNIQUE, NOT NULL | Code |
| sort_order | INT | DEFAULT 0 | Sort order |
| created_at | TIMESTAMP | | Created timestamp |

### 2.5 orders

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| org_id | UUID | INDEX | Organization ID |
| user_id | UUID | NOT NULL, INDEX | User ID |
| instrument_id | UUID | NOT NULL | Instrument ID |
| level | VARCHAR(20) | NOT NULL | Lease level |
| lease_term | INT | NOT NULL | Lease term (months) |
| deposit_mode | VARCHAR(20) | DEFAULT 'standard' | Deposit mode |
| monthly_rent | DECIMAL(10,2) | NOT NULL | Monthly rent |
| deposit | DECIMAL(10,2) | DEFAULT 0 | Deposit amount |
| accumulated_months | INT | DEFAULT 0 | Accumulated months |
| status | VARCHAR(20) | DEFAULT 'pending', INDEX | Order status |
| start_date | DATE | | Start date |
| end_date | DATE | | End date |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

### 2.6 sites

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| org_id | UUID | INDEX | Organization ID |
| parent_id | UUID | INDEX | Parent site ID |
| manager_id | UUID | INDEX | Manager ID |
| name | VARCHAR(255) | NOT NULL | Site name |
| address | VARCHAR(500) | | Address |
| type | VARCHAR(50) | | Site type |
| latitude | DECIMAL(6,6) | | Latitude |
| longitude | DECIMAL(6,6) | | Longitude |
| phone | VARCHAR(50) | | Contact phone |
| business_hours | VARCHAR(100) | | Business hours |
| status | VARCHAR(20) | DEFAULT 'active' | Status |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

### 2.7 maintenance_tickets

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| org_id | UUID | INDEX | Organization ID |
| order_id | UUID | NOT NULL | Order ID |
| instrument_id | UUID | NOT NULL | Instrument ID |
| user_id | UUID | NOT NULL, INDEX | User ID |
| problem_description | TEXT | | Problem description |
| images | JSONB | DEFAULT '[]' | Problem images |
| service_type | VARCHAR(20) | | Service type |
| status | VARCHAR(20) | DEFAULT 'PENDING', INDEX | Status |
| assigned_site_id | UUID | | Assigned site ID |
| technician_id | UUID | INDEX | Technician ID |
| progress_notes | TEXT | | Progress notes |
| repair_report | TEXT | | Repair report |
| repair_photos | JSONB | DEFAULT '[]' | Repair photos |
| estimated_cost | DECIMAL(10,2) | DEFAULT 0 | Estimated cost |
| accepted_at | TIMESTAMP | | Accepted timestamp |
| completion_notes | TEXT | | Completion notes |
| completion_photos | JSONB | DEFAULT '[]' | Completion photos |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |
| completed_at | TIMESTAMP | INDEX | Completed timestamp |

### 2.8 properties

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| name | VARCHAR(100) | NOT NULL | Property name |
| property_type | VARCHAR(20) | NOT NULL | Property type |
| is_required | BOOLEAN | DEFAULT false | Required flag |
| unit | VARCHAR(50) | | Unit |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

### 2.9 property_options

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| property_id | UUID | INDEX, NOT NULL | Property ID |
| value | VARCHAR(255) | NOT NULL | Option value |
| status | VARCHAR(20) | DEFAULT 'pending' | Status |
| alias | UUID | INDEX | Alias target |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

### 2.10 labels

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| tenant_id | UUID | INDEX, NOT NULL | Tenant ID |
| name | VARCHAR(100) | NOT NULL, INDEX | Label name |
| alias | JSONB | DEFAULT '[]' | Aliases |
| audit_status | VARCHAR(20) | DEFAULT 'pending' | Audit status |
| normalized_to_id | UUID | INDEX | Normalized target ID |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

### 2.11 tenants

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() | Primary key |
| name | VARCHAR(100) | NOT NULL | Tenant name |
| status | VARCHAR(20) | DEFAULT 'active' | Status |
| description | TEXT | | Description |
| created_at | TIMESTAMP | | Created timestamp |
| updated_at | TIMESTAMP | | Updated timestamp |

---

## Appendix A: Entity Relationships

```
users (1) ---> (N) orders
users (1) ---> (N) maintenance_tickets
orders (1) ---> (1) instruments
orders (1) ---> (1) ownership_certificates
instruments (N) ---> (1) categories
instruments (N) ---> (1) instrument_levels
categories (1) ---> (N) categories (self-reference)
sites (1) ---> (N) sites (self-reference)
tenants (1) ---> (N) users
tenants (1) ---> (N) clients
```

---

*Model: glm-5*
