// Command fixtureprep prepares and independently verifies an offline, local fixture for the hosted load harness (T23.60).
//
// It builds synthetic accounts, brains, credentials, entitlements and memories in a new directory using the real hosted
// store, provisioner, credential issuer, writer queue and pool open path, with an infrastructure-only hash embedder in
// place of a provider. It has no network code, reads no environment variable and contacts no billing or provider system.
// The fixture is preparation and an inventory. It is not a cold-open, load, steady-state or storage-saturation result.
//
//	fixtureprep plan    -profile full|smoke
//	fixtureprep prepare -local-fixture-only -out /new/absolute/dir -profile full|smoke
//	fixtureprep verify  -dir /prepared/dir -expect full|smoke
//
// Exit status: 0 success, 1 verification found a difference, 2 refusal or usage error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
)

const defaultWorkload = "evals/hosted-load/workload.json"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: fixtureprep plan|prepare|verify [flags]")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	switch args[0] {
	case "plan":
		return runPlan(args[1:], stdout, stderr)
	case "prepare":
		return runPrepare(ctx, args[1:], stdout, stderr)
	case "verify":
		return runVerify(ctx, args[1:], stdout, stderr)
	}
	_, _ = fmt.Fprintf(stderr, "unknown command %q: want plan, prepare or verify\n", args[0])
	return 2
}

func newFlags(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func printJSON(w io.Writer, v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	_, _ = fmt.Fprintf(w, "%s\n", data)
}

func fail(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "fixtureprep: %v\n", err)
	return 2
}

func runPlan(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("plan", stderr)
	workload := fs.String("workload", defaultWorkload, "frozen workload.json")
	profile := fs.String("profile", ProfileFull, "full or smoke")
	smoke := fs.Int("smoke-facts", DefaultSmokeFactsPerBrain, "smoke profile: facts per brain")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	wl, err := LoadWorkload(*workload)
	if err != nil {
		return fail(stderr, err)
	}
	p, err := BuildPlan(wl, *profile, *smoke)
	if err != nil {
		return fail(stderr, err)
	}
	printJSON(stdout, map[string]any{"workload_sha256": wl.SHA256, "plan": p, "not_claimed": notClaimed(p)})
	return 0
}

func runPrepare(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlags("prepare", stderr)
	o := Options{Log: stderr}
	fs.StringVar(&o.Out, "out", "", "new absolute directory to create (must not exist)")
	fs.StringVar(&o.Workload, "workload", defaultWorkload, "frozen workload.json")
	fs.StringVar(&o.Profile, "profile", ProfileFull, "full or smoke")
	fs.IntVar(&o.SmokeFacts, "smoke-facts", DefaultSmokeFactsPerBrain, "smoke profile: facts per brain")
	fs.IntVar(&o.Dim, "dim", DefaultDim, "hash embedder vector width")
	fs.IntVar(&o.CommitEvery, "commit-every", 500, "facts per Git commit (production commits after every remember)")
	fs.IntVar(&o.Workers, "workers", 1, "brains prepared concurrently, 1..4")
	fs.IntVar(&o.EntitlementDays, "entitlement-days", 30, "days the synthetic paid entitlement period lasts")
	fs.StringVar(&o.Endpoint, "endpoint", "", "optional loopback origin recorded in the marker as the only allowed target; never contacted")
	fs.BoolVar(&o.LocalFixture, "local-fixture-only", false, "required: confirms this creates synthetic data in a new local directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	m, err := Prepare(ctx, o)
	if err != nil {
		return fail(stderr, err)
	}
	printJSON(stdout, map[string]any{"prepared": true, "profile": m.Plan.Profile, "reduced": m.Plan.Reduced, "accounts": m.Plan.TotalAccounts, "brains": m.Plan.TotalBrains, "facts": m.Plan.TotalFacts, "timing": m.Timing, "next": "run fixtureprep verify; the manifest is not evidence"})
	return 0
}

func runVerify(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlags("verify", stderr)
	o := VerifyOptions{}
	report := fs.String("report", "", "write the full report here instead of stdout")
	fs.StringVar(&o.Dir, "dir", "", "prepared fixture directory")
	fs.StringVar(&o.Workload, "workload", defaultWorkload, "frozen workload.json")
	fs.StringVar(&o.Expect, "expect", "", "full or smoke: what the directory is supposed to be (required)")
	fs.IntVar(&o.SmokeFacts, "smoke-facts", 0, "smoke profile: facts per brain expected (0 = read the marker)")
	fs.IntVar(&o.Workers, "workers", 2, "brains verified concurrently")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if o.Dir == "" || o.Expect == "" {
		_, _ = fmt.Fprintln(stderr, "fixtureprep: -dir and -expect are required")
		return 2
	}
	r, err := Verify(ctx, o)
	if err != nil {
		return fail(stderr, err)
	}
	if *report != "" {
		data, _ := json.MarshalIndent(r, "", "  ")
		if err = os.WriteFile(*report, append(data, '\n'), 0o644); err != nil {
			return fail(stderr, err)
		}
		printJSON(stdout, map[string]any{"pass": r.Pass, "satisfies_full_cardinality": r.SatisfiesFullCardinality, "expect": r.Expect, "totals": r.Totals, "report": *report})
	} else {
		printJSON(stdout, r)
	}
	if !r.Pass {
		return 1
	}
	return 0
}
