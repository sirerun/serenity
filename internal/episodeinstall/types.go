// Package episodeinstall prepares and forward-installs a narrowly qualified
// inactive hosted episode erasure candidate. It never authenticates production
// authority, enables capture, removes retained copies or finishes maintenance.
package episodeinstall

import (
	"context"
	"errors"
	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

const headerSchema = "serenity.prepared-episode-install/v1"
const headerName = "prepared.json"
const progressName = "install.json"
const maxSourceBytes int64 = 256 << 20
const maxSourceEntries = 65536
const maxOperationBytes int64 = 2 << 30
const maxOperationEntries = 300000
const maxHeaderBytes = 64 << 20

var ErrUnqualified = errors.New("episodeinstall: unsafe, changed or unqualified state")
var ErrIncomplete = errors.New("episodeinstall: incomplete installation; maintenance and copies retained")

// Request is a trusted owner's preparation input, never a model/HTTP grant.
// Source and OperationDirectory are canonical absolute routing, omitted from JSON.
type Request struct {
	Source             string                   `json:"-"`
	OperationDirectory string                   `json:"-"`
	AccountID          string                   `json:"account_id"`
	BrainID            string                   `json:"brain_id"`
	OperationID        string                   `json:"operation_id"`
	SourceHead         string                   `json:"source_head"`
	CurrentFences      []store.EpisodeRecord    `json:"current_fences"`
	IndependentSources []episodeerase.SourcePin `json:"independent_sources"`
	IndependentPaths   []episodeerase.PathPin   `json:"independent_paths"`
}

// PreparedRef pins immutable preparation evidence; it confers no authority.
type PreparedRef struct {
	Source         string                     `json:"-"`
	Directory      string                     `json:"-"`
	HeaderSHA256   string                     `json:"header_sha256"`
	Identity       writer.MaintenanceIdentity `json:"identity"`
	ResidualCopies bool                       `json:"residual_copies"`
}

// AdmissionPins must be checked without assuming the canonical tree is intact:
// Resume may occur between component renames. Check authenticates actual cold
// account/brain/root custody and complete CURRENT relevant exclusion records.
// A matching historical digest or a live preparation lease is insufficient.
type AdmissionPins struct {
	Source        string `json:"-"`
	AccountID     string `json:"account_id"`
	BrainID       string `json:"brain_id"`
	OperationID   string `json:"operation_id"`
	SourceHead    string `json:"source_head"`
	CandidateHead string `json:"candidate_head"`
	FenceDigest   string `json:"fence_digest"`
	HeaderSHA256  string `json:"header_sha256"`
}

type ColdAdmission interface {
	Check(context.Context, AdmissionPins) error
}

// Result contains content-free custody state, never physical completion.
type Result struct {
	State          string `json:"state"`
	HeaderSHA256   string `json:"header_sha256"`
	ResidualCopies bool   `json:"residual_copies"`
	MarkerHeld     bool   `json:"marker_held"`
}

// inventory is sorted by Path, with a mandatory root entry Path=".".
// Kind is absent (sole entry, zero fields), directory (Mode only), or file
// (Mode, Bytes and SHA256). Mode is exact permission bits; directory entries
// preserve empty directories. Paths are slash-relative, never absolute. Hashes
// bind canonical JSON of ALL entries, including modes, paths and absences.
type inventoryEntry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Mode   uint32 `json:"mode"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type inventory []inventoryEntry

type component struct {
	Name string    `json:"name"`
	Old  inventory `json:"old"`
	New  inventory `json:"new"`
}

// preparedHeader is immutable canonical JSON with a single trailing newline.
// Routing hashes bind private absolute routes without exporting their bytes.
// Original is immutable canonical proof (five components only); OldIndex is
// a separate exact family copy; inspection/ owns all SQLite inspection effects.
type preparedHeader struct {
	Schema                string                     `json:"schema"`
	SourceRouteSHA256     string                     `json:"source_route_sha256"`
	OperationRouteSHA256  string                     `json:"operation_route_sha256"`
	AccountID             string                     `json:"account_id"`
	Identity              writer.MaintenanceIdentity `json:"identity"`
	Candidate             episodeerase.Result        `json:"candidate"`
	CurrentFences         []store.EpisodeRecord      `json:"current_fences"`
	IndependentSources    []episodeerase.SourcePin   `json:"independent_sources"`
	IndependentPaths      []episodeerase.PathPin     `json:"independent_paths"`
	Components            []component                `json:"components"`
	Original              inventory                  `json:"original"`
	Inspection            inventory                  `json:"inspection"`
	OldIndex              inventory                  `json:"old_index"`
	CandidateFiles        inventory                  `json:"candidate_files"`
	Qualification         index.Qualification        `json:"qualification"`
	NewIndexQualification index.Qualification        `json:"new_index_qualification"`
}

var componentNames = [...]string{".git", "brain", ".dira", "serenity.yml", ".gitignore", ".serenity/index.db", ".serenity/index.db-wal", ".serenity/index.db-shm"}

// Fixed layout: original/{five canonical components}, old-index/{family},
// inspection/{family}, candidate/{repository.git,manifest.json}, replacement/
// {five canonical components}, new-index/{family}, retained/{component paths},
// prepared.json and (after installation starts) install.json. No arbitrary
// filenames or disposable cleanup stages are admitted. Original/candidate/
// old-index/inspection remain untouched by Install; replacement/new-index
// entries move forward to canonical slots and old slots move to retained/.
