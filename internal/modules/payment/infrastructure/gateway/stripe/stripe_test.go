package stripe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
)

func TestParseAndValidateConfig(t *testing.T) {
	cfg, err := ParseConfig(map[string]interface{}{
		"secret_key":           " sk_test_123 ",
		"webhook_secret":       " whsec_123 ",
		"success_url":          "https://example.com/payment?stripe_return=1",
		"cancel_url":           "https://example.com/payment?stripe_cancel=1",
		"payment_method_types": []interface{}{"card"},
	})
	if err != nil {
		t.Fatalf("parse config failed: %v", err)
	}
	if cfg.SecretKey != "sk_test_123" {
		t.Fatalf("unexpected secret key: %s", cfg.SecretKey)
	}
	if cfg.APIBaseURL != defaultAPIBaseURL {
		t.Fatalf("unexpected default api base url: %s", cfg.APIBaseURL)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("validate config failed: %v", err)
	}
}

func TestCreatePaymentWeChatPayClientOption(t *testing.T) {
	tests := []struct {
		name               string
		paymentMethodTypes []interface{}
		expectClientOption bool
	}{
		{name: "WeChatPayWithCard", paymentMethodTypes: []interface{}{"card", "wechat_pay"}, expectClientOption: true},
		{name: "CardOnly", paymentMethodTypes: []interface{}{"card"}, expectClientOption: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var form url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				form, _ = url.ParseQuery(string(body))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"cs_test_123","url":"https://checkout.stripe.com/c/pay/cs_test_123","status":"open"}`))
			}))
			defer server.Close()

			cfg, err := ParseConfig(map[string]interface{}{
				"secret_key":           "sk_test_123",
				"webhook_secret":       "whsec_123",
				"success_url":          "https://example.com/payment?stripe_return=1",
				"cancel_url":           "https://example.com/payment?stripe_cancel=1",
				"api_base_url":         server.URL,
				"payment_method_types": tc.paymentMethodTypes,
			})
			if err != nil {
				t.Fatalf("parse config failed: %v", err)
			}

			result, err := CreatePayment(context.Background(), cfg, CreateInput{
				OrderNo:  "ORDER-1001",
				Amount:   "12.88",
				Currency: "CNY",
			})
			if err != nil {
				t.Fatalf("create payment failed: %v", err)
			}
			if result.SessionID != "cs_test_123" {
				t.Fatalf("unexpected session id: %s", result.SessionID)
			}
			clientOption := form.Get("payment_method_options[wechat_pay][client]")
			if tc.expectClientOption && clientOption != "web" {
				t.Fatalf("expected wechat_pay client option web, got %q, form: %v", clientOption, form)
			}
			if !tc.expectClientOption && clientOption != "" {
				t.Fatalf("unexpected wechat_pay client option: %q", clientOption)
			}
		})
	}
}

func TestVerifyAndParseWebhookCheckoutCompleted(t *testing.T) {
	now := time.Unix(1760000000, 0)
	cfg := &Config{
		WebhookSecret:           "whsec_test_abc",
		WebhookToleranceSeconds: 300,
	}
	payload := map[string]interface{}{
		"id":   "evt_test_1",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"object":         "checkout.session",
				"id":             "cs_test_123",
				"payment_status": "paid",
				"currency":       "usd",
				"amount_total":   1288,
				"created":        now.Unix(),
				"metadata": map[string]interface{}{
					"order_no": "ORDER-1001",
				},
			},
		},
	}
	body, _ := json.Marshal(payload)
	sig := computeSignature(cfg.WebhookSecret, now.Unix(), body)
	headers := map[string]string{
		"Stripe-Signature": "t=1760000000,v1=" + sig,
	}

	result, err := VerifyAndParseWebhook(cfg, headers, body, now)
	if err != nil {
		t.Fatalf("verify and parse webhook failed: %v", err)
	}
	if result.EventType != "checkout.session.completed" {
		t.Fatalf("unexpected event type: %s", result.EventType)
	}
	if result.ProviderRef != "cs_test_123" {
		t.Fatalf("unexpected provider ref: %s", result.ProviderRef)
	}
	if result.Status != "success" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Amount != "12.88" {
		t.Fatalf("unexpected amount: %s", result.Amount)
	}
}

func TestVerifyAndParseWebhookInvalidSignature(t *testing.T) {
	now := time.Unix(1760000000, 0)
	cfg := &Config{
		WebhookSecret:           "whsec_test_abc",
		WebhookToleranceSeconds: 300,
	}
	payload := map[string]interface{}{
		"id":   "evt_test_1",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"object": "checkout.session",
				"id":     "cs_test_123",
			},
		},
	}
	body, _ := json.Marshal(payload)
	headers := map[string]string{
		"Stripe-Signature": "t=1760000000,v1=invalid-signature",
	}

	_, err := VerifyAndParseWebhook(cfg, headers, body, now)
	if err == nil {
		t.Fatalf("expected verify error")
	}
}

func TestMapPaymentIntentStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "Succeeded", input: stripePIStatusSucceeded, expect: constants.PaymentStatusSuccess},
		{name: "Processing", input: stripePIStatusProcessing, expect: constants.PaymentStatusPending},
		{name: "Canceled", input: stripePIStatusCanceled, expect: constants.PaymentStatusFailed},
		{name: "RequiresPaymentMethod", input: stripePIStatusReqPayMeth, expect: constants.PaymentStatusFailed},
		{name: "UnknownDefaultsPending", input: "unknown", expect: constants.PaymentStatusPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapPaymentIntentStatus(tc.input); got != tc.expect {
				t.Fatalf("unexpected status: got %s, want %s", got, tc.expect)
			}
		})
	}
}

func TestMapEventTypeStatus(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expect    string
		ok        bool
	}{
		{name: "CheckoutCompleted", eventType: stripeEventCheckoutSessionCompleted, expect: constants.PaymentStatusSuccess, ok: true},
		{name: "CheckoutExpired", eventType: stripeEventCheckoutSessionExpired, expect: constants.PaymentStatusExpired, ok: true},
		{name: "CheckoutAsyncFailed", eventType: stripeEventCheckoutSessionAsyncPaymentFailed, expect: constants.PaymentStatusFailed, ok: true},
		{name: "PIProcessing", eventType: stripeEventPaymentIntentProcessing, expect: constants.PaymentStatusPending, ok: true},
		{name: "Unknown", eventType: "unknown.event", expect: "", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mapEventTypeStatus(tc.eventType)
			if ok != tc.ok {
				t.Fatalf("unexpected ok: got %v, want %v", ok, tc.ok)
			}
			if got != tc.expect {
				t.Fatalf("unexpected status: got %s, want %s", got, tc.expect)
			}
		})
	}
}

func TestMapCheckoutSessionStatus(t *testing.T) {
	tests := []struct {
		name          string
		paymentStatus string
		sessionStatus string
		expect        string
	}{
		{name: "PaidWins", paymentStatus: stripePaymentStatusPaid, sessionStatus: stripeSessionComplete, expect: constants.PaymentStatusSuccess},
		{name: "SessionExpired", paymentStatus: "", sessionStatus: stripeSessionExpired, expect: constants.PaymentStatusExpired},
		{name: "NoPaymentRequiredComplete", paymentStatus: stripePaymentNoRequired, sessionStatus: stripeSessionComplete, expect: constants.PaymentStatusSuccess},
		{name: "UnknownDefaultsPending", paymentStatus: "unpaid", sessionStatus: "open", expect: constants.PaymentStatusPending},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapCheckoutSessionStatus(tc.paymentStatus, tc.sessionStatus); got != tc.expect {
				t.Fatalf("unexpected status: got %s, want %s", got, tc.expect)
			}
		})
	}
}

func TestCreatePaymentCustomerEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "PlainAddress", input: "buyer@example.com", want: "buyer@example.com"},
		{name: "TrimsSurroundingSpace", input: "  buyer@example.com  ", want: "buyer@example.com"},
		// 下面这些一律降级成「不传」，让用户在收银台自己填，而不是让整笔支付 400
		{name: "EmptyOmitted", input: "", want: ""},
		{name: "MalformedOmitted", input: "not-an-email", want: ""},
		{name: "InnerSpaceOmitted", input: "a b@example.com", want: ""},
		{name: "NewlineOmitted", input: "buyer@example.com\nBcc: x@y.com", want: ""},
		// 只收裸地址：带显示名的形式同样放弃预填，不去猜该取哪一段
		{name: "DisplayNameOmitted", input: "Buyer <buyer@example.com>", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var form url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				form, _ = url.ParseQuery(string(body))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"cs_test_123","url":"https://checkout.stripe.com/c/pay/cs_test_123","status":"open"}`))
			}))
			defer server.Close()

			cfg, err := ParseConfig(map[string]interface{}{
				"secret_key":     "sk_test_123",
				"webhook_secret": "whsec_123",
				"success_url":    "https://example.com/payment?stripe_return=1",
				"cancel_url":     "https://example.com/payment?stripe_cancel=1",
				"api_base_url":   server.URL,
			})
			if err != nil {
				t.Fatalf("parse config failed: %v", err)
			}

			if _, err := CreatePayment(context.Background(), cfg, CreateInput{
				OrderNo:       "ORDER-1001",
				Amount:        "12.88",
				Currency:      "CNY",
				CustomerEmail: tc.input,
			}); err != nil {
				t.Fatalf("create payment failed: %v", err)
			}

			if got := form.Get("customer_email"); got != tc.want {
				t.Fatalf("customer_email = %q, want %q", got, tc.want)
			}
			if tc.want == "" {
				if _, present := form["customer_email"]; present {
					t.Fatal("customer_email must be omitted entirely, not sent empty")
				}
			}
		})
	}
}
