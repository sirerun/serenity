package deletion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// NewWriterID creates a process-unique opaque identifier for journal entries.
func NewWriterID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("hosted/deletion: create writer identity: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

// NewAWSJournal creates the production journal using the default AWS
// credential chain and the explicitly configured bucket, region, and
// generation. It never falls back to a local or empty journal.
func NewAWSJournal(ctx context.Context, bucket, region, writerID string, generation int64) (*Journal, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(region) == "" || generation < 1 {
		return nil, errors.New("hosted/deletion: production journal requires bucket, region, and positive generation")
	}
	if writerID == "" {
		var err error
		writerID, err = NewWriterID()
		if err != nil {
			return nil, err
		}
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("hosted/deletion: load AWS configuration: %w", err)
	}
	store, err := NewS3ObjectStore(s3.NewFromConfig(cfg), bucket)
	if err != nil {
		return nil, err
	}
	return NewJournal(store, writerID, generation, time.Now), nil
}

// NewFilesystemJournal creates the explicit local filesystem fake. Production
// service configuration must use NewAWSJournal instead.
func NewFilesystemJournal(root, writerID string, generation int64, now func() time.Time) (*Journal, error) {
	if generation < 1 {
		return nil, errors.New("hosted/deletion: journal generation must be positive")
	}
	if writerID == "" {
		var err error
		writerID, err = NewWriterID()
		if err != nil {
			return nil, err
		}
	}
	store, err := NewFileObjectStore(root)
	if err != nil {
		return nil, err
	}
	return NewJournal(store, writerID, generation, now), nil
}

var _ contracts.DeletionJournal = (*Journal)(nil)
