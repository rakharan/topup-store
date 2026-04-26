require("dotenv").config();
const express = require("express");
const axios = require("axios");

const GO_BACKEND_URL = process.env.GO_BACKEND_URL || "http://localhost:8080";
const BOT_PORT = process.env.BOT_PORT || 3001;
const WA_TOKEN = process.env.WHATSAPP_TOKEN;
const WA_PHONE_ID = process.env.WHATSAPP_PHONE_NUMBER_ID;
const WA_VERIFY_TOKEN = process.env.WHATSAPP_VERIFY_TOKEN || "topup-store-verify";
const BOT_NOTIFY_TOKEN = process.env.BOT_NOTIFY_TOKEN;

const WA_API = `https://graph.facebook.com/v18.0/${WA_PHONE_ID}/messages`;

const app = express();
app.use(express.json());

app.get("/webhook", (req, res) => {
  const mode = req.query["hub.mode"];
  const token = req.query["hub.verify_token"];
  const challenge = req.query["hub.challenge"];

  if (mode === "subscribe" && token === WA_VERIFY_TOKEN) {
    console.log("WhatsApp webhook verified.");
    res.status(200).send(challenge);
  } else {
    res.sendStatus(403);
  }
});

app.post("/webhook", async (req, res) => {
  const body = req.body;

  if (body.object === "whatsapp_business_account") {
    for (const entry of body.entry) {
      for (const change of entry.changes) {
        if (change.value.messages) {
          for (const msg of change.value.messages) {
            if (msg.type === "text") {
              await handleIncoming(msg);
            }
          }
        }
      }
    }
  }

  res.sendStatus(200);
});

async function handleIncoming(msg) {
  const sender = msg.from;
  const text = msg.text?.body?.trim();
  if (!text) return;

  const order = parseOrderMessage(text);
  if (order) {
    await handleOrder(sender, order);
  } else {
    await sendHelpMenu(sender);
  }
}

async function handleOrder(sender, order) {
  try {
    const resp = await axios.post(
      `${GO_BACKEND_URL}/api/orders`,
      {
        game: order.game,
        diamonds: order.diamonds,
        game_uid: order.game_uid,
        game_server: order.game_server,
        phone: sender,
      },
      { headers: { "Content-Type": "application/json" } }
    );

    const { order_id, qris_url, qris_base64, amount_idr } = resp.data;

    const gameLabel = gameToLabel(order.game);
    const caption =
      `Halo! Berikut QRIS untuk pembayaran ${gameLabel} ${order.diamonds} diamonds.\n` +
      `Total: Rp${amount_idr.toLocaleString("id-ID")}\n` +
      `ID Order: ${order_id}\n` +
      `Scan QRIS di bawah untuk membayar:`;

    if (qris_url) {
      await sendImageWithCaption(sender, qris_url, caption);
    } else if (qris_base64) {
      await sendImageBase64(sender, qris_base64, caption);
    } else {
      await sendText(sender, `${caption}\nQRIS URL: ${qris_url}`);
    }

    console.log(`Order created for ${sender}: ${order_id}`);
  } catch (err) {
    console.error("Failed to create order:", err.message);
    await sendText(sender, "Maaf, terjadi kesalahan saat memproses pesanan. Silakan coba lagi.");
  }
}

async function sendHelpMenu(sender) {
  const helpText =
    "🎮 *Top-Up Game*\n\n" +
    "Kirim pesanan dengan format:\n" +
    "`FF <diamonds> UID:<uid>` — Free Fire\n" +
    "`ML <diamonds> UID:<uid>|<server>` — Mobile Legends\n" +
    "`PUBG <diamonds> UID:<uid>` — PUBG Mobile\n\n" +
    "Contoh:\n" +
    "`FF 100 UID:12345678`\n" +
    "`ML 86 UID:12345|1234`\n" +
    "`PUBG 60 UID:12345678`\n\n" +
    "Silakan pilih game dan jumlah diamond yang diinginkan.";

  await sendText(sender, helpText);
}

async function sendText(to, text) {
  try {
    await axios.post(WA_API, {
      messaging_product: "whatsapp",
      to,
      type: "text",
      text: { body: text },
    }, {
      headers: {
        Authorization: `Bearer ${WA_TOKEN}`,
        "Content-Type": "application/json",
      },
    });
  } catch (err) {
    console.error("Failed to send text:", err.response?.data || err.message);
  }
}

async function sendImageWithCaption(to, imageUrl, caption) {
  try {
    await axios.post(WA_API, {
      messaging_product: "whatsapp",
      to,
      type: "image",
      image: { link: imageUrl, caption },
    }, {
      headers: {
        Authorization: `Bearer ${WA_TOKEN}`,
        "Content-Type": "application/json",
      },
    });
  } catch (err) {
    console.error("Failed to send image:", err.response?.data || err.message);
    await sendText(to, `${caption}\nQRIS URL: ${imageUrl}`);
  }
}

async function sendImageBase64(to, base64Data, caption) {
  try {
    const buffer = Buffer.from(base64Data, "base64");
    const FormData = (await import("form-data")).default;
    const formData = new FormData();
    formData.append("messaging_product", "whatsapp");
    formData.append("file", buffer, { filename: "qris.png", contentType: "image/png" });
    formData.append("type", "image/png");

    const uploadResp = await axios.post(
      `https://graph.facebook.com/v18.0/${WA_PHONE_ID}/media`,
      formData,
      {
        headers: {
          Authorization: `Bearer ${WA_TOKEN}`,
          ...formData.getHeaders(),
        },
      }
    );

    const mediaId = uploadResp.data.id;

    await axios.post(WA_API, {
      messaging_product: "whatsapp",
      to,
      type: "image",
      image: { id: mediaId, caption },
    }, {
      headers: {
        Authorization: `Bearer ${WA_TOKEN}`,
        "Content-Type": "application/json",
      },
    });
  } catch (err) {
    console.error("Failed to send image base64:", err.response?.data || err.message);
    await sendText(to, `${caption}\n(Gambar QRIS tidak dapat dikirim)`);
  }
}

function gameToLabel(game) {
  switch (game) {
    case "free_fire":
      return "Free Fire";
    case "mobile_legends":
      return "Mobile Legends";
    case "pubg_mobile":
      return "PUBG Mobile";
    default:
      return game;
  }
}

function parseOrderMessage(text) {
  const patterns = [
    /^(FF|Free\s*Fire)\s+(\d+)\s+UID[:\s]+(\d+)$/i,
    /^(ML|Mobile\s*Legends)\s+(\d+)\s+UID[:\s]+(\d+)\|(\d+)$/i,
    /^(PUBG|PUBG\s*Mobile)\s+(\d+)\s+UID[:\s]+(\d+)$/i,
  ];

  for (const pattern of patterns) {
    const match = text.match(pattern);
    if (match) {
      const gameRaw = match[1].toLowerCase();
      let game;
      if (gameRaw.startsWith("ff") || gameRaw.includes("fire")) {
        game = "free_fire";
      } else if (gameRaw.startsWith("ml") || gameRaw.includes("legend")) {
        game = "mobile_legends";
      } else {
        game = "pubg_mobile";
      }

      return {
        game,
        diamonds: parseInt(match[2], 10),
        game_uid: match[3],
        game_server: match[4] || "",
      };
    }
  }

  return null;
}

app.get("/health", (_req, res) => {
  res.json({ status: "ok" });
});

app.post("/notify", async (req, res) => {
  if (BOT_NOTIFY_TOKEN) {
    const token = req.headers["x-bot-token"];
    if (!token || token !== BOT_NOTIFY_TOKEN) {
      return res.status(401).json({ error: "unauthorized" });
    }
  }
  const { phone, message } = req.body;
  if (!phone || !message) {
    return res.status(400).json({ error: "phone and message are required" });
  }
  try {
    await sendText(phone, message);
    res.status(200).json({ status: "ok" });
  } catch (err) {
    console.error("Failed to send notification:", err.message);
    res.status(500).json({ error: "failed to send notification" });
  }
});

app.listen(BOT_PORT, () => {
  console.log(`WhatsApp bot API listening on port ${BOT_PORT}`);
});
