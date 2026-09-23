// Package website embeds the public website in the hosted release, so the
// landing page and account UI are deployed and rolled back together.
package website

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
)

//go:embed *.html assets brand chat docs get-started product content.json llms.txt robots.txt sitemap.xml
var files embed.FS

// Handler serves only public files. Directory listings and Go sources are never exposed.
func Handler() http.Handler {
	server := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean("/" + r.URL.Path)[1:]
		if name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(files, name)
		if err == nil && info.IsDir() {
			_, err = fs.Stat(files, path.Join(name, "index.html"))
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		server.ServeHTTP(w, r)
	})
}
