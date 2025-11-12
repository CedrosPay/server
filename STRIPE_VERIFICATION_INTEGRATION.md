# Stripe Payment Verification - Frontend Integration Guide

## Security Issue Identified

**Problem:** Users can bypass payment by manually entering success URLs with fake `session_id` parameters.

**Example Attack:**
```
cedrospay.com#demo?stripe_success=true&session_id=fake123
```

Anyone could paste this URL and get the ebook without paying.

---

## Solution: Backend Verification Endpoint

A new endpoint has been added to verify Stripe checkout sessions before granting access to content.

### Endpoint

```
GET /paywall/v1/stripe-session/verify?session_id={session_id}
```

**Production URL:**
```
https://pay.cedros.io/paywall/v1/stripe-session/verify?session_id={session_id}
```

**Local Testing URL:**
```
http://localhost:8080/paywall/v1/stripe-session/verify?session_id={session_id}
```

---

## Integration Steps

### 1. Update Success Flow

**Before (INSECURE):**
```javascript
// ❌ BAD: Opens ebook based solely on URL parameter
const urlParams = new URLSearchParams(window.location.search);
const sessionId = urlParams.get('session_id');
const stripeSuccess = urlParams.get('stripe_success');

if (stripeSuccess === 'true' && sessionId) {
  // Anyone can fake this!
  window.open('/Cedros_Guide_To_Generational_Wealth.pdf', '_blank');
}
```

**After (SECURE):**
```javascript
// ✅ GOOD: Verifies payment with backend first
const urlParams = new URLSearchParams(window.location.search);
const sessionId = urlParams.get('session_id');
const stripeSuccess = urlParams.get('stripe_success');

if (stripeSuccess === 'true' && sessionId) {
  try {
    // Call backend to verify the session
    const response = await fetch(
      `https://pay.cedros.io/paywall/v1/stripe-session/verify?session_id=${sessionId}`,
      { method: 'GET' }
    );

    if (response.ok) {
      const data = await response.json();

      if (data.verified) {
        // Payment confirmed! Open the ebook
        window.open('/Cedros_Guide_To_Generational_Wealth.pdf', '_blank');

        // Show success notification with reopen link
        showSuccessNotification({
          title: 'Payment Successful!',
          message: 'Your ebook is ready.',
          link: {
            text: 'Open Your Ebook',
            url: '/Cedros_Guide_To_Generational_Wealth.pdf'
          }
        });
      } else {
        // This shouldn't happen, but handle it
        showError('Payment verification returned invalid status');
      }
    } else {
      // Payment not verified
      const errorData = await response.json();
      showError(
        `Payment verification failed: ${errorData.error?.message || 'Unknown error'}`
      );
    }
  } catch (error) {
    console.error('Verification error:', error);
    showError('Unable to verify payment. Please contact support.');
  }
}
```

### 2. Success Response Format

**HTTP 200 - Payment Verified:**
```json
{
  "verified": true,
  "resource_id": "demo-item-id-5",
  "paid_at": "2025-11-11T13:45:00Z",
  "amount": "$1.00 USD",
  "customer": "cus_abc123xyz",
  "metadata": {
    "userId": "12345",
    "email": "user@example.com",
    "itemName": "Premium Ebook"
  }
}
```

### 3. Error Response Format

**HTTP 404 - Session Not Found:**
```json
{
  "error": {
    "code": "session_not_found",
    "message": "Payment not completed or session invalid"
  }
}
```

**Possible Reasons:**
- User hasn't completed payment yet
- Stripe webhook hasn't been processed yet (rare, usually <1 second)
- Fake/invalid `session_id`
- Session belongs to a different Stripe account

---

## Testing

### Test with Real Stripe Payment

1. **Make a test payment:**
   - Go to `http://localhost:3000` (or your frontend URL)
   - Click "Pay with Card"
   - Use Stripe test card: `4242 4242 4242 4242`
   - Complete checkout

2. **After redirect, verify the flow:**
   - You'll be redirected to: `cedrospay.com#demo?stripe_success=true&session_id=cs_test_xyz...`
   - Frontend should call verification endpoint
   - Console should show verification request/response
   - Ebook should open automatically
   - Success notification should appear

3. **Test fake session:**
   - Manually enter: `cedrospay.com#demo?stripe_success=true&session_id=FAKE123`
   - Verification should fail with 404
   - Ebook should NOT open
   - Error message should display

### Test with curl

```bash
# Test with real session (get session_id from Stripe Dashboard or after test payment)
curl "http://localhost:8080/paywall/v1/stripe-session/verify?session_id=cs_test_a1b2c3d4e5"

# Test with fake session (should return 404)
curl "http://localhost:8080/paywall/v1/stripe-session/verify?session_id=FAKE123"
```

---

## Timeline

**How long after payment is the session verified?**

1. User completes payment on Stripe: **T+0s**
2. Stripe webhook fires to backend: **T+0.5s** (usually < 1 second)
3. Backend processes webhook and stores payment: **T+1s**
4. Frontend can verify session: **T+1s** ✅

**Recommendation:** If verification returns 404, wait 2 seconds and retry once. This handles the rare case where the frontend redirect beats the webhook.

---

## Production Checklist

Before deploying to production:

- [ ] Update all verification endpoint URLs to use `https://pay.cedros.io`
- [ ] Remove any hardcoded `session_id` values from testing
- [ ] Test with live Stripe payments (not test mode)
- [ ] Verify webhook is configured in Stripe Dashboard for production
- [ ] Test error handling (expired sessions, invalid sessions, network errors)
- [ ] Add retry logic for webhook timing edge cases
- [ ] Monitor error rates in production logs

---

## Security Benefits

✅ **Before:** Anyone could bypass payment by guessing/sharing session URLs
✅ **After:** Backend verifies every session against stored payment records
✅ **Defense in Depth:** Even if URL is shared, only the legitimate payer gets access
✅ **Audit Trail:** All verification attempts are logged with request IDs

---

## Questions?

- **Backend API:** See `docs/API_REFERENCE.md` in the server repo
- **Stripe Webhooks:** See `internal/stripe/client.go` for webhook handling logic
- **Payment Storage:** Payments are stored with signature format: `stripe:{session_id}`

---

## Example: Complete Integration Code

```typescript
// src/utils/stripeVerification.ts

export interface StripeVerificationResult {
  verified: boolean;
  resourceId?: string;
  paidAt?: string;
  amount?: string;
  customer?: string;
  metadata?: Record<string, string>;
}

export interface StripeVerificationError {
  code: string;
  message: string;
}

/**
 * Verify a Stripe checkout session was completed and paid.
 *
 * @param sessionId - The Stripe checkout session ID from the URL
 * @param apiBaseUrl - Base URL of the Cedros Pay API (default: production)
 * @returns Promise resolving to verification result or null if failed
 */
export async function verifyStripeSession(
  sessionId: string,
  apiBaseUrl: string = 'https://pay.cedros.io'
): Promise<StripeVerificationResult | null> {
  try {
    const response = await fetch(
      `${apiBaseUrl}/paywall/v1/stripe-session/verify?session_id=${sessionId}`,
      {
        method: 'GET',
        headers: {
          'Content-Type': 'application/json',
        },
      }
    );

    if (response.ok) {
      const data = await response.json();
      return {
        verified: data.verified,
        resourceId: data.resource_id,
        paidAt: data.paid_at,
        amount: data.amount,
        customer: data.customer,
        metadata: data.metadata,
      };
    } else if (response.status === 404) {
      // Payment not found - might need to wait for webhook
      console.warn('Stripe session not verified yet (404)');
      return null;
    } else {
      // Other error
      const errorData = await response.json();
      console.error('Verification failed:', errorData);
      return null;
    }
  } catch (error) {
    console.error('Network error during verification:', error);
    return null;
  }
}

/**
 * Verify Stripe session with automatic retry for webhook timing.
 *
 * @param sessionId - The Stripe checkout session ID
 * @param maxRetries - Maximum number of retries (default: 1)
 * @param retryDelayMs - Delay between retries in milliseconds (default: 2000)
 */
export async function verifyStripeSessionWithRetry(
  sessionId: string,
  maxRetries: number = 1,
  retryDelayMs: number = 2000
): Promise<StripeVerificationResult | null> {
  let result = await verifyStripeSession(sessionId);

  if (!result && maxRetries > 0) {
    // Webhook might not have processed yet - wait and retry
    console.log(`Retrying verification in ${retryDelayMs}ms...`);
    await new Promise(resolve => setTimeout(resolve, retryDelayMs));
    result = await verifyStripeSessionWithRetry(sessionId, maxRetries - 1, retryDelayMs);
  }

  return result;
}
```

**Usage in your component:**

```typescript
import { verifyStripeSessionWithRetry } from './utils/stripeVerification';

// In your payment success handler
const handleStripeSuccess = async (sessionId: string) => {
  setLoading(true);

  const result = await verifyStripeSessionWithRetry(sessionId);

  if (result?.verified) {
    // Payment confirmed! Open ebook
    window.open('/Cedros_Guide_To_Generational_Wealth.pdf', '_blank');

    showSuccessNotification({
      title: 'Payment Successful!',
      message: 'Your ebook is ready.',
    });
  } else {
    // Verification failed
    showError('Payment verification failed. Please contact support with your order details.');
  }

  setLoading(false);
};
```
