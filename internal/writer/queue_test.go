package writer

import (
	"errors"
	"strings"
	"testing"
)

// TestQueueRecoversRenderPanic pins CON-06 (docs/deep-reviews/001-full-codebase.md):
// a panic inside a job's Render must never escape the drain goroutine. The
// review reproduced the defect in a subprocess and watched the process die;
// here the same panic is raised in-process, so the test binary surviving to
// run the assertions below is itself the proof, and the second Submit proves
// the queue kept draining rather than wedging on the recovered job.
func TestQueueRecoversRenderPanic(t *testing.T) {
	var hooked []Result
	q := NewQueue(func(r Result) { hooked = append(hooked, r) })
	defer q.Close()

	boom := errors.New("kaboom")
	res := q.Submit(Job{
		Kind:   "fence",
		Path:   "brain/people/alice.md",
		Render: func() ([]byte, error) { panic(boom) },
	})
	if res.Err == nil {
		t.Fatal("panicking job returned nil error")
	}
	if got, want := res.Err.Error(), "writer: job fence panicked: kaboom"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if res.Bytes != nil {
		t.Fatalf("panicking job returned bytes %q, want nil", res.Bytes)
	}
	var pe *PanicError
	if !errors.As(res.Err, &pe) {
		t.Fatalf("error %T is not *PanicError", res.Err)
	}
	if pe.Kind != "fence" || pe.Value != boom {
		t.Fatalf("PanicError = %+v, want Kind fence and the panic value", pe)
	}
	if !strings.Contains(string(pe.Stack), "TestQueueRecoversRenderPanic") {
		t.Fatalf("stack summary does not name the panicking frame:\n%s", pe.Stack)
	}
	if !errors.Is(res.Err, boom) {
		t.Fatal("PanicError does not unwrap to a panic value that is an error")
	}
	if touched := q.takeTouched(); len(touched) != 0 {
		t.Fatalf("panicking job marked paths touched: %v", touched)
	}

	// The queue must still be draining: a later job on the same path lands
	// with the next sequence number and its bytes intact.
	next := q.Submit(Job{
		Path:   "brain/people/alice.md",
		Render: func() ([]byte, error) { return []byte("ok"), nil },
	})
	if next.Err != nil {
		t.Fatalf("job after the panic failed: %v", next.Err)
	}
	if next.Seq != 2 || string(next.Bytes) != "ok" {
		t.Fatalf("job after the panic = seq %d bytes %q, want seq 2 bytes ok", next.Seq, next.Bytes)
	}
	if touched := q.takeTouched(); len(touched) != 1 || touched[0] != "brain/people/alice.md" {
		t.Fatalf("touched after the successful job = %v", touched)
	}
	if len(hooked) != 2 || hooked[0].Err == nil || hooked[1].Err != nil {
		t.Fatalf("hook saw %d results (%v), want the recovered failure then the success", len(hooked), hooked)
	}
}

// TestQueuePanicErrorKindFallback covers the callers that never set Kind:
// the message names the path when there is one, and otherwise the Render
// function's symbol, so an unlabelled job (memoryfact, publish, import) is
// still locatable from the error alone.
func TestQueuePanicErrorKindFallback(t *testing.T) {
	q := NewQueue(nil)
	defer q.Close()

	byPath := q.Submit(Job{Path: "brain/x.md", Render: func() ([]byte, error) { panic("p") }})
	if got, want := byPath.Err.Error(), "writer: job brain/x.md panicked: p"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}

	byFunc := q.Submit(Job{Render: func() ([]byte, error) { panic(42) }})
	msg := byFunc.Err.Error()
	if !strings.HasPrefix(msg, "writer: job github.com/sirerun/serenity/internal/writer.TestQueuePanicErrorKindFallback") ||
		!strings.HasSuffix(msg, " panicked: 42") {
		t.Fatalf("error = %q, want the Render symbol as kind and the panic value", msg)
	}

	// A nil panic is still a panic (Go 1.21+ reports it as *runtime.PanicNilError).
	nilPanic := q.Submit(Job{Kind: "nil", Render: func() ([]byte, error) { panic(nil) }})
	if nilPanic.Err == nil || !strings.HasPrefix(nilPanic.Err.Error(), "writer: job nil panicked: ") {
		t.Fatalf("panic(nil) error = %v, want a recovered PanicError", nilPanic.Err)
	}
}
