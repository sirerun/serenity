package runtimebroker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/sirerun/serenity/internal/hosted/authority"
)

type remote struct {
	endpoint, region string
	client           *http.Client
	credentials      aws.CredentialsProvider
	signer           *v4.Signer
}

func newRemote(c Config) (*remote, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 16 * 1024}
	denyRedirect := func(*http.Request, []*http.Request) error { return errors.New("broker: redirect refused") }
	metadata := imds.New(imds.Options{Endpoint: "http://169.254.169.254", ClientEnableState: imds.ClientEnabled, EnableFallback: aws.FalseTernary, HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext}, Timeout: 5 * time.Second, CheckRedirect: denyRedirect}})
	creds := aws.NewCredentialsCache(ec2rolecreds.New(func(o *ec2rolecreds.Options) { o.Client = metadata }))
	return &remote{endpoint: c.Endpoint, region: c.Region, client: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: denyRedirect}, credentials: creds, signer: v4.NewSigner()}, nil
}
func (r *remote) Invoke(ctx context.Context, q authority.Request) (authority.Response, error) {
	var out authority.Response
	if ctx == nil {
		return out, authority.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	b, err := json.Marshal(q)
	if err != nil {
		return out, err
	}
	if len(b) > authority.MaxBodyBytes {
		return out, authority.ErrInvalid
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(b))
	if err != nil {
		return out, err
	}
	request.Header.Set("Content-Type", "application/json")
	creds, err := r.credentials.Retrieve(ctx)
	if err != nil {
		return out, fmt.Errorf("broker: explicit credentials unavailable: %w", err)
	}
	sum := sha256.Sum256(b)
	if err = r.signer.SignHTTP(ctx, creds, request, hex.EncodeToString(sum[:]), "execute-api", r.region, time.Now()); err != nil {
		return out, err
	}
	resp, err := r.client.Do(request)
	if err != nil {
		return out, errors.Join(authority.ErrUnavailable, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, authority.MaxBodyBytes+1))
	closeErr := resp.Body.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return out, errors.Join(authority.ErrUnavailable, err)
	}
	if len(body) > authority.MaxBodyBytes {
		return out, authority.ErrUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return out, authority.ErrInvalid
		case http.StatusUnauthorized, http.StatusForbidden:
			return out, authority.ErrForbidden
		case http.StatusConflict:
			return out, authority.ErrConflict
		case http.StatusGone:
			return out, authority.ErrExpired
		default:
			return out, authority.ErrUnavailable
		}
	}
	if err = strictJSON(body, &out); err != nil {
		return out, errors.Join(authority.ErrUnavailable, err)
	}
	return out, nil
}
func (r *remote) Close() { r.client.CloseIdleConnections() }
