package authority

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// GatewayBinding pins the only trusted AWS_IAM API Gateway REST integration.
// Lambda invoke policy must independently deny every other invocation route.
type GatewayBinding struct {
	APIID           string
	AccountID       string
	Stage           string
	DomainName      string
	RuntimePath     string
	OwnerPath       string
	RuntimeARN      string
	RuntimeCallerID string
	OwnerARN        string
	OwnerCallerID   string
}

type Gateway struct {
	server  *Server
	binding GatewayBinding
}

func NewGateway(s *Server, b GatewayBinding) (*Gateway, error) {
	if s == nil || b.APIID == "" || len(b.AccountID) != 12 || b.Stage == "" || b.DomainName == "" || b.RuntimePath == "" || b.OwnerPath == "" || b.RuntimePath == b.OwnerPath || b.RuntimeARN != s.config.RuntimePrincipal || b.OwnerARN != s.config.OwnerPrincipal || b.RuntimeCallerID == "" || b.OwnerCallerID == "" || !strings.HasPrefix(b.RuntimeARN, "arn:aws:") || !strings.HasPrefix(b.OwnerARN, "arn:aws:") {
		return nil, ErrInvalid
	}
	return &Gateway{server: s, binding: b}, nil
}
func (g *Gateway) Handle(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	b := g.binding
	rc := event.RequestContext
	i := rc.Identity
	if rc.APIID != b.APIID || rc.AccountID != b.AccountID || rc.Stage != b.Stage || rc.DomainName != b.DomainName || rc.HTTPMethod != "POST" || event.HTTPMethod != "POST" || rc.RequestID == "" || i.AccessKey == "" || len(rc.Authorizer) != 0 {
		return errorResponse(ErrForbidden), nil
	}
	principal := ""
	if event.Path == b.RuntimePath && event.Resource == b.RuntimePath && rc.ResourcePath == b.RuntimePath && i.UserArn == b.RuntimeARN && i.Caller == b.RuntimeCallerID {
		principal = b.RuntimeARN
	}
	if event.Path == b.OwnerPath && event.Resource == b.OwnerPath && rc.ResourcePath == b.OwnerPath && i.UserArn == b.OwnerARN && i.Caller == b.OwnerCallerID {
		principal = b.OwnerARN
	}
	if principal == "" {
		return errorResponse(ErrForbidden), nil
	}
	if len(event.Body) > 2*MaxBodyBytes {
		return errorResponse(ErrInvalid), nil
	}
	body := []byte(event.Body)
	if event.IsBase64Encoded {
		if base64.StdEncoding.DecodedLen(len(event.Body)) > MaxBodyBytes {
			return errorResponse(ErrInvalid), nil
		}
		var err error
		body, err = base64.StdEncoding.DecodeString(event.Body)
		if err != nil {
			return errorResponse(ErrInvalid), nil
		}
	}
	if len(body) > MaxBodyBytes {
		return errorResponse(ErrInvalid), nil
	}
	var q Request
	if err := strictDecode(body, &q); err != nil {
		return errorResponse(err), nil
	}
	response, err := g.server.execute(ctx, principal, q)
	if err != nil {
		return errorResponse(err), nil
	}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > MaxBodyBytes {
		return errorResponse(ErrUnavailable), nil
	}
	return events.APIGatewayProxyResponse{StatusCode: 200, Headers: map[string]string{"Content-Type": "application/json", "Cache-Control": "no-store"}, Body: string(encoded)}, nil
}
func errorResponse(err error) events.APIGatewayProxyResponse {
	code := "unavailable"
	status := 503
	switch {
	case errors.Is(err, ErrInvalid):
		code = "invalid"
		status = 400
	case errors.Is(err, ErrForbidden):
		code = "forbidden"
		status = 403
	case errors.Is(err, ErrConflict):
		code = "conflict"
		status = 409
	case errors.Is(err, ErrExpired):
		code = "expired"
		status = 410
	}
	return events.APIGatewayProxyResponse{StatusCode: status, Headers: map[string]string{"Content-Type": "application/json", "Cache-Control": "no-store"}, Body: `{"code":"` + code + `"}`}
}
