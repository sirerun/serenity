package billing

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type recoveryObject map[string]any

func (w *recoveryObservationWork) get(path string) (recoveryObject, error) {
	if err := w.ctx.Err(); err != nil {
		return nil, err
	}
	if w.gets >= w.observer.limits.ProviderGETs {
		return nil, ambiguous()
	}
	w.gets++
	u := "https://api.stripe.com/v1" + path
	req, err := http.NewRequestWithContext(w.ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, unavailable()
	}
	req.SetBasicAuth(w.observer.key, "")
	req.Header.Set("Stripe-Version", APIVersion)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := w.observer.client.Do(req)
	if err != nil {
		if ctxErr := w.ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, unavailable()
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, unavailable()
	}
	remaining := w.observer.limits.TotalResponseBytes - w.bytes
	cap := w.observer.limits.ResponseBytes
	if remaining < cap {
		cap = remaining
	}
	if cap < 0 {
		return nil, ambiguous()
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, cap+1))
	w.bytes += int64(len(body))
	if err != nil {
		if ctxErr := w.ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, unavailable()
	}
	if int64(len(body)) > cap || w.bytes > w.observer.limits.TotalResponseBytes {
		return nil, ambiguous()
	}
	value, err := decodeRecoveryJSON(body, w.observer.limits.JSONDepth, w.observer.limits.JSONNodes)
	if err != nil {
		return nil, ambiguous()
	}
	object, ok := value.(recoveryObject)
	if !ok {
		return nil, ambiguous()
	}
	return object, nil
}

func decodeRecoveryJSON(body []byte, maxDepth, maxNodes int) (any, error) {
	if !utf8.Valid(body) || !validRecoveryUnicodeEscapes(body) {
		return nil, errors.New("invalid JSON Unicode")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	count := 0
	value, err := parseRecoveryValue(d, 1, maxDepth, maxNodes, &count)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("trailing JSON")
		}
		return nil, err
	}
	return value, nil
}

// encoding/json replaces malformed UTF-8 and unpaired escaped UTF-16
// surrogates with U+FFFD. Reject those inputs before decoding so identifiers
// and decision keys never depend on replacement behavior.
func validRecoveryUnicodeEscapes(body []byte) bool {
	inString := false
	for i := 0; i < len(body); i++ {
		if !inString {
			if body[i] == '"' {
				inString = true
			}
			continue
		}
		switch body[i] {
		case '"':
			inString = false
		case '\\':
			i++
			if i >= len(body) {
				return false
			}
			if body[i] != 'u' {
				continue
			}
			code, next, ok := recoveryHexEscape(body, i-1)
			if !ok || (code >= 0xDC00 && code <= 0xDFFF) {
				return false
			}
			if code >= 0xD800 && code <= 0xDBFF {
				low, after, ok := recoveryHexEscape(body, next)
				if !ok || low < 0xDC00 || low > 0xDFFF {
					return false
				}
				i = after - 1
			} else {
				i = next - 1
			}
		}
	}
	return !inString
}

func recoveryHexEscape(body []byte, start int) (code, next int, ok bool) {
	if start < 0 || start+6 > len(body) || body[start] != '\\' || body[start+1] != 'u' {
		return 0, 0, false
	}
	value, err := strconv.ParseUint(string(body[start+2:start+6]), 16, 16)
	return int(value), start + 6, err == nil
}

func parseRecoveryValue(d *json.Decoder, depth, maxDepth, maxNodes int, count *int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("JSON depth exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	*count++
	if *count > maxNodes {
		return nil, errors.New("JSON node limit exceeded")
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			object := recoveryObject{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("invalid JSON key")
				}
				*count++
				if *count > maxNodes {
					return nil, errors.New("JSON node limit exceeded")
				}
				if _, exists := object[key]; exists {
					return nil, errors.New("duplicate JSON key")
				}
				item, err := parseRecoveryValue(d, depth+1, maxDepth, maxNodes, count)
				if err != nil {
					return nil, err
				}
				object[key] = item
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return nil, errors.New("invalid JSON object")
			}
			return object, nil
		case '[':
			array := make([]any, 0)
			for d.More() {
				item, err := parseRecoveryValue(d, depth+1, maxDepth, maxNodes, count)
				if err != nil {
					return nil, err
				}
				array = append(array, item)
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return nil, errors.New("invalid JSON array")
			}
			return array, nil
		default:
			return nil, errors.New("unexpected JSON delimiter")
		}
	case nil, bool, string, json.Number:
		return value, nil
	default:
		return nil, errors.New("invalid JSON value")
	}
}

func recoveryField(object recoveryObject, key string) (any, bool, error) {
	value, found := object[key]
	for candidate := range object {
		if candidate != key && strings.EqualFold(candidate, key) {
			return nil, false, errors.New("noncanonical key casing")
		}
	}
	return value, found, nil
}

// recoveryOnlyKeys pins fields inside decision-bearing subtrees. Top-level
// resource objects intentionally use a finite projection and may carry bounded
// unrelated Stripe attributes that are never surfaced or interpreted.
func recoveryOnlyKeys(object recoveryObject, keys ...string) error {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	for present := range object {
		for _, key := range keys {
			if present != key && strings.EqualFold(present, key) {
				return errors.New("noncanonical key casing")
			}
		}
		if !allowed[present] {
			return errors.New("unknown decision field")
		}
	}
	return nil
}

func recoveryRequired(object recoveryObject, key string) (any, error) {
	value, found, err := recoveryField(object, key)
	if err != nil || !found {
		return nil, errors.New("missing or aliased field")
	}
	return value, nil
}

func recoveryString(object recoveryObject, key string) (string, error) {
	value, err := recoveryRequired(object, key)
	if err != nil {
		return "", err
	}
	result, ok := value.(string)
	if !ok {
		return "", errors.New("field is not a string")
	}
	return result, nil
}

func recoveryBool(object recoveryObject, key string) (bool, error) {
	value, err := recoveryRequired(object, key)
	if err != nil {
		return false, err
	}
	result, ok := value.(bool)
	if !ok {
		return false, errors.New("field is not boolean")
	}
	return result, nil
}

func recoveryInt(object recoveryObject, key string) (int64, error) {
	value, err := recoveryRequired(object, key)
	if err != nil {
		return 0, err
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, errors.New("field is not integer")
	}
	result, err := strconv.ParseInt(string(number), 10, 64)
	if err != nil {
		return 0, errors.New("field is not integer")
	}
	return result, nil
}

func recoveryObjectField(object recoveryObject, key string) (recoveryObject, error) {
	value, err := recoveryRequired(object, key)
	if err != nil {
		return nil, err
	}
	result, ok := value.(recoveryObject)
	if !ok {
		return nil, errors.New("field is not object")
	}
	return result, nil
}

func recoveryArrayField(object recoveryObject, key string) ([]any, error) {
	value, err := recoveryRequired(object, key)
	if err != nil {
		return nil, err
	}
	result, ok := value.([]any)
	if !ok {
		return nil, errors.New("field is not array")
	}
	return result, nil
}

func recoveryStringID(object recoveryObject, key, prefix string) (string, error) {
	value, err := recoveryString(object, key)
	if err != nil || !validTypedID(value, prefix) {
		return "", errors.New("invalid typed ID")
	}
	return value, nil
}

func recoveryList(object recoveryObject) ([]any, bool, error) {
	kind, err := recoveryString(object, "object")
	if err != nil || kind != "list" {
		return nil, false, errors.New("invalid list object")
	}
	data, err := recoveryArrayField(object, "data")
	if err != nil {
		return nil, false, err
	}
	more, err := recoveryBool(object, "has_more")
	if err != nil {
		return nil, false, err
	}
	return data, more, nil
}

type recoverySubscription struct {
	id, customer, status, itemID, priceID, plan string
	start, end                                  int64
	quantity                                    int64
	cancelAtPeriodEnd                           bool
}

func parseRecoverySubscription(object recoveryObject, customer string, prices map[string]string, requireWindow bool) (recoverySubscription, error) {
	var sub recoverySubscription
	kind, err := recoveryString(object, "object")
	if err != nil || kind != "subscription" {
		return sub, errors.New("invalid subscription object")
	}
	sub.id, err = recoveryStringID(object, "id", "sub_")
	if err != nil {
		return sub, err
	}
	sub.customer, err = recoveryStringID(object, "customer", "cus_")
	if err != nil || (customer != "" && sub.customer != customer) {
		return sub, errors.New("subscription customer mismatch")
	}
	sub.status, err = recoveryString(object, "status")
	if err != nil {
		return sub, err
	}
	switch sub.status {
	case "active", "trialing", "past_due", "canceled", "unpaid", "paused", "incomplete", "incomplete_expired":
	default:
		return sub, errors.New("unsupported subscription status")
	}
	sub.cancelAtPeriodEnd, err = recoveryBool(object, "cancel_at_period_end")
	if err != nil {
		return sub, err
	}
	items, err := recoveryObjectField(object, "items")
	if err != nil {
		return sub, err
	}
	if err = recoveryOnlyKeys(items, "object", "data", "has_more", "url"); err != nil {
		return sub, err
	}
	rows, more, err := recoveryList(items)
	if err != nil || more || len(rows) != 1 {
		return sub, errors.New("incomplete subscription items")
	}
	item, ok := rows[0].(recoveryObject)
	if !ok {
		return sub, errors.New("invalid subscription item")
	}
	itemKind, err := recoveryString(item, "object")
	if err != nil || itemKind != "subscription_item" {
		return sub, errors.New("invalid subscription item object")
	}
	sub.itemID, err = recoveryStringID(item, "id", "si_")
	if err != nil {
		return sub, err
	}
	price, err := recoveryObjectField(item, "price")
	if err != nil {
		return sub, err
	}
	priceKind, err := recoveryString(price, "object")
	if err != nil || priceKind != "price" {
		return sub, errors.New("invalid price object")
	}
	sub.priceID, err = recoveryStringID(price, "id", "price_")
	if err != nil {
		return sub, err
	}
	sub.plan = prices[sub.priceID]
	quantity, err := recoveryInt(item, "quantity")
	if err != nil || quantity < 0 {
		return sub, errors.New("invalid quantity")
	}
	sub.quantity = quantity
	if requireWindow || sub.status == "active" || sub.status == "trialing" || sub.status == "past_due" {
		sub.start, err = recoveryInt(item, "current_period_start")
		if err != nil {
			return sub, err
		}
		sub.end, err = recoveryInt(item, "current_period_end")
		if err != nil || !validUnix(sub.start) || !validUnix(sub.end) || sub.end <= sub.start {
			return sub, errors.New("invalid subscription period")
		}
	}
	if customer != "" && (sub.status == "active" || sub.status == "trialing" || sub.status == "past_due") && sub.plan == "" {
		return sub, errors.New("unconfigured plan")
	}
	return sub, nil
}

func validUnix(seconds int64) bool { return seconds > 0 && seconds <= 253402300799 }

func (w *recoveryObservationWork) observe() (contracts.RecoveryBillingObservation, error) {
	var out contracts.RecoveryBillingObservation
	query := url.Values{"customer": {w.customer}, "status": {"all"}, "limit": {"100"}}
	list, err := w.get("/subscriptions?" + query.Encode())
	if err != nil {
		return out, err
	}
	if err := requireObjectType(list, "list"); err != nil {
		return out, ambiguous()
	}
	if err := recoveryOnlyKeys(list, "object", "data", "has_more", "url"); err != nil {
		return out, ambiguous()
	}
	rows, more, err := recoveryList(list)
	if err != nil || more {
		return out, ambiguous()
	}
	seen := map[string]bool{}
	nonterminal := make([]recoverySubscription, 0, 1)
	for _, row := range rows {
		object, ok := row.(recoveryObject)
		if !ok {
			return out, ambiguous()
		}
		sub, err := parseRecoverySubscription(object, w.customer, w.observer.prices, false)
		if err != nil || seen[sub.id] {
			return out, ambiguous()
		}
		seen[sub.id] = true
		if !subscriptionTerminalObserver(sub.status) {
			nonterminal = append(nonterminal, sub)
		}
	}
	if len(nonterminal) == 0 {
		out.PlanID = "free"
		return out, nil
	}
	if len(nonterminal) != 1 {
		return out, ambiguous()
	}
	sub := nonterminal[0]
	switch sub.status {
	case "active", "trialing":
		out.PlanID, out.CurrentWindowStart, out.CurrentWindowEnd = sub.plan, time.Unix(sub.start, 0).UTC(), time.Unix(sub.end, 0).UTC()
		out.Eligible = periodEnd(sub.end).After(w.observedAt)
		return out, nil
	case "unpaid", "paused":
		out.PlanID = "free"
		return out, nil
	case "incomplete":
		return out, ambiguous()
	case "past_due":
		if time.Unix(sub.start, 0).After(w.observedAt) {
			return out, ambiguous()
		}
		failureAt, err := w.deriveFailure(sub)
		if err != nil {
			return out, err
		}
		grace, err := safeGrace(failureAt)
		if err != nil {
			return out, ambiguous()
		}
		out.PlanID, out.CurrentWindowStart, out.CurrentWindowEnd, out.GraceUntil = sub.plan, time.Unix(sub.start, 0).UTC(), time.Unix(sub.end, 0).UTC(), grace
		out.Eligible = grace.After(w.observedAt)
		return out, nil
	default:
		return out, ambiguous()
	}
}

func periodEnd(seconds int64) time.Time { return time.Unix(seconds, 0).UTC() }

func requireObjectType(object recoveryObject, expected string) error {
	kind, err := recoveryString(object, "object")
	if err != nil || kind != expected {
		return errors.New("unexpected object type")
	}
	return nil
}

func subscriptionTerminalObserver(status string) bool {
	return status == "canceled" || status == "incomplete_expired"
}

func safeGrace(at time.Time) (time.Time, error) {
	if at.IsZero() || at.After(time.Now().Add(time.Second)) || at.Year() > 9996 {
		return time.Time{}, errors.New("invalid failure time")
	}
	return at.Add(72 * time.Hour).UTC(), nil
}

type recoveryEvent struct {
	id, kind              string
	created               int64
	subscription          *recoverySubscription
	selectedSubscription  bool
	selectedCurrentStatus string
	previousStatus        string
	invoice               recoveryObject
}

func (w *recoveryObservationWork) deriveFailure(current recoverySubscription) (time.Time, error) {
	cutoff := w.observedAt.Add(-30 * 24 * time.Hour)
	if !time.Unix(current.start, 0).After(cutoff.Add(60 * time.Second)) {
		return time.Time{}, ambiguous()
	}
	query := url.Values{}
	query.Add("types[]", "customer.subscription.created")
	query.Add("types[]", "customer.subscription.updated")
	query.Add("types[]", "invoice.payment_failed")
	query.Set("created[gte]", strconv.FormatInt(current.start, 10))
	query.Set("created[lte]", strconv.FormatInt(w.observedAt.Unix(), 10))
	query.Set("limit", "100")
	events := make([]recoveryEvent, 0)
	seenIDs := map[string]bool{}
	cursor := ""
	var priorCreated int64 = math.MaxInt64
	for page := 0; ; page++ {
		if page >= w.observer.limits.EventPages {
			return time.Time{}, ambiguous()
		}
		pageQuery := strings.Clone(query.Encode())
		if cursor != "" {
			pageQuery += "&starting_after=" + url.QueryEscape(cursor)
		}
		response, err := w.get("/events?" + pageQuery)
		if err != nil {
			return time.Time{}, err
		}
		if err := requireObjectType(response, "list"); err != nil {
			return time.Time{}, ambiguous()
		}
		rows, more, err := recoveryList(response)
		if err != nil {
			return time.Time{}, ambiguous()
		}
		var pageLast string
		for _, raw := range rows {
			object, ok := raw.(recoveryObject)
			if !ok {
				return time.Time{}, ambiguous()
			}
			event, err := w.parseEvent(object, current)
			if err != nil {
				return time.Time{}, ambiguous()
			}
			if seenIDs[event.id] || event.created > priorCreated || event.created < current.start || event.created > w.observedAt.Unix() {
				return time.Time{}, ambiguous()
			}
			seenIDs[event.id] = true
			priorCreated = event.created
			pageLast = event.id
			events = append(events, event)
		}
		if more {
			if len(rows) == 0 || pageLast == "" || pageLast == cursor {
				return time.Time{}, ambiguous()
			}
			cursor = pageLast
			continue
		}
		break
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].created == events[j].created {
			return events[i].id < events[j].id
		}
		return events[i].created < events[j].created
	})
	return w.resolveEpisode(events, current)
}

func (w *recoveryObservationWork) parseEvent(object recoveryObject, current recoverySubscription) (recoveryEvent, error) {
	var event recoveryEvent
	if err := requireObjectType(object, "event"); err != nil {
		return event, err
	}
	var err error
	event.id, err = recoveryStringID(object, "id", "evt_")
	if err != nil {
		return event, err
	}
	event.kind, err = recoveryString(object, "type")
	if err != nil {
		return event, err
	}
	switch event.kind {
	case "customer.subscription.created", "customer.subscription.updated", "invoice.payment_failed":
	default:
		return event, errors.New("unknown event type")
	}
	version, err := recoveryString(object, "api_version")
	if err != nil || version != APIVersion {
		return event, errors.New("unsupported event version")
	}
	event.created, err = recoveryInt(object, "created")
	if err != nil || !validUnix(event.created) {
		return event, errors.New("invalid event time")
	}
	live, err := recoveryBool(object, "livemode")
	if err != nil || live != w.observer.live {
		return event, errors.New("event mode mismatch")
	}
	data, err := recoveryObjectField(object, "data")
	if err != nil {
		return event, err
	}
	if err := recoveryOnlyKeys(data, "object", "previous_attributes"); err != nil {
		return event, err
	}
	resource, err := recoveryObjectField(data, "object")
	if err != nil {
		return event, err
	}
	if event.kind == "invoice.payment_failed" {
		if err := requireObjectType(resource, "invoice"); err != nil {
			return event, err
		}
		event.invoice = resource
		return event, nil
	}
	sub, err := parseRecoverySubscription(resource, "", w.observer.prices, false)
	if err != nil {
		return event, err
	}
	if sub.id == current.id && sub.customer == w.customer {
		event.selectedSubscription = true
		event.selectedCurrentStatus = sub.status
		if sub.plan == "" && (sub.status == "active" || sub.status == "trialing" || sub.status == "past_due") {
			return event, errors.New("unconfigured selected plan")
		}
		if sub.itemID != current.itemID || sub.priceID != current.priceID || sub.start != current.start || sub.end != current.end {
			if sub.start == 0 || sub.end == 0 || periodsOverlap(sub.start, sub.end, current.start, current.end) {
				return event, errors.New("subscription item changed within observation window")
			}
		} else {
			event.subscription = &sub
		}
	}
	if event.kind == "customer.subscription.updated" {
		previous, err := recoveryObjectField(data, "previous_attributes")
		if err != nil {
			return event, err
		}
		if event.subscription != nil {
			if oldItems, found, e := recoveryField(previous, "items"); e != nil {
				return event, e
			} else if found {
				old, e := parsePreviousRecoveryItem(oldItems)
				if e != nil || old.id != event.subscription.itemID || old.priceID != event.subscription.priceID || old.start != event.subscription.start || old.end != event.subscription.end {
					return event, errors.New("subscription item changed")
				}
			}
		}
		if status, ok, e := recoveryField(previous, "status"); e != nil {
			return event, e
		} else if ok {
			event.previousStatus, ok = status.(string)
			if !ok {
				return event, errors.New("invalid previous status")
			}
			switch event.previousStatus {
			case "active", "trialing", "past_due", "canceled", "unpaid", "paused", "incomplete", "incomplete_expired":
			default:
				return event, errors.New("unknown previous status")
			}
		}
	}
	return event, nil
}

func periodsOverlap(aStart, aEnd, bStart, bEnd int64) bool { return aStart <= bEnd && bStart <= aEnd }

type previousRecoveryItem struct {
	id, priceID string
	start, end  int64
}

func parsePreviousRecoveryItem(value any) (previousRecoveryItem, error) {
	var result previousRecoveryItem
	items, ok := value.(recoveryObject)
	if !ok {
		return result, errors.New("invalid previous items")
	}
	if err := recoveryOnlyKeys(items, "object", "data", "has_more", "url"); err != nil {
		return result, err
	}
	rows, more, err := recoveryList(items)
	if err != nil || more || len(rows) != 1 {
		return result, errors.New("incomplete previous items")
	}
	item, ok := rows[0].(recoveryObject)
	if !ok {
		return result, errors.New("invalid previous item")
	}
	result.id, err = recoveryStringID(item, "id", "si_")
	if err != nil {
		return result, err
	}
	price, err := recoveryObjectField(item, "price")
	if err != nil {
		return result, err
	}
	result.priceID, err = recoveryStringID(price, "id", "price_")
	if err != nil {
		return result, err
	}
	result.start, err = recoveryInt(item, "current_period_start")
	if err != nil {
		return result, err
	}
	result.end, err = recoveryInt(item, "current_period_end")
	if err != nil || !validUnix(result.start) || !validUnix(result.end) || result.end <= result.start {
		return result, errors.New("invalid previous period")
	}
	return result, nil
}

func (w *recoveryObservationWork) resolveEpisode(events []recoveryEvent, current recoverySubscription) (time.Time, error) {
	failures := make([]time.Time, 0)
	resets := make([]time.Time, 0)
	transitionsBySecond := make(map[int64]map[string]struct{})
	transitionSeconds := make(map[int64]struct{})
	for _, event := range events {
		if !event.selectedSubscription || event.kind != "customer.subscription.updated" || event.previousStatus == "" || event.previousStatus == event.selectedCurrentStatus {
			continue
		}
		transitionSeconds[event.created] = struct{}{}
		if transitionsBySecond[event.created] == nil {
			transitionsBySecond[event.created] = make(map[string]struct{})
		}
		transitionsBySecond[event.created][event.previousStatus+"\x00"+event.selectedCurrentStatus] = struct{}{}
		if len(transitionsBySecond[event.created]) > 1 {
			return time.Time{}, ambiguous()
		}
	}
	for _, event := range events {
		if event.kind == "invoice.payment_failed" {
			applies, err := w.invoiceFailureApplies(event, current)
			if err != nil {
				return time.Time{}, err
			}
			if applies {
				failures = append(failures, time.Unix(event.created, 0).UTC())
			}
		} else if event.subscription != nil && (event.subscription.status == "active" || event.subscription.status == "trialing") {
			if event.kind == "customer.subscription.created" || (event.previousStatus != "" && event.previousStatus != "active" && event.previousStatus != "trialing") {
				resets = append(resets, time.Unix(event.created, 0).UTC())
			}
		}
	}
	if len(failures) == 0 {
		return time.Time{}, ambiguous()
	}
	var reset time.Time
	for _, candidate := range resets {
		if candidate.After(reset) {
			reset = candidate
		}
	}
	anchor := time.Time{}
	for _, failure := range failures {
		if failure.After(reset) && (anchor.IsZero() || failure.Before(anchor)) {
			anchor = failure
		}
	}
	if anchor.IsZero() {
		return time.Time{}, ambiguous()
	}
	for _, resetAt := range resets {
		for _, failure := range failures {
			if resetAt.Equal(failure) {
				return time.Time{}, ambiguous()
			}
		}
	}
	for second := range transitionSeconds {
		transitionAt := time.Unix(second, 0)
		for _, failure := range failures {
			if transitionAt.Equal(failure) {
				return time.Time{}, ambiguous()
			}
		}
	}
	return anchor, nil
}

type recoveryLineProjection struct {
	id, parentType, invoiceItem, subscription, itemID string
	priceID                                           string
	proration                                         bool
	start, end                                        int64
	quantity                                          int64
	hasQuantity                                       bool
}

func (w *recoveryObservationWork) invoiceFailureApplies(event recoveryEvent, current recoverySubscription) (bool, error) {
	invoice := event.invoice
	id, err := recoveryStringID(invoice, "id", "in_")
	if err != nil {
		return false, ambiguous()
	}
	kind, err := recoveryString(invoice, "object")
	if err != nil || kind != "invoice" {
		return false, ambiguous()
	}
	customer, err := recoveryStringID(invoice, "customer", "cus_")
	if err != nil {
		return false, ambiguous()
	}
	live, err := recoveryBool(invoice, "livemode")
	if err != nil || live != w.observer.live {
		return false, ambiguous()
	}
	linesObj, err := recoveryObjectField(invoice, "lines")
	if err != nil {
		return false, ambiguous()
	}
	if err := recoveryOnlyKeys(linesObj, "object", "data", "has_more", "url"); err != nil {
		return false, ambiguous()
	}
	eventRows, eventMore, err := recoveryList(linesObj)
	if err != nil || eventMore {
		return false, ambiguous()
	}
	eventLines, err := parseRecoveryLines(eventRows, w.observer.live)
	if err != nil {
		return false, ambiguous()
	}
	matching, prorationTouches, containedProration, baseWindow := false, false, false, false
	for _, line := range eventLines {
		if line.parentType != "subscription_item_details" || line.subscription != current.id {
			continue
		}
		if line.itemID == current.itemID && line.priceID == current.priceID && line.start == current.start && line.end == current.end && !line.proration {
			baseWindow = true
		}
		if line.itemID == current.itemID && line.priceID == current.priceID && line.start == current.start && line.end == current.end && !line.proration {
			matching = true
		}
		if line.itemID != current.itemID || line.priceID != current.priceID {
			if periodsOverlap(line.start, line.end, current.start, current.end) {
				return false, ambiguous()
			}
			continue
		}
		if line.proration && periodsOverlap(line.start, line.end, current.start, current.end) {
			prorationTouches = true
			if line.start > current.start && line.end < current.end {
				containedProration = true
			}
		}
		if (line.itemID == current.itemID && line.priceID == current.priceID) && periodsOverlap(line.start, line.end, current.start, current.end) && (line.start != current.start || line.end != current.end) && !line.proration {
			return false, ambiguous()
		}
	}
	if prorationTouches && (!containedProration || !baseWindow) {
		return false, ambiguous()
	}
	if !matching && (!containedProration || !baseWindow) {
		return false, nil
	}
	if customer != w.customer {
		return false, ambiguous()
	}
	if w.seenInvoices == nil {
		w.seenInvoices = map[string]bool{}
	}
	if w.invoiceLines == nil {
		w.invoiceLines = map[string][]recoveryLineProjection{}
	}
	if w.seenInvoices[id] {
		if !sameLineProjection(w.invoiceLines[id], eventLines) {
			return false, ambiguous()
		}
		return true, nil
	}
	if w.invoiceCount >= w.observer.limits.Invoices {
		return false, ambiguous()
	}
	w.seenInvoices[id] = true
	w.invoiceCount++
	currentInvoice, err := w.get("/invoices/" + url.PathEscape(id))
	if err != nil {
		return false, err
	}
	if err := verifyInvoiceCore(currentInvoice, id, customer, w.observer.live); err != nil {
		return false, ambiguous()
	}
	currentLines, err := w.fetchInvoiceLines(id)
	if err != nil {
		return false, err
	}
	if !sameLineProjection(eventLines, currentLines) {
		return false, ambiguous()
	}
	w.invoiceLines[id] = append([]recoveryLineProjection(nil), eventLines...)
	return true, nil
}

func verifyInvoiceCore(object recoveryObject, id, customer string, live bool) error {
	if err := requireObjectType(object, "invoice"); err != nil {
		return err
	}
	gotID, err := recoveryStringID(object, "id", "in_")
	if err != nil || gotID != id {
		return errors.New("invoice identity mismatch")
	}
	gotCustomer, err := recoveryStringID(object, "customer", "cus_")
	if err != nil || gotCustomer != customer {
		return errors.New("invoice customer mismatch")
	}
	gotLive, err := recoveryBool(object, "livemode")
	if err != nil || gotLive != live {
		return errors.New("invoice mode mismatch")
	}
	return nil
}

func parseRecoveryLines(rows []any, live bool) ([]recoveryLineProjection, error) {
	lines := make([]recoveryLineProjection, 0, len(rows))
	seen := map[string]bool{}
	for _, raw := range rows {
		line, ok := raw.(recoveryObject)
		if !ok {
			return nil, errors.New("invalid line")
		}
		if err := requireObjectType(line, "line_item"); err != nil {
			return nil, err
		}
		var item recoveryLineProjection
		var err error
		item.id, err = recoveryStringID(line, "id", "il_")
		if err != nil || seen[item.id] {
			return nil, errors.New("duplicate line identity")
		}
		seen[item.id] = true
		lineLive, err := recoveryBool(line, "livemode")
		if err != nil || lineLive != live {
			return nil, errors.New("line mode mismatch")
		}
		parent, err := recoveryObjectField(line, "parent")
		if err != nil {
			return nil, err
		}
		if err := recoveryOnlyKeys(parent, "type", "subscription_item_details", "invoice_item_details"); err != nil {
			return nil, err
		}
		item.parentType, err = recoveryString(parent, "type")
		if err != nil {
			return nil, err
		}
		switch item.parentType {
		case "subscription_item_details":
			branch, err := recoveryObjectField(parent, "subscription_item_details")
			if err != nil {
				return nil, err
			}
			if err := recoveryOnlyKeys(branch, "subscription", "subscription_item", "proration", "proration_details", "invoice_item"); err != nil {
				return nil, err
			}
			item.subscription, err = recoveryStringID(branch, "subscription", "sub_")
			if err != nil {
				return nil, err
			}
			item.itemID, err = recoveryStringID(branch, "subscription_item", "si_")
			if err != nil {
				return nil, err
			}
			item.proration, err = recoveryBool(branch, "proration")
			if err != nil {
				return nil, err
			}
		case "invoice_item_details":
			branch, err := recoveryObjectField(parent, "invoice_item_details")
			if err != nil {
				return nil, err
			}
			if err := recoveryOnlyKeys(branch, "invoice_item", "proration", "proration_details", "subscription"); err != nil {
				return nil, err
			}
			item.invoiceItem, err = recoveryStringID(branch, "invoice_item", "ii_")
			if err != nil {
				return nil, err
			}
			item.proration, err = recoveryBool(branch, "proration")
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("unknown line parent")
		}
		pricing, err := recoveryObjectField(line, "pricing")
		if err != nil {
			return nil, err
		}
		if err := recoveryOnlyKeys(pricing, "type", "price_details", "unit_amount_decimal"); err != nil {
			return nil, err
		}
		pricingType, err := recoveryString(pricing, "type")
		if err != nil || pricingType != "price_details" {
			return nil, errors.New("unsupported pricing type")
		}
		priceDetails, err := recoveryObjectField(pricing, "price_details")
		if err != nil {
			return nil, err
		}
		if err := recoveryOnlyKeys(priceDetails, "price", "product"); err != nil {
			return nil, err
		}
		item.priceID, err = recoveryStringID(priceDetails, "price", "price_")
		if err != nil {
			return nil, err
		}
		period, err := recoveryObjectField(line, "period")
		if err != nil {
			return nil, err
		}
		if err := recoveryOnlyKeys(period, "start", "end"); err != nil {
			return nil, err
		}
		item.start, err = recoveryInt(period, "start")
		if err != nil {
			return nil, err
		}
		item.end, err = recoveryInt(period, "end")
		if err != nil || !validUnix(item.start) || !validUnix(item.end) || item.end < item.start {
			return nil, errors.New("invalid line period")
		}
		quantityValue, found, e := recoveryField(line, "quantity")
		if e != nil || !found {
			return nil, errors.New("missing line quantity")
		}
		if quantityValue != nil {
			number, ok := quantityValue.(json.Number)
			if !ok {
				return nil, errors.New("invalid line quantity")
			}
			item.quantity, err = strconv.ParseInt(string(number), 10, 64)
			if err != nil || item.quantity < 0 {
				return nil, errors.New("invalid line quantity")
			}
			item.hasQuantity = true
		}
		lines = append(lines, item)
	}
	return lines, nil
}

func (w *recoveryObservationWork) fetchInvoiceLines(invoiceID string) ([]recoveryLineProjection, error) {
	result := make([]recoveryLineProjection, 0)
	seen := map[string]bool{}
	cursor := ""
	for page := 0; ; page++ {
		if page >= w.observer.limits.InvoiceLinePages {
			return nil, ambiguous()
		}
		query := url.Values{"limit": {"100"}}
		if cursor != "" {
			query.Set("starting_after", cursor)
		}
		response, err := w.get("/invoices/" + url.PathEscape(invoiceID) + "/lines?" + query.Encode())
		if err != nil {
			return nil, err
		}
		if err := requireObjectType(response, "list"); err != nil {
			return nil, ambiguous()
		}
		if err := recoveryOnlyKeys(response, "object", "data", "has_more", "url"); err != nil {
			return nil, ambiguous()
		}
		rows, more, err := recoveryList(response)
		if err != nil {
			return nil, ambiguous()
		}
		lines, err := parseRecoveryLines(rows, w.observer.live)
		if err != nil {
			return nil, ambiguous()
		}
		for _, line := range lines {
			if seen[line.id] {
				return nil, ambiguous()
			}
			seen[line.id] = true
			result = append(result, line)
		}
		if !more {
			break
		}
		if len(lines) == 0 {
			return nil, ambiguous()
		}
		cursor = lines[len(lines)-1].id
	}
	return result, nil
}

func sameLineProjection(a, b []recoveryLineProjection) bool {
	if len(a) != len(b) {
		return false
	}
	project := func(rows []recoveryLineProjection) []recoveryLineProjection {
		result := append([]recoveryLineProjection(nil), rows...)
		sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
		return result
	}
	left, right := project(a), project(b)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
