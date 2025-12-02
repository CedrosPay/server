package subscriptions

import (
	"time"
)

// Status represents the current state of a subscription.
type Status string

const (
	// StatusActive indicates the subscription is active and grants access.
	StatusActive Status = "active"

	// StatusPastDue indicates a Stripe payment failed but subscription not yet cancelled.
	StatusPastDue Status = "past_due"

	// StatusCancelled indicates the user cancelled the subscription.
	StatusCancelled Status = "cancelled"

	// StatusExpired indicates the subscription period ended without renewal.
	StatusExpired Status = "expired"

	// StatusTrialing indicates the subscription is in a free trial period.
	StatusTrialing Status = "trialing"
)

// BillingPeriod represents the unit of time for subscription billing.
type BillingPeriod string

const (
	PeriodDay   BillingPeriod = "day"
	PeriodWeek  BillingPeriod = "week"
	PeriodMonth BillingPeriod = "month"
	PeriodYear  BillingPeriod = "year"
)

// PaymentMethod indicates how the subscription is paid.
type PaymentMethod string

const (
	PaymentMethodStripe PaymentMethod = "stripe"
	PaymentMethodX402   PaymentMethod = "x402"
)

// Subscription represents an active or historical subscription record.
type Subscription struct {
	ID        string `json:"id"`
	ProductID string `json:"productId"`

	// Subscriber identity (exactly one of these is set)
	Wallet               string `json:"wallet,omitempty"`               // x402: wallet address
	StripeCustomerID     string `json:"stripeCustomerId,omitempty"`     // Stripe: customer ID
	StripeSubscriptionID string `json:"stripeSubscriptionId,omitempty"` // Stripe: subscription ID

	// Billing configuration
	PaymentMethod   PaymentMethod `json:"paymentMethod"`
	BillingPeriod   BillingPeriod `json:"billingPeriod"`
	BillingInterval int           `json:"billingInterval"` // e.g., 1 month, 3 months

	// Status and dates
	Status             Status     `json:"status"`
	CurrentPeriodStart time.Time  `json:"currentPeriodStart"`
	CurrentPeriodEnd   time.Time  `json:"currentPeriodEnd"`
	TrialEnd           *time.Time `json:"trialEnd,omitempty"`
	CancelledAt        *time.Time `json:"cancelledAt,omitempty"`
	CancelAtPeriodEnd  bool       `json:"cancelAtPeriodEnd"` // Cancel at end of current period

	// Metadata
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// IsActive checks if the subscription currently grants access.
func (s Subscription) IsActive() bool {
	return s.IsActiveAt(time.Now())
}

// IsActiveAt checks if the subscription grants access at the given time.
func (s Subscription) IsActiveAt(t time.Time) bool {
	switch s.Status {
	case StatusActive, StatusTrialing:
		// Active or trialing: check if within period
		return !t.Before(s.CurrentPeriodStart) && t.Before(s.CurrentPeriodEnd)
	case StatusPastDue:
		// Past due: still allow access during grace period (within current period)
		return !t.Before(s.CurrentPeriodStart) && t.Before(s.CurrentPeriodEnd)
	default:
		return false
	}
}

// IsTrialing checks if the subscription is currently in a trial period.
func (s Subscription) IsTrialing() bool {
	if s.Status != StatusTrialing {
		return false
	}
	if s.TrialEnd == nil {
		return false
	}
	return time.Now().Before(*s.TrialEnd)
}

// DaysUntilExpiration returns the number of days until the current period ends.
// Returns 0 if already expired or negative if past expiration.
func (s Subscription) DaysUntilExpiration() int {
	duration := time.Until(s.CurrentPeriodEnd)
	return int(duration.Hours() / 24)
}

// NextPeriodEnd calculates what the next period end would be after renewal.
func (s Subscription) NextPeriodEnd() time.Time {
	return CalculatePeriodEnd(s.CurrentPeriodEnd, s.BillingPeriod, s.BillingInterval)
}

// CalculatePeriodEnd calculates the end of a billing period given a start time.
func CalculatePeriodEnd(start time.Time, period BillingPeriod, interval int) time.Time {
	switch period {
	case PeriodDay:
		return start.AddDate(0, 0, interval)
	case PeriodWeek:
		return start.AddDate(0, 0, interval*7)
	case PeriodMonth:
		return start.AddDate(0, interval, 0)
	case PeriodYear:
		return start.AddDate(interval, 0, 0)
	default:
		// Default to monthly if unknown
		return start.AddDate(0, interval, 0)
	}
}

// ProductConfig defines subscription configuration for a product.
type ProductConfig struct {
	BillingPeriod       BillingPeriod `json:"billingPeriod" yaml:"billing_period"`
	BillingInterval     int           `json:"billingInterval" yaml:"billing_interval"`
	TrialDays           int           `json:"trialDays,omitempty" yaml:"trial_days"`
	StripePriceID       string        `json:"stripePriceId,omitempty" yaml:"stripe_price_id"`
	AllowX402           bool          `json:"allowX402" yaml:"allow_x402"`             // Allow x402 payment for this subscription
	GracePeriodHours    int           `json:"gracePeriodHours" yaml:"grace_period_hours"` // Hours after expiry before blocking
}

// IsValid validates the product configuration.
func (c ProductConfig) IsValid() bool {
	if c.BillingPeriod == "" {
		return false
	}
	if c.BillingInterval < 1 {
		return false
	}
	return true
}
