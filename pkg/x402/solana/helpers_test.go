package solana

import (
	"errors"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
)

func TestPubkeysEqual(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{
			name:     "identical keys",
			expected: "11111111111111111111111111111111",
			actual:   "11111111111111111111111111111111",
			want:     true,
		},
		{
			name:     "different keys",
			expected: "11111111111111111111111111111111",
			actual:   "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA",
			want:     false,
		},
		{
			name:     "invalid expected key",
			expected: "invalid",
			actual:   "11111111111111111111111111111111",
			want:     false,
		},
		{
			name:     "invalid actual key",
			expected: "11111111111111111111111111111111",
			actual:   "invalid",
			want:     false,
		},
		{
			name:     "both invalid",
			expected: "invalid1",
			actual:   "invalid2",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pubkeysEqual(tt.expected, tt.actual)
			if got != tt.want {
				t.Errorf("pubkeysEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaxDuration(t *testing.T) {
	tests := []struct {
		name string
		a    time.Duration
		b    time.Duration
		want time.Duration
	}{
		{
			name: "a greater than b",
			a:    5 * time.Second,
			b:    3 * time.Second,
			want: 5 * time.Second,
		},
		{
			name: "b greater than a",
			a:    2 * time.Second,
			b:    7 * time.Second,
			want: 7 * time.Second,
		},
		{
			name: "equal durations",
			a:    4 * time.Second,
			b:    4 * time.Second,
			want: 4 * time.Second,
		},
		{
			name: "zero vs positive",
			a:    0,
			b:    1 * time.Second,
			want: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maxDuration(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("maxDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommitmentFromString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  rpc.CommitmentType
	}{
		{
			name:  "processed",
			input: "processed",
			want:  rpc.CommitmentProcessed,
		},
		{
			name:  "processed uppercase",
			input: "PROCESSED",
			want:  rpc.CommitmentProcessed,
		},
		{
			name:  "confirmed",
			input: "confirmed",
			want:  rpc.CommitmentConfirmed,
		},
		{
			name:  "confirmed with whitespace",
			input: "  confirmed  ",
			want:  rpc.CommitmentConfirmed,
		},
		{
			name:  "finalized",
			input: "finalized",
			want:  rpc.CommitmentFinalized,
		},
		{
			name:  "finalised (British spelling)",
			input: "finalised",
			want:  rpc.CommitmentFinalized,
		},
		{
			name:  "empty string defaults to finalized",
			input: "",
			want:  rpc.CommitmentFinalized,
		},
		{
			name:  "unknown defaults to finalized",
			input: "unknown",
			want:  rpc.CommitmentFinalized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commitmentFromString(tt.input)
			if got != tt.want {
				t.Errorf("commitmentFromString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeriveWebsocketURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "https to wss",
			input:   "https://api.mainnet-beta.solana.com",
			want:    "wss://api.mainnet-beta.solana.com",
			wantErr: false,
		},
		{
			name:    "http to ws",
			input:   "http://localhost:8899",
			want:    "ws://localhost:8899",
			wantErr: false,
		},
		{
			name:    "already wss",
			input:   "wss://api.mainnet-beta.solana.com",
			want:    "wss://api.mainnet-beta.solana.com",
			wantErr: false,
		},
		{
			name:    "already ws",
			input:   "ws://localhost:8899",
			want:    "ws://localhost:8899",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "no scheme",
			input:   "api.mainnet-beta.solana.com",
			want:    "",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			input:   "ftp://example.com",
			want:    "",
			wantErr: true,
		},
		{
			name:    "https with path",
			input:   "https://api.mainnet-beta.solana.com/v1/rpc",
			want:    "wss://api.mainnet-beta.solana.com/v1/rpc",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deriveWebsocketURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("deriveWebsocketURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("deriveWebsocketURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokenAllowed(t *testing.T) {
	tests := []struct {
		name    string
		symbol  string
		allowed []string
		want    bool
	}{
		{
			name:    "exact match",
			symbol:  "USDC",
			allowed: []string{"USDC", "SOL", "USDT"},
			want:    true,
		},
		{
			name:    "case insensitive match",
			symbol:  "usdc",
			allowed: []string{"USDC", "SOL"},
			want:    true,
		},
		{
			name:    "not in list",
			symbol:  "ETH",
			allowed: []string{"USDC", "SOL"},
			want:    false,
		},
		{
			name:    "empty allowed list",
			symbol:  "USDC",
			allowed: []string{},
			want:    false,
		},
		{
			name:    "mixed case in allowed list",
			symbol:  "USDC",
			allowed: []string{"usdc", "sol"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenAllowed(tt.symbol, tt.allowed)
			if got != tt.want {
				t.Errorf("tokenAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAlreadyProcessedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "contains 'Transaction already processed'",
			err:  errors.New("Transaction already processed"),
			want: true,
		},
		{
			name: "contains 'already been processed'",
			err:  errors.New("Transaction already been processed"),
			want: true,
		},
		{
			name: "partial match",
			err:  errors.New("Error: Transaction already processed in slot 12345"),
			want: true,
		},
		{
			name: "different error",
			err:  errors.New("Transaction failed"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAlreadyProcessedError(tt.err)
			if got != tt.want {
				t.Errorf("isAlreadyProcessedError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAccountNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "account not found",
			err:  errors.New("account not found"),
			want: true,
		},
		{
			name: "could not find account",
			err:  errors.New("could not find account"),
			want: true,
		},
		{
			name: "invalid account owner",
			err:  errors.New("invalid account owner"),
			want: true,
		},
		{
			name: "uppercase",
			err:  errors.New("ACCOUNT NOT FOUND"),
			want: true,
		},
		{
			name: "different error",
			err:  errors.New("network timeout"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAccountNotFoundError(tt.err)
			if got != tt.want {
				t.Errorf("isAccountNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsInsufficientFundsTokenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "SPL token insufficient funds (0x1)",
			err:  errors.New("custom program error: 0x1"),
			want: true,
		},
		{
			name: "insufficient funds (not lamports)",
			err:  errors.New("insufficient funds for transfer"),
			want: true,
		},
		{
			name: "insufficient lamports (SOL not token)",
			err:  errors.New("insufficient lamports"),
			want: false,
		},
		{
			name: "uppercase",
			err:  errors.New("CUSTOM PROGRAM ERROR: 0x1"),
			want: true,
		},
		{
			name: "different error",
			err:  errors.New("network timeout"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInsufficientFundsTokenError(tt.err)
			if got != tt.want {
				t.Errorf("isInsufficientFundsTokenError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsInsufficientFundsSOLError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "insufficient lamports",
			err:  errors.New("insufficient lamports"),
			want: true,
		},
		{
			name: "insufficient funds for fee payer",
			err:  errors.New("insufficient funds for fee payer"),
			want: true,
		},
		{
			name: "uppercase",
			err:  errors.New("INSUFFICIENT LAMPORTS"),
			want: true,
		},
		{
			name: "token insufficient funds (not SOL)",
			err:  errors.New("custom program error: 0x1"),
			want: false,
		},
		{
			name: "different error",
			err:  errors.New("network timeout"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInsufficientFundsSOLError(tt.err)
			if got != tt.want {
				t.Errorf("isInsufficientFundsSOLError() = %v, want %v", got, tt.want)
			}
		})
	}
}
