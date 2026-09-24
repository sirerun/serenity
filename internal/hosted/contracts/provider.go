package contracts

// Provider pin (interfaces.md "Provider pin", owner task42). FROZEN shape;
// task42 verifies the live response/usage encoding before treating a given
// ProviderPin value as qualified — this type only fixes the fields a pin
// must carry, not that any particular value is approved.
//
// A model-version change is an explicit maintenance operation, never a
// silent fallback: callers holding a ProviderPin must refuse to embed with
// any provider/model/version that does not exactly match one already
// verified.
type ProviderPin struct {
	Provider   string // provider-neutral key, e.g. "openrouter"
	BaseURL    string
	Model      string
	Version    string
	Dimensions int
}

// Accounting units (interfaces.md "Accounting units", owner task44 with42).
// FROZEN: the product-facing allowance unit and the provider-billed unit are
// named separately so the two are never silently conflated in plans/docs/UI
// or in cost reconciliation.
type AccountingUnit string

const (
	// UnitProductInputToken is the existing cl100k_base-counted product
	// allowance unit (gateway.go's tokenizer.Cl100kBase call). It remains a
	// documented product unit only; it does not measure actual provider
	// cost.
	UnitProductInputToken AccountingUnit = "product_input_token"
	// UnitProviderBilledToken is the provider's own reported usage unit —
	// what actually drives operator cost, including readiness/recovery
	// calls (interfaces.md: "every call including readiness/recovery is
	// accounted in operator costs").
	UnitProviderBilledToken AccountingUnit = "provider_billed_token"
)

// Registration mode (interfaces.md "Registration mode", owner task46, config
// owner task41). FROZEN.
type RegistrationMode string

const (
	RegistrationPublic     RegistrationMode = "public"
	RegistrationInviteOnly RegistrationMode = "invite_only"
)
