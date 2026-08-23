package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type digiflazzItem struct {
	BuyerSKUCode string `json:"buyer_sku_code"`
	ProductName  string `json:"product_name"`
	Price        int    `json:"price"`
	Category     string `json:"category"`
	Brand        string `json:"brand"`
	Desc         string `json:"desc"`
}

func main() {
	user := os.Getenv("DIGIFLAZZ_USERNAME")
	key := os.Getenv("DIGIFLAZZ_API_KEY")
	if user == "" || key == "" {
		fmt.Fprintln(os.Stderr, "DIGIFLAZZ_USERNAME and DIGIFLAZZ_API_KEY must be set")
		os.Exit(1)
	}

	raw := user + key + "pricelist"
	sum := md5.Sum([]byte(raw))
	sign := hex.EncodeToString(sum[:])

	body, _ := json.Marshal(map[string]string{"username": user, "sign": sign})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := "https://api.digiflazz.com/v1/transaction/../price-list"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, "request:", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "do:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "status %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		fmt.Fprintf(os.Stderr, "parse envelope: %v body=%s\n", err, string(respBody))
		os.Exit(1)
	}

	var errResp struct {
		RC      string `json:"rc"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(envelope.Data, &errResp); err == nil && errResp.RC != "" && errResp.RC != "00" {
		fmt.Fprintf(os.Stderr, "digiflazz error rc=%s: %s\n", errResp.RC, errResp.Message)
		os.Exit(1)
	}

	var items []digiflazzItem
	if err := json.Unmarshal(envelope.Data, &items); err != nil {
		fmt.Fprintf(os.Stderr, "parse items: %v\n", err)
		os.Exit(1)
	}

	want := strings.ToLower(strings.TrimSpace(arg(1, "tri")))
	fmt.Printf("%-15s %-45s %10s  %-15s %-15s\n", "SKU", "NAME", "PRICE", "BRAND", "CATEGORY")
	sort.Slice(items, func(i, j int) bool {
		if items[i].Brand != items[j].Brand {
			return items[i].Brand < items[j].Brand
		}
		if items[i].Price != items[j].Price {
			return items[i].Price < items[j].Price
		}
		return items[i].BuyerSKUCode < items[j].BuyerSKUCode
	})
	matched := 0
	for _, it := range items {
		if it.BuyerSKUCode == "" || it.Price <= 0 {
			continue
		}
		haystack := strings.ToLower(it.Brand + " " + it.Category + " " + it.ProductName + " " + it.BuyerSKUCode)
		if want == "all" {
			matched++
			fmt.Printf("%-15s %-45s %10d  %-15s %-15s\n", it.BuyerSKUCode, trunc(it.ProductName, 45), it.Price, it.Brand, it.Category)
			continue
		}
		if strings.Contains(haystack, want) {
			matched++
			fmt.Printf("%-15s %-45s %10d  %-15s %-15s\n", it.BuyerSKUCode, trunc(it.ProductName, 45), it.Price, it.Brand, it.Category)
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d matching items (filter=%q, total=%d)\n", matched, want, len(items))
}

func arg(i int, def string) string {
	if i < len(os.Args) && os.Args[i] != "" {
		return os.Args[i]
	}
	return def
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
