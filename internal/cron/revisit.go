package cron

import (
	"context"
	"fmt"

	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/providers"
)

// Revisit runs the weekly review sweep with immutable precept inputs.
func Revisit(ctx context.Context, root string, clock Clock) error {
	_, err := RunRevisit(ctx, root, clock)
	return err
}

// RunRevisit exposes delivery and unsupported-prose counts to CLI clients.
func RunRevisit(ctx context.Context, root string, clock Clock) (direction.RevisitResult, error) {
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return direction.RevisitResult{}, fmt.Errorf("cron revisit: %w", err)
	}
	defer func() { _ = eng.Close() }()
	now := clock.Now()
	result, err := direction.SweepRevisit(ctx, direction.NewStore(root, nil), eng, now)
	if err != nil {
		return result, err
	}
	return result, recordRun(root, "revisit", now)
}
