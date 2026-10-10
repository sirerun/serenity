package runtimebroker

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sirerun/serenity/internal/hosted/privatefs"
)

// Config is root-owned deployment configuration, never a service request.
type Config struct {
	AdoptionSHA256   string `json:"adoption_sha256,omitempty"`
	DataDir          string `json:"data_dir"`
	Socket           string `json:"socket"`
	ServiceUID       uint32 `json:"service_uid"`
	ServiceGID       uint32 `json:"service_gid"`
	Unit             string `json:"unit"`
	ExecutableSHA256 string `json:"executable_sha256"`
	BuildSHA         string `json:"build_sha"`
	Endpoint         string `json:"endpoint"`
	Region           string `json:"region"`
	CredentialSource string `json:"credential_source"`
}

func LoadConfig(ctx context.Context, path string) (Config, error) {
	var c Config
	if os.Geteuid() != 0 {
		return c, errors.New("broker: root ownership required")
	}
	b, err := privatefs.ReadFile(ctx, filepath.Dir(path), filepath.Base(path), 16*1024)
	if err != nil {
		return c, err
	}
	if err = strictJSON(b, &c); err != nil {
		return c, err
	}
	return c, c.validate()
}

var regionPattern = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-[0-9]+$`)
var apiPattern = regexp.MustCompile(`^[a-z0-9]{10}$`)

func (c Config) validate() error {
	if c.AdoptionSHA256 != "" {
		adoption, err := hex.DecodeString(c.AdoptionSHA256)
		if err != nil || len(adoption) != 32 || strings.ToLower(c.AdoptionSHA256) != c.AdoptionSHA256 {
			return errors.New("broker: exact adoption digest required")
		}
	}
	digest, err := hex.DecodeString(c.ExecutableSHA256)
	if err != nil || len(digest) != 32 || strings.ToLower(c.ExecutableSHA256) != c.ExecutableSHA256 {
		return errors.New("broker: exact executable digest required")
	}
	build, err := hex.DecodeString(c.BuildSHA)
	if err != nil || (len(build) != 20 && len(build) != 32) || strings.ToLower(c.BuildSHA) != c.BuildSHA {
		return errors.New("broker: exact release source required")
	}
	if !filepath.IsAbs(c.DataDir) || filepath.Clean(c.DataDir) != c.DataDir {
		return errors.New("broker: pinned data directory required")
	}
	if c.ServiceUID == 0 || c.ServiceGID == 0 || !validUnit(c.Unit) || !filepath.IsAbs(c.Socket) || filepath.Clean(c.Socket) != c.Socket {
		return errors.New("broker: service identity and canonical socket required")
	}
	if c.CredentialSource != "imds-v2-instance-role" || !regionPattern.MatchString(c.Region) {
		return errors.New("broker: explicit IMDSv2 region required")
	}
	return validateEndpoint(c.Endpoint, c.Region)
}

func validateEndpoint(endpoint, region string) error {
	if !regionPattern.MatchString(region) {
		return errors.New("broker: invalid region")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || u.RawPath != "" || u.Path == "" || strings.Contains(u.Path, "//") || strings.Contains(u.Path, "..") {
		return errors.New("broker: pinned API Gateway endpoint required")
	}
	suffix := ".execute-api." + region + ".amazonaws.com"
	id, ok := strings.CutSuffix(u.Host, suffix)
	if !ok || !apiPattern.MatchString(id) {
		return errors.New("broker: endpoint region/host mismatch")
	}
	return nil
}
func strictJSON(data []byte, dst any) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("broker: JSON object required")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSON(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("broker: trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	return nil
}
func scanJSON(d *json.Decoder) error { return scanJSONDepth(d, 0) }
func scanJSONDepth(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("broker: JSON nesting exceeds limit")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return errors.New("broker: duplicate JSON field")
			}
			seen[key] = true
			if err := scanJSONDepth(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := scanJSONDepth(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("broker: invalid JSON delimiter")
	}
	_, err = d.Token()
	return err
}

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*\.service$`)

func validUnit(s string) bool { return len(s) <= 128 && unitPattern.MatchString(s) }
