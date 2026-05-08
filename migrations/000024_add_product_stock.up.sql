ALTER TABLE products ADD COLUMN stock INT NOT NULL DEFAULT -1;

COMMENT ON COLUMN products.stock IS '-1 = unlimited, 0 = out of stock, >0 = available quantity';

ALTER TABLE orders ADD COLUMN stock_reserved BOOLEAN NOT NULL DEFAULT false;
