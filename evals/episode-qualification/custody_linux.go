//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/store"
	"golang.org/x/sys/unix"
)

type maintenanceHold struct {
	ManifestSHA256        string `json:"manifest_sha256"`
	ExecutableSHA256      string `json:"executable_sha256"`
	Source                string `json:"source"`
	PreviousStateSHA256   string `json:"previous_state_sha256"`
	ExpectedStateSHA256   string `json:"expected_state_sha256"`
	Supervisor            string `json:"supervisor"`
	Stage                 string `json:"stage"`
	BackupTimerPriorState string `json:"backup_timer_prior_state"`
	Phase                 string `json:"phase"`
	Timestamp             int64  `json:"timestamp"`
}

type custodyProof struct {
	Version                  int             `json:"version"`
	Purpose                  string          `json:"purpose"`
	Operation                string          `json:"operation"`
	Nonce                    string          `json:"nonce"`
	IssuedAt                 int64           `json:"issued_at"`
	ExpiresAt                int64           `json:"expires_at"`
	BootID                   string          `json:"boot_id"`
	UID                      int             `json:"uid"`
	GID                      int             `json:"gid"`
	ToolSource               string          `json:"tool_source"`
	ToolPath                 string          `json:"tool_path"`
	ToolSHA256               string          `json:"tool_sha256"`
	WrapperPath              string          `json:"wrapper_path"`
	WrapperSHA256            string          `json:"wrapper_sha256"`
	RuntimePath              string          `json:"runtime_path"`
	RuntimeSHA256            string          `json:"runtime_sha256"`
	ServiceBinaryPath        string          `json:"service_binary_path"`
	ServiceBinarySHA256      string          `json:"service_binary_sha256"`
	ServiceSource            string          `json:"service_source"`
	ConfigPath               string          `json:"config_path"`
	ConfigSHA256             string          `json:"config_sha256"`
	DataDir                  string          `json:"data_dir"`
	PrivateDir               string          `json:"private_dir"`
	AccountCap               int             `json:"account_cap"`
	HoldSHA256               string          `json:"hold_sha256"`
	Hold                     maintenanceHold `json:"hold"`
	StateSHA256              string          `json:"state_sha256"`
	ReadySHA256              string          `json:"ready_sha256"`
	AuthorityReceiptPath     string          `json:"authority_receipt_path"`
	AuthorityReceiptSHA256   string          `json:"authority_receipt_sha256"`
	AuthorityGeneratorPath   string          `json:"authority_generator_path"`
	AuthorityGeneratorSHA256 string          `json:"authority_generator_sha256"`
	LockDevice               uint64          `json:"lock_device"`
	LockInode                uint64          `json:"lock_inode"`
	AuthorityScope           string          `json:"authority_scope"`
	AuthoritySessionID       string          `json:"authority_session_id"`
	ConcurrencyTarget        string          `json:"concurrency_target"`
	SchemaSHA256             string          `json:"schema_sha256"`
}

var bootPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var supervisorPattern = regexp.MustCompile(`^serenity-release-[a-zA-Z0-9_.-]+\.service$`)

type rootPin struct {
	file *os.File
	stat syscall.Stat_t
	hash string
}
type custody struct {
	proof       custodyProof
	pins        []rootPin
	lock, probe *os.File
}

func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func strictDecode(data []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errRefused
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errRefused
	}
	return nil
}
func sameStat(a, b syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
func pinFD(fd int, private bool) (rootPin, []byte, error) {
	var pin rootPin
	if fd < 3 {
		return pin, nil, errRefused
	}
	if syscall.Fstat(fd, &pin.stat) != nil || pin.stat.Uid != 0 || pin.stat.Nlink != 1 || pin.stat.Mode&syscall.S_IFMT != syscall.S_IFREG || pin.stat.Mode&0022 != 0 || pin.stat.Size < 0 || pin.stat.Size > 65536 || (private && pin.stat.Mode&0077 != 0) {
		return pin, nil, errRefused
	}
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	if err != nil || flags&unix.O_ACCMODE != unix.O_RDONLY {
		return pin, nil, errRefused
	}
	data := make([]byte, pin.stat.Size)
	if len(data) > 0 {
		if n, readErr := syscall.Pread(fd, data, 0); readErr != nil || n != len(data) {
			return pin, nil, errRefused
		}
	}
	var after syscall.Stat_t
	if syscall.Fstat(fd, &after) != nil || !sameStat(pin.stat, after) {
		return pin, nil, errRefused
	}
	pin.hash = hashBytes(data)
	pin.file = os.NewFile(uintptr(fd), "root-custody")
	syscall.CloseOnExec(fd)
	return pin, data, nil
}
func (p custodyProof) valid(now int64, source string, uid, gid int) bool {
	if p.Version != 1 || p.Purpose != "serenity-episode-fixture-offline-v1" || (p.Operation != "prepare" && p.Operation != "reissue" && p.Operation != "revoke") || uid <= 0 || gid <= 0 || p.UID != uid || p.GID != gid || p.ToolSource != source || !sourceID.MatchString(source) || p.AccountCap <= 0 {
		return false
	}
	if p.IssuedAt <= 0 || now < p.IssuedAt || now-p.IssuedAt > 300 || p.ExpiresAt <= now || p.ExpiresAt-p.IssuedAt > 300 || p.ExpiresAt <= p.IssuedAt {
		return false
	}
	for _, s := range []string{p.AuthorityScope, p.AuthoritySessionID, p.ConcurrencyTarget} {
		if len(s) == 0 || len(s) > 256 {
			return false
		}
	}
	for _, s := range []string{p.Nonce, p.ToolSHA256, p.WrapperSHA256, p.RuntimeSHA256, p.ServiceBinarySHA256, p.ConfigSHA256, p.HoldSHA256, p.StateSHA256, p.ReadySHA256, p.AuthorityReceiptSHA256, p.AuthorityGeneratorSHA256, p.SchemaSHA256, p.Hold.ManifestSHA256, p.Hold.ExecutableSHA256, p.Hold.PreviousStateSHA256, p.Hold.ExpectedStateSHA256} {
		if !hexDigest.MatchString(s) {
			return false
		}
	}
	for _, s := range []string{p.ToolPath, p.WrapperPath, p.RuntimePath, p.ServiceBinaryPath, p.ConfigPath, p.DataDir, p.PrivateDir, p.AuthorityReceiptPath, p.AuthorityGeneratorPath, p.Hold.Stage} {
		if !filepath.IsAbs(s) || filepath.Clean(s) != s || strings.ContainsAny(s, "\x00\r\n\t ") {
			return false
		}
	}
	return bootPattern.MatchString(p.BootID) && supervisorPattern.MatchString(p.Hold.Supervisor) && strings.HasPrefix(p.Hold.Stage, "/var/lib/serenity-release/") && p.Hold.Timestamp > 0 && p.ConfigPath == "/etc/serenity/hosted.json" && p.Hold.Phase == "maintenance-ready" && p.Hold.BackupTimerPriorState == "active" && p.Hold.Timestamp <= p.IssuedAt && p.StateSHA256 == p.Hold.PreviousStateSHA256 && p.RuntimePath == p.Hold.Stage+"/activation-runtime.py" && sourceID.MatchString(p.ServiceSource) && sourceID.MatchString(p.Hold.Source) && p.LockDevice > 0 && p.LockInode > 0
}

func newCustody(args []string) (*custody, error) {
	if len(args) != 2 || args[0] != "--custody-fds" {
		return nil, errRefused
	}
	parts := strings.Split(args[1], ",")
	if len(parts) != 6 {
		return nil, errRefused
	}
	ids := make([]int, 6)
	seen := map[int]bool{}
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil || n < 3 || seen[n] {
			return nil, errRefused
		}
		ids[i] = n
		seen[n] = true
	}
	c := &custody{}
	var documents [4][]byte
	for i := 0; i < 4; i++ {
		pin, data, err := pinFD(ids[i], i != 3)
		if err != nil {
			return nil, errRefused
		}
		c.pins = append(c.pins, pin)
		documents[i] = data
	}
	if strictDecode(documents[0], &c.proof) != nil || !c.proof.valid(time.Now().Unix(), sourceRevision, os.Geteuid(), os.Getegid()) {
		return nil, errRefused
	}
	p := c.proof
	var h maintenanceHold
	if strictDecode(documents[1], &h) != nil || h != p.Hold || hashBytes(documents[1]) != p.HoldSHA256 || hashBytes(documents[2]) != p.StateSHA256 || hashBytes(documents[3]) != p.ConfigSHA256 {
		return nil, errRefused
	}
	var config struct {
		DataDir    string   `json:"data_dir"`
		AccountCap int      `json:"account_cap"`
		Enabled    bool     `json:"episode_archive_enabled"`
		Brains     []string `json:"episode_synthetic_brains"`
	}
	if json.Unmarshal(documents[3], &config) != nil || config.DataDir != p.DataDir || config.AccountCap != p.AccountCap || config.Enabled || len(config.Brains) != 0 {
		return nil, errRefused
	}
	// Verify actual executed bytes, not a caller-supplied version string alone.
	exe, err := os.Open("/proc/self/exe")
	if err != nil {
		return nil, errRefused
	}
	hasher := sha256.New()
	_, err = io.Copy(hasher, io.LimitReader(exe, 268435457))
	closeErr := exe.Close()
	if err != nil || closeErr != nil || hex.EncodeToString(hasher.Sum(nil)) != p.ToolSHA256 {
		return nil, errRefused
	}
	c.lock = os.NewFile(uintptr(ids[4]), "release-lock")
	c.probe = os.NewFile(uintptr(ids[5]), "release-lock-probe")
	syscall.CloseOnExec(ids[4])
	syscall.CloseOnExec(ids[5])
	if c.check() != nil {
		return nil, errRefused
	}
	return c, nil
}

func (c *custody) check() error {
	p := c.proof
	if !p.valid(time.Now().Unix(), sourceRevision, os.Geteuid(), os.Getegid()) {
		return errRefused
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil || strings.TrimSpace(string(boot)) != p.BootID {
		return errRefused
	}
	for _, pin := range c.pins {
		var st syscall.Stat_t
		if syscall.Fstat(int(pin.file.Fd()), &st) != nil || !sameStat(pin.stat, st) {
			return errRefused
		}
		data := make([]byte, st.Size)
		if len(data) > 0 {
			if _, err := pin.file.ReadAt(data, 0); err != nil {
				return errRefused
			}
		}
		if hashBytes(data) != pin.hash {
			return errRefused
		}
	}
	var a, b syscall.Stat_t
	if c.lock == nil || c.probe == nil || syscall.Fstat(int(c.lock.Fd()), &a) != nil || syscall.Fstat(int(c.probe.Fd()), &b) != nil || !sameStat(a, b) || a.Uid != 0 || a.Mode&0077 != 0 || a.Mode&syscall.S_IFMT != syscall.S_IFREG || a.Nlink != 1 || uint64(a.Dev) != p.LockDevice || a.Ino != p.LockInode {
		return errRefused
	}
	err = syscall.Flock(int(c.probe.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		syscall.Flock(int(c.probe.Fd()), syscall.LOCK_UN)
		return errRefused
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
		return errRefused
	}
	if syscall.Flock(int(c.lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errRefused
	}
	return nil
}

// schemaDigest uses only schema metadata. It cannot read a customer row. A proof
// must pin the parent-reviewed schema fingerprint, not just an integer version.
func schemaDigest(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name`)
	if err != nil {
		return "", errRefused
	}
	defer rows.Close()
	expected := map[string]bool{
		"accounts":                     false,
		"accounts_created_by_partner":  false,
		"audit_log":                    false,
		"billing_failures":             false,
		"billing_failures_account":     false,
		"brains":                       false,
		"brains_account_state":         false,
		"brains_default_active":        false,
		"checkout_attempts":            false,
		"client_credentials":           false,
		"client_credentials_partner":   false,
		"link_requests":                false,
		"link_requests_account":        false,
		"login_tokens":                 false,
		"oauth_clients":                false,
		"oauth_clients_expiry":         false,
		"oauth_codes":                  false,
		"oauth_codes_client":           false,
		"oauth_codes_expiry":           false,
		"oauth_consents":               false,
		"oauth_consents_client":        false,
		"oauth_consents_expiry":        false,
		"oauth_epochs":                 false,
		"oauth_grants":                 false,
		"oauth_grants_client":          false,
		"oauth_grants_expiry":          false,
		"oauth_refresh":                false,
		"oauth_refresh_grant":          false,
		"oauth_tokens":                 false,
		"oauth_tokens_expiry":          false,
		"oauth_tokens_grant":           false,
		"operations":                   false,
		"operations_active_client_key": false,
		"operations_expired_reserved":  false,
		"operations_pending_review":    false,
		"partner_entitlements":         false,
		"partner_links":                false,
		"partner_links_account":        false,
		"partners":                     false,
		"reservation_open_expiry":      false,
		"reservation_operation":        false,
		"reservation_released_age":     false,
		"reservation_replay":           false,
		"reservations":                 false,
		"schema_migrations":            false,
		"sessions":                     false,
		"stripe_events":                false,
		"subscriptions":                false,
		"usage_windows":                false,
		"workos_identities":            false,
		"workos_logins":                false,
	}
	hasher := sha256.New()
	for rows.Next() {
		var v [4]string
		if rows.Scan(&v[0], &v[1], &v[2], &v[3]) != nil {
			return "", errRefused
		}
		seen, known := expected[v[1]]
		if !known || seen || (v[0] != "table" && v[0] != "index") {
			return "", errRefused
		}
		expected[v[1]] = true
		for _, field := range v {
			if strings.ContainsRune(field, 0) {
				return "", errRefused
			}
			io.WriteString(hasher, field)
			hasher.Write([]byte{0})
		}
	}
	if rows.Err() != nil {
		return "", errRefused
	}
	for _, seen := range expected {
		if !seen {
			return "", errRefused
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func preflightDB(ctx context.Context, p custodyProof) error {
	for _, dir := range []string{p.DataDir, filepath.Join(p.DataDir, "brains"), p.PrivateDir} {
		if privatefs.ValidateDirectory(ctx, dir) != nil {
			return errRefused
		}
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		info, err := os.Lstat(filepath.Join(p.DataDir, "control.db"+suffix))
		if suffix != "" && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errRefused
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
			return errRefused
		}
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(p.DataDir, "control.db")}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro")
	if err != nil {
		return errRefused
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var n, min, max int
	if db.QueryRowContext(ctx, `SELECT count(*),min(version),max(version) FROM schema_migrations`).Scan(&n, &min, &max) != nil || n != 10 || min != 1 || max != 10 {
		return errRefused
	}
	sha, err := schemaDigest(ctx, db)
	if err != nil || sha != p.SchemaSHA256 {
		return errRefused
	}
	return nil
}

func executeCustody(args []string) error {
	c, err := newCustody(args)
	if err != nil {
		return errRefused
	}
	// Keep descriptor wrappers alive until operation exit; child owns no root paths.
	defer func() {
		for _, pin := range c.pins {
			pin.file.Close()
		}
		c.probe.Close()
		c.lock.Close()
	}()
	p := c.proof
	deadline := time.Now().Add(110 * time.Second)
	if expiry := time.Unix(p.ExpiresAt, 0); expiry.Before(deadline) {
		deadline = expiry
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	if c.check() != nil {
		return errRefused
	}
	dbPath := filepath.Join(p.DataDir, "control.db")
	before, statErr := os.Lstat(dbPath)
	if statErr != nil {
		return errRefused
	}
	if preflightDB(ctx, p) != nil || c.check() != nil {
		return errRefused
	}
	current, statErr := os.Lstat(dbPath)
	if statErr != nil || !os.SameFile(before, current) {
		return errRefused
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return errRefused
	}
	defer db.Close()
	e := engine{store: db, privateDir: p.PrivateDir, target: target{DataDir: p.DataDir, ConfigPath: p.ConfigPath, ConfigSHA256: p.ConfigSHA256, Source: p.ToolSource, AccountCap: p.AccountCap}, authorize: c.check, custodyEpoch: p.HoldSHA256}
	if e.operate(ctx, p.Operation) != nil || c.check() != nil {
		return errRefused
	}
	return nil
}
