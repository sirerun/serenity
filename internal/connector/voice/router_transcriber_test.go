package voice_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/sirerun/serenity/internal/connector/voice"
	"github.com/sirerun/serenity/internal/router"
)

// fakeRouterProvider is a test double implementing router.Provider,
// recording the exact text it was sent so RouterTranscriber's encoding
// convention can be asserted directly. Test-file only, per the zero-stub
// policy.
type fakeRouterProvider struct {
	gotText string
	resp    router.Response
	err     error
}

func (f *fakeRouterProvider) Name() string         { return "fake" }
func (f *fakeRouterProvider) ModelVersion() string { return "fake@v1" }
func (f *fakeRouterProvider) Send(_ context.Context, prompt string) (router.Response, error) {
	f.gotText = prompt
	if f.err != nil {
		return router.Response{}, f.err
	}
	return f.resp, nil
}

type fakeLedger struct{}

func (fakeLedger) Record(_ context.Context, _ router.SpendEntry) error { return nil }

// TestRouterTranscriberRoutesThroughTranscriptionTaskClass proves the
// production Transcriber implementation calls Router.Complete on
// TaskClassTranscription (RFC section 9's local-cheap tier -- "voice
// note (transcription via router)") with the audio base64-encoded into
// the text-only Prompt, and returns the router's response text
// unchanged as the transcript.
func TestRouterTranscriberRoutesThroughTranscriptionTaskClass(t *testing.T) {
	fp := &fakeRouterProvider{resp: router.Response{Text: "hello from the transcriber"}}
	r := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: fp}, fakeLedger{})
	rt := &voice.RouterTranscriber{Router: r}

	audio := []byte("raw-audio-bytes")
	transcript, err := rt.Transcribe(context.Background(), audio)
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "hello from the transcriber" {
		t.Fatalf("transcript = %q, want %q", transcript, "hello from the transcriber")
	}

	wantEncoded := base64.StdEncoding.EncodeToString(audio)
	if fp.gotText != wantEncoded {
		t.Fatalf("provider received %q, want the base64-encoded audio %q", fp.gotText, wantEncoded)
	}
}

func TestRouterTranscriberPropagatesProviderError(t *testing.T) {
	fp := &fakeRouterProvider{err: errors.New("provider unreachable")}
	r := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: fp}, fakeLedger{})
	rt := &voice.RouterTranscriber{Router: r}

	if _, err := rt.Transcribe(context.Background(), []byte("audio")); err == nil {
		t.Fatal("expected Transcribe to propagate a provider failure")
	}
}

// TestRouterTranscriberErrorsWithoutJudgmentProvider proves transcription
// stays on the local-cheap tier: a router configured with only a
// judgment-tier provider (never local-cheap) must fail, not silently
// route to the wrong tier.
func TestRouterTranscriberErrorsWithoutLocalCheapProvider(t *testing.T) {
	fp := &fakeRouterProvider{resp: router.Response{Text: "should never be reached"}}
	r := router.New(map[router.Tier]router.Provider{router.TierJudgment: fp}, fakeLedger{})
	rt := &voice.RouterTranscriber{Router: r}

	if _, err := rt.Transcribe(context.Background(), []byte("audio")); err == nil {
		t.Fatal("expected Transcribe to error when no local-cheap provider is configured")
	}
	if fp.gotText != "" {
		t.Fatal("provider was called despite being registered on the wrong tier")
	}
}
