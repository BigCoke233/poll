package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOutboxRetriesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	attempted := make(chan struct{}, 1)
	q, err := openOutbox(dir, func(Mail) error {
		attempted <- struct{}{}
		return errors.New("smtp unavailable")
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.enqueue(Mail{To: "a@example.com", Subject: "test", Text: "test"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-attempted:
	case <-time.After(time.Second):
		t.Fatal("outbox did not attempt delivery")
	}

	var path string
	var job mailJob
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(dir, "mail", "mail-*.json"))
		if len(files) == 1 {
			b, _ := os.ReadFile(files[0])
			if json.Unmarshal(b, &job) == nil && job.Attempts == 1 {
				path = files[0]
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	if path == "" {
		t.Fatal("failed mail was not persisted for retry")
	}
	if err := q.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	job.Next = time.Time{}
	b, _ := json.Marshal(job)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	delivered := make(chan Mail, 1)
	q, err = openOutbox(dir, func(m Mail) error {
		delivered <- m
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer q.shutdown(context.Background())
	select {
	case m := <-delivered:
		if m.To != "a@example.com" {
			t.Fatalf("delivered to %q", m.To)
		}
	case <-time.After(time.Second):
		t.Fatal("outbox did not retry after restart")
	}
	deadline = time.Now().Add(time.Second)
	for q.pending() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if q.pending() {
		t.Fatal("delivered mail remained in the outbox")
	}
}
