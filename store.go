package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	errNotFound = errors.New("not found")
	errExists   = errors.New("already exists")
)

type Option struct {
	Text   string    `json:"text"`
	Start  time.Time `json:"start,omitzero"`
	End    time.Time `json:"end,omitzero"`
	AllDay bool      `json:"allDay,omitempty"`
}

func (o Option) HasDate() bool { return !o.Start.IsZero() }

type Answer struct {
	Token   string    `json:"token"`
	Name    string    `json:"name"`
	Email   string    `json:"email"`
	Lang    string    `json:"lang"`
	Choices []int     `json:"choices,omitempty"`
	Created time.Time `json:"created,omitzero"`
}

func (a *Answer) Chose(i int) bool { return slices.Contains(a.Choices, i) }

type Poll struct {
	ID             string    `json:"-"`
	Title          string    `json:"title"`
	Description    string    `json:"description"`
	Multiple       bool      `json:"multiple"`
	ResultsVisible bool      `json:"resultsVisible"`
	Options        []Option  `json:"options"`
	Answers        []*Answer `json:"answers"`
	Closed         bool      `json:"closed"`
	Outcome        int       `json:"outcome"`
	Created        time.Time `json:"created"`

	OwnerName  string `json:"owner_name,omitempty"`
	OwnerEmail string `json:"owner_email,omitempty"`
	OwnerLang  string `json:"owner_lang,omitempty"`
	OwnerToken string `json:"owner_token,omitempty"`
	Theme      string `json:"theme,omitempty"`
}

func (p *Poll) byToken(token string) *Answer {
	for _, a := range p.Answers {
		if a.Token == token {
			return a
		}
	}
	return nil
}

func (p *Poll) byEmail(email string) *Answer {
	for _, a := range p.Answers {
		if a.Email == email {
			return a
		}
	}
	return nil
}

func (p *Poll) remove(token string) {
	p.Answers = slices.DeleteFunc(p.Answers, func(a *Answer) bool { return a.Token == token })
}

func (p *Poll) OutcomeOption() Option { return p.Options[p.Outcome] }

func (p *Poll) OptionsText() string {
	lines := make([]string, len(p.Options))
	for i, o := range p.Options {
		lines[i] = o.Text
	}
	return strings.Join(lines, "\n")
}

type store struct {
	dir string
	mu  sync.Mutex
}

func openStore(dataDir string) (*store, error) {
	dir := filepath.Join(dataDir, "polls")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	tmps, _ := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	for _, f := range tmps {
		os.Remove(f)
	}
	return &store{dir: dir}, nil
}

func (s *store) path(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *store) read(id string) (*Poll, error) {
	if !validID(id) {
		return nil, errNotFound
	}
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	p := Poll{ID: id}
	return &p, json.Unmarshal(b, &p)
}

func (s *store) write(p *Poll) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), s.path(p.ID)); err != nil {
		return err
	}
	return s.syncDir()
}

func (s *store) get(id string) (*Poll, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

func (s *store) update(id, action string, fn func(*Poll) error) (*Poll, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.read(id)
	if err != nil {
		return nil, err
	}
	if err := fn(p); err != nil {
		return p, err
	}
	return p, s.logged(id, action, s.write(p))
}

func (s *store) logged(id, action string, err error) error {
	if err == nil {
		log.Printf("poll %s: %s", id, action)
	}
	return err
}

func (s *store) create(p *Poll, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stat(s.path(p.ID)); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			return errExists
		}
		return err
	}
	return s.logged(p.ID, action, s.write(p))
}

func (s *store) delete(id, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(id) {
		return errNotFound
	}
	if err := os.Remove(s.path(id)); errors.Is(err, fs.ErrNotExist) {
		return errNotFound
	} else if err != nil {
		return err
	}
	return s.logged(id, action, s.syncDir())
}

func (s *store) syncDir() error {
	d, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (s *store) list() ([]*Poll, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var polls []*Poll
	for _, f := range files {
		p, err := s.read(strings.TrimSuffix(filepath.Base(f), ".json"))
		if err != nil {
			return nil, err
		}
		polls = append(polls, p)
	}
	slices.SortFunc(polls, func(a, b *Poll) int { return b.Created.Compare(a.Created) })
	return polls, nil
}
