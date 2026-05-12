INSERT INTO products (game, name, description, price_idr, diamonds, sku) VALUES
('free_fire', '12 Diamonds', '12 Free Fire Diamonds', 1811, 12, 'ff_12'),
('free_fire', '50 Diamonds', '50 Free Fire Diamonds', 6405, 50, 'ff_50'),
('free_fire', '70 Diamonds', '70 Free Fire Diamonds', 8960, 70, 'ff_70'),
('free_fire', '140 Diamonds', '140 Free Fire Diamonds', 18360, 140, 'ff_140'),
('free_fire', '355 Diamonds', '355 Free Fire Diamonds', 44800, 355, 'ff_355'),
('mobile_legends', '5 Diamonds', '5 Mobile Legends Diamonds', 1680, 5, 'ml_5'),
('mobile_legends', '10 Diamonds', '10 Mobile Legends Diamonds', 2855, 10, 'ml_10'),
('mobile_legends', '12 Diamonds', '12 Mobile Legends Diamonds', 3303, 12, 'ml_12'),
('pubg_mobile', '60 UC', '60 PUBG Mobile UC', 15000, 60, 'pubg_60'),
('pubg_mobile', '180 UC', '180 PUBG Mobile UC', 42000, 180, 'pubg_180'),
('pubg_mobile', '325 UC', '325 PUBG Mobile UC', 72000, 325, 'pubg_325')
ON CONFLICT (sku) DO NOTHING;
