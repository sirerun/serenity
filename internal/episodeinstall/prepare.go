package episodeinstall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/episodeerase"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Prepare creates a durable INACTIVE operation. The input is a trusted owner's
// inventory assertion, not authenticated deletion authority. Nothing is deleted
// or installed. Once its directory exists, every failure retains attributable
// sensitive copies and returns ResidualCopies with no usable header pin.
func Prepare(ctx context.Context, req Request) (ref PreparedRef, err error) {
	return prepare(ctx, req, nil)
}

// observe is a per-call preparation fault/interleaving seam for owned fixtures.
func prepare(ctx context.Context, req Request, observe func(string)) (ref PreparedRef, err error) {
	ref.Source = req.Source
	ref.Directory = req.OperationDirectory
	if ctx == nil {
		return ref, ErrUnqualified
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if ctx.Err() != nil || validateRoutes(req.Source, req.OperationDirectory, req.OperationID) != nil || !accountPattern.MatchString(req.AccountID) || !brainPattern.MatchString(req.BrainID) || !gitPattern.MatchString(req.SourceHead) || len(req.CurrentFences) == 0 || len(req.CurrentFences) > store.EpisodeMaxFences || len(req.IndependentSources) > 2048 || len(req.IndependentPaths) < 2 || len(req.IndependentPaths) > 4096 {
		return ref, ErrUnqualified
	}
	for _, f := range req.CurrentFences {
		if f.Schema != store.EpisodeSchema || f.Kind != "exclude" || f.BrainID != req.BrainID || f.Input != nil || len(f.Refs) != 0 || !digestPattern.MatchString(f.CommandKey) || !digestPattern.MatchString(f.ConversationKey) || !digestPattern.MatchString(f.RequestDigest) || f.RecordedAt.IsZero() {
			return ref, ErrUnqualified
		}
	}
	for _, p := range req.IndependentSources {
		if !digestPattern.MatchString(p.SHA256) || !digestPattern.MatchString(p.MetadataSHA256) {
			return ref, ErrUnqualified
		}
	}
	for _, p := range req.IndependentPaths {
		if !safeRelative(p.Path) || !digestPattern.MatchString(p.SHA256) {
			return ref, ErrUnqualified
		}
	}
	// Detach all caller-owned slices before ownership or copying begins.
	raw, marshalErr := json.Marshal(req)
	if marshalErr != nil || len(raw) > maxHeaderBytes {
		return ref, ErrUnqualified
	}
	source, operation := req.Source, req.OperationDirectory
	if json.Unmarshal(raw, &req) != nil {
		return ref, ErrUnqualified
	}
	req.Source = source
	req.OperationDirectory = operation
	sort.Slice(req.CurrentFences, func(i, j int) bool { return req.CurrentFences[i].Generation < req.CurrentFences[j].Generation })
	ref.Identity = writer.MaintenanceIdentity{Schema: "serenity.canonical-maintenance/v1", OperationID: req.OperationID, BrainID: req.BrainID, SourceHead: req.SourceHead, FenceDigest: store.EpisodeHash(req.CurrentFences)}
	owner, ownerErr := writer.AcquireBrain(req.Source)
	if ownerErr != nil {
		return ref, ErrUnqualified
	}
	defer func() {
		if owner.Close() != nil {
			ref.HeaderSHA256 = ""
			err = ErrUnqualified
		}
	}()
	if validateCanonicalLayout(ctx, req.Source) != nil || validateRuntime(ctx, req.Source, false) != nil {
		return ref, ErrUnqualified
	}
	git := &prepareGit{}
	if prepareHead(ctx, git, req.Source) != req.SourceHead || !prepareClean(ctx, git, req.Source) {
		return ref, ErrUnqualified
	}
	old := make([]inventory, len(componentNames))
	var sourceBytes int64
	sourceEntries := 0
	for i, name := range componentNames {
		inv, inventoryErr := inventoryPath(ctx, filepath.Join(req.Source, filepath.FromSlash(name)))
		if inventoryErr != nil {
			return ref, ErrUnqualified
		}
		old[i] = inv
		for _, entry := range inv {
			sourceEntries++
			sourceBytes += entry.Bytes
		}
	}
	if sourceBytes > maxSourceBytes || sourceEntries > maxSourceEntries {
		return ref, ErrUnqualified
	}
	if old[5][0].Kind == "absent" && (old[6][0].Kind != "absent" || old[7][0].Kind != "absent") {
		return ref, ErrUnqualified
	}
	if prepareCapacity(filepath.Dir(req.OperationDirectory), maxOperationBytes) != nil {
		return ref, ErrUnqualified
	}
	if os.Mkdir(req.OperationDirectory, 0700) != nil {
		return ref, ErrUnqualified
	}
	ref.ResidualCopies = true
	if syncDirectory(req.OperationDirectory) != nil || syncDirectory(filepath.Dir(req.OperationDirectory)) != nil {
		return ref, ErrUnqualified
	}
	if observe != nil {
		observe("operation-created")
	}
	original := filepath.Join(req.OperationDirectory, "original")
	oldIndex := filepath.Join(req.OperationDirectory, "old-index")
	inspection := filepath.Join(req.OperationDirectory, "inspection")
	newIndex := filepath.Join(req.OperationDirectory, "new-index")
	for _, directory := range []string{original, oldIndex, inspection, newIndex} {
		if os.Mkdir(directory, 0700) != nil {
			return ref, ErrUnqualified
		}
	}
	for i := 0; i < 5; i++ {
		if copyInventory(ctx, filepath.Join(req.Source, componentNames[i]), filepath.Join(original, componentNames[i]), old[i]) != nil {
			return ref, ErrUnqualified
		}
	}
	for i := 5; i < len(componentNames); i++ {
		basename := filepath.Base(componentNames[i])
		if copyInventory(ctx, filepath.Join(req.Source, filepath.FromSlash(componentNames[i])), filepath.Join(oldIndex, basename), old[i]) != nil {
			return ref, ErrUnqualified
		}
		if copyInventory(ctx, filepath.Join(req.Source, filepath.FromSlash(componentNames[i])), filepath.Join(inspection, basename), old[i]) != nil {
			return ref, ErrUnqualified
		}
	}
	var qualification index.Qualification
	if old[5][0].Kind != "absent" {
		if preparePrivateFamily(ctx, inspection) != nil {
			return ref, ErrUnqualified
		}
		qualification, err = index.QualifyMaintenanceCopy(ctx, filepath.Join(inspection, "index.db"))
		if err != nil {
			return ref, ErrUnqualified
		}
	}
	if prepareOperationBounds(ctx, req.OperationDirectory) != nil {
		return ref, ErrUnqualified
	}
	originalInventory, inventoryErr := inventoryPath(ctx, original)
	if inventoryErr != nil {
		return ref, ErrUnqualified
	}
	oldIndexInventory, inventoryErr := inventoryPath(ctx, oldIndex)
	if inventoryErr != nil {
		return ref, ErrUnqualified
	}
	inspectionInventory, inventoryErr := inventoryPath(ctx, inspection)
	if inventoryErr != nil {
		return ref, ErrUnqualified
	}
	if observe != nil {
		observe("original-copied")
	}
	candidateDir := filepath.Join(req.OperationDirectory, "candidate")
	candidate, buildErr := episodeerase.BuildHosted(ctx, episodeerase.HostedRequest{Request: episodeerase.Request{Source: original, Destination: candidateDir, BrainID: req.BrainID, CurrentFences: req.CurrentFences}, IndependentSources: req.IndependentSources, IndependentPaths: req.IndependentPaths})
	if buildErr != nil || candidate.SourceHead != req.SourceHead || candidate.FenceDigest != ref.Identity.FenceDigest {
		return ref, ErrUnqualified
	}
	replacement := filepath.Join(req.OperationDirectory, "replacement")
	if prepareMaterialize(ctx, git, original, candidateDir, replacement, candidate.CandidateHead) != nil {
		return ref, ErrUnqualified
	}
	if validateCanonicalLayout(ctx, replacement) != nil || prepareHead(ctx, git, replacement) != candidate.CandidateHead || !prepareClean(ctx, git, replacement) {
		return ref, ErrUnqualified
	}
	projection, projectionErr := store.LoadEpisodes(ctx, store.NewSourceStore(replacement))
	if projectionErr != nil || projection.BrainID != req.BrainID || projection.Generation != candidate.Generation {
		return ref, ErrUnqualified
	}
	for _, f := range req.CurrentFences {
		if projection.Fences[f.ConversationKey] != f.Generation || store.EpisodeHash(projection.Records[f.CommandKey]) != store.EpisodeHash(f) {
			return ref, ErrUnqualified
		}
	}
	audit, auditErr := index.AuditPages(replacement)
	if auditErr != nil || len(audit.Quarantined) != 0 || len(audit.NonConformingSlugs) != 0 {
		return ref, ErrUnqualified
	}
	cfg, configErr := config.Load(filepath.Join(replacement, config.FileName))
	if configErr != nil {
		return ref, ErrUnqualified
	}
	engine, indexErr := index.Open(filepath.Join(newIndex, "index.db"))
	if indexErr != nil {
		return ref, ErrUnqualified
	}
	rebuildErr := index.Rebuild(ctx, replacement, cfg, engine)
	closeErr := engine.Close()
	if rebuildErr != nil || closeErr != nil {
		return ref, ErrUnqualified
	}
	if preparePrivateFamily(ctx, newIndex) != nil {
		return ref, ErrUnqualified
	}
	newQualification, indexErr := index.QualifyMaintenanceCopy(ctx, filepath.Join(newIndex, "index.db"))
	if indexErr != nil {
		return ref, ErrUnqualified
	}
	if observe != nil {
		observe("replacement-built")
	}
	if prepareOperationBounds(ctx, req.OperationDirectory) != nil {
		return ref, ErrUnqualified
	}
	// Close the handoff race against source mutation without upgrading trusted
	// fence inputs into external authority. Install must repeat under maintenance.
	for i, name := range componentNames {
		if verifyInventory(ctx, filepath.Join(req.Source, filepath.FromSlash(name)), old[i]) != nil {
			return ref, ErrUnqualified
		}
	}
	if validateRuntime(ctx, req.Source, false) != nil || prepareHead(ctx, git, req.Source) != req.SourceHead || !prepareClean(ctx, git, req.Source) {
		return ref, ErrUnqualified
	}
	if verifyInventory(ctx, original, originalInventory) != nil || verifyInventory(ctx, oldIndex, oldIndexInventory) != nil || verifyInventory(ctx, inspection, inspectionInventory) != nil {
		return ref, ErrUnqualified
	}
	header := preparedHeader{Schema: headerSchema, SourceRouteSHA256: hashBytes([]byte(req.Source)), OperationRouteSHA256: hashBytes([]byte(req.OperationDirectory)), AccountID: req.AccountID, Identity: ref.Identity, Candidate: candidate, CurrentFences: req.CurrentFences, IndependentSources: req.IndependentSources, IndependentPaths: req.IndependentPaths, Original: originalInventory, OldIndex: oldIndexInventory, Inspection: inspectionInventory, Qualification: qualification, NewIndexQualification: newQualification}
	for i, name := range componentNames {
		path := filepath.Join(replacement, filepath.FromSlash(name))
		if i >= 5 {
			path = filepath.Join(newIndex, filepath.Base(name))
		}
		inv, inventoryErr := inventoryPath(ctx, path)
		if inventoryErr != nil {
			return ref, ErrUnqualified
		}
		header.Components = append(header.Components, component{Name: name, Old: old[i], New: inv})
	}
	header.CandidateFiles, inventoryErr = inventoryPath(ctx, candidateDir)
	if inventoryErr != nil {
		return ref, ErrUnqualified
	}
	if syncTree(ctx, req.OperationDirectory) != nil {
		return ref, ErrUnqualified
	}
	if observe != nil {
		observe("before-header")
	}
	headerHash, headerErr := writeHeader(ctx, req.OperationDirectory, header)
	if headerErr != nil {
		return ref, ErrUnqualified
	}
	ref.HeaderSHA256 = headerHash
	if _, headerErr = loadHeader(ctx, ref); headerErr != nil || prepareOperationBounds(ctx, req.OperationDirectory) != nil {
		ref.HeaderSHA256 = ""
		return ref, ErrUnqualified
	}
	return ref, nil
}

func prepareHead(ctx context.Context, g *prepareGit, root string) string {
	out, err := g.read(ctx, gitrun.CanonicalReadOnly(root), nil, 256, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func prepareClean(ctx context.Context, g *prepareGit, root string) bool {
	out, err := g.read(ctx, gitrun.CanonicalReadOnly(root), nil, 1<<20, "status", "--porcelain=v1", "--untracked-files=all")
	return err == nil && len(out) == 0
}
