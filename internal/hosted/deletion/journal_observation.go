package deletion

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	maxObservedGenerations = int64(10_000)
	maxObservedObjects     = 100_000
	maxObservedObjectBytes = 64 * 1024
	maxObservedTotalBytes  = 64 * 1024 * 1024
	maxWriterIDBytes       = 256
	maxJournalSequence     = int64(9_999_999_999)
	observationPageSize    = 50
)

// ReadOnlyJournal is a read-only view over an object reader and a
// caller-authorized logical position. It does not authenticate that position.
type ReadOnlyJournal struct {
	reader   contracts.JournalObjectReader
	position contracts.JournalPosition
}

// NewJournalReader validates the caller-provided active journal position.
func NewJournalReader(reader contracts.JournalObjectReader, position contracts.JournalPosition) (*ReadOnlyJournal, error) {
	if isNilCapability(reader) {
		return nil, fmt.Errorf("nil journal object reader")
	}
	if err := validateJournalPosition(position); err != nil {
		return nil, err
	}
	return &ReadOnlyJournal{reader: reader, position: position}, nil
}

// isNilCapability catches typed nil pointers hidden behind interfaces.
func isNilCapability(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func validAuthorityID(id string) bool {
	return id != "" && len(id) <= maxWriterIDBytes && strings.TrimSpace(id) == id
}

func validateJournalPosition(p contracts.JournalPosition) error {
	if p.ActiveGeneration < 1 || p.ActiveGeneration > maxObservedGenerations {
		return fmt.Errorf("invalid active journal generation %d", p.ActiveGeneration)
	}
	if p.ActiveGeneration == 1 {
		if !validAuthorityID(p.GenesisIssuanceID) || p.SuccessorAllocationID != "" || !p.PredecessorSeal.IsZero() {
			return fmt.Errorf("invalid genesis journal position")
		}
	} else if !validAuthorityID(p.SuccessorAllocationID) || p.GenesisIssuanceID != "" || !validWatermark(p.PredecessorSeal, p.ActiveGeneration-1) {
		return fmt.Errorf("invalid successor journal position")
	}
	if p.LastObjectInGeneration.IsZero() {
		return nil
	}
	if p.LastObjectInGeneration.Generation != p.ActiveGeneration || p.LastObjectInGeneration.SequenceID < 1 || p.LastObjectInGeneration.SequenceID > maxJournalSequence || !validHash(p.LastObjectInGeneration.EntryHash) {
		return fmt.Errorf("invalid active journal head")
	}
	return nil
}

func validWatermark(w contracts.DeletionWatermark, generation int64) bool {
	return !w.IsZero() && w.Generation == generation && w.SequenceID > 0 && w.SequenceID <= maxJournalSequence && validHash(w.EntryHash)
}

func validHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (r *ReadOnlyJournal) Observe(ctx context.Context) (contracts.JournalObservation, error) {
	if ctx == nil {
		return contracts.JournalObservation{}, fmt.Errorf("nil observation context")
	}
	if err := ctx.Err(); err != nil {
		return contracts.JournalObservation{}, err
	}
	position := r.position
	if err := validateJournalPosition(position); err != nil {
		return contracts.JournalObservation{}, err
	}
	if err := r.validateNamespaceKeys(ctx, position.ActiveGeneration); err != nil {
		return contracts.JournalObservation{}, err
	}
	h := sha256.New()
	result := contracts.JournalObservation{Position: position}
	writerGeneration := make(map[string]int64)
	seenWriter := make(map[string]bool)
	var totalBytes int64
	var objectCount int
	var predecessor contracts.DeletionWatermark
	var activeHead contracts.DeletionWatermark
	var previousIsSeal bool
	result.Read = contracts.DeletionRead{To: contracts.DeletionWatermark{}}
	for generation := int64(1); generation <= position.ActiveGeneration; generation++ {
		if err := ctx.Err(); err != nil {
			return contracts.JournalObservation{}, err
		}
		startAfter := ""
		sequence := int64(0)
		previous := ""
		if generation == 1 {
			if !predecessor.IsZero() {
				return contracts.JournalObservation{}, fmt.Errorf("unexpected genesis predecessor")
			}
		} else {
			if !previousIsSeal {
				return contracts.JournalObservation{}, fmt.Errorf("%w: generation %d has no predecessor seal", contracts.ErrDeletionJournalIncomplete, generation-1)
			}
			if generation == position.ActiveGeneration && predecessor != position.PredecessorSeal {
				return contracts.JournalObservation{}, contracts.ErrDeletionJournalHistoryMismatch
			}
			previous = predecessor.EntryHash
		}
		previousIsSeal = false
		sealed := false
		for {
			if err := ctx.Err(); err != nil {
				return contracts.JournalObservation{}, err
			}
			keys, more, err := r.reader.ListAfter(ctx, contracts.JournalPrefix(generation), startAfter, observationPageSize)
			if err != nil {
				return contracts.JournalObservation{}, err
			}
			if len(keys) > observationPageSize || (more && len(keys) == 0) {
				return contracts.JournalObservation{}, fmt.Errorf("journal reader returned invalid page")
			}
			for _, key := range keys {
				if err := ctx.Err(); err != nil {
					return contracts.JournalObservation{}, err
				}
				if sealed {
					return contracts.JournalObservation{}, contracts.ErrDeletionJournalFenceViolated
				}
				if sequence == int64(^uint64(0)>>1) {
					return contracts.JournalObservation{}, fmt.Errorf("journal sequence overflow")
				}
				sequence++
				expected := contracts.JournalKey(generation, sequence)
				if key != expected || (startAfter != "" && key <= startAfter) {
					return contracts.JournalObservation{}, fmt.Errorf("%w: expected %s, found %s", contracts.ErrDeletionJournalIncomplete, expected, key)
				}
				if objectCount >= maxObservedObjects {
					return contracts.JournalObservation{}, fmt.Errorf("journal observation exceeds object limit")
				}
				body, found, err := r.reader.Get(ctx, key)
				if err != nil {
					return contracts.JournalObservation{}, err
				}
				if err := ctx.Err(); err != nil {
					return contracts.JournalObservation{}, err
				}
				if !found {
					return contracts.JournalObservation{}, fmt.Errorf("%w: %s vanished", contracts.ErrDeletionJournalIncomplete, key)
				}
				if len(body) > maxObservedObjectBytes || totalBytes+int64(len(body)) > maxObservedTotalBytes {
					return contracts.JournalObservation{}, fmt.Errorf("journal observation exceeds byte limit")
				}
				obj, err := decodeJournalObject(body)
				if err != nil {
					return contracts.JournalObservation{}, fmt.Errorf("%w: object %s is malformed: %v", contracts.ErrDeletionJournalIncomplete, key, err)
				}
				if err := validateJournalObject(obj, generation, sequence, previous, true); err != nil {
					return contracts.JournalObservation{}, fmt.Errorf("%w: object %s breaks the chain: %v", contracts.ErrDeletionJournalIncomplete, key, err)
				}
				if !validAuthorityID(obj.WriterID) {
					return contracts.JournalObservation{}, fmt.Errorf("journal writer identity exceeds limit")
				}
				if prior, ok := writerGeneration[obj.WriterID]; ok && prior != generation {
					return contracts.JournalObservation{}, fmt.Errorf("journal writer identity reused across generations")
				}
				writerGeneration[obj.WriterID] = generation
				if !seenWriter[obj.WriterID] {
					seenWriter[obj.WriterID] = true
					result.WriterUses = append(result.WriterUses, contracts.JournalWriterUse{Generation: generation, WriterID: obj.WriterID})
				}
				hash := contracts.HashObject(body)
				if err := ctx.Err(); err != nil {
					return contracts.JournalObservation{}, err
				}
				var length [8]byte
				binary.BigEndian.PutUint64(length[:], uint64(len(body)))
				_, _ = h.Write(length[:])
				_, _ = h.Write(body)
				if err := ctx.Err(); err != nil {
					return contracts.JournalObservation{}, err
				}
				totalBytes += int64(len(body))
				objectCount++
				previous = hash
				previousIsSeal = obj.Kind == contracts.JournalKindSeal
				sealed = previousIsSeal
				watermark := contracts.DeletionWatermark{Generation: generation, SequenceID: sequence, EntryHash: hash}
				result.Read.To = watermark
				result.Read.Sealed = sealed
				if obj.Kind == contracts.JournalKindEntry {
					result.Read.Entries = append(result.Read.Entries, contracts.DeletionEntry{Watermark: watermark, SubjectType: obj.SubjectType, SubjectID: obj.SubjectID, Outcome: obj.Outcome, RecordedAt: obj.RecordedAt})
				}
				if generation == position.ActiveGeneration {
					activeHead = watermark
				}
				startAfter = key
			}
			if !more {
				break
			}
		}
		if generation < position.ActiveGeneration && !previousIsSeal {
			return contracts.JournalObservation{}, fmt.Errorf("%w: generation %d is not sealed", contracts.ErrDeletionJournalIncomplete, generation)
		}
		if generation == position.ActiveGeneration && previousIsSeal {
			return contracts.JournalObservation{}, contracts.ErrDeletionJournalSealed
		}
		if sequence > 0 {
			predecessor = contracts.DeletionWatermark{Generation: generation, SequenceID: sequence, EntryHash: previous}
		}
	}
	if position.ActiveGeneration > 1 {
		if predecessor != position.PredecessorSeal && position.LastObjectInGeneration.IsZero() {
			return contracts.JournalObservation{}, contracts.ErrDeletionJournalHistoryMismatch
		}
		if position.LastObjectInGeneration.IsZero() {
			result.Read.To = position.PredecessorSeal
			result.Read.Sealed = true
		}
	}
	if activeHead != position.LastObjectInGeneration {
		return contracts.JournalObservation{}, contracts.ErrDeletionJournalHistoryMismatch
	}
	// Refuse any visible object beyond the authority-selected active generation.
	var after string
	if !position.LastObjectInGeneration.IsZero() {
		after = contracts.JournalKey(position.ActiveGeneration, position.LastObjectInGeneration.SequenceID)
	} else if position.ActiveGeneration == 1 {
		after = ""
	} else {
		after = contracts.JournalKey(position.PredecessorSeal.Generation, position.PredecessorSeal.SequenceID)
	}
	if err := ctx.Err(); err != nil {
		return contracts.JournalObservation{}, err
	}
	later, more, err := r.reader.ListAfter(ctx, "deletion-journal/", after, 1)
	if err != nil {
		return contracts.JournalObservation{}, err
	}
	if len(later) > 1 || more {
		return contracts.JournalObservation{}, fmt.Errorf("journal reader returned invalid tail page")
	}
	if len(later) != 0 {
		return contracts.JournalObservation{}, fmt.Errorf("%w: object exists after reserved journal position", contracts.ErrDeletionJournalHistoryMismatch)
	}
	if err := ctx.Err(); err != nil {
		return contracts.JournalObservation{}, err
	}
	result.HistorySHA256 = hex.EncodeToString(h.Sum(nil))
	return result, nil
}

func (r *ReadOnlyJournal) validateNamespaceKeys(ctx context.Context, activeGeneration int64) error {
	startAfter := ""
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		keys, more, err := r.reader.ListAfter(ctx, "deletion-journal/", startAfter, observationPageSize)
		if err != nil {
			return err
		}
		if len(keys) > observationPageSize || (more && len(keys) == 0) {
			return fmt.Errorf("journal reader returned invalid namespace page")
		}
		for _, key := range keys {
			if err := ctx.Err(); err != nil {
				return err
			}
			if startAfter != "" && key <= startAfter {
				return fmt.Errorf("journal reader returned non-progressing namespace page")
			}
			generation, _, ok := parseJournalKey(key)
			if !ok {
				return fmt.Errorf("%w: malformed journal key %q", contracts.ErrDeletionJournalIncomplete, key)
			}
			if generation > activeGeneration {
				return contracts.ErrDeletionJournalHistoryMismatch
			}
			count++
			if count > maxObservedObjects {
				return fmt.Errorf("journal observation exceeds object limit")
			}
			startAfter = key
		}
		if !more {
			return nil
		}
	}
}

func parseJournalKey(key string) (int64, int64, bool) {
	const prefix = "deletion-journal/"
	if !strings.HasPrefix(key, prefix) {
		return 0, 0, false
	}
	remaining := strings.TrimPrefix(key, prefix)
	if len(remaining) != 10+1+10+len(".json") || remaining[10] != '/' || !strings.HasSuffix(remaining, ".json") {
		return 0, 0, false
	}
	generation, err := strconv.ParseInt(remaining[:10], 10, 64)
	if err != nil || generation < 1 || contracts.JournalPrefix(generation) != prefix+remaining[:11] {
		return 0, 0, false
	}
	sequenceText := remaining[11 : 11+10]
	sequence, err := strconv.ParseInt(sequenceText, 10, 64)
	if err != nil || sequence < 1 || contracts.JournalKey(generation, sequence) != key {
		return 0, 0, false
	}
	return generation, sequence, true
}
