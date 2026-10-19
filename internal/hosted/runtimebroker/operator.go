package runtimebroker

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/episodemaintenance"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
)

type operatorConfig struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Profile         string `json:"profile"`
	CredentialsFile string `json:"credentials_file"`
}

// OpenOperator selects a named static credentials profile from one explicit
// root-only file. It never invokes the AWS default provider chain.
func OpenOperator(ctx context.Context, path string) (*authority.Client, func(), error) {
	return openOperator(ctx, path, false)
}

// OpenOwner selects the separately authenticated authority owner endpoint.
// Config and credentials must be private to the current effective UID. Owner
// authority is authenticated by IAM, not local root; the client cannot
// impersonate runtime measurements or ordinary appends.
func OpenOwner(ctx context.Context, path string) (*authority.Client, func(), error) {
	return openOperator(ctx, path, true)
}

// OpenClosedMaintenance constructs the CLOSED history source only from the
// authenticated owner client opened by this trusted factory. Service-UID
// children must not receive this source or the owner credentials behind it.
func OpenClosedMaintenance(ctx context.Context, path string) (episodemaintenance.ClosedSource, func(), error) {
	client, closeClient, err := OpenOwner(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	source, err := episodemaintenance.NewClosedSource(client)
	if err != nil {
		closeClient()
		return nil, nil, err
	}
	return source, closeClient, nil
}
func openOperator(ctx context.Context, path string, owner bool) (*authority.Client, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("broker: context required")
	}
	if !owner && os.Geteuid() != 0 {
		return nil, nil, errors.New("broker: root operator required")
	}
	b, err := privatefs.ReadFile(ctx, filepath.Dir(path), filepath.Base(path), 16*1024)
	if err != nil {
		return nil, nil, err
	}
	var c operatorConfig
	if err = strictJSON(b, &c); err != nil {
		return nil, nil, err
	}
	if err = validateEndpoint(c.Endpoint, c.Region); err != nil {
		return nil, nil, err
	}
	if c.Profile == "" || strings.ContainsAny(c.Profile, "[]\r\n\x00") {
		return nil, nil, errors.New("broker: explicit credential profile required")
	}
	b, err = privatefs.ReadFile(ctx, filepath.Dir(c.CredentialsFile), filepath.Base(c.CredentialsFile), 64*1024)
	if err != nil {
		return nil, nil, err
	}
	access, secret, token, err := selectedProfile(b, c.Profile)
	clear(b)
	if err != nil {
		return nil, nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.ResponseHeaderTimeout = 15 * time.Second
	tr.MaxResponseHeaderBytes = 16 * 1024
	r := &remote{endpoint: c.Endpoint, region: c.Region, client: &http.Client{Transport: tr, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("broker: redirect refused") }}, credentials: credentials.NewStaticCredentialsProvider(access, secret, token), signer: v4.NewSigner()}
	var transport authority.Transport = operatorTransport{r}
	if owner {
		transport = ownerTransport{r}
	}
	client, err := authority.NewClient(transport)
	if err != nil {
		r.Close()
		return nil, nil, err
	}
	return client, r.Close, nil
}

// Operator transport cannot invoke runtime writer/admission actions even if
// accidental callers obtain this client; remote IAM/route repeats this gate.
type operatorTransport struct{ remote *remote }

func (t operatorTransport) Invoke(ctx context.Context, q authority.Request) (authority.Response, error) {
	if q.Action == "closed_current" || q.ClosedPosition != nil {
		return authority.Response{}, authority.ErrForbidden
	}
	switch q.Action {
	case "read_finalized_cut", "get", "list":
		return t.remote.Invoke(ctx, q)
	default:
		return authority.Response{}, authority.ErrForbidden
	}
}
func selectedProfile(data []byte, wanted string) (string, string, string, error) {
	current := ""
	seenSections := map[string]bool{}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return "", "", "", errors.New("broker: malformed credential profile")
			}
			current = strings.TrimSpace(line[1 : len(line)-1])
			if seenSections[current] {
				return "", "", "", errors.New("broker: duplicate credential profile")
			}
			seenSections[current] = true
			continue
		}
		if current != wanted {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if !ok || v == "" || values[k] != "" {
			return "", "", "", errors.New("broker: invalid selected credential profile")
		}
		switch k {
		case "aws_access_key_id", "aws_secret_access_key", "aws_session_token":
			values[k] = v
		default:
			return "", "", "", errors.New("broker: unsupported selected credential field")
		}
	}
	if values["aws_access_key_id"] == "" || values["aws_secret_access_key"] == "" {
		return "", "", "", errors.New("broker: selected static credentials unavailable")
	}
	return values["aws_access_key_id"], values["aws_secret_access_key"], values["aws_session_token"], nil
}

type ownerTransport struct{ remote *remote }

func (t ownerTransport) Invoke(ctx context.Context, q authority.Request) (authority.Response, error) {
	switch q.Action {
	case "inspect", "close", "enroll", "restore", "seal":
		return t.remote.Invoke(ctx, q)
	case "closed_current":
		if onlyClosedCurrent(q) {
			return t.remote.Invoke(ctx, q)
		}
		return authority.Response{}, authority.ErrForbidden
	case "get", "list":
		if closedPositionRead(q) {
			return t.remote.Invoke(ctx, q)
		}
		return authority.Response{}, authority.ErrForbidden
	default:
		return authority.Response{}, authority.ErrForbidden
	}
}

func onlyClosedCurrent(q authority.Request) bool {
	return q.Action == "closed_current" && q.Adoption == nil && q.DataInventorySHA256 == "" && q.OperationID == "" && q.ChallengeID == "" && q.Nonce == "" && q.Measurement == nil && q.ReservationID == "" && q.LeaseID == "" && q.CutID == "" && q.ClosedPosition == nil && q.HistorySHA256 == "" && q.ManifestSHA256 == "" && q.Key == "" && len(q.Body) == 0 && q.Prefix == "" && q.StartAfter == "" && q.Limit == 0 && q.SnapshotAttemptID == ""
}

func closedPositionRead(q authority.Request) bool {
	if q.ClosedPosition == nil || q.LeaseID != "" || q.ReservationID != "" || q.CutID != "" || q.OperationID != "" || q.ChallengeID != "" || q.Nonce != "" || q.Measurement != nil || q.Adoption != nil || q.DataInventorySHA256 != "" || q.HistorySHA256 != "" || q.ManifestSHA256 != "" || len(q.Body) != 0 || q.SnapshotAttemptID != "" {
		return false
	}
	if q.Action == "get" {
		return q.Key != "" && q.Prefix == "" && q.StartAfter == "" && q.Limit == 0
	}
	return q.Action == "list" && q.Key == "" && q.Prefix != "" && q.Limit > 0
}
