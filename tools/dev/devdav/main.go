// devdav: minimaler WebDAV-Server zum Testen des Sync-Assistenten.
//
//	go run ./tools/dev/devdav            # http://127.0.0.1:5005, Benutzer dominik / nas-pw
//	go run ./tools/dev/devdav -tls       # https mit selbstsigniertem Zertifikat (wie ein NAS)
//
// Nur PROPFIND/MKCOL/GET/PUT/DELETE mit ETag, If-Match und If-None-Match.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

type dav struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
	user  string
	pass  string
}

func etag(b []byte) string { return fmt.Sprintf(`"%x"`, sha256.Sum256(b)) }

func (d *dav) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)
	if u, p, ok := r.BasicAuth(); !ok || u != d.user || p != d.pass {
		w.Header().Set("WWW-Authenticate", `Basic realm="devdav"`)
		w.WriteHeader(401)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	p, _ := url.PathUnescape(r.URL.Path)
	parent := p[:strings.LastIndex(strings.TrimSuffix(p, "/"), "/")+1]
	switch r.Method {
	case "PROPFIND":
		if d.dirs[p] || d.files[p] != nil {
			w.WriteHeader(207)
			return
		}
		w.WriteHeader(404)
	case "MKCOL":
		if !d.dirs[parent] {
			w.WriteHeader(409)
			return
		}
		d.dirs[p] = true
		w.WriteHeader(201)
	case "GET":
		b, ok := d.files[p]
		if !ok {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("ETag", etag(b))
		w.Write(b)
	case "PUT":
		cur, exists := d.files[p]
		if im := r.Header.Get("If-Match"); im != "" && (!exists || im != etag(cur)) {
			w.WriteHeader(412)
			return
		}
		if r.Header.Get("If-None-Match") == "*" && exists {
			w.WriteHeader(412)
			return
		}
		b, _ := io.ReadAll(r.Body)
		d.files[p] = b
		w.WriteHeader(201)
	case "DELETE":
		delete(d.files, p)
		w.WriteHeader(204)
	default:
		w.WriteHeader(405)
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:5005", "Adresse")
	useTLS := flag.Bool("tls", false, "https mit selbstsigniertem Zertifikat")
	user := flag.String("user", "dominik", "Benutzer")
	pass := flag.String("pass", "nas-pw", "Passwort")
	flag.Parse()
	d := &dav{files: map[string][]byte{}, dirs: map[string]bool{"/": true, "/home/": true}, user: *user, pass: *pass}
	if *useTLS {
		s := httptest.NewUnstartedServer(d)
		ln, err := net.Listen("tcp", *addr)
		if err != nil {
			log.Fatal(err)
		}
		s.Listener = ln
		s.StartTLS()
		log.Printf("WebDAV (https, selbstsigniert) auf %s", s.URL)
		select {}
	}
	log.Printf("WebDAV (http) auf http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, d))
}
