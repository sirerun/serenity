// Generate the synthetic message corpus used by the import budget benchmark.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/sirerun/serenity/internal/eval/messages"
)

func main() {
	count := flag.Int("n", 10000, "number of synthetic messages")
	seed := flag.Int64("seed", 1, "deterministic corpus seed")
	out := flag.String("out", "evals/generated/messages", "output directory (new or byte-identical)")
	perFile := flag.Int("messages-per-file", 1000, "messages per mbox file")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	manifest, err := messages.Generate(ctx, *out, messages.Options{Count: *count, Seed: *seed, MessagesPerFile: *perFile})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("generated %d synthetic messages in %d mbox files\nmanifest: %s\n", manifest.Count, len(manifest.Files), filepath.Join(*out, "manifest.json"))
}
