package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
)

func runDistillEdit(ctx context.Context, ds *disposition.Store, sw reconcilePublisher, item disposition.Item, in *bufio.Reader, out io.Writer, actor string, now time.Time) (bool, error) {
	_, _ = fmt.Fprint(out, "Type the fact you personally assert, then Enter (original evidence remains unchanged): ")
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	object := strings.TrimSpace(line)
	if object == "" {
		_, _ = fmt.Fprintln(out, "inbox: empty assertion; item remains pending")
		return false, nil
	}
	decision, err := sw.PreviewDistillAssertion(ctx, item, object, actor, now)
	if err != nil {
		return false, fmt.Errorf("inbox: assertion not recorded: %w", err)
	}
	_, _ = fmt.Fprintf(out, "Human assertion: %s %s=%q (confidence 1; actor %s).\n", decision.Claim.SubjectSlug, decision.Claim.Predicate, decision.Claim.Object, actor)
	if decision.Prior.ID != "" {
		_, _ = fmt.Fprintf(out, "This will supersede canonical claim %s: %q.\n", decision.Prior.ID, decision.Prior.Object)
	} else {
		_, _ = fmt.Fprintln(out, "This will add a canonical claim.")
	}
	_, _ = fmt.Fprint(out, "Type y and Enter to record this decision and commit its effect: ")
	confirm, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	if strings.TrimSpace(confirm) != "y" {
		_, _ = fmt.Fprintln(out, "inbox: assertion cancelled; item remains pending")
		return false, nil
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		return false, err
	}
	res, err := ds.Dispose(ctx, item.ID, disposition.VerdictEditAccept, raw, "explicit typed human assertion", actor, "", now)
	if err != nil {
		return false, err
	}
	if res.Item.Verdict != disposition.VerdictEditAccept {
		_, _ = fmt.Fprintf(out, "inbox: existing decision %s retained; assertion not applied\n", res.Item.Verdict)
		return false, nil
	}
	id, err := sw.ApplyAndCommitDistill(ctx, ds, res.Item, now)
	if err != nil {
		return false, fmt.Errorf("inbox: publication incomplete for %s: %w; resolve the target and retry with inbox --apply %s", item.ID, err, item.ID)
	}
	_, _ = fmt.Fprintf(out, "applied %s -> human claim %s committed; original machine evidence retained\n", item.ID, id)
	return true, nil
}
