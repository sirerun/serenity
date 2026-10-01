// Package billing reconciles hosted entitlements from Stripe, never return URLs.
package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const APIVersion = "2025-06-30.basil"

type Config struct {
	SecretKey, WebhookSecret, BuilderPrice, ScalePrice, Origin string
	Client                                                     *http.Client
	BaseURL                                                    string
}

type Service struct {
	Store           *store.Store
	Identity        *identity.Service
	Config          Config
	locks           keyedLocks
	checkoutLimiter checkoutRateLimiter
	// priorSubscriptionReader is nil in production. It lets package tests inject
	// a statement-level read failure while exercising the real SQLite
	// transaction and its rollback behavior.
	priorSubscriptionReader func(context.Context, *sql.Tx, string, string) (priorSubscription, error)
}

type priorSubscription struct {
	status, grace, graceInvoice, periodStart, periodEnd string
}

type persistedBillingReadError struct {
	operation string
	err       error
}

func (e *persistedBillingReadError) Error() string { return e.operation + ": " + e.err.Error() }

func (e *persistedBillingReadError) Unwrap() error { return e.err }

var _ contracts.BillingReconciler = (*Service)(nil)

var _ contracts.BillingCloser = (*Service)(nil)

func (s *Service) request(ctx context.Context, method, path string, form url.Values, key string, result any) error {
	base := s.Config.BaseURL
	if base == "" {
		base = "https://api.stripe.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.Config.SecretKey, "")
	req.Header.Set("Stripe-Version", APIVersion)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	client := s.Config.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("billing provider unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("billing provider status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result)
}

func (s *Service) providerRequest(ctx context.Context, method, path string, form url.Values, key string, result any) error {
	if err := s.request(ctx, method, path, form, key, result); err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrBillingProviderUnavailable, err)
	}
	return nil
}

type subscriptionList struct {
	Data    []subscription `json:"data"`
	HasMore bool           `json:"has_more"`
}

func (s *Service) listSubscriptions(ctx context.Context, customer string) (subscriptionList, error) {
	var out subscriptionList
	if customer == "" || !strings.HasPrefix(customer, "cus_") || strings.ContainsAny(customer, "/?#") {
		return out, fmt.Errorf("%w: invalid customer reference", contracts.ErrBillingProviderAmbiguous)
	}
	path := "/subscriptions?" + url.Values{"customer": {customer}, "status": {"all"}, "limit": {"100"}}.Encode()
	if err := s.providerRequest(ctx, "GET", path, nil, "", &out); err != nil {
		return out, err
	}
	if out.HasMore {
		return out, fmt.Errorf("%w: subscription list has more results", contracts.ErrBillingProviderAmbiguous)
	}
	for _, sub := range out.Data {
		if sub.ID == "" || !strings.HasPrefix(sub.ID, "sub_") || strings.ContainsAny(sub.ID, "/?#") || sub.Customer != customer {
			return out, fmt.Errorf("%w: provider returned an unowned subscription", contracts.ErrBillingProviderAmbiguous)
		}
		if len(sub.Items.Data) != 1 {
			return out, fmt.Errorf("%w: unsupported subscription shape", contracts.ErrBillingProviderAmbiguous)
		}
	}
	return out, nil
}
