package subscriptions

import (
	"testing"
	"time"
)

func TestSubscription_IsActive(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		sub  Subscription
		want bool
	}{
		{
			name: "active subscription within period",
			sub: Subscription{
				Status:             StatusActive,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			want: true,
		},
		{
			name: "active subscription expired",
			sub: Subscription{
				Status:             StatusActive,
				CurrentPeriodStart: now.Add(-48 * time.Hour),
				CurrentPeriodEnd:   now.Add(-24 * time.Hour),
			},
			want: false,
		},
		{
			name: "cancelled subscription",
			sub: Subscription{
				Status:             StatusCancelled,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			want: false,
		},
		{
			name: "past due subscription within period",
			sub: Subscription{
				Status:             StatusPastDue,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			want: true,
		},
		{
			name: "trialing subscription within period",
			sub: Subscription{
				Status:             StatusTrialing,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			want: true,
		},
		{
			name: "expired status",
			sub: Subscription{
				Status:             StatusExpired,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sub.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculatePeriodEnd(t *testing.T) {
	start := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		period   BillingPeriod
		interval int
		want     time.Time
	}{
		{
			name:     "1 day",
			period:   PeriodDay,
			interval: 1,
			want:     time.Date(2025, 1, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "7 days",
			period:   PeriodDay,
			interval: 7,
			want:     time.Date(2025, 1, 22, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "1 week",
			period:   PeriodWeek,
			interval: 1,
			want:     time.Date(2025, 1, 22, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "2 weeks",
			period:   PeriodWeek,
			interval: 2,
			want:     time.Date(2025, 1, 29, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "1 month",
			period:   PeriodMonth,
			interval: 1,
			want:     time.Date(2025, 2, 15, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "3 months (quarterly)",
			period:   PeriodMonth,
			interval: 3,
			want:     time.Date(2025, 4, 15, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "1 year",
			period:   PeriodYear,
			interval: 1,
			want:     time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePeriodEnd(start, tt.period, tt.interval)
			if !got.Equal(tt.want) {
				t.Errorf("CalculatePeriodEnd() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubscription_DaysUntilExpiration(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		end     time.Time
		wantMin int
		wantMax int
	}{
		{
			name:    "7 days remaining",
			end:     now.Add(7*24*time.Hour + time.Hour), // Add buffer
			wantMin: 6,
			wantMax: 7,
		},
		{
			name:    "1 day remaining",
			end:     now.Add(24*time.Hour + time.Hour), // Add buffer
			wantMin: 0,
			wantMax: 1,
		},
		{
			name:    "expired yesterday",
			end:     now.Add(-24 * time.Hour),
			wantMin: -2,
			wantMax: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub := Subscription{CurrentPeriodEnd: tt.end}
			got := sub.DaysUntilExpiration()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("DaysUntilExpiration() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestProductConfig_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		config ProductConfig
		want   bool
	}{
		{
			name: "valid monthly config",
			config: ProductConfig{
				BillingPeriod:   PeriodMonth,
				BillingInterval: 1,
			},
			want: true,
		},
		{
			name: "valid yearly config",
			config: ProductConfig{
				BillingPeriod:   PeriodYear,
				BillingInterval: 1,
				TrialDays:       14,
			},
			want: true,
		},
		{
			name: "missing billing period",
			config: ProductConfig{
				BillingInterval: 1,
			},
			want: false,
		},
		{
			name: "zero interval",
			config: ProductConfig{
				BillingPeriod:   PeriodMonth,
				BillingInterval: 0,
			},
			want: false,
		},
		{
			name: "negative interval",
			config: ProductConfig{
				BillingPeriod:   PeriodMonth,
				BillingInterval: -1,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
