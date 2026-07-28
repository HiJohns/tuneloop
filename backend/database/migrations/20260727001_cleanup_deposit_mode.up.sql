-- Deposit mode cleanup: existing "standard" orders -> ratio (deposit already calculated correctly)
UPDATE orders SET deposit_mode = 'ratio' WHERE deposit_mode = 'standard';
UPDATE instruments SET deposit_mode = 'ratio' WHERE deposit_mode = 'standard';
