ALTER TABLE merchants ADD CONSTRAINT merchants_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants(id);
