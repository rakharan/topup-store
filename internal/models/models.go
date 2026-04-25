package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Product struct {
	ID          string    `json:"id"`
	Game        string    `json:"game"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceIDR    int       `json:"price_idr"`
	Diamonds    int       `json:"diamonds"`
	SKU         string    `json:"sku"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

type Order struct {
	ID              string    `json:"id"`
	ProductID       string    `json:"product_id"`
	UserPhone       string    `json:"user_phone"`
	GameUID         string    `json:"game_uid"`
	GameServer      string    `json:"game_server"`
	AmountIDR       int       `json:"amount_idr"`
	Status          string    `json:"status"`
	MidtransOrderID *string   `json:"midtrans_order_id"`
	QRISURL         *string   `json:"qris_url"`
	QRISImageBase64 *string   `json:"qris_image_base64"`
	Channel         string    `json:"channel"`
	SerialNumber    *string   `json:"serial_number"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
