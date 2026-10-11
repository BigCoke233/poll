package main

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

type mgr struct {
	base string
	poll *Poll
}

type manageFunc func(http.ResponseWriter, *http.Request, mgr)

func (s *server) asOwner(h manageFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.store.get(r.PathValue("slug"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		tok := r.PathValue("token")
		if p.OwnerToken == "" || subtle.ConstantTimeCompare([]byte(p.OwnerToken), []byte(tok)) != 1 {
			s.fail(w, r, errNotFound)
			return
		}
		h(w, r, mgr{base: "/p/" + p.Slug + "/m/" + tok, poll: p})
	})
}

func (s *server) manageRoutes(mux *http.ServeMux) {
	for _, rt := range []struct {
		pattern string
		h       manageFunc
	}{
		{"GET {base}", s.managePoll},
		{"POST {base}/edit", s.manageEdit},
		{"GET {base}/close", s.closeConfirm},
		{"POST {base}/close", s.closePoll},
		{"POST {base}/reopen", s.reopenPoll},
		{"POST {base}/remove", s.manageRemove},
		{"GET {base}/delete", s.deleteConfirm},
		{"POST {base}/delete", s.deletePoll},
	} {
		mux.Handle(strings.Replace(rt.pattern, "{base}", "/p/{slug}/m/{token}", 1), s.asOwner(rt.h))
	}
	mux.Handle("GET /p/{slug}/m/{token}/created", s.asOwner(s.created))
}

func (s *server) ownerLink(p *Poll) string {
	return s.cfg.BaseURL + "/p/" + p.Slug + "/m/" + p.OwnerToken
}

func (s *server) mailOwner(p *Poll) error {
	return s.mail(p.OwnerEmail, "manage", T(p.OwnerLang, "mail.manage.subject", p.Title),
		mailData{Lang: p.OwnerLang, Poll: p, Link: s.ownerLink(p), Public: s.cfg.BaseURL + "/p/" + p.Slug}, nil)
}

func (s *server) adminIndex(w http.ResponseWriter, r *http.Request) {
	polls, err := s.store.list()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l := lang(r)
	s.render(w, r, http.StatusOK, "admin", &page{Lang: l, Title: T(l, "admin.title"), Polls: polls})
}

func (s *server) newPage(w http.ResponseWriter, r *http.Request, status int, f url.Values, errKey string) {
	l := lang(r)
	s.render(w, r, status, "new", &page{Lang: l, Title: T(l, "admin.new"), Error: errKey, Form: f, Action: "/new", Button: "new.create",
		Public: true, Challenge: s.pow.issue(), Difficulty: s.cfg.Difficulty})
}

func (s *server) newPoll(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	theme := s.validTheme(q.Get("theme"))
	s.newPage(w, r, http.StatusOK, url.Values{"mode": {"multiple"}, "theme": {theme}}, "")
}

func (s *server) validTheme(name string) string {
	if _, ok := s.themes[name]; ok {
		return name
	}
	return ""
}

func (s *server) pollFromForm(f url.Values) (*Poll, string) {
	p := &Poll{
		Title:          strings.TrimSpace(f.Get("title")),
		Description:    strings.TrimSpace(f.Get("description")),
		Multiple:       f.Get("mode") != "single",
		ResultsVisible: f.Get("results") == "1",
		Options:        parseOptions(f.Get("options"), s.cfg.Loc, s.now()),
		Created:        s.now(),
		Theme:          s.validTheme(f.Get("theme")),
	}
	return p, validatePoll(p)
}

func (s *server) create(p *Poll) error {
	for {
		p.Slug = newID()
		if err := s.store.create(p, "created"); !errors.Is(err, errExists) {
			return err
		}
	}
}

func (s *server) createPoll(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	f := r.PostForm
	p, errKey := s.pollFromForm(f)
	name := strings.TrimSpace(f.Get("name"))
	email := strings.ToLower(strings.TrimSpace(f.Get("email")))
	switch {
	case errKey != "":
	case name == "" || len(name) > 100:
		errKey = "err.name"
	case email != "" && !validEmail(email):
		errKey = "err.email"
	case !s.pow.verify(f.Get("pow"), f.Get("nonce")):
		errKey = "err.pow"
	}
	if errKey != "" {
		s.newPage(w, r, http.StatusBadRequest, f, errKey)
		return
	}
	p.OwnerName, p.OwnerEmail, p.OwnerLang, p.OwnerToken = name, email, lang(r), rand.Text()
	if err := s.create(p); err != nil {
		s.fail(w, r, err)
		return
	}
	s.mailOwner(p)
	http.Redirect(w, r, "/p/"+p.Slug+"/m/"+p.OwnerToken+"/created", http.StatusSeeOther)
}

func (s *server) created(w http.ResponseWriter, r *http.Request, m mgr) {
	text := "created.text"
	if m.poll.OwnerEmail == "" {
		text = "created.text.nomail"
	}
	s.message(w, r, http.StatusOK, "created.title", text, m.base, "created.link")
}

func validatePoll(p *Poll) string {
	switch {
	case p.Title == "" || len(p.Title) > 200:
		return "err.title"
	case len(p.Description) > 5000:
		return "err.description"
	case len(p.Options) == 0 || len(p.Options) > 100:
		return "err.options"
	}
	for _, o := range p.Options {
		if len(o.Text) > 200 {
			return "err.options"
		}
	}
	return ""
}

func (s *server) managePoll(w http.ResponseWriter, r *http.Request, m mgr) {
	p, err := s.store.get(r.PathValue("slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	notice := r.URL.Query().Get("ok")
	if !slices.Contains([]string{"saved", "removed", "closed", "reopened"}, notice) {
		notice = ""
	}
	f := url.Values{"title": {p.Title}, "description": {p.Description}, "options": {p.OptionsText()}, "mode": {"single"}, "theme": {p.Theme}}
	if p.Multiple {
		f.Set("mode", "multiple")
	}
	if p.ResultsVisible {
		f.Set("results", "1")
	}
	s.renderManage(w, r, http.StatusOK, m, p, f, notice, "")
}

func (s *server) renderManage(w http.ResponseWriter, r *http.Request, status int, m mgr, p *Poll, f url.Values, notice, errKey string) {
	s.render(w, r, status, "manage", &page{Title: p.Title, Poll: p, Notice: notice, Error: errKey, Form: f, Base: m.base,
		Action: m.base + "/edit", Button: "admin.save", Results: results(p)})
}

func (s *server) manageEdit(w http.ResponseWriter, r *http.Request, m mgr) {
	if !s.parseForm(w, r) {
		return
	}
	f := r.PostForm
	var errKey string
	p, err := s.store.update(r.PathValue("slug"), "edited", func(p *Poll) error {
		p.Title = strings.TrimSpace(f.Get("title"))
		p.Description = strings.TrimSpace(f.Get("description"))
		if f.Has("options") {
			if len(p.Answers) > 0 || p.Closed {
				return errLocked
			}
			p.Options = parseOptions(f.Get("options"), s.cfg.Loc, s.now())
			p.Multiple = f.Get("mode") != "single"
		}
		p.ResultsVisible = f.Get("results") == "1"
		if f.Has("theme") {
			p.Theme = s.validTheme(f.Get("theme"))
		}
		if errKey = validatePoll(p); errKey != "" {
			return errInvalid
		}
		return nil
	})
	if errKey != "" && autosave(r) {
		http.Error(w, T(lang(r), errKey), http.StatusBadRequest)
		return
	}
	if errKey != "" {

		if p, err = s.store.get(r.PathValue("slug")); err == nil {
			s.renderManage(w, r, http.StatusBadRequest, m, p, f, "", errKey)
			return
		}
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if autosave(r) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, m.base+"?ok=saved", http.StatusSeeOther)
}

func (s *server) deleteConfirm(w http.ResponseWriter, r *http.Request, m mgr) {
	p, err := s.store.get(r.PathValue("slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "confirm", &page{Title: p.Title, Poll: p, Text: "admin.delete.confirm", Detail: p.Title,
		Action: m.base + "/delete", Button: "admin.delete", Link: m.base, LinkText: "remove.cancel"})
}

func (s *server) deletePoll(w http.ResponseWriter, r *http.Request, m mgr) {
	if err := s.store.delete(r.PathValue("slug"), "deleted"); err != nil {
		s.fail(w, r, err)
		return
	}
	s.message(w, r, http.StatusOK, "done.title", "notice.deleted", "", "")
}

func (s *server) closeConfirm(w http.ResponseWriter, r *http.Request, m mgr) {
	p, err := s.store.get(r.PathValue("slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	i, err := strconv.Atoi(r.URL.Query().Get("option"))
	if err != nil || i < 0 || i >= len(p.Options) {
		s.fail(w, r, errInvalid)
		return
	}
	if p.Closed {
		s.fail(w, r, errClosed)
		return
	}
	s.render(w, r, http.StatusOK, "confirm", &page{Title: p.Title, Poll: p, Text: "admin.close.confirm", Detail: p.Options[i].Text,
		Action: m.base + "/close", Form: url.Values{"option": {strconv.Itoa(i)}}, Button: "admin.close.button",
		Link: m.base, LinkText: "remove.cancel"})
}

func (s *server) closePoll(w http.ResponseWriter, r *http.Request, m mgr) {
	if !s.parseForm(w, r) {
		return
	}
	p, err := s.store.update(r.PathValue("slug"), "closed", func(p *Poll) error {
		i, err := strconv.Atoi(r.PostForm.Get("option"))
		if err != nil || i < 0 || i >= len(p.Options) {
			return errInvalid
		}
		if p.Closed {
			return errClosed
		}
		p.Closed, p.Outcome = true, i
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, a := range p.Answers {
		s.mailClosed(p, a)
	}
	http.Redirect(w, r, m.base+"?ok=closed", http.StatusSeeOther)
}

func (s *server) reopenPoll(w http.ResponseWriter, r *http.Request, m mgr) {
	_, err := s.store.update(r.PathValue("slug"), "reopened", func(p *Poll) error {
		p.Closed, p.Outcome = false, 0
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, m.base+"?ok=reopened", http.StatusSeeOther)
}

func (s *server) manageRemove(w http.ResponseWriter, r *http.Request, m mgr) {
	if !s.parseForm(w, r) {
		return
	}
	_, err := s.store.update(r.PathValue("slug"), "answer removed by owner", func(p *Poll) error {
		if p.byToken(r.PostForm.Get("token")) == nil {
			return errNotFound
		}
		p.remove(r.PostForm.Get("token"))
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, m.base+"?ok=removed", http.StatusSeeOther)
}
