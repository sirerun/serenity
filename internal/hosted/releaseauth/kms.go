package releaseauth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

type KMSSigner interface {
	Sign(context.Context, *kms.SignInput, ...func(*kms.Options)) (*kms.SignOutput, error)
}

// NewKMSSigner does not load ambient credentials, endpoint overrides,
// configuration files, credential processes, or environment proxies.
func NewKMSSigner(region, credentialsFile, profile string) (*kms.Client, error) {
	if region == "" || profile == "" || strings.ContainsAny(profile, "\r\n[]") {
		return nil, fmt.Errorf("explicit region and profile required")
	}
	raw, err := ReadBounded(credentialsFile, MaxDocument)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(credentialsFile)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("credentials must be a private regular file")
	}
	selected := false
	found := false
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			selected = line == "["+profile+"]"
			if selected {
				if found {
					return nil, fmt.Errorf("duplicate credential profile")
				}
				found = true
			}
			continue
		}
		if !selected {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid selected profile")
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if _, ok := values[key]; ok {
			return nil, fmt.Errorf("duplicate profile field")
		}
		switch key {
		case "aws_access_key_id", "aws_secret_access_key", "aws_session_token":
		default:
			return nil, fmt.Errorf("selected profile permits static credentials only")
		}
		if value == "" {
			return nil, fmt.Errorf("empty profile field")
		}
		values[key] = value
	}
	if values["aws_access_key_id"] == "" || values["aws_secret_access_key"] == "" {
		return nil, fmt.Errorf("selected profile lacks static credentials")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("KMS redirects denied") }}
	cfg := aws.Config{Region: region, Credentials: credentials.NewStaticCredentialsProvider(values["aws_access_key_id"], values["aws_secret_access_key"], values["aws_session_token"]), HTTPClient: client, RetryMaxAttempts: 1}
	return kms.NewFromConfig(cfg), nil
}

func Sign(ctx context.Context, client KMSSigner, canonical []byte, p Policy, now time.Time) ([]byte, error) {
	if ctx == nil || client == nil {
		return nil, fmt.Errorf("sign context and client required")
	}
	var m Manifest
	if err := DecodeCanonical(canonical, &m); err != nil {
		return nil, err
	}
	if err := m.Validate(now); err != nil {
		return nil, err
	}
	if _, err := p.PublicKey(); err != nil {
		return nil, err
	}
	if m.PolicyID != p.PolicyID || m.Repository != p.Repository || m.KeyID != p.KeyID || m.Algorithm != p.Algorithm || m.OS != p.OS || m.Arch != p.Arch {
		return nil, fmt.Errorf("manifest does not match signing policy")
	}
	digest := sha256.Sum256(canonical)
	response, err := client.Sign(ctx, &kms.SignInput{KeyId: aws.String(p.KeyID), Message: digest[:], MessageType: types.MessageTypeDigest, SigningAlgorithm: types.SigningAlgorithmSpecEcdsaSha256})
	if err != nil {
		return nil, fmt.Errorf("KMS signing failed: %w", err)
	}
	if response == nil || aws.ToString(response.KeyId) != p.KeyID || response.SigningAlgorithm != types.SigningAlgorithmSpecEcdsaSha256 {
		return nil, fmt.Errorf("KMS signer response identity mismatch")
	}
	if err = VerifySignature(m, canonical, response.Signature, p, now); err != nil {
		return nil, fmt.Errorf("verifying KMS response: %w", err)
	}
	return response.Signature, nil
}
