package solana

import (
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	apierrors "github.com/CedrosPay/server/internal/errors"
	"github.com/CedrosPay/server/pkg/x402"
)

func TestNewVerificationError(t *testing.T) {
	err := errors.New("test error")
	testCode := apierrors.ErrorCode("test_code")
	verr := newVerificationError(testCode, err)

	if verr.Code != testCode {
		t.Errorf("newVerificationError() Code = %q, want %q", verr.Code, testCode)
	}
	if verr.Err != err {
		t.Errorf("newVerificationError() Err = %v, want %v", verr.Err, err)
	}
}

func TestResolveTokenAccount(t *testing.T) {
	validOwner := "11111111111111111111111111111111"
	validMint := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v" // USDC mint
	validTokenAccount := "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"

	tests := []struct {
		name        string
		requirement x402.Requirement
		wantErr     bool
		errCode     apierrors.ErrorCode
	}{
		{
			name: "explicit recipient token account",
			requirement: x402.Requirement{
				RecipientTokenAccount: validTokenAccount,
			},
			wantErr: false,
		},
		{
			name: "derive ATA from owner and mint",
			requirement: x402.Requirement{
				RecipientOwner: validOwner,
				TokenMint:      validMint,
			},
			wantErr: false,
		},
		{
			name: "invalid recipient token account",
			requirement: x402.Requirement{
				RecipientTokenAccount: "invalid",
			},
			wantErr: true,
			errCode: apierrors.ErrCodeInvalidRecipient,
		},
		{
			name: "invalid recipient owner",
			requirement: x402.Requirement{
				RecipientOwner: "invalid",
				TokenMint:      validMint,
			},
			wantErr: true,
			errCode: apierrors.ErrCodeInvalidRecipient,
		},
		{
			name: "invalid token mint",
			requirement: x402.Requirement{
				RecipientOwner: validOwner,
				TokenMint:      "invalid",
			},
			wantErr: true,
			errCode: apierrors.ErrCodeInvalidTokenMint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, err := resolveTokenAccount(tt.requirement)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveTokenAccount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if verr, ok := err.(x402.VerificationError); ok {
					if verr.Code != tt.errCode {
						t.Errorf("resolveTokenAccount() error code = %q, want %q", verr.Code, tt.errCode)
					}
				} else {
					t.Error("resolveTokenAccount() should return VerificationError")
				}
			}
			if !tt.wantErr && account.IsZero() {
				t.Error("resolveTokenAccount() returned zero account")
			}
		})
	}
}

func TestPostBalanceMatches(t *testing.T) {
	validDestStr := "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	validMintStr := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

	validDest, _ := solana.PublicKeyFromBase58(validDestStr)
	validMint, _ := solana.PublicKeyFromBase58(validMintStr)
	differentDest, _ := solana.PublicKeyFromBase58("11111111111111111111111111111111")

	tests := []struct {
		name    string
		meta    *rpc.ParsedTransactionMeta
		message *rpc.ParsedMessage
		dest    solana.PublicKey
		mint    solana.PublicKey
		want    bool
	}{
		{
			name: "matching post balance",
			meta: &rpc.ParsedTransactionMeta{
				PostTokenBalances: []rpc.TokenBalance{
					{
						AccountIndex: 0,
						Mint:         validMint,
					},
				},
			},
			message: &rpc.ParsedMessage{
				AccountKeys: []rpc.ParsedMessageAccount{
					{PublicKey: validDest},
				},
			},
			dest: validDest,
			mint: validMint,
			want: true,
		},
		{
			name: "no matching account",
			meta: &rpc.ParsedTransactionMeta{
				PostTokenBalances: []rpc.TokenBalance{
					{
						AccountIndex: 0,
						Mint:         validMint,
					},
				},
			},
			message: &rpc.ParsedMessage{
				AccountKeys: []rpc.ParsedMessageAccount{
					{PublicKey: differentDest},
				},
			},
			dest: validDest,
			mint: validMint,
			want: false,
		},
		{
			name: "nil meta",
			meta: nil,
			message: &rpc.ParsedMessage{
				AccountKeys: []rpc.ParsedMessageAccount{
					{PublicKey: validDest},
				},
			},
			dest: validDest,
			mint: validMint,
			want: false,
		},
		{
			name: "nil message",
			meta: &rpc.ParsedTransactionMeta{
				PostTokenBalances: []rpc.TokenBalance{
					{
						AccountIndex: 0,
						Mint:         validMint,
					},
				},
			},
			message: nil,
			dest:    validDest,
			mint:    validMint,
			want:    false,
		},
		{
			name: "account index out of bounds",
			meta: &rpc.ParsedTransactionMeta{
				PostTokenBalances: []rpc.TokenBalance{
					{
						AccountIndex: 999,
						Mint:         validMint,
					},
				},
			},
			message: &rpc.ParsedMessage{
				AccountKeys: []rpc.ParsedMessageAccount{
					{PublicKey: validDest},
				},
			},
			dest: validDest,
			mint: validMint,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := postBalanceMatches(tt.meta, tt.message, tt.dest, tt.mint)
			if got != tt.want {
				t.Errorf("postBalanceMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindParsedPayer(t *testing.T) {
	signer1, _ := solana.PublicKeyFromBase58("11111111111111111111111111111111")
	signer2, _ := solana.PublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	nonSigner, _ := solana.PublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

	tests := []struct {
		name string
		tx   *rpc.ParsedTransaction
		want string
	}{
		{
			name: "first signer found",
			tx: &rpc.ParsedTransaction{
				Message: rpc.ParsedMessage{
					AccountKeys: []rpc.ParsedMessageAccount{
						{PublicKey: signer1, Signer: true},
						{PublicKey: signer2, Signer: true},
						{PublicKey: nonSigner, Signer: false},
					},
				},
			},
			want: signer1.String(),
		},
		{
			name: "second account is first signer",
			tx: &rpc.ParsedTransaction{
				Message: rpc.ParsedMessage{
					AccountKeys: []rpc.ParsedMessageAccount{
						{PublicKey: nonSigner, Signer: false},
						{PublicKey: signer2, Signer: true},
					},
				},
			},
			want: signer2.String(),
		},
		{
			name: "no signers",
			tx: &rpc.ParsedTransaction{
				Message: rpc.ParsedMessage{
					AccountKeys: []rpc.ParsedMessageAccount{
						{PublicKey: nonSigner, Signer: false},
					},
				},
			},
			want: "",
		},
		{
			name: "nil transaction",
			tx:   nil,
			want: "",
		},
		{
			name: "empty account keys",
			tx: &rpc.ParsedTransaction{
				Message: rpc.ParsedMessage{
					AccountKeys: []rpc.ParsedMessageAccount{},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findParsedPayer(tt.tx)
			if got != tt.want {
				t.Errorf("findParsedPayer() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractTokenTransfer_EdgeCases(t *testing.T) {
	validDest, _ := solana.PublicKeyFromBase58("TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA")
	validMint, _ := solana.PublicKeyFromBase58("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")

	tests := []struct {
		name        string
		tx          *rpc.GetParsedTransactionResult
		destination solana.PublicKey
		mint        solana.PublicKey
		decimals    uint8
		minAmount   float64
		wantErr     bool
	}{
		{
			name: "nil transaction field",
			tx: &rpc.GetParsedTransactionResult{
				Transaction: nil,
			},
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantErr:     true,
		},
		{
			name: "nil meta",
			tx: &rpc.GetParsedTransactionResult{
				Transaction: &rpc.ParsedTransaction{},
				Meta:        nil,
			},
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantErr:     true,
		},
		{
			name: "no matching transfer",
			tx: &rpc.GetParsedTransactionResult{
				Transaction: &rpc.ParsedTransaction{
					Message: rpc.ParsedMessage{
						Instructions: []*rpc.ParsedInstruction{},
					},
				},
				Meta: &rpc.ParsedTransactionMeta{},
			},
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractTokenTransfer(tt.tx, tt.destination, tt.mint, tt.decimals, tt.minAmount)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractTokenTransfer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseTokenTransfer(t *testing.T) {
	validDestStr := "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	validMintStr := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

	validDest, _ := solana.PublicKeyFromBase58(validDestStr)
	validMint, _ := solana.PublicKeyFromBase58(validMintStr)

	tests := []struct {
		name        string
		inst        *rpc.ParsedInstruction
		destination solana.PublicKey
		mint        solana.PublicKey
		decimals    uint8
		minAmount   float64
		wantOk      bool
	}{
		{
			name:        "nil instruction",
			inst:        nil,
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantOk:      false,
		},
		{
			name: "nil parsed",
			inst: &rpc.ParsedInstruction{
				Parsed:  nil,
				Program: "spl-token",
			},
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantOk:      false,
		},
		{
			name: "wrong program",
			inst: &rpc.ParsedInstruction{
				Program: "system",
			},
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseTokenTransfer(tt.inst, nil, nil, tt.destination, tt.mint, tt.decimals, tt.minAmount)
			if ok != tt.wantOk {
				t.Errorf("parseTokenTransfer() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestScanParsedInstructions(t *testing.T) {
	validDestStr := "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	validMintStr := "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

	validDest, _ := solana.PublicKeyFromBase58(validDestStr)
	validMint, _ := solana.PublicKeyFromBase58(validMintStr)

	tests := []struct {
		name         string
		instructions []*rpc.ParsedInstruction
		destination  solana.PublicKey
		mint         solana.PublicKey
		decimals     uint8
		minAmount    float64
		wantOk       bool
	}{
		{
			name:         "empty instructions",
			instructions: []*rpc.ParsedInstruction{},
			destination:  validDest,
			mint:         validMint,
			decimals:     6,
			minAmount:    1.0,
			wantOk:       false,
		},
		{
			name: "nil instruction in list",
			instructions: []*rpc.ParsedInstruction{
				nil,
			},
			destination: validDest,
			mint:        validMint,
			decimals:    6,
			minAmount:   1.0,
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := scanParsedInstructions(tt.instructions, nil, nil, tt.destination, tt.mint, tt.decimals, tt.minAmount)
			if ok != tt.wantOk {
				t.Errorf("scanParsedInstructions() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}
