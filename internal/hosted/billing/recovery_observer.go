package billing

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

var (
	ErrRecoveryObserverConfiguration = errors.New("billing: invalid recovery observer configuration")
	ErrRecoveryObserverAccountState  = errors.New("billing: recovery observer account state is unavailable")
)

const recoverySource = "stripe_recovery_observation"

// RecoveryObserverLimits are explicit resource ceilings copied by the
// constructor. Zero is never interpreted as a default.
type RecoveryObserverLimits struct {
	Timeout            time.Duration
	ResponseBytes      int64
	TotalResponseBytes int64
	JSONDepth          int
	JSONNodes          int
	ProviderGETs       int
	EventPages         int
	Invoices           int
	InvoiceLinePages   int
}

type RecoveryObserver struct {
	service *Service
	store   *store.Store
	key     string
	live    bool
	prices  map[string]string
	limits  RecoveryObserverLimits
	client  *http.Client
	trans   *http.Transport
}

var _ contracts.RecoveryBillingObserver = (*RecoveryObserver)(nil)

func NewRecoveryObserver(service *Service, limits RecoveryObserverLimits) (*RecoveryObserver, error) {
	if service == nil || service.Store == nil || service.Store.DB() == nil || !validRecoveryLimits(limits) {
		return nil, ErrRecoveryObserverConfiguration
	}
	key := service.Config.SecretKey
	if !validRecoveryKey(key) || !validPriceID(service.Config.BuilderPrice) || !validPriceID(service.Config.ScalePrice) || service.Config.BuilderPrice == service.Config.ScalePrice {
		return nil, ErrRecoveryObserverConfiguration
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: -1}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableKeepAlives:     true,
		DisableCompression:    true,
	}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &RecoveryObserver{
		service: service,
		store:   service.Store,
		key:     key,
		live:    strings.HasPrefix(key, "sk_live_") || strings.HasPrefix(key, "rk_live_"),
		prices:  map[string]string{service.Config.BuilderPrice: "builder", service.Config.ScalePrice: "scale"},
		limits:  limits,
		client:  client,
		trans:   transport,
	}, nil
}

func validRecoveryLimits(l RecoveryObserverLimits) bool {
	return l.Timeout >= time.Millisecond && l.Timeout <= 30*time.Second &&
		l.ResponseBytes >= 1 && l.ResponseBytes <= 1<<20 &&
		l.TotalResponseBytes >= 1 && l.TotalResponseBytes <= 64<<20 &&
		l.JSONDepth >= 1 && l.JSONDepth <= 32 && l.JSONNodes >= 1 && l.JSONNodes <= 100_000 &&
		l.ProviderGETs >= 1 && l.ProviderGETs <= 128 && l.EventPages >= 1 && l.EventPages <= 32 &&
		l.Invoices >= 1 && l.Invoices <= 16 && l.InvoiceLinePages >= 1 && l.InvoiceLinePages <= 4
}

func validRecoveryKey(key string) bool {
	prefixes := []string{"sk_live_", "rk_live_", "sk_test_", "rk_test_"}
	validPrefix := false
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix) && len(key) > len(prefix) {
			validPrefix = true
			break
		}
	}
	if !validPrefix || len(key) > 256 {
		return false
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x20 || key[i] > 0x7e {
			return false
		}
	}
	return true
}

func validTypedID(value, prefix string) bool {
	if len(value) <= len(prefix) || len(value) > 256 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

func validPriceID(value string) bool { return validTypedID(value, "price_") }

func validRecoveryAccountID(value string) bool {
	if len(value) < 16 || len(value) > 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

type recoveryLocalState struct {
	status, customer string
	checkout         bool
}

func (o *RecoveryObserver) readLocalState(ctx context.Context, accountID string) (recoveryLocalState, error) {
	var state recoveryLocalState
	err := o.store.DB().QueryRowContext(ctx, `SELECT status,COALESCE(stripe_customer_id,'') FROM accounts WHERE id=?`, accountID).Scan(&state.status, &state.customer)
	if err != nil {
		return recoveryLocalState{}, err
	}
	err = o.store.DB().QueryRowContext(ctx, `SELECT 1 FROM checkout_attempts WHERE account_id=? LIMIT 1`, accountID).Scan(new(int))
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return recoveryLocalState{}, err
	default:
		state.checkout = true
	}
	return state, nil
}

func (o *RecoveryObserver) ObserveRestorePending(ctx context.Context, accountID string) (contracts.RecoveryBillingObservation, error) {
	var zero contracts.RecoveryBillingObservation
	if o == nil || o.service == nil || o.store == nil || o.client == nil || !validRecoveryLimits(o.limits) {
		return zero, ErrRecoveryObserverConfiguration
	}
	if ctx == nil {
		return zero, context.Canceled
	}
	callCtx, cancel := context.WithTimeout(ctx, o.limits.Timeout)
	defer cancel()
	if err := callCtx.Err(); err != nil {
		return zero, err
	}
	if !validRecoveryAccountID(accountID) {
		return zero, ErrRecoveryObserverAccountState
	}
	release, err := o.service.locks.acquire(callCtx, "account:"+accountID)
	if err != nil {
		return zero, err
	}
	defer release()
	initial, err := o.readLocalState(callCtx, accountID)
	if err != nil || initial.status != "restore_pending" || !validTypedID(initial.customer, "cus_") || initial.checkout {
		if callCtx.Err() != nil {
			return zero, callCtx.Err()
		}
		return zero, ErrRecoveryObserverAccountState
	}
	observedAt := time.Now().UTC()
	work := recoveryObservationWork{observer: o, ctx: callCtx, customer: initial.customer, observedAt: observedAt}
	result, err := work.observe()
	if err != nil {
		if callCtx.Err() != nil {
			return zero, callCtx.Err()
		}
		return zero, err
	}
	final, err := o.readLocalState(callCtx, accountID)
	if err != nil || final != initial || final.status != "restore_pending" || final.checkout {
		if callCtx.Err() != nil {
			return zero, callCtx.Err()
		}
		return zero, ErrRecoveryObserverAccountState
	}
	if err := callCtx.Err(); err != nil {
		return zero, err
	}
	result.AccountID = accountID
	result.ObservedAt = observedAt
	result.Source = recoverySource
	return result, nil
}

type recoveryObservationWork struct {
	observer     *RecoveryObserver
	ctx          context.Context
	customer     string
	observedAt   time.Time
	gets         int
	bytes        int64
	seenInvoices map[string]bool
	invoiceLines map[string][]recoveryLineProjection
	invoiceCount int
}

func ambiguous() error   { return contracts.ErrBillingProviderAmbiguous }
func unavailable() error { return contracts.ErrBillingProviderUnavailable }
