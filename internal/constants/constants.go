package constants

const (
	StatusPending    = "pending"
	StatusPaid       = "paid"
	StatusProcessing = "processing"
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
)

const (
	GameFreeFire      = "free_fire"
	GameMobileLegends = "mobile_legends"
	GamePUBGMobile    = "pubg_mobile"
)

const (
	ChannelWeb      = "web"
	ChannelWhatsApp = "whatsapp"
)

const (
	DigiflazzAPIURL     = "https://api.digiflazz.com/v1/transaction"
	WhatsAppCloudAPIURL = "https://graph.facebook.com/v18.0"
)

var ValidGames = map[string]bool{
	GameFreeFire:      true,
	GameMobileLegends: true,
	GamePUBGMobile:    true,
}

var GameLabels = map[string]string{
	GameFreeFire:      "Free Fire",
	GameMobileLegends: "Mobile Legends",
	GamePUBGMobile:    "PUBG Mobile",
}

var StatusLabels = map[string]string{
	StatusPending:    "Menunggu Pembayaran",
	StatusPaid:       "Lunas - Sedang Diproses",
	StatusProcessing: "Sedang Diproses",
	StatusSuccess:    "Berhasil",
	StatusFailed:     "Gagal",
	StatusExpired:    "Kadaluarsa",
}
