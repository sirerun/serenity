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
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Review the DISPOSITION queue: j/k navigate, space/d/r dispose, grouped items dispose together",
		Long: "inbox is the human-facing review surface for the DISPOSITION queue (RFC\n" +
			"0001 section 8.2). With no flags it drives an interactive j/k/space\n" +
			"loop over every pending or deferred item, oldest first, grouped by\n" +
			"GroupID so a group's members are reviewed and disposed together.\n" +
			"--bulk-defer and --parked are non-interactive one-shot modes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInbox(cmd.Context(), flagRoot, cmd.InOrStdin(), cmd.OutOrStdout(),
				inboxOptions{BulkDefer: bulkDefer, Parked: parked}, time.Now())
		},
	}
	cmd.Flags().StringVar(&bulkDefer, "bulk-defer", "",
		`non-interactively defer every pending/deferred item matching filter (only "family=<value>" is supported)`)
	cmd.Flags().BoolVar(&parked, "parked", false, "list parked items only -- read-only, no interactive review")
	return cmd
}

type inboxOptions struct {
	BulkDefer string
	Parked    bool
}

// runInbox opens root's derived index (creating it if needed, same as
// runCapture/runStatus) and dispatches to one of the three modes. now is
// injected -- the same single-snapshot convention runCapture/runStatus and
// internal/reconcile/decay.go's Sweep already use -- rather than read live
// per dispose call: a review session's disposals sharing one timestamp is
// an accepted, existing-convention simplification (nothing downstream
// depends on sub-session dispose-time precision -- the expiry sweeper
// T2.6 built operates on day-granularity thresholds), not a new gap this
// task introduces.
func runInbox(ctx context.Context, root string, in io.Reader, out io.Writer, opts inboxOptions, now time.Time) error {
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

	switch {
	case opts.BulkDefer != "":
		return runBulkDefer(ctx, dispStore, opts.BulkDefer, currentActor(), out, now)
	case opts.Parked:
		return runListParked(ctx, dispStore, out)
	default:
		q := writer.NewQueue(nil)
		defer q.Close()
		sw := supersede.New(q, store.NewFenceWriter(root), store.NewShardStore(root), cfg)
		// dirStore shares q with sw: a Queue is subsystem-agnostic (any
		// Path+Render job), so a decompose accept's .dira write and an
		// edit_accept's brain-repo write land in the same session-ending
		// Flush below, exactly like the "one commit per review session"
		// comment already promises for edit_accept alone.
		dirStore := direction.NewStore(root, q)
		if err := runInteractive(ctx, dispStore, sw, dirStore, in, out, currentActor(), now); err != nil {
			return err
		}
		// One commit per review session (RFC 0001 section 7.7's Flush --
		// "commits every path the queue has written since the last
		// Flush"), not one per write-through: batching every write-through
		// this session made (edit_accept, T2.7; a decompose accept, T3.11)
		// into a single commit is Flush's own intended shape, the same one
		// internal/writer's own tests exercise.
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
// distill, dirty_edit -- the three kinds anything in this repo actually
// stages today) and falling back to a bare kind+id line for any other
// kind (precept_draft, effect, tombstone -- staged by no shipped code yet).
func itemSummary(item disposition.Item) string {
	switch item.Kind {
	case disposition.KindReconcile:
		var p reconcile.ReconcilePayload
		if err := json.Unmarshal(item.Payload, &p); err == nil {
			return fmt.Sprintf("%s verdict=%s  A: %s %s=%s  vs  B: %s=%s",
				item.Kind, p.Verdict, p.A.SubjectSlug, p.A.Predicate, p.A.Object, p.B.Predicate, p.B.Object)
		}
	case disposition.KindDistill:
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
// parked and disposed items are excluded (parked review is --parked's own
// read-only mode; disposed items are terminal, nothing left to review).
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

// runInteractive drives the j/k/space(/d/r/e) review loop, reading one raw
// byte at a time from in so a scripted test can supply exactly the bytes
// it wants to exercise with no line buffering in between (see newInboxCmd's
// doc comment for the real-terminal raw-mode disclosure). Unrecognized
// bytes (a stray newline from a hand-written test script, an unmapped
// key) are silently ignored rather than erroring.
//
// space/d/r dispose the whole current row (accept/defer/reject) without
// ever touching the brain repo, for every Kind except KindDecompose --
// exactly T2.5's original behavior, unchanged for KindReconcile,
// KindEntityMerge, KindTombstone, KindDirtyEdit, KindCompact, and every
// other kind. e (edit_accept, T2.7) is the one key that writes through sw
// to the canonical brain repo for a KindReconcile row (internal/supersede's
// tier-dispatching Apply, T2.3), because unlike a plain accept an edited
// value has no earlier write anywhere to fall back on. Wiring space's own
// accept to also write through sw for those other kinds is a disclosed,
// deliberately separate gap this package still does not close:
// internal/supersede's Apply has had no real caller at all until T2.7, and
// folding that wiring in silently would go beyond each task's own acc
// line and this session's established practice of disclosing rather than
// silently absorbing adjacent gaps.
//
// KindDecompose (T3.11) is the one deliberate exception: a plain accept
// on a decompose child ALSO writes through dirStore to .dira
// (direction.Store.ApplyDisposedDecompose) -- not e, plain space. This is
// not the same gap as the others, and closing it here is not scope creep:
// a decompose proposal has no meaningful "accepted but not yet written"
// state the way a reconcile item does (space there stops short of a write
// on purpose, since a human may still want to edit first via e). A
// decompose child is already the complete proposed content -- title plus
// rationale -- with nothing left to edit, so accepting it and writing it
// are the same human decision; deferring the write to some other,
// not-yet-built keystroke would leave "confirming writes valid dira
// entries with the edge" (T3.11's own acc line) permanently unmet by
// design, not merely unwired yet.
func runInteractive(ctx context.Context, dispStore *disposition.Store, sw *supersede.Writer, dirStore *direction.Store, in io.Reader, out io.Writer, actor string, now time.Time) error {
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
			for _, it := range row.Items {
				res, err := dispStore.Dispose(ctx, it.ID, verdict, nil, note, actor, "", now)
				if err != nil {
					return fmt.Errorf("inbox: dispose %s: %w", it.ID, err)
				}
				_, _ = fmt.Fprintf(out, "disposed %s verdict=%s\n", it.ID, res.Item.Verdict)
				// KindDecompose (T3.11): a plain accept also writes
				// through -- see runInteractive's own doc comment for why
				// this Kind is the deliberate exception to "space never
				// touches the brain repo". defer/reject never reach here.
				if verdict == disposition.VerdictAccept && it.Kind == disposition.KindDecompose {
					entry, aerr := dirStore.ApplyDisposedDecompose(ctx, res.Item, now)
					if aerr != nil {
						return fmt.Errorf("inbox: apply decompose %s: %w", it.ID, aerr)
					}
					_, _ = fmt.Fprintf(out, "applied %s -> %s written to ledger (%s)\n", it.ID, entry.ID, entry.Title)
				}
			}
			if advance() {
				return nil
			}

		case 'e':
			// edit_accept (T2.7): only defined for a single ungrouped
			// KindReconcile row -- "which edited value" is per-item
			// information a human supplies one item at a time, and no
			// current producer groups reconcile items in the first place
			// (internal/reconcile.Engine.Process always stages with an
			// empty GroupID), so fanning one edit across several group
			// members has no real caller to motivate guessing at it.
			row := rows[cursor]
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
			applyRes, err := sw.ApplyDisposedReconcile(ctx, dispStore, res.Item, now)
			if err != nil {
				return fmt.Errorf("inbox: apply %s: %w", it.ID, err)
			}
			_, _ = fmt.Fprintf(out, "applied %s -> claim written to brain repo (tier=%s)\n", it.ID, applyRes.Tier)
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
