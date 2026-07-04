// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier

// Package web is the local web UI: stdlib net/http + html/template + embedded
// assets, htmx (vendored, pinned) for interactivity. Zero third-party Go deps.
// Like every front-end it only talks to the shared CRUD API; per the design no
// records render until a table is asked for. Binding to 127.0.0.1 is the
// access control - this is a single-user local UI, not a service.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/jim-collier/nano-git-db/internal/core/config"
	"github.com/jim-collier/nano-git-db/internal/core/crud"
	"github.com/jim-collier/nano-git-db/internal/core/schema"
	"github.com/jim-collier/nano-git-db/script"
)

// resolveArgs turns the front-end args into the open triple: an explicit
// <ddl> <sqlite> <logdir>, else a lone DDL in the current directory. The web UI
// has no interactive picker, so with neither it errors and asks for the paths.
func resolveArgs(args []string) (ddlPath, sqlitePath, logDir string, err error) {
	if len(args) >= 3 {
		return args[0], args[1], args[2], nil
	}
	if d, s, l, ok := config.PWDTriple(); ok {
		return d, s, l, nil
	}
	return "", "", "", fmt.Errorf("usage: nanogitdb --serve <ddl> <sqlite> <logdir> (or run from a directory containing a .ddl)")
}

//go:embed assets
var assetsFS embed.FS

//go:embed templates
var tplFS embed.FS

// Run serves the web UI: nanogitdb --serve <ddl> <sqlite> <logdir>. With no
// paths it uses a DDL in the current directory; the web front-end has no
// picker, so with neither it requires the explicit triple.
func Run(args []string) error {
	ddlPath, sqlitePath, logDir, err := resolveArgs(args)
	if err != nil {
		return err
	}
	keyFile, pref := config.ResolveEncryption(ddlPath, nil)
	c, err := schema.OpenClientWith(schema.OpenOpts{
		DDLPath: ddlPath, DBPath: sqlitePath, LogDir: logDir, KeyFile: keyFile, EncryptPref: pref,
	})
	if err != nil {
		return err
	}
	defer c.Close()
	for _, w := range c.Warnings {
		fmt.Println("warning:", w)
	}
	bs, err := schema.Builtins()
	if err != nil {
		return err
	}
	tw, err := script.Attach(c.API, ddlPath, logDir, c.Schema, bs)
	if err != nil {
		return err
	}
	for _, w := range tw {
		fmt.Println("warning:", w)
	}
	cat, err := schema.NewCatalog(c.API, c.Schema, bs)
	if err != nil {
		return err
	}
	cat.Queries = c.Queries
	for _, w := range cat.Warnings {
		fmt.Println("warning:", w)
	}
	s, err := newServer(c.API, cat)
	if err != nil {
		return err
	}

	stop := c.StartAutoSync(c.Schema.TunableInt("git_sync_frequency", 60),
		func(w string) { fmt.Println("warning:", w) })
	defer stop()

	addr := "127.0.0.1:8765" // local UI - do not bind to all interfaces
	// Explicit timeouts: the stdlib defaults are infinite, so a stalled client
	// could pin connections open forever.
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	fmt.Printf("nanogitdb web UI: http://%s\n", addr)
	return srv.ListenAndServe()
}

type server struct {
	api *crud.API
	cat *schema.Catalog
	tpl *template.Template
}

func newServer(api *crud.API, cat *schema.Catalog) (*server, error) {
	tpl := template.New("").Funcs(template.FuncMap{
		"short": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
	})
	tpl, err := tpl.ParseFS(tplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &server{api: api, cat: cat, tpl: tpl}, nil
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(assetsFS, "assets")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /v/{view}", s.viewPage)
	mux.HandleFunc("GET /v/{view}/b/{i}/rows", s.viewBlockRows)
	mux.HandleFunc("GET /v/{view}/q", s.viewQuery)
	mux.HandleFunc("GET /t/{table}", s.rows)
	mux.HandleFunc("GET /t/{table}/new", s.form)
	mux.HandleFunc("GET /t/{table}/{id}/edit", s.form)
	mux.HandleFunc("POST /t/{table}", s.create)
	mux.HandleFunc("POST /t/{table}/{id}", s.update)
	mux.HandleFunc("POST /t/{table}/{id}/delete", s.del)
	mux.HandleFunc("POST /t/{table}/{id}/comment", s.comment)
	mux.HandleFunc("POST /t/{table}/{id}/attach", s.attach)
	return mux
}

// table pulls and validates the table path segment; "" means already handled.
func (s *server) table(w http.ResponseWriter, r *http.Request) string {
	t := r.PathValue("table")
	if !s.cat.Has(t) {
		http.Error(w, "no such table", http.StatusNotFound)
		return ""
	}
	return t
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	if err := s.tpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	s.render(w, "layout.html", map[string]any{
		"Tables": s.cat.Tables, "Views": s.cat.Views, "DefaultView": s.cat.DefaultView,
	})
}

// rows renders the grid partial - also the response to every write, so the
// user always lands back on the refreshed table.
func (s *server) rows(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	rows, err := s.cat.LiveRows(s.api, t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "rows.html", map[string]any{
		"Table": t, "Cols": s.cat.ColumnsFor(t), "Rows": rows,
	})
}

func (s *server) form(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	id := r.PathValue("id")
	row := map[string]string{}
	if id != "" {
		var ok bool
		var err error
		row, ok, err = s.api.Get(t, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "no such row", http.StatusNotFound)
			return
		}
	}
	data := map[string]any{
		"Table": t, "ID": id, "Fields": s.cat.Fields[t], "Row": row,
	}
	// the row's opt-in extras only exist once the row does
	if id != "" {
		feats := s.cat.Features[t]
		data["CanComment"] = feats.Comments
		data["CanAttachURI"] = feats.URIAttachments
		data["CanAttachFile"] = feats.LocalAttachments
		if feats.Comments {
			cs, err := s.api.CommentsFor(t, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data["Comments"] = cs
		}
		if feats.URIAttachments || feats.LocalAttachments {
			atts, err := s.api.AttachmentsFor(t, id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data["Attachments"] = atts
		}
	}
	s.render(w, "form.html", data)
}

// comment adds a comment and lands back on the refreshed form.
func (s *server) comment(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if text := r.PostForm.Get("comment"); text != "" {
		if _, err := s.api.CommentAdd(t, r.PathValue("id"), text); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.form(w, r)
}

// attach adds a URI (kind=uri) or copied-file (kind=file, server-local path -
// this UI is 127.0.0.1-only, the browser runs on the same machine) attachment.
func (s *server) attach(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target, desc := r.PostForm.Get("target"), r.PostForm.Get("description")
	if target != "" {
		var err error
		if r.PostForm.Get("kind") == "file" {
			_, err = s.api.AttachFile(t, r.PathValue("id"), target, desc)
		} else {
			_, err = s.api.AttachURI(t, r.PathValue("id"), target, desc)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.form(w, r)
}

// formFields reads only the table's declared fields from the request - posted
// extras (or system columns) never reach the CRUD layer.
func (s *server) formFields(r *http.Request, t string) map[string]string {
	vals := map[string]string{}
	for _, f := range s.cat.Fields[t] {
		if _, ok := r.PostForm[f]; ok {
			vals[f] = r.PostForm.Get(f)
		}
	}
	return vals
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.api.Create(t, s.formFields(r, t)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.rows(w, r)
}

// update writes only the changed fields, so the tx-log stays field-granular.
func (s *server) update(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	cur, ok, err := s.api.Get(t, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "no such row", http.StatusNotFound)
		return
	}
	changed := map[string]string{}
	for f, v := range s.formFields(r, t) {
		if cur[f] != v {
			changed[f] = v
		}
	}
	if len(changed) > 0 {
		if err := s.api.Update(t, id, changed); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.rows(w, r)
}

func (s *server) del(w http.ResponseWriter, r *http.Request) {
	t := s.table(w, r)
	if t == "" {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	var err error
	if r.PostForm.Get("hard") == "1" {
		err = s.api.Delete(t, id)
	} else {
		err = s.api.MarkDelete(t, id)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.rows(w, r)
}
