package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func readManifestBytes(ctx context.Context, root *os.Root) (data []byte, resultErr error) {
	if isNilInterface(ctx) {
		return nil, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := openSnapshotRegular(root, manifestFile)
	if err != nil {
		return nil, fmt.Errorf("hosted/backup: open manifest: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			data = nil
			resultErr = errors.Join(resultErr, fmt.Errorf("hosted/backup: close manifest: %w", closeErr))
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("hosted/backup: stat manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 0 {
		return nil, errors.New("hosted/backup: manifest must be a regular file")
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("hosted/backup: manifest exceeds the %d-byte limit", maxManifestBytes)
	}
	data = make([]byte, 0, min(int(info.Size()), maxManifestBytes))
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if len(data) > maxManifestBytes-n {
				return nil, fmt.Errorf("hosted/backup: manifest exceeds the %d-byte limit", maxManifestBytes)
			}
			data = append(data, buf[:n]...)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("hosted/backup: read manifest: %w", readErr)
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func verifyAndCopyContext(ctx context.Context, root *os.Root, ref contracts.ArtifactRef, destPath string) (copied int64, resultErr error) {
	if isNilInterface(ctx) {
		return 0, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if ref.LengthBytes <= 0 || !isSHA256(ref.SHA256) {
		return 0, errors.New("hosted/backup: invalid artifact size or digest")
	}
	src, err := openSnapshotRegular(root, ref.RelativePath)
	if err != nil {
		return 0, fmt.Errorf("open artifact %s: %w", ref.RelativePath, err)
	}
	defer func() {
		if closeErr := src.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close artifact %s: %w", ref.RelativePath, closeErr))
		}
		if resultErr != nil {
			copied = 0
		}
	}()
	info, err := src.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat artifact %s: %w", ref.RelativePath, err)
	}
	if !info.Mode().IsRegular() || info.Size() != ref.LengthBytes {
		return 0, fmt.Errorf("artifact %s is not a regular file of its declared size", ref.RelativePath)
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	dstClosed := false
	defer func() {
		if !dstClosed {
			resultErr = errors.Join(resultErr, dst.Close())
		}
	}()
	copied, err = copyExpectedArtifact(ctx, src, dst, ref.LengthBytes, ref.SHA256)
	if err != nil {
		return 0, fmt.Errorf("artifact %s: %w", ref.RelativePath, err)
	}
	if err = dst.Sync(); err != nil {
		return 0, err
	}
	if err = dst.Close(); err != nil {
		return 0, err
	}
	dstClosed = true
	return copied, nil
}

func copyExpectedArtifact(ctx context.Context, src io.Reader, dst io.Writer, expected int64, digest string) (int64, error) {
	if isNilInterface(ctx) {
		return 0, ErrNilContext
	}
	if expected <= 0 || !isSHA256(digest) {
		return 0, errors.New("invalid expected artifact size or digest")
	}
	remaining := expected
	var copied int64
	h := sha256.New()
	buf := make([]byte, 64*1024)
	for remaining > 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		chunk := int64(len(buf))
		if remaining < chunk {
			chunk = remaining
		}
		n, readErr := src.Read(buf[:int(chunk)])
		if n > 0 {
			written := 0
			for written < n {
				m, writeErr := dst.Write(buf[written:n])
				if m > 0 {
					_, _ = h.Write(buf[written : written+m])
					written += m
					copied += int64(m)
				}
				if writeErr != nil {
					return 0, writeErr
				}
				if m == 0 {
					return 0, io.ErrShortWrite
				}
			}
			remaining -= int64(n)
		}
		if readErr != nil {
			if readErr == io.EOF && remaining == 0 {
				break
			}
			if readErr == io.EOF {
				return 0, errors.New("artifact is shorter than declared")
			}
			return 0, readErr
		}
		if n == 0 {
			return 0, io.ErrNoProgress
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var extra [1]byte
	n, readErr := src.Read(extra[:])
	if n != 0 {
		return 0, errors.New("artifact grew beyond its declared length")
	}
	if readErr != io.EOF {
		if readErr == nil {
			return 0, io.ErrNoProgress
		}
		return 0, readErr
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return 0, errors.New("artifact checksum mismatch")
	}
	return copied, nil
}

func openSnapshotControlDB(ctx context.Context, path string, expectedSchema int) (*sql.DB, error) {
	if isNilInterface(ctx) {
		return nil, ErrNilContext
	}
	if err := validateScratchDBPath(path); err != nil {
		return nil, err
	}
	if expectedSchema < 1 || expectedSchema > store.SchemaVersion {
		return nil, fmt.Errorf("%w: manifest claims schema %d; supported versions are 1 through %d", ErrUnsupportedSchemaVersion, expectedSchema, store.SchemaVersion)
	}
	db, err := sql.Open("sqlite", sqliteReadOnlyURI(path))
	if err != nil {
		return nil, fmt.Errorf("hosted/backup: open control database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	closeOnError := func(cause error) (*sql.DB, error) {
		return nil, errors.Join(cause, db.Close())
	}
	if err = ctx.Err(); err != nil {
		return closeOnError(err)
	}
	var integrity string
	if err = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return closeOnError(fmt.Errorf("hosted/backup: check control database integrity: %w", err))
	}
	if integrity != "ok" {
		return closeOnError(fmt.Errorf("hosted/backup: control database failed integrity check: %s", integrity))
	}
	var version int
	if err = db.QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		return closeOnError(fmt.Errorf("hosted/backup: read control database schema version: %w", err))
	}
	if version != expectedSchema {
		return closeOnError(fmt.Errorf("%w: manifest claims schema %d, artifact is schema %d", ErrSchemaMismatch, expectedSchema, version))
	}
	if err = ctx.Err(); err != nil {
		return closeOnError(err)
	}
	return db, nil
}
