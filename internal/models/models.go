package models

import "time"

type Product struct {
	ID               string     `json:"id"`
	Game             string     `json:"game"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	PriceIDR         int        `json:"price_idr"`
	CostPriceIDR     int        `json:"cost_price_idr"`
	Diamonds         int        `json:"diamonds"`
	ProductType      string     `json:"product_type"`
	SKU              string     `json:"sku"`
	CustomerNoFormat string     `json:"customer_no_format"`
	IsActive         bool       `json:"is_active"`
	Stock            int        `json:"stock"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type Order struct {
	ID              string    `json:"id"`
	OrderNumber     string    `json:"order_number"`
	ProductID       string    `json:"product_id"`
	UserPhone       string    `json:"user_phone"`
	GameUID         string    `json:"game_uid"`
	GameServer      string    `json:"game_server"`
	AmountIDR       int       `json:"amount_idr"`
	Status          string    `json:"status"`
	MidtransOrderID *string   `json:"midtrans_order_id"`
	Channel         string    `json:"channel"`
	SerialNumber    *string   `json:"serial_number"`
	DigiflazzRefID  string    `json:"digiflazz_ref_id"`
	StockReserved   bool      `json:"stock_reserved"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OrderQRIS struct {
	OrderID         string     `json:"order_id"`
	QRISURL         *string    `json:"qris_url"`
	QRString        *string    `json:"qr_string"`
	QRISImageBase64 *string    `json:"qris_image_base64"`
	ExpiryTime      *time.Time `json:"expiry_time"`
	CreatedAt       time.Time  `json:"created_at"`
}

type OrderSnap struct {
	OrderID         string    `json:"order_id"`
	SnapToken       string    `json:"snap_token"`
	SnapRedirectURL string    `json:"snap_redirect_url"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OrderStatusHistory struct {
	ID         string    `json:"id"`
	OrderID    string    `json:"order_id"`
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     *string   `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}

type WebhookLog struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	RefID     *string   `json:"ref_id"`
	Payload   string    `json:"payload"`
	Signature *string   `json:"signature"`
	UserAgent *string   `json:"user_agent"`
	Status    string    `json:"status"`
	Error     *string   `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}
