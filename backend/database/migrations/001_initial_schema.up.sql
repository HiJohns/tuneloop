CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    iam_sub VARCHAR(255) UNIQUE NOT NULL,
    tenant_id UUID NOT NULL,
    org_id UUID NOT NULL,
    name VARCHAR(255),
    phone VARCHAR(50),
    email VARCHAR(255),
    credit_score INT DEFAULT 600,
    deposit_mode VARCHAR(20) DEFAULT 'standard',
    is_shadow BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_iam_sub ON users(iam_sub);

CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    icon VARCHAR(255),
    parent_id UUID REFERENCES categories(id),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE instruments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    brand VARCHAR(100),
    level VARCHAR(20) NOT NULL,
    level_name VARCHAR(50),
    description TEXT,
    images JSONB DEFAULT '[]',
    video VARCHAR(500),
    specifications JSONB DEFAULT '{}',
    pricing JSONB DEFAULT '{}',
    stock_status VARCHAR(20) DEFAULT 'available',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    instrument_id UUID REFERENCES instruments(id),
    level VARCHAR(20) NOT NULL,
    lease_term INT NOT NULL,
    deposit_mode VARCHAR(20) DEFAULT 'standard',
    monthly_rent DECIMAL(10,2) NOT NULL,
    deposit DECIMAL(10,2) DEFAULT 0,
    accumulated_months INT DEFAULT 0,
    status VARCHAR(20) DEFAULT 'pending',
    start_date DATE,
    end_date DATE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_orders_user ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);

CREATE TABLE sites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    address VARCHAR(500),
    latitude DECIMAL(10,6),
    longitude DECIMAL(10,6),
    phone VARCHAR(50),
    business_hours VARCHAR(100),
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE maintenance_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID REFERENCES orders(id),
    instrument_id UUID REFERENCES instruments(id),
    user_id UUID REFERENCES users(id),
    problem_description TEXT,
    images JSONB DEFAULT '[]',
    service_type VARCHAR(20),
    status VARCHAR(20) DEFAULT 'pending',
    assigned_site_id UUID REFERENCES sites(id),
    estimated_cost DECIMAL(10,2) DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_maintenance_user ON maintenance_tickets(user_id);
CREATE INDEX idx_maintenance_status ON maintenance_tickets(status);

CREATE TABLE brand_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id VARCHAR(100) UNIQUE NOT NULL,
    primary_color VARCHAR(20) DEFAULT '#6366F1',
    logo_url VARCHAR(500),
    brand_name VARCHAR(100),
    support_phone VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_brand_client ON brand_configs(client_id);