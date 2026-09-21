-- #2010 S1 down：移除并入 orders 的收货地址列
ALTER TABLE orders DROP COLUMN IF EXISTS delivery_address;
