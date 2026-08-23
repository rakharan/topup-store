-- Cannot remove an enum value in PG; leave 'admin_direct' in channel_enum.
-- Soft-mark any admin_direct orders as 'web' for downgrade purposes (rare).
UPDATE orders SET channel = 'web' WHERE channel = 'admin_direct';
