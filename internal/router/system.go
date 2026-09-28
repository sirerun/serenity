package router

import (
	"context"
	"crypto/rand"
	"strings"
)

// SystemSender is implemented by a Provider whose API has a system role
// (AnthropicProvider, OpenAICompatibleProvider). Complete hands such a
// provider Prompt.System as the system role and Prompt.Text as the only
// user message, so instructions never share a channel with the untrusted
// document they govern (T24.25, AI-L04).
type SystemSender interface {
	SendSystem(ctx context.Context, system, prompt string) (Response, error)
}

// sendPrompt issues one provider call for p. A provider with no system
// role (an embeddings adapter, a test double) receives the instructions
// ahead of the document in its single prompt rather than losing them.
func sendPrompt(ctx context.Context, provider Provider, p Prompt) (Response, error) {
	if p.System == "" {
		return provider.Send(ctx, p.Text)
	}
	if ss, ok := provider.(SystemSender); ok {
		return ss.SendSystem(ctx, p.System, p.Text)
	}
	return provider.Send(ctx, p.System+"\n\n"+p.Text)
}

// nonceLen and nonceAlphabet give a fence nonce about 122 bits of
// entropy. Letters only: a digit run inside the nonce could otherwise be
// rewritten by internal/redact's card-number pass on the way out.
const (
	nonceLen      = 26
	nonceAlphabet = "abcdefghijklmnopqrstuvwxyz"
)

// DocumentFence delimits untrusted documents inside one model call. Its
// nonce is drawn fresh from crypto/rand for every fence, so a document
// cannot know, and therefore cannot forge, the closing fence of the call
// it lands in (T24.25, AI-L04). Build one per call with NewDocumentFence
// and name its fences in the system instructions.
type DocumentFence struct {
	nonce string
}

// NewDocumentFence returns a fence whose nonce occurs in none of docs.
func NewDocumentFence(docs ...string) DocumentFence {
	return newDocumentFence(randomNonce, docs...)
}

func newDocumentFence(next func() string, docs ...string) DocumentFence {
	for {
		n := next()
		clash := false
		for _, d := range docs {
			if strings.Contains(d, n) {
				clash = true
				break
			}
		}
		if !clash {
			return DocumentFence{nonce: n}
		}
	}
}

// randomNonce draws nonceLen letters uniformly, rejecting bytes that
// would bias the modulo. crypto/rand.Read never returns an error.
func randomNonce() string {
	const limit = 256 - 256%len(nonceAlphabet)
	out := make([]byte, 0, nonceLen)
	var buf [nonceLen * 2]byte
	for len(out) < nonceLen {
		_, _ = rand.Read(buf[:])
		for _, b := range buf {
			if int(b) >= limit {
				continue
			}
			out = append(out, nonceAlphabet[int(b)%len(nonceAlphabet)])
			if len(out) == nonceLen {
				break
			}
		}
	}
	return string(out)
}

// Open is the opening fence, <<<doc-<nonce>>>.
func (f DocumentFence) Open() string { return "<<<doc-" + f.nonce + ">>>" }

// Close is the closing fence, <<</doc-<nonce>>>.
func (f DocumentFence) Close() string { return "<<</doc-" + f.nonce + ">>>" }

// Wrap encloses doc between the opening and closing fences.
func (f DocumentFence) Wrap(doc string) string {
	return f.Open() + "\n" + doc + "\n" + f.Close()
}
