//go:build !episode_archive_disabled

package gateway

// The normal binary still requires explicit synthetic-only runtime enrollment.
const episodeArchiveBuildEnabled = true
