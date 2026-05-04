ALTER TYPE order_status_enum ADD VALUE IF NOT EXISTS 'cancelled';
ALTER TYPE order_status_transition_enum ADD VALUE IF NOT EXISTS 'cancelled';
