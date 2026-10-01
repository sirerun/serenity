package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const operatorReviewRequestLimit = 4096

// ApprovedOperatorReview is returned only by a trusted admission dependency
// after it authenticates the operator and loads the bound immutable case.
type ApprovedOperatorReview struct {
	OperationID          string
	ReviewRef            string
	OperatorID           string
	Outcome              contracts.OperationPhase
	ExpectedCanonicalRef string
	ApprovedAt           time.Time
}

// OperatorReviewAdmission is an optional private-admin capability. Implementors
// must authenticate the request and verify the durable immutable case before
// returning its binding.
type OperatorReviewAdmission interface {
	Authorize(ctx context.Context, request *http.Request, operationID, reviewRef string) (ApprovedOperatorReview, error)
}

type operatorReviewLedger interface {
	Lookup(context.Context, string) (contracts.OperationRecord, error)
	ResolvePendingReview(context.Context, string, contracts.OperationPhase, contracts.Evidence) (contracts.OperationRecord, error)
}

type operatorReviewFence interface {
	Fence(context.Context, string) (func(), error)
	Check(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error)
}

type operatorReviewRequest struct {
	operationID string
	reviewRef   string
}

func parseOperatorReviewRequest(w http.ResponseWriter, r *http.Request) (operatorReviewRequest, error) {
	var request operatorReviewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, operatorReviewRequestLimit))
	root, err := decoder.Token()
	if err != nil || root != json.Delim('{') {
		return request, errors.New("invalid request")
	}
	seen := make(map[string]bool, 2)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return request, errors.New("invalid request")
		}
		seen[key] = true
		value, err := decoder.Token()
		text, ok := value.(string)
		if err != nil || !ok {
			return request, errors.New("invalid request")
		}
		switch key {
		case "operation_id":
			request.operationID = text
		case "review_ref":
			request.reviewRef = text
		default:
			return request, errors.New("invalid request")
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || !seen["operation_id"] || !seen["review_ref"] {
		return request, errors.New("invalid request")
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return request, errors.New("invalid request")
	}
	if !validOperatorReviewOpaque(request.operationID) || !validOperatorReviewOpaque(request.reviewRef) {
		return request, errors.New("invalid request")
	}
	return request, nil
}

func validOperatorReviewOpaque(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func (s *Service) resolveOperatorReview(w http.ResponseWriter, r *http.Request) {
	if isNilDependency(s.operatorReviewAdmission) {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	request, err := parseOperatorReviewRequest(w, r)
	if err != nil {
		http.Error(w, "Invalid operator review request", http.StatusBadRequest)
		return
	}
	if err = r.Context().Err(); err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	approved, err := s.operatorReviewAdmission.Authorize(r.Context(), r, request.operationID, request.reviewRef)
	if err != nil || !validApprovedOperatorReview(approved, request) {
		http.Error(w, "Operator review denied", http.StatusForbidden)
		return
	}
	if err = r.Context().Err(); err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.operatorReviewLedger == nil || s.operatorReviewFence == nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	row, err := s.operatorReviewLedger.Lookup(r.Context(), request.operationID)
	if errors.Is(err, contracts.ErrOperationNotFound) {
		http.Error(w, "Operation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.operatorReviewReplay(w, row, approved) {
		return
	}
	if row.Phase != contracts.OperationPendingReview {
		http.Error(w, "Operation cannot be resolved", http.StatusConflict)
		return
	}
	if err = r.Context().Err(); err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	release, err := s.operatorReviewFence.Fence(r.Context(), row.BrainID)
	if err != nil {
		if r.Context().Err() != nil {
			http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Operation cannot be resolved", http.StatusConflict)
		return
	}
	if release == nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	defer release()
	current, err := s.operatorReviewLedger.Lookup(r.Context(), request.operationID)
	if err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	if current.ID != row.ID || current.BrainID != row.BrainID || current.AccountID != row.AccountID {
		http.Error(w, "Operation changed", http.StatusConflict)
		return
	}
	if s.operatorReviewReplay(w, current, approved) {
		return
	}
	if current.Phase != contracts.OperationPendingReview {
		http.Error(w, "Operation cannot be resolved", http.StatusConflict)
		return
	}
	if err = r.Context().Err(); err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	verdict, err := s.operatorReviewFence.Check(r.Context(), current)
	if err != nil {
		http.Error(w, "Canonical proof unavailable", http.StatusServiceUnavailable)
		return
	}
	if err = r.Context().Err(); err != nil {
		http.Error(w, "Operator review unavailable", http.StatusServiceUnavailable)
		return
	}
	if verdict.Outcome != contracts.CanonicalLanded || verdict.Ref != approved.ExpectedCanonicalRef {
		http.Error(w, "Canonical proof does not match approval", http.StatusConflict)
		return
	}
	resolved, err := s.operatorReviewLedger.ResolvePendingReview(r.Context(), request.operationID, contracts.OperationCommitted, contracts.Evidence{Kind: contracts.EvidenceOperatorReview, Ref: request.reviewRef})
	if err != nil {
		http.Error(w, "Operation cannot be resolved", http.StatusConflict)
		return
	}
	if resolved.Phase != contracts.OperationCommitted || resolved.Evidence.Kind != contracts.EvidenceOperatorReview || resolved.Evidence.Ref != request.reviewRef {
		http.Error(w, "Operation cannot be resolved", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validApprovedOperatorReview(approved ApprovedOperatorReview, request operatorReviewRequest) bool {
	return approved.OperationID == request.operationID &&
		approved.ReviewRef == request.reviewRef &&
		validOperatorReviewOpaque(approved.OperatorID) &&
		approved.Outcome == contracts.OperationCommitted &&
		validOperatorReviewOpaque(approved.ExpectedCanonicalRef) &&
		!approved.ApprovedAt.IsZero()
}

func (s *Service) operatorReviewReplay(w http.ResponseWriter, row contracts.OperationRecord, approved ApprovedOperatorReview) bool {
	if row.Phase != contracts.OperationCommitted {
		return false
	}
	if row.Evidence.Kind == contracts.EvidenceOperatorReview && row.Evidence.Ref == approved.ReviewRef {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, "Operation already resolved differently", http.StatusConflict)
	}
	return true
}
