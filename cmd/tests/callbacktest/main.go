package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/CedrosPay/server/internal/callbacks"
	"github.com/CedrosPay/server/internal/config"
)

func main() {
	configPath := flag.String("config", "configs/local.yaml", "path to config yaml")
	resource := flag.String("resource", "callback-test", "resource identifier to send")
	method := flag.String("method", "test", "payment method label")
	amount := flag.Float64("amount", 0, "amount used in the synthetic callback event")
	wallet := flag.String("wallet", "", "wallet identifier used in the synthetic callback event")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.Callbacks.PaymentSuccessURL == "" {
		log.Fatalf("callback payment_success_url is not configured")
	}

	event := callbacks.PaymentEvent{
		ResourceID:         *resource,
		Method:             *method,
		FiatAmountCents:    int64(*amount * 100),
		CryptoAtomicAmount: int64(*amount * 1000000), // Assuming USDC (6 decimals)
		Wallet:             *wallet,
		PaidAt:             time.Now().UTC(),
	}

	if err := callbacks.SendOnce(context.Background(), cfg.Callbacks, event); err != nil {
		log.Fatalf("send callback: %v", err)
	}

	fmt.Println("callback delivered to", cfg.Callbacks.PaymentSuccessURL)
}
