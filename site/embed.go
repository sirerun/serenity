// Package website embeds the public website in the hosted release, so the
// landing page and account UI are deployed and rolled back together.
package website

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

//go:embed *.html assets brand chat docs get-started product pricing content.json llms.txt robots.txt sitemap.xml
var files embed.FS

// Handler serves only public files. Directory listings and Go sources are never exposed.
func Handler() http.Handler {
	server := http.FileServer(http.FS(files))
	policy := contentSecurityPolicy()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", policy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		name := path.Clean("/" + r.URL.Path)[1:]
		if name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(files, name)
		if err == nil && info.IsDir() {
			name = path.Join(name, "index.html")
			_, err = fs.Stat(files, name)
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data, err := files.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		tag := fmt.Sprintf("\"%x\"", sha256.Sum256(data))
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		server.ServeHTTP(w, r)
	})
}

// Hash only scripts shipped in the immutable website, never request content.
// Inline styles are needed by the existing landing design; scripts stay hashed.
func contentSecurityPolicy() string {
	script := regexp.MustCompile(`(?s)<script\b[^>]*>(.*?)</script>`)
	sources := []string{"'self'"}
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".html") {
			return nil
		}
		data, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		for _, match := range script.FindAllSubmatch(data, -1) {
			if len(match[1]) == 0 {
				continue
			}
			digest := sha256.Sum256(match[1])
			sources = append(sources, "'sha256-"+base64.StdEncoding.EncodeToString(digest[:])+"'")
		}
		return nil
	})
	if err != nil {
		panic("invalid embedded website: " + err.Error())
	}
	return "default-src 'none'; script-src " + strings.Join(sources, " ") +
		"; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data: https://d2ol7oe51mr4n9.cloudfront.net; media-src https://d8j0ntlcm91z4.cloudfront.net; connect-src 'self' https://rxhf2uaca425xd4li7vdki53lm0hedux.lambda-url.us-west-2.on.aws; form-action 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'"
}
