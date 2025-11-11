package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/memo"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/CedrosPay/server/internal/config"
	"github.com/CedrosPay/server/pkg/x402"
)

type quoteResponse struct {
	ResourceID string `json:"resourceId"`
	Granted    bool   `json:"granted"`
	Method     string `json:"method"`
	X402       *struct {
		Address   string    `json:"address"`
		Token     string    `json:"token"`
		Account   string    `json:"account"`
		Amount    float64   `json:"amount"`
		Memo      string    `json:"memo"`
		Network   string    `json:"network"`
		ExpiresAt time.Time `json:"expiresAt"`
	} `json:"x402"`
}

func main() {
	var (
		cfgPath    = flag.String("config", "configs/local.yaml", "path to Cedros config file")
		serverURL  = flag.String("server", "http://localhost:8080", "Cedros server base URL")
		resourceID = flag.String("resource", "", "paywall resource id to purchase")
		keypair    = flag.String("keypair", "", "path to Solana keypair (JSON produced by solana-keygen)")
		post       = flag.Bool("post", false, "call the paywall endpoint with the generated proof")
	)
	flag.Parse()

	if *resourceID == "" {
		log.Fatal("resource flag is required")
	}
	if *keypair == "" {
		log.Fatal("keypair flag is required")
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	baseURL := strings.TrimRight(*serverURL, "/")
	quote, err := fetchQuote(baseURL, *resourceID)
	if err != nil {
		log.Fatalf("fetch quote: %v", err)
	}
	if quote.Granted {
		log.Printf("resource %s already granted via method %s", *resourceID, quote.Method)
	}
	if quote.X402 == nil {
		log.Fatalf("resource %s does not expose an x402 option", *resourceID)
	}

	payerKey, err := solana.PrivateKeyFromSolanaKeygenFile(*keypair)
	if err != nil {
		log.Fatalf("load keypair: %v", err)
	}
	payerPub := payerKey.PublicKey()

	mintKey, err := solana.PublicKeyFromBase58(cfg.X402.TokenMint)
	if err != nil {
		log.Fatalf("invalid token mint: %v", err)
	}
	sourceTokenAccount, _, err := solana.FindAssociatedTokenAddress(payerPub, mintKey)
	if err != nil {
		log.Fatalf("derive payer ATA: %v", err)
	}

	destAccount := quote.X402.Account
	if destAccount == "" {
		destAccountPub, err := solana.PublicKeyFromBase58(cfg.X402.PaymentAddress)
		if err != nil {
			log.Fatalf("derive recipient account: %v", err)
		}
		derived, _, err := solana.FindAssociatedTokenAddress(destAccountPub, mintKey)
		if err != nil {
			log.Fatalf("derive recipient ATA: %v", err)
		}
		destAccount = derived.String()
	}

	destPub, err := solana.PublicKeyFromBase58(destAccount)
	if err != nil {
		log.Fatalf("invalid quote account: %v", err)
	}

	rpcClient := rpc.New(cfg.X402.RPCURL)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	blockhash, err := rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentProcessed)
	if err != nil {
		log.Fatalf("latest blockhash: %v", err)
	}

	tokenAmount := uint64(math.Round(quote.X402.Amount * math.Pow10(int(cfg.X402.TokenDecimals))))

	transferInst := token.NewTransferCheckedInstruction(
		tokenAmount,
		cfg.X402.TokenDecimals,
		sourceTokenAccount,
		mintKey,
		destPub,
		payerPub,
		nil,
	).Build()

	instructions := []solana.Instruction{transferInst}

	if memoText := strings.TrimSpace(quote.X402.Memo); memoText != "" {
		memoInst, err := memo.NewMemoInstruction([]byte(memoText), payerPub).ValidateAndBuild()
		if err != nil {
			log.Fatalf("memo instruction: %v", err)
		}
		instructions = append([]solana.Instruction{memoInst}, instructions...)
	}

	tx, err := solana.NewTransaction(
		instructions,
		blockhash.Value.Blockhash,
		solana.TransactionPayer(payerPub),
	)
	if err != nil {
		log.Fatalf("build transaction: %v", err)
	}

	if _, err := tx.Sign(func(key solana.PublicKey) *solana.PrivateKey {
		if key.Equals(payerPub) {
			return &payerKey
		}
		return nil
	}); err != nil {
		log.Fatalf("sign transaction: %v", err)
	}

	txB64, err := tx.ToBase64()
	if err != nil {
		log.Fatalf("encode transaction: %v", err)
	}
	if len(tx.Signatures) == 0 {
		log.Fatal("transaction missing signature")
	}

	// Create x402 Payment Payload
	paymentPayload := x402.PaymentPayload{
		X402Version: 0,
		Scheme:      "solana-spl-transfer",
		Network:     quote.X402.Network,
		Payload: x402.SolanaPayload{
			Signature:   tx.Signatures[0].String(),
			Transaction: txB64,
			Memo:        quote.X402.Memo,
		},
	}

	payload, err := json.Marshal(paymentPayload)
	if err != nil {
		log.Fatalf("marshal payment payload: %v", err)
	}
	headerValue := base64.StdEncoding.EncodeToString(payload)

	log.Printf("Generated x402 payment payload for resource %s", *resourceID)
	log.Printf("Signature: %s", tx.Signatures[0].String())
	log.Printf("Scheme: %s", paymentPayload.Scheme)
	log.Printf("Network: %s", paymentPayload.Network)

	fmt.Printf("export X_PAYMENT_HEADER=%q\n", headerValue)
	fmt.Printf("curl -i %s/api/paywall/%s -H \"X-PAYMENT: %s\"\n", baseURL, *resourceID, headerValue)

	if *post {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/paywall/%s", baseURL, *resourceID), nil)
		if err != nil {
			log.Fatalf("new request: %v", err)
		}
		req.Header.Set("X-PAYMENT", headerValue)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("execute request: %v", err)
		}
		defer resp.Body.Close()
		log.Printf("Cedros response: %s", resp.Status)
		if resp.StatusCode >= 400 {
			os.Exit(1)
		}
	}
}

func fetchQuote(baseURL, resourceID string) (*quoteResponse, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/paywall/%s", baseURL, resourceID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var qr quoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, err
	}
	return &qr, nil
}
