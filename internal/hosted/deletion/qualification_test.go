package deletion

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type qualificationReceipt struct {
	Outcome             string    `json:"outcome"`
	StartedAt           time.Time `json:"started_at"`
	FinishedAt          time.Time `json:"finished_at"`
	Region              string    `json:"region"`
	BucketCreated       bool      `json:"bucket_created"`
	Versioned           bool      `json:"versioning_enabled"`
	PolicyEnforced      bool      `json:"conditional_write_and_delete_policy_enforced"`
	ConditionalRace     bool      `json:"atomic_conditional_write_race_passed"`
	JournalRoundTrip    bool      `json:"journal_round_trip_passed"`
	SealVerified        bool      `json:"generation_seal_verified"`
	DeleteMarkerVisible bool      `json:"delete_marker_visible_to_reader"`
	Cleanup             string    `json:"disposable_bucket_cleanup"`
}

// TestS3Qualification is an opt-in live qualification against a new temporary
// bucket. It refuses to run unless the caller opts in and names a receipt path.
func TestS3Qualification(t *testing.T) {
	if os.Getenv("SERENITY_T2348_S3_QUALIFY") != "1" {
		t.Skip("set SERENITY_T2348_S3_QUALIFY=1 to create and clean up a disposable S3 bucket")
	}
	receiptPath := os.Getenv("SERENITY_T2348_EVIDENCE")
	if receiptPath == "" {
		t.Fatal("SERENITY_T2348_EVIDENCE must name the qualification receipt path")
	}
	receipt, err := qualifyDisposableS3(context.Background(), os.Getenv("SERENITY_T2348_AWS_REGION"))
	if receipt != nil {
		data, marshalErr := json.MarshalIndent(receipt, "", "  ")
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if err := os.MkdirAll(filepath.Dir(receiptPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(receiptPath, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}

func qualifyDisposableS3(ctx context.Context, region string) (receipt *qualificationReceipt, resultErr error) {
	if region == "" {
		region = "us-west-2"
	}
	started := time.Now().UTC()
	receipt = &qualificationReceipt{Outcome: "FAIL", StartedAt: started, Region: region}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return receipt, fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	client := s3.NewFromConfig(cfg)
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		return receipt, err
	}
	bucket := "serenity-t2348-qualify-" + hex.EncodeToString(suffix[:])
	bucketARN := "arn:aws:s3:::" + bucket
	journalARN := bucketARN + "/deletion-journal/*"
	receipt.Cleanup = "not_created"
	defer func() {
		if receipt.BucketCreated {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			cleanupErr := cleanupQualificationBucket(cleanupCtx, client, bucket)
			if cleanupErr != nil {
				receipt.Cleanup = "FAILED"
				resultErr = errors.Join(resultErr, fmt.Errorf("cleanup disposable bucket %s: %w", bucket, cleanupErr))
			} else {
				receipt.Cleanup = "deleted"
			}
		}
		receipt.FinishedAt = time.Now().UTC()
		if resultErr == nil {
			receipt.Outcome = "PASS"
		}
	}()

	create := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if region != "us-east-1" {
		create.CreateBucketConfiguration = &types.CreateBucketConfiguration{LocationConstraint: types.BucketLocationConstraint(region)}
	}
	if _, err = client.CreateBucket(ctx, create); err != nil {
		return receipt, fmt.Errorf("create disposable S3 bucket: %w", err)
	}
	receipt.BucketCreated = true
	receipt.Cleanup = "pending"
	if _, err = client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucket),
		PublicAccessBlockConfiguration: &types.PublicAccessBlockConfiguration{
			BlockPublicAcls: aws.Bool(true), IgnorePublicAcls: aws.Bool(true),
			BlockPublicPolicy: aws.Bool(true), RestrictPublicBuckets: aws.Bool(true),
		},
	}); err != nil {
		return receipt, fmt.Errorf("block public access on disposable bucket: %w", err)
	}
	if _, err = client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket:                  aws.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled},
	}); err != nil {
		return receipt, fmt.Errorf("enable disposable bucket versioning: %w", err)
	}
	versioning, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
	if err != nil {
		return receipt, fmt.Errorf("verify bucket versioning: %w", err)
	}
	if versioning.Status != types.BucketVersioningStatusEnabled {
		return receipt, fmt.Errorf("verify bucket versioning: status=%q", versioning.Status)
	}
	receipt.Versioned = true
	if _, err = client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: aws.String(bucket)}); err == nil {
		return receipt, errors.New("new disposable bucket unexpectedly has a lifecycle rule")
	} else {
		var apiErr smithy.APIError
		if !errors.As(err, &apiErr) || apiErr.ErrorCode() != "NoSuchLifecycleConfiguration" {
			return receipt, fmt.Errorf("verify journal prefix has no lifecycle configuration: %w", err)
		}
	}
	if err = putQualificationPolicy(ctx, client, bucket, journalARN); err != nil {
		return receipt, err
	}
	unconditionalKey := "deletion-journal/0000000001/0000000001-unconditional.json"
	_, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(unconditionalKey), Body: strings.NewReader("unconditional")})
	if err == nil {
		return receipt, errors.New("bucket policy accepted an unconditional PutObject")
	}
	deleteKey := "deletion-journal/0000000001/0000000001-delete-denied.json"
	if _, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(deleteKey)}); err == nil {
		return receipt, errors.New("bucket policy accepted DeleteObject on the journal prefix")
	}
	receipt.PolicyEnforced = true

	store, err := NewS3ObjectStore(client, bucket)
	if err != nil {
		return receipt, err
	}
	raceKey := "deletion-journal/qualification-race/slot"
	raceBody := []byte("one atomic object")
	start := make(chan struct{})
	type putResult struct {
		created bool
		err     error
	}
	results := make(chan putResult, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			created, putErr := store.PutIfAbsent(ctx, raceKey, raceBody)
			results <- putResult{created: created, err: putErr}
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	createdCount, occupiedCount := 0, 0
	for res := range results {
		if res.err != nil {
			return receipt, fmt.Errorf("conditional write race: %w", res.err)
		}
		if res.created {
			createdCount++
		} else {
			occupiedCount++
		}
	}
	if createdCount != 1 || occupiedCount != 1 {
		return receipt, fmt.Errorf("conditional write race returned created=%d occupied=%d", createdCount, occupiedCount)
	}
	receipt.ConditionalRace = true

	journal := NewJournal(store, "qualification-writer", 1, time.Now)
	if _, err = journal.AppendDeletion(ctx, contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: "qualification-account", Outcome: contracts.DeletionIntentRequested}); err != nil {
		return receipt, fmt.Errorf("append live S3 journal record: %w", err)
	}
	read, err := journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil || len(read.Entries) != 1 || read.Entries[0].SubjectID != "qualification-account" {
		return receipt, fmt.Errorf("live S3 journal round trip: entries=%d err=%w", len(read.Entries), err)
	}
	receipt.JournalRoundTrip = true
	seal, err := journal.Seal(ctx, 1)
	if err != nil {
		return receipt, fmt.Errorf("seal live S3 generation: %w", err)
	}
	read, err = journal.ReadThrough(ctx, seal)
	if err != nil || !read.Sealed || read.To != seal {
		return receipt, fmt.Errorf("verify live S3 generation seal: %+v err=%w", read, err)
	}
	receipt.SealVerified = true

	markerKey := contracts.JournalKey(1, seal.SequenceID+1)
	if _, err = client.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{Bucket: aws.String(bucket)}); err != nil {
		return receipt, fmt.Errorf("temporarily lift deny policy on disposable bucket for delete-marker check: %w", err)
	}
	if _, err = client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(markerKey), Body: strings.NewReader("marker fixture")}); err != nil {
		return receipt, fmt.Errorf("create delete-marker fixture object: %w", err)
	}
	if _, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(markerKey)}); err != nil {
		return receipt, fmt.Errorf("create delete-marker fixture: %w", err)
	}
	if err = putQualificationPolicy(ctx, client, bucket, journalARN); err != nil {
		return receipt, err
	}
	// ListAfter uses object keys as cursors. The seal watermark's sequence key
	// is the correct cursor.
	keys, _, err := store.ListAfter(ctx, contracts.JournalPrefix(1), contracts.JournalKey(seal.Generation, seal.SequenceID), 10)
	if err != nil {
		return receipt, fmt.Errorf("list delete-marker fixture after seal: %w", err)
	}
	foundMarker := false
	for _, key := range keys {
		foundMarker = foundMarker || key == markerKey
	}
	body, found, getErr := store.Get(ctx, markerKey)
	if !foundMarker || getErr != nil || found || len(body) != 0 {
		return receipt, fmt.Errorf("delete marker visibility: listed=%v found=%v err=%w", foundMarker, found, getErr)
	}
	if _, err = journal.ReadThrough(ctx, contracts.DeletionWatermark{}); !errors.Is(err, contracts.ErrDeletionJournalFenceViolated) {
		return receipt, fmt.Errorf("journal accepted object after seal/delete marker: %w", err)
	}
	receipt.DeleteMarkerVisible = true
	return receipt, nil
}

func putQualificationPolicy(ctx context.Context, client *s3.Client, bucket, journalARN string) error {
	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Sid": "RequireIfNoneMatch", "Effect": "Deny", "Principal": "*", "Action": "s3:PutObject", "Resource": journalARN,
				"Condition": map[string]any{"StringNotEquals": map[string]string{"s3:if-none-match": "*"}},
			},
			map[string]any{
				"Sid": "DenyJournalDeletes", "Effect": "Deny", "Principal": "*", "Action": []string{"s3:DeleteObject", "s3:DeleteObjectVersion"}, "Resource": journalARN,
			},
		},
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	if _, err = client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(bucket), Policy: aws.String(string(data))}); err != nil {
		return fmt.Errorf("install disposable journal safety policy: %w", err)
	}
	return nil
}

func cleanupQualificationBucket(ctx context.Context, client *s3.Client, bucket string) error {
	if _, err := client.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{Bucket: aws.String(bucket)}); err != nil {
		var apiErr smithy.APIError
		if !errors.As(err, &apiErr) || apiErr.ErrorCode() != "NoSuchBucketPolicy" {
			return err
		}
	}
	var keyMarker, versionMarker *string
	for {
		page, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket: aws.String(bucket), KeyMarker: keyMarker, VersionIdMarker: versionMarker, MaxKeys: aws.Int32(1000),
		})
		if err != nil {
			return err
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Versions)+len(page.DeleteMarkers))
		for _, version := range page.Versions {
			objects = append(objects, types.ObjectIdentifier{Key: version.Key, VersionId: version.VersionId})
		}
		for _, marker := range page.DeleteMarkers {
			objects = append(objects, types.ObjectIdentifier{Key: marker.Key, VersionId: marker.VersionId})
		}
		if len(objects) > 0 {
			if _, err = client.DeleteObjects(ctx, &s3.DeleteObjectsInput{Bucket: aws.String(bucket), Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)}}); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		keyMarker, versionMarker = page.NextKeyMarker, page.NextVersionIdMarker
	}
	_, err := client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	return err
}
