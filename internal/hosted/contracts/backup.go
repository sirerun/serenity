package contracts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Backup manifest v2 (interfaces.md "Backup manifestv2", owner task49).
// FROZEN shape, except JournalWatermark: it carries the architect-approved
// deletion-journal position and requires task48's live service qualification
// before production use. Every other field is independent of the four design
// decisions.
//
// This revision adds what task49 step 1 and acceptance require and the earlier
// draft could not represent (independent audit D2): the control database
// artifact with its SHA256 and length, and each non-empty brain's canonical
// bundle heads. Task49 has not started, and nothing outside this package
// references the type, so the change breaks no consumer. It is still a change
// to a frozen shape, recorded in interfaces.md, and task49 must rebase on it.
// No field is reserved for a future incremental-backup scheme.
//
// ManifestV2 supersedes backup.Manifest (version 1, internal/hosted/backup).
// Version 1 records only brain IDs and an empty flag; v2 adds artifact
// checksums/lengths, the source revision, the schema version and the
// deletion journal watermark current at manifest creation, so restore
// (task50) can prove journal completeness from the manifest's own recorded
// watermark forward.
//
// Validate is the structural check any reader runs before it trusts a manifest
// to name files. It proves the manifest is well formed and internally
// consistent. It does not prove the named files exist or match their recorded
// checksums, that the brain inventory equals the control database's, or that
// the manifest is authentic: task49 checks those, and a hash detects
// corruption but does not authorize an untrusted upload.
type ManifestV2 struct {
	Version   int       `json:"version"` // 2
	Source    SourceRef `json:"source"`
	CreatedAt time.Time `json:"created_at"` // UTC
	// ControlDB is the VACUUM INTO snapshot of the control database. Its
	// checksum and length let restore reject a truncated or altered database
	// before it opens it.
	ControlDB ArtifactRef `json:"control_db"`
	// Brains is the full brain inventory, strictly ascending by ID (which makes
	// it sorted and unique in one check).
	Brains []BrainArtifact `json:"brains"`
	// JournalWatermark is the deletion journal position read from the
	// independent journal (never the local control database) BEFORE the backup
	// began copying data. Recovery replays every journal entry after it;
	// replaying an entry the snapshot already reflects is safe because purge
	// is idempotent, whereas reading the watermark after the copy could skip a
	// deletion the snapshot missed. This ordering is the safety argument.
	// Gateway.Maintenance blocks DeleteBrain while Service.Backup holds it, but
	// DeleteAccount, Export and RecoverDeletions do not take it, so it cannot
	// be the guarantee. The zero value means the journal was empty.
	JournalWatermark DeletionWatermark `json:"journal_watermark"`
}

type SourceRef struct {
	BuildSHA      string `json:"build_sha"`
	SchemaVersion int    `json:"schema_version"`
}

// ArtifactRef names one file in the snapshot by its manifest-relative path
// (never an absolute path, and a single path element, so it cannot traverse),
// with the length and SHA256 that make a mismatch name a concrete,
// reproducible file.
type ArtifactRef struct {
	RelativePath string `json:"relative_path"` // e.g. "control.db"
	LengthBytes  int64  `json:"length_bytes"`
	SHA256       string `json:"sha256"` // 64 lowercase hex characters
}

// BundleHead is one ref a canonical Git bundle carries and the object it
// points at, as `git bundle list-heads` reports them. Recording them lets
// restore prove the cloned repository reproduces the same canonical state and
// not merely a bundle whose bytes match.
type BundleHead struct {
	Ref      string `json:"ref"`       // "HEAD" or a "refs/..." name
	ObjectID string `json:"object_id"` // 40 or 64 lowercase hex characters
}

// BrainArtifact names one brain's bundle. A brain with no canonical repository
// yet is Empty: it has no artifact and no heads. A non-empty brain has a
// bundle and at least one head, so a ready brain missing its repository is
// corruption and is never representable as an empty success.
type BrainArtifact struct {
	ID string `json:"id"`
	// ArtifactRef is the bundle file; zero exactly when Empty is true.
	ArtifactRef
	Empty bool `json:"empty"`
	// Heads are the bundle's refs, strictly ascending by Ref; none exactly when
	// Empty is true.
	Heads []BundleHead `json:"heads,omitempty"`
}

var (
	// ErrManifestVersion: the manifest is not version 2. Version 1 handling is
	// task49's explicit support-or-refuse policy, decided outside this type.
	ErrManifestVersion = errors.New("hosted/contracts: unsupported backup manifest version")
	// ErrManifestInvalid: the manifest is malformed or internally inconsistent.
	ErrManifestInvalid = errors.New("hosted/contracts: invalid backup manifest")
)

// manifestFileName is the fixed name backup writes the manifest under
// (internal/hosted/backup). No artifact may take it.
const manifestFileName = "manifest.json"

// Validate reports the first structural defect in m, wrapping ErrManifestVersion
// or ErrManifestInvalid.
func (m ManifestV2) Validate() error {
	if m.Version != 2 {
		return fmt.Errorf("%w: got %d, want 2", ErrManifestVersion, m.Version)
	}
	if m.Source.BuildSHA == "" || strings.ContainsFunc(m.Source.BuildSHA, unicode.IsSpace) {
		return manifestErr("source build must be a non-empty token")
	}
	if m.Source.SchemaVersion <= 0 {
		return manifestErr("source schema version %d must be positive", m.Source.SchemaVersion)
	}
	if m.CreatedAt.IsZero() {
		return manifestErr("created_at is unset")
	}
	if _, offset := m.CreatedAt.Zone(); offset != 0 {
		return manifestErr("created_at must be UTC")
	}

	// Paths are unique across the control database and every bundle, compared
	// case-insensitively so two names cannot collide on a case-folding
	// filesystem, and none may take the manifest's own name.
	paths := map[string]string{}
	claim := func(owner string, a ArtifactRef) error {
		if err := a.validate(); err != nil {
			return manifestErr("%s: %v", owner, err)
		}
		folded := strings.ToLower(a.RelativePath)
		if folded == manifestFileName {
			return manifestErr("%s: path %q collides with the manifest", owner, a.RelativePath)
		}
		if prior, dup := paths[folded]; dup {
			return manifestErr("%s: path %q duplicates %s", owner, a.RelativePath, prior)
		}
		paths[folded] = owner
		return nil
	}
	if err := claim("control_db", m.ControlDB); err != nil {
		return err
	}

	for i, b := range m.Brains {
		if !validName(b.ID) {
			return manifestErr("brains[%d]: id %q is not a safe identifier", i, b.ID)
		}
		if i > 0 && b.ID <= m.Brains[i-1].ID {
			return manifestErr("brains[%d]: id %q is not strictly after %q (inventory must be sorted and unique)", i, b.ID, m.Brains[i-1].ID)
		}
		owner := "brain " + b.ID
		if b.Empty {
			if b.ArtifactRef != (ArtifactRef{}) || len(b.Heads) != 0 {
				return manifestErr("%s: an empty brain has no bundle and no heads", owner)
			}
			continue
		}
		if err := claim(owner, b.ArtifactRef); err != nil {
			return err
		}
		if len(b.Heads) == 0 {
			return manifestErr("%s: a non-empty brain records at least one head", owner)
		}
		for k, h := range b.Heads {
			if !validRef(h.Ref) {
				return manifestErr("%s: heads[%d]: ref %q is not HEAD or a refs/ name", owner, k, h.Ref)
			}
			if k > 0 && h.Ref <= b.Heads[k-1].Ref {
				return manifestErr("%s: heads[%d]: ref %q is not strictly after %q", owner, k, h.Ref, b.Heads[k-1].Ref)
			}
			if !validObjectID(h.ObjectID) {
				return manifestErr("%s: heads[%d]: object id is not 40 or 64 lowercase hex characters", owner, k)
			}
		}
	}

	w := m.JournalWatermark
	if !w.IsZero() && (w.Generation < 1 || w.SequenceID < 1 || !validSHA256(w.EntryHash)) {
		return manifestErr("journal watermark is partially populated")
	}
	return nil
}

func (a ArtifactRef) validate() error {
	if !validName(a.RelativePath) {
		return fmt.Errorf("path %q is not a single safe path element", a.RelativePath)
	}
	if a.LengthBytes <= 0 {
		return fmt.Errorf("length %d must be positive", a.LengthBytes)
	}
	if !validSHA256(a.SHA256) {
		return errors.New("sha256 is not 64 lowercase hex characters")
	}
	return nil
}

func manifestErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrManifestInvalid, fmt.Sprintf(format, args...))
}

// validName accepts one path element: no separator, no traversal, no
// whitespace or control character, not "." or "..".
func validName(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 255 {
		return false
	}
	for _, r := range s {
		if r == '/' || r == '\\' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validRef(s string) bool {
	if s == "HEAD" {
		return true
	}
	rest, ok := strings.CutPrefix(s, "refs/")
	if !ok || rest == "" {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validSHA256(s string) bool { return isLowerHex(s, 64) }

func validObjectID(s string) bool { return isLowerHex(s, 40) || isLowerHex(s, 64) }

func isLowerHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Recovery CLI (interfaces.md "Recovery CLI", owner task50, registration
// owner task41/57). The plan/apply shape is FROZEN, including journal and
// fence fields from the architect-approved decisions 3 and 4. Live journal
// qualification and production implementation remain pending. Apply's
// eligibility rule is checked by RecoveryApplyResult.Consistent.
//
// Plan is immutable once produced: PlanHash pins the exact source snapshot,
// deletion-journal watermark and provider truth the plan was computed
// against. Apply refuses to run against a plan whose hash does not match
// what the operator approved, and never accepts an "activate-all" request —
// every phase is scoped to one account at a time and is safe to resume.
type RecoveryPlan struct {
	PlanHash         string
	SourceSnapshot   string // manifest v2 source SHA this plan was computed from
	JournalWatermark DeletionWatermark
	// Generation is the journal generation the plan's activation closes with
	// DeletionJournal.Seal; the restored service then writes generation+1.
	Generation      int64
	ProviderTruthAt time.Time
	Accounts        []string // opaque account IDs eligible for this plan
}

type RecoveryPlanRequest struct {
	SnapshotPath string
}

// RecoveryPlanner computes an immutable, single-source restore plan. It pins
// the exact snapshot, journal watermark and provider truth in PlanHash.
type RecoveryPlanner interface {
	Plan(ctx context.Context, req RecoveryPlanRequest) (RecoveryPlan, error)
}

type RecoveryApplyRequest struct {
	PlanHash  string
	AccountID string // exactly one account per Apply call; never "all"
}

type RecoveryApplyResult struct {
	AccountID      string
	PlanGeneration int64 // generation from the exact immutable plan named by the apply request's PlanHash
	Unfrozen       bool
	Reason         string // set and non-empty whenever Unfrozen is false
	// Fence is the proof the old writer was fenced. It must be Sufficient
	// for PlanGeneration whenever Unfrozen is true.
	Fence FenceReceipt
}

// RecoveryApplier applies one account from the exact plan approved by the
// operator. It rejects stale or mismatched plan hashes and never accepts an
// apply-all request.
type RecoveryApplier interface {
	Apply(ctx context.Context, req RecoveryApplyRequest) (RecoveryApplyResult, error)
}

// Consistent reports whether the result obeys the activation rule: an account
// is unfrozen only with a sufficient fence receipt, and every refusal names a
// reason. Task50's Apply must never return a result for which this fails.
func (r RecoveryApplyResult) Consistent() error {
	if r.AccountID == "" {
		return errors.New("hosted/contracts: recovery result has no account")
	}
	if r.Unfrozen {
		return r.Fence.Sufficient(r.PlanGeneration)
	}
	if r.Reason == "" {
		return errors.New("hosted/contracts: refused recovery result must state a reason")
	}
	return nil
}
