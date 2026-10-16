//go:build episode_archive_disabled

package gateway

// The compatible safety binary replays the new journal subjects at startup but
// cannot serve the episode API, even if a stale runtime config enables it.
const episodeArchiveBuildEnabled = false
