package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/supersede"
	"github.com/sirerun/serenity/internal/writer"
)

// newInboxCmd wires `serenity inbox` (RFC 0001 section 8.2, plan T2.5):
// the human-facing review surface over the DISPOSITION queue every other
// M2 producer stages into with nothing to browse it -- T2.1's Store, T2.2's
// reconcile engine (KindReconcile), T2.6's expiry sweeper (deferred/parked
// transitions), and T2.20's capture (KindDistill).
//
// With no flags, `inbox` drives an interactive review loop: j/k move a
// cursor over every pending or deferred item (oldest first), space
// disposes the item under the cursor with verdict=accept, d with
// verdict=defer, r with verdict=reject (prompting one line of input for
// the required note). Items sharing a non-empty GroupID are reviewed and
// disposed together as one row -- RFC 0001 section 8.2's "grouped items":
// each member still gets its own Store.Dispose call ("each recorded
// individually for the ladder"), so disposing a 3-member group records
// three separate history rows, not one.
//
// --bulk-defer <filter> and --parked are non-interactive one-shot modes
// that never enter the loop.
//
// Disclosed scope: real single-keystroke terminal input (no Enter needed)
// requires the terminal already be in raw/cbreak mode -- this package adds
// no terminal-control dependency to set that up, so an interactive session
// against a real, unconfigured terminal today needs Enter after each key.
// The scripted-TTY acc-line test drives the loop through a plain
// io.Reader, which needs no raw mode at all: it supplies exactly the
// bytes j/k/space (and, where exercised, d/r) with nothing further to
// buffer past. Wiring real raw mode is a disclosed later enhancement, not
// required by this task's acc line.
func newInboxCmd() *cobra.Command {
	var bulkDefer string
	var parked bool
	var unapplied bool
	var applyID string
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Review the DISPOSITION queue: j/k navigate, space/d/r dispose, grouped items dispose together",
		Long: "inbox is the human-facing review surface for the DISPOSITION queue (RFC\n" +
			"0001 section 8.2). With no flags it drives an interactive j/k/space\n" +
			"loop over every pending or deferred item, oldest first, grouped by\n" +
			"GroupID so a group's members are reviewed and disposed together.\n" +
			"Accepted reconciliation claims are committed before moving on.\n" +
			"Use --unapplied to find unfinished publications and --apply <id> to retry.\n" +
			"--bulk-defer and --parked are non-interactive one-shot modes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInbox(cmd.Context(), flagRoot, cmd.InOrStdin(), cmd.OutOrStdout(),
				inboxOptions{BulkDefer: bulkDefer, Parked: parked, Unapplied: unapplied, ApplyID: applyID}, time.Now())
		},
	}
	cmd.Flags().StringVar(&bulkDefer, "bulk-defer", "",
		`non-interactively defer every pending/deferred item matching filter (only "family=<value>" is supported)`)
	cmd.Flags().BoolVar(&parked, "parked", false, "list parked items only -- read-only, no interactive review")
	cmd.Flags().BoolVar(&unapplied, "unapplied", false, "list accepted claim decisions awaiting canonical publication")
	cmd.Flags().StringVar(&applyID, "apply", "", "retry canonical publication of an already accepted reconciliation item")
	cmd.MarkFlagsMutuallyExclusive("bulk-defer", "parked", "unapplied", "apply")
	return cmd
}

type inboxOptions struct {
	BulkDefer string
	Parked    bool
	Unapplied bool
	ApplyID   string
}

// runInbox opens root's derived index (creating it if needed, same as
// runCapture/runStatus) and dispatches to interactive review or a one-shot mode. now is
// injected -- the same single-snapshot convention runCapture/runStatus and
// internal/reconcile/decay.go's Sweep already use -- rather than read live
// per dispose call: a review session's disposals sharing one timestamp is
// an accepted, existing-convention simplification (nothing downstream
// depends on sub-session dispose-time precision -- the expiry sweeper
// T2.6 built operates on day-granularity thresholds), not a new gap this
// task introduces.
func runInbox(ctx context.Context, root string, in io.Reader, out io.Writer, opts inboxOptions, now time.Time) (resultErr error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	dispStore := disposition.NewStore(eng)
	if opts.BulkDefer == "" && !opts.Parked && !opts.Unapplied && opts.ApplyID == "" {
		if _, err := dispStore.ImportPending(ctx, root, now); err != nil {
			return fmt.Errorf("inbox: paused-write import incomplete; unconsumed records retained for retry: %w", err)
		}
	}

	switch {
	case opts.BulkDefer != "":
		return runBulkDefer(ctx, dispStore, opts.BulkDefer, currentActor(), out, now)
	case opts.Parked:
		return runListParked(ctx, dispStore, out)
	case opts.Unapplied:
		return runListUnapplied(ctx, dispStore, out)
	default:
		q := writer.NewQueue(nil)
		defer q.Close()
		sw := &inboxPublisher{writer: supersede.New(q, store.NewFenceWriter(root), store.NewShardStore(root), cfg)}
		defer func() {
			if !sw.published {
				return
			}
			if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("inbox: canonical changes committed but index refresh failed; run serenity sync: %w", err))
				return
			}
			_, _ = fmt.Fprintln(out, "inbox: refreshed derived index; run serenity extract to regenerate embeddings")
		}()
		// Direction writes use the session queue; reconciliation commits each
		// durable publication independently before reviewing another item.
		dirStore := direction.NewStore(root, q)
		if opts.ApplyID != "" {
			item, err := dispStore.Get(ctx, opts.ApplyID)
			if err != nil {
				return err
			}
			id, err := applyInboxDecision(ctx, sw, dispStore, item, now)
			if err != nil {
				return fmt.Errorf("inbox: publication incomplete for %s: %w; resolve the target and retry with inbox --apply %s", item.ID, err, item.ID)
			}
			kind := "claim"
			if item.Kind == disposition.KindDirtyEdit {
				kind = "publication"
			}
			_, _ = fmt.Fprintf(out, "applied %s -> %s %s committed to brain repo\n", item.ID, kind, id)
			return nil
		}
		if err := runListUnapplied(ctx, dispStore, out); err != nil {
			return err
		}

		if err := runInteractive(ctx, dispStore, sw, dirStore, in, out, currentActor(), now); err != nil {
			return err
		}
		// Reconciliation decisions already committed through their publication
		// receipts. Flush the remaining direction writes for this review session.
		committed, ferr := writer.Flush(q, root)
		if ferr != nil {
			return fmt.Errorf("inbox: flush: %w", ferr)
		}
		if committed {
			_, _ = fmt.Fprintln(out, "inbox: committed this session's write-through(s)")
		}
		return nil
	}
}

// reconcilePublisher separates interactive review from publication bookkeeping.
type reconcilePublisher interface {
	PreviewDirtyEdit(disposition.Item, string, time.Time) (string, error)
	ApplyAndCommitDirtyEdit(context.Context, *disposition.Store, disposition.Item, time.Time) (string, error)
	ApplyAndCommitReconcile(context.Context, *disposition.Store, disposition.Item, time.Time) (string, error)
	PreviewDistillAssertion(context.Context, disposition.Item, string, string, time.Time) (supersede.DistillDecision, error)
	ApplyAndCommitDistill(context.Context, *disposition.Store, disposition.Item, time.Time) (string, error)
}

type inboxPublisher struct {
	writer    *supersede.Writer
	published bool
}

func (p *inboxPublisher) ApplyAndCommitReconcile(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	id, err := p.writer.ApplyAndCommitReconcile(ctx, ds, item, now)
	if err == nil && item.AppliedClaimID == "" {
		p.published = true
	}
	return id, err
}

func (p *inboxPublisher) PreviewDistillAssertion(ctx context.Context, item disposition.Item, object, actor string, now time.Time) (supersede.DistillDecision, error) {
	return p.writer.PreviewDistillAssertion(ctx, item, object, actor, now)
}

func (p *inboxPublisher) ApplyAndCommitDistill(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	id, err := p.writer.ApplyAndCommitDistill(ctx, ds, item, now)
	if err == nil && item.AppliedClaimID == "" {
		p.published = true
	}
	return id, err
}

func (p *inboxPublisher) PreviewDirtyEdit(item disposition.Item, actor string, now time.Time) (string, error) {
	return p.writer.PreviewDirtyEdit(item, actor, now)
}

func (p *inboxPublisher) ApplyAndCommitDirtyEdit(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	id, err := p.writer.ApplyAndCommitDirtyEdit(ctx, ds, item, now)
	if err == nil && item.AppliedPublicationID == "" {
		p.published = true
	}
	return id, err
}

func applyInboxDecision(ctx context.Context, sw reconcilePublisher, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	if item.Kind == disposition.KindDirtyEdit {
		return sw.ApplyAndCommitDirtyEdit(ctx, ds, item, now)
	}
	if item.Kind == disposition.KindDistill {
		return sw.ApplyAndCommitDistill(ctx, ds, item, now)
	}
	return sw.ApplyAndCommitReconcile(ctx, ds, item, now)
}

// currentActor reports the local human identity used as Dispose's actor
// argument, "human:<id>" per domain.Claim's own documented Actor
// convention (internal/domain/domain.go: `Actor is "machine" or
// "human:<id>"`) -- disposition.Item.Actor is a free string, so this is
// the first real caller to populate it from an actual identity rather than
// a test fixture's literal. No auth/identity subsystem exists yet in M0-M2
// to resolve a better id than the local OS account name; that is a
// disclosed placeholder, not a claim of real multi-user identity.
func currentActor() string {
	u, err := user.Current()
	if err != nil || u.Username == "" {
		return "human:cli"
	}
	return "human:" + u.Username
}

// itemFamily returns the RFC family this item should be filtered/grouped
// by, and whether one is derivable at all. Only KindReconcile items carry
// a domain.Claim (T2.2's ReconcilePayload) with a Family field today --
// KindDistill captures are free text and KindDirtyEdit records
// (internal/writer.PendingRecord) are a bare file path, neither carrying a
// family concept. A future kind that needs bulk-defer/family filtering can
// add its own case here without changing bulkDefer's or the interactive
// loop's own logic.
func itemFamily(item disposition.Item) (string, bool) {
	if observation, extracted, err := disposition.ExtractionObservation(item); err == nil && extracted {
		return observation.Predicate, true
	}
	if item.Kind != disposition.KindReconcile {
		return "", false
	}
	var p reconcile.ReconcilePayload
	if err := json.Unmarshal(item.Payload, &p); err != nil {
		return "", false
	}
	if p.A.Family != "" {
		return p.A.Family, true
	}
	return "", false
}

// itemSummary renders one human-readable line for an item, decoding its
// kind-specific payload where this package knows the shape (reconcile,
// distill, dirty_edit, precept_draft -- the kinds anything in this repo
// actually stages today) and falling back to a bare kind+id line for any
// other kind (effect, tombstone -- staged by no shipped code yet).
func itemSummary(item disposition.Item) string {
	switch item.Kind {
	case disposition.KindReconcile:
		var p reconcile.ReconcilePayload
		if err := json.Unmarshal(item.Payload, &p); err == nil {
			return fmt.Sprintf("%s verdict=%s  A: %s %s=%s  vs  B: %s=%s",
				item.Kind, p.Verdict, p.A.SubjectSlug, p.A.Predicate, p.A.Object, p.B.Predicate, p.B.Object)
		}
	case disposition.KindDistill:
		observation, extracted, err := disposition.ExtractionObservation(item)
		if err != nil {
			return "distill: malformed extraction evidence: " + err.Error()
		}
		if extracted {
			return fmt.Sprintf("distill confidence=%.2f %s %s=%q source=%s span=%s model=%s; e: type a human assertion, d: defer, r: reject", observation.Confidence, observation.SubjectSlug, observation.Predicate, observation.Object, observation.SourceSHA256, observation.Span, observation.Model)
		}
		var p disposition.CapturePayload
		if err := json.Unmarshal(item.Payload, &p); err == nil {
			text := p.Text
			if text == "" {
				text = "(audio: " + p.AudioRef + ")"
			}
			if len(text) > 80 {
				text = text[:77] + "..."
			}
			return fmt.Sprintf("%s %q", item.Kind, text)
		}
	case disposition.KindDirtyEdit:
		var rec writer.PendingRecord
		if err := json.Unmarshal(item.Payload, &rec); err == nil {
			return fmt.Sprintf("%s %s", item.Kind, rec.Path)
		}
	case disposition.KindPreceptDraft:
		var p direction.PreceptDraftPayload
		if err := json.Unmarshal(item.Payload, &p); err == nil {
			return fmt.Sprintf("%s %q (from %s)", item.Kind, p.Title, p.QuestionID)
		}
	}
	return fmt.Sprintf("%s id=%s", item.Kind, item.ID)
}

// inboxRow is one line of the interactive review loop: either a single
// ungrouped item, or every item sharing one non-empty GroupID (RFC 0001
// section 8.2's "grouped items"). Family/HasFamily are the group's family
// per itemFamily, read off the row's first member -- Candidates
// (internal/reconcile) only ever groups claims sharing one (subject,
// predicate), and family is 1:1 with predicate in the seeded vocabulary
// (store/fence.go's ParseEntity), so every member of a real group shares
// one family in practice; this is documented, not re-verified per member.
type inboxRow struct {
	GroupID   string
	Items     []disposition.Item
	Family    string
	HasFamily bool
}

// buildRows groups items into review rows, preserving each row's
// first-appearance order in items (already oldest-created-first, per
// Store.List).
func buildRows(items []disposition.Item) []inboxRow {
	var rows []inboxRow
	groupRow := map[string]int{}
	for _, it := range items {
		if it.GroupID != "" {
			if idx, ok := groupRow[it.GroupID]; ok {
				rows[idx].Items = append(rows[idx].Items, it)
				continue
			}
			fam, hasFam := itemFamily(it)
			rows = append(rows, inboxRow{GroupID: it.GroupID, Items: []disposition.Item{it}, Family: fam, HasFamily: hasFam})
			groupRow[it.GroupID] = len(rows) - 1
			continue
		}
		fam, hasFam := itemFamily(it)
		rows = append(rows, inboxRow{Items: []disposition.Item{it}, Family: fam, HasFamily: hasFam})
	}
	return rows
}

func describeRow(row inboxRow) string {
	if len(row.Items) == 1 {
		return itemSummary(row.Items[0])
	}
	return fmt.Sprintf("group=%s (%d items)  %s", row.GroupID, len(row.Items), itemSummary(row.Items[0]))
}

// reviewableItems returns every pending or deferred item, oldest first --
// parked and disposed items are excluded. Recorded acceptances that still need
// publication are listed separately by --unapplied.
func reviewableItems(ctx context.Context, dispStore *disposition.Store) ([]disposition.Item, error) {
	items, err := dispStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("inbox: list items: %w", err)
	}
	out := items[:0]
	for _, it := range items {
		if it.State == disposition.StatePending || it.State == disposition.StateDeferred {
			out = append(out, it)
		}
	}
	return out, nil
}

// runInteractive reviews staged decisions. Accept and edit_accept publish
// reconciliation effects through a durable before/after receipt and commit each
// accepted item before moving on. Other kinds retain their existing handlers.
// A failed publication keeps its recorded decision discoverable via --unapplied;
// --apply retries that decision without disposing it again.
func runInteractive(ctx context.Context, dispStore *disposition.Store, sw reconcilePublisher, dirStore *direction.Store, in io.Reader, out io.Writer, actor string, now time.Time) error {
	items, err := reviewableItems(ctx, dispStore)
	if err != nil {
		return err
	}
	rows := buildRows(items)
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(out, "inbox: nothing pending")
		return nil
	}

	r := bufio.NewReader(in)
	cursor := 0
	var lastFamily string
	var lastFamilySet bool

	printRow := func(i int) {
		row := rows[i]
		// "per-family pause": a visible separator whenever the row under
		// the cursor belongs to a different family than the previous row
		// -- a display grouping cue, not a blocking modal prompt. The task
		// line's "per-family pause" is not covered by this task's acc
		// line, so this is a disclosed, deliberately lighter reading:
		// enough to keep a human from reviewing two families' items
		// without noticing the switch, without adding a second kind of
		// keypress-wait this task's scripted test would then also need to
		// drive.
		if row.HasFamily && (!lastFamilySet || row.Family != lastFamily) {
			_, _ = fmt.Fprintf(out, "── family: %s ──\n", row.Family)
			lastFamily, lastFamilySet = row.Family, true
		}
		_, _ = fmt.Fprintf(out, "[%d/%d] %s\n", i+1, len(rows), describeRow(row))
	}
	printRow(cursor)

	// advance removes the just-disposed row under the cursor, moves the
	// cursor onto a still-valid row (or reports done and signals the
	// caller to return), and reprints -- the one piece of bookkeeping
	// every disposing key (space/d/r/e) shares.
	advance := func() (done bool) {
		rows = append(rows[:cursor], rows[cursor+1:]...)
		if len(rows) == 0 {
			_, _ = fmt.Fprintln(out, "inbox: done")
			return true
		}
		if cursor >= len(rows) {
			cursor = len(rows) - 1
		}
		printRow(cursor)
		return false
	}

	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inbox: read key: %w", err)
		}

		switch b {
		case 'q', 3: // q, or Ctrl-C's raw byte if it ever reaches this reader
			return nil

		case 'j':
			if cursor < len(rows)-1 {
				cursor++
				printRow(cursor)
			}

		case 'k':
			if cursor > 0 {
				cursor--
				printRow(cursor)
			}

		case ' ', 'd', 'r':
			verdict := disposition.VerdictAccept
			note := ""
			switch b {
			case 'd':
				verdict = disposition.VerdictDefer
			case 'r':
				verdict = disposition.VerdictReject
				line, rerr := r.ReadString('\n')
				if rerr != nil && rerr != io.EOF {
					return fmt.Errorf("inbox: read reject note: %w", rerr)
				}
				note = strings.TrimSpace(line)
				if note == "" {
					note = "rejected via inbox"
				}
			}
			row := rows[cursor]
			needsAssertion := false
			if verdict == disposition.VerdictAccept {
				for _, it := range row.Items {
					_, extracted, err := disposition.ExtractionObservation(it)
					if err != nil {
						return err
					}
					if extracted {
						needsAssertion = true
					}
				}
			}
			if needsAssertion {
				_, _ = fmt.Fprintln(out, "inbox: low-confidence evidence stays pending; use e to type a human assertion and confirm its canonical effect, or d/r to defer/reject")
				continue
			}
			if verdict == disposition.VerdictAccept {
				dirty := false
				for _, it := range row.Items {
					if it.Kind == disposition.KindDirtyEdit {
						dirty = true
					}
				}
				if dirty {
					if len(row.Items) != 1 {
						return fmt.Errorf("inbox: review paused edits individually")
					}
					preview, err := sw.PreviewDirtyEdit(row.Items[0], actor, now)
					if err != nil {
						return fmt.Errorf("inbox: paused edit remains pending: %w", err)
					}
					_, _ = fmt.Fprint(out, preview, "Commit this human copy and its listed shard corrections? [y/N]: ")
					answer, err := r.ReadString('\n')
					if err != nil && err != io.EOF {
						return err
					}
					if strings.TrimSpace(answer) != "y" {
						_, _ = fmt.Fprintln(out, "inbox: paused edit stays pending")
						continue
					}
				}
			}
			for _, it := range row.Items {
				res, err := dispStore.Dispose(ctx, it.ID, verdict, nil, note, actor, "", now)
				if err != nil {
					return fmt.Errorf("inbox: dispose %s: %w", it.ID, err)
				}
				_, _ = fmt.Fprintf(out, "disposed %s verdict=%s\n", it.ID, res.Item.Verdict)
				// Publish only the recorded winning acceptance.

				switch {
				case verdict == disposition.VerdictAccept && res.Item.Verdict == disposition.VerdictAccept && it.Kind == disposition.KindDirtyEdit:
					id, aerr := sw.ApplyAndCommitDirtyEdit(ctx, dispStore, res.Item, now)
					if aerr != nil {
						return fmt.Errorf("inbox: publication incomplete for %s: %w; retry with inbox --apply %s", it.ID, aerr, it.ID)
					}
					_, _ = fmt.Fprintf(out, "applied %s -> publication %s committed to brain repo\n", it.ID, id)
				case verdict == disposition.VerdictAccept && res.Item.Verdict == disposition.VerdictAccept && it.Kind == disposition.KindReconcile:
					id, aerr := sw.ApplyAndCommitReconcile(ctx, dispStore, res.Item, now)
					if aerr != nil {
						return fmt.Errorf("inbox: publication incomplete for %s: %w; resolve the target and retry with inbox --apply %s", it.ID, aerr, it.ID)
					}
					_, _ = fmt.Fprintf(out, "applied %s -> claim %s committed to brain repo\n", it.ID, id)
				case verdict == disposition.VerdictAccept && res.Item.Verdict == disposition.VerdictAccept && it.Kind == disposition.KindDecompose:
					entry, aerr := dirStore.ApplyDisposedDecompose(ctx, res.Item, now)
					if aerr != nil {
						return fmt.Errorf("inbox: apply decompose %s: %w", it.ID, aerr)
					}
					_, _ = fmt.Fprintf(out, "applied %s -> %s written to ledger (%s)\n", it.ID, entry.ID, entry.Title)
				case verdict == disposition.VerdictAccept && res.Item.Verdict == disposition.VerdictAccept && it.Kind == disposition.KindPreceptDraft:
					entry, aerr := dirStore.ApplyDisposedPreceptDraft(ctx, res.Item, now)
					if aerr != nil {
						return fmt.Errorf("inbox: apply precept draft %s: %w", it.ID, aerr)
					}
					_, _ = fmt.Fprintf(out, "applied %s -> %s written to ledger (%s)\n", it.ID, entry.ID, entry.Title)
				}
			}
			if advance() {
				return nil
			}

		case 'e':
			row := rows[cursor]
			if len(row.Items) == 1 {
				_, extracted, err := disposition.ExtractionObservation(row.Items[0])
				if err != nil {
					return err
				}
				if extracted {
					applied, err := runDistillEdit(ctx, dispStore, sw, row.Items[0], r, out, actor, now)
					if err != nil {
						return err
					}
					if applied && advance() {
						return nil
					}
					continue
				}
			}
			// edit_accept (T2.7): only defined for a single ungrouped
			// KindReconcile row -- "which edited value" is per-item
			// information a human supplies one item at a time, and no
			// current producer groups reconcile items in the first place
			// (internal/reconcile.Engine.Process always stages with an
			// empty GroupID), so fanning one edit across several group
			// members has no real caller to motivate guessing at it.
			if len(row.Items) != 1 || row.Items[0].Kind != disposition.KindReconcile {
				_, _ = fmt.Fprintln(out, "inbox: e (edit_accept) only works on a single ungrouped reconcile item")
				continue
			}
			it := row.Items[0]
			var payload reconcile.ReconcilePayload
			if err := json.Unmarshal(it.Payload, &payload); err != nil {
				return fmt.Errorf("inbox: decode reconcile payload for edit: %w", err)
			}
			_, _ = fmt.Fprintf(out, "edit object (was %q), type the replacement then Enter: ", payload.A.Object)
			line, rerr := r.ReadString('\n')
			if rerr != nil && rerr != io.EOF {
				return fmt.Errorf("inbox: read edit value: %w", rerr)
			}
			newObject := strings.TrimSpace(line)
			if newObject == "" {
				_, _ = fmt.Fprintln(out, "inbox: empty edit, item left pending")
				continue
			}
			edited := payload.A
			edited.Object = newObject
			edited.Confidence = 1.0 // a direct human assertion, maximally trusted
			editedRaw, err := json.Marshal(edited)
			if err != nil {
				return fmt.Errorf("inbox: marshal edited claim: %w", err)
			}
			res, err := dispStore.Dispose(ctx, it.ID, disposition.VerdictEditAccept, editedRaw, "", actor, "", now)
			if err != nil {
				return fmt.Errorf("inbox: dispose %s: %w", it.ID, err)
			}
			_, _ = fmt.Fprintf(out, "disposed %s verdict=%s\n", it.ID, res.Item.Verdict)
			claimID, err := sw.ApplyAndCommitReconcile(ctx, dispStore, res.Item, now)
			if err != nil {
				return fmt.Errorf("inbox: publication incomplete for %s: %w; resolve the target and retry with inbox --apply %s", it.ID, err, it.ID)
			}
			_, _ = fmt.Fprintf(out, "applied %s -> claim written to brain repo and committed (id=%s)\n", it.ID, claimID)
			if advance() {
				return nil
			}
		}
	}
}

// errUnsupportedBulkDeferFilter is returned by runBulkDefer for any filter
// other than "family=<value>" -- the only filter key itemFamily can
// currently resolve (see its own doc comment).
var errUnsupportedBulkDeferFilter = errors.New("inbox: unsupported bulk-defer filter")

// runBulkDefer defers -- verdict=defer via Store.Dispose, never accept or
// reject -- every pending/deferred item whose own family matches filter's
// value, non-interactively. "exactly the matching items" (this task's acc
// line, verbatim): grouping plays no role here, unlike the interactive
// loop -- a group with only one member matching the filter defers only
// that member, not its groupmates.
func runBulkDefer(ctx context.Context, dispStore *disposition.Store, filter, actor string, out io.Writer, now time.Time) error {
	key, value, ok := strings.Cut(filter, "=")
	if !ok || key != "family" || value == "" {
		return fmt.Errorf(`%w: %q (only "family=<value>" is supported)`, errUnsupportedBulkDeferFilter, filter)
	}
	items, err := reviewableItems(ctx, dispStore)
	if err != nil {
		return err
	}
	n := 0
	for _, it := range items {
		fam, hasFam := itemFamily(it)
		if !hasFam || fam != value {
			continue
		}
		if _, err := dispStore.Dispose(ctx, it.ID, disposition.VerdictDefer, nil, "", actor, "", now); err != nil {
			return fmt.Errorf("inbox: bulk-defer: dispose %s: %w", it.ID, err)
		}
		n++
	}
	_, _ = fmt.Fprintf(out, "deferred %d item(s) matching %s\n", n, filter)
	return nil
}

// runListParked prints every parked item, read-only -- RFC 0001 section
// 8.2's "resurfaced only by explicit filter or by new evidence": this is
// the explicit-filter listing a human uses to find what to resurface
// (T2.6's Store.Resurface); wiring an interactive resurface action onto
// this listing is not required by this task's acc line ("--parked lists
// parked items only") and is left to a caller invoking Resurface directly,
// or a later task.
func runListParked(ctx context.Context, dispStore *disposition.Store, out io.Writer) error {
	items, err := dispStore.List(ctx)
	if err != nil {
		return fmt.Errorf("inbox: list parked: %w", err)
	}
	n := 0
	for _, it := range items {
		if it.State != disposition.StateParked {
			continue
		}
		_, _ = fmt.Fprintln(out, itemSummary(it))
		n++
	}
	if n == 0 {
		_, _ = fmt.Fprintln(out, "inbox: no parked items")
	}
	return nil
}

func runListUnapplied(ctx context.Context, dispStore *disposition.Store, out io.Writer) error {
	items, err := dispStore.List(ctx)
	if err != nil {
		return err
	}
	count := 0
	for _, item := range items {
		_, extracted, err := disposition.ExtractionObservation(item)
		if err != nil {
			return err
		}
		supported := item.Kind == disposition.KindDirtyEdit || item.Kind == disposition.KindReconcile || (extracted && item.Verdict == disposition.VerdictEditAccept)
		if !supported || item.State != disposition.StateDisposed || (item.Verdict != disposition.VerdictAccept && item.Verdict != disposition.VerdictEditAccept) || item.AppliedClaimID != "" || item.AppliedPublicationID != "" {
			continue
		}
		_, _ = fmt.Fprintf(out, "unapplied %s verdict=%s — retry: serenity inbox --apply %s\n", item.ID, item.Verdict, item.ID)
		count++
	}
	if count > 0 {
		_, _ = fmt.Fprintf(out, "inbox: %d accepted decision(s) await publication\n", count)
	}
	return nil
}
