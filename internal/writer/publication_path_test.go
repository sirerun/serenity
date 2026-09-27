package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublicationPathRejectsControlCharacters pins the writer's own
// re-check for SEC-H03: a publication path with a control character in
// any segment (a newline that would split a YAML header, an escape
// sequence, a carriage return, DEL) is refused before any read or write,
// even though such a path is otherwise clean, relative and inside
// brain/entities or brain/claims.
func TestPublicationPathRejectsControlCharacters(t *testing.T) {
	cases := []string{
		"brain/entities/topic/acme\ntype: person.md",
		"brain/entities/topic\nslug: x/acme.md",
		"brain/entities/topic/acme\rcorp.md",
		"brain/entities/topic/acme\x1bcorp.md",
		"brain/entities/topic/acme\x7fcorp.md",
		"brain/entities/topic/acme\tcorp.md",
		"brain/claims/acme\ncorp/works_at.jsonl",
	}
	for _, path := range cases {
		t.Run(strings.NewReplacer("\n", "\\n", "\r", "\\r", "\x1b", "\\x1b", "\x7f", "\\x7f", "\t", "\\t").Replace(path), func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "brain/entities/topic"), 0755); err != nil {
				t.Fatal(err)
			}
			q := NewQueue(nil)
			defer q.Close()
			if err := PublishFiles(q, root, []FileChange{{Path: path, After: []byte("replacement")}}); err == nil {
				t.Fatalf("publication path %q accepted", path)
			}
			var written []string
			err := filepath.WalkDir(filepath.Join(root, "brain"), func(p string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() {
					written = append(written, p)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(written) != 0 {
				t.Fatalf("refused path still produced files: %v", written)
			}
		})
	}
}
