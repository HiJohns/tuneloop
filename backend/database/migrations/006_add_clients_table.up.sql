-- Migration 006: Add clients table for IAM bootstrap
-- This table stores OAuth2/OIDC client information

CREATE TABLE clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id VARCHAR(100) UNIQUE NOT NULL,
    client_secret VARCHAR(255),
    name VARCHAR(100),
    redirect_uris TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_clients_client_id ON clients(client_id);