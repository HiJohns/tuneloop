-- #1786: Electronic invoice application flow
-- orders: add invoice_applied flag
ALTER TABLE orders ADD COLUMN IF NOT EXISTS invoice_applied    boolean      NOT NULL DEFAULT false;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS invoice_applied_at timestamptz;

-- invoice_applications: one row per merchant per customer submission
CREATE TABLE IF NOT EXISTS invoice_applications (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL,
  tenant_id    uuid NOT NULL,
  status       varchar(20) NOT NULL DEFAULT 'pending',
  total_amount bigint NOT NULL DEFAULT 0,
  order_count  int    NOT NULL DEFAULT 0,
  reply        text,
  invoice_file text,
  replied_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invoice_app_user ON invoice_applications(user_id);
CREATE INDEX IF NOT EXISTS idx_invoice_app_tenant ON invoice_applications(tenant_id);

-- invoice_application_orders: join table linking application → orders
CREATE TABLE IF NOT EXISTS invoice_application_orders (
  application_id uuid NOT NULL REFERENCES invoice_applications(id) ON DELETE CASCADE,
  order_id       uuid NOT NULL,
  PRIMARY KEY (application_id, order_id)
);
