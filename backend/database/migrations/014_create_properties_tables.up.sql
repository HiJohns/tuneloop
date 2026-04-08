-- Migration 014: Create property management tables
-- Issue #218: Missing "properties" table in database

-- Create properties table
CREATE TABLE properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    name VARCHAR(100) NOT NULL,
    property_type VARCHAR(20) NOT NULL,
    is_required BOOLEAN DEFAULT false,
    unit VARCHAR(50),
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Create property_options table
CREATE TABLE property_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    property_id UUID NOT NULL,
    value VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    alias UUID,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Create instrument_properties table
CREATE TABLE instrument_properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    instrument_id UUID NOT NULL,
    property_id UUID NOT NULL,
    value VARCHAR(255),
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
