package report

import (
	"io/fs"
	"os"
	"path/filepath"
)

// MeasureRepoSize sums the byte size of every regular file under root's
// canonical brain content -- brain/ (entities, sources, claims) and
// .dira/ (precept entries) -- excluding .git and the derived .serenity
// index, which internal/index.ResetAll already treats as disposable and
// therefore not part of "the repo" this metric is about (RFC section 16:
// "repo growth/month"). A subdirectory that does not exist yet (a fresh
// `serenity init` with no .dira, say) contributes zero rather than
// erroring.
//
// This does no network I/O and performs no writes -- TestReportNoNetwork
// (report_test.go) exercises it directly alongside Build.
func MeasureRepoSize(root string) (int64, error) {
	var total int64
	for _, sub := range []string{"brain", ".dira"} {
		dir := filepath.Join(root, sub)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				total += info.Size()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return 0, err
		}
	}
	return total, nil
}
