package constants

const (
	StatusPending    = "pending"
	StatusPaid       = "paid"
	StatusProcessing = "processing"
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusExpired    = "expired"
	StatusCancelled  = "cancelled"
)

const (
	GameFreeFire        = "free_fire"
	GameMobileLegends   = "mobile_legends"
	GamePUBGMobile      = "pubg_mobile"
	GameGenshinImpact   = "genshin_impact"
	GameHonkaiStarRail  = "honkai_star_rail"
	GameZenlessZoneZero = "zenless_zone_zero"
	GameHonkaiImpact3   = "honkai_impact_3"
	GamePulsa           = "pulsa"
)

const (
	ChannelWeb      = "web"
	ChannelWhatsApp = "whatsapp"
)

const (
	ProductTypeDiamond      = "diamond"
	ProductTypeSubscription = "subscription"
	ProductTypeOther        = "other"
	ProductTypeValidation   = "validation"
	ProductTypePulsa        = "pulsa"
)

const (
	DigiflazzAPIURL = "https://api.digiflazz.com/v1/transaction"
)

var ValidGames = map[string]bool{
	GameFreeFire:        true,
	GameMobileLegends:   true,
	GamePUBGMobile:      true,
	GameGenshinImpact:   true,
	GameHonkaiStarRail:  true,
	GameZenlessZoneZero: true,
	GameHonkaiImpact3:   true,
	GamePulsa:           true,
}

var GameLabels = map[string]string{
	GameFreeFire:        "Free Fire",
	GameMobileLegends:   "Mobile Legends",
	GamePUBGMobile:      "PUBG Mobile",
	GameGenshinImpact:   "Genshin Impact",
	GameHonkaiStarRail:  "Honkai: Star Rail",
	GameZenlessZoneZero: "Zenless Zone Zero",
	GameHonkaiImpact3:   "Honkai Impact 3",
	GamePulsa:           "Pulsa",
}

var ServerRequiredGames = map[string]bool{
	GameMobileLegends:   true,
	GameGenshinImpact:   true,
	GameHonkaiStarRail:  true,
	GameZenlessZoneZero: true,
	GameHonkaiImpact3:   true,
}

var PipeServerCustomerNoGames = map[string]bool{
	GameGenshinImpact:   true,
	GameHonkaiStarRail:  true,
	GameZenlessZoneZero: true,
	GameHonkaiImpact3:   true,
}

var StatusLabels = map[string]string{
	StatusPending:    "Menunggu Pembayaran",
	StatusPaid:       "Lunas - Sedang Diproses",
	StatusProcessing: "Sedang Diproses",
	StatusSuccess:    "Berhasil",
	StatusFailed:     "Gagal",
	StatusExpired:    "Kadaluarsa",
	StatusCancelled:  "Dibatalkan",
}
