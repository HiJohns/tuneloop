CREATE TABLE IF NOT EXISTS membership_levels (
    id INTEGER PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    min_amount DECIMAL NOT NULL
);

INSERT INTO membership_levels (id, name, min_amount) VALUES
    (1, '初级', 0),
    (2, '中级', 5000),
    (3, '高级', 10000);
