UPDATE orders SET deposit_mode = 'standard' WHERE deposit_mode = 'ratio';
UPDATE instruments SET deposit_mode = 'standard' WHERE deposit_mode = 'ratio';
