package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var errOutboxStopped = errors.New("mail outbox stopped")

type mailJob struct {
	Mail     Mail      `json:"mail"`
	Attempts int       `json:"attempts"`
	Next     time.Time `json:"next"`
}

type outbox struct {
	dir  string
	send func(Mail) error

	mu   sync.Mutex
	wake chan struct{}
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func openOutbox(dataDir string, send func(Mail) error) (*outbox, error) {
	dir := filepath.Join(dataDir, "mail")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if tmps, _ := filepath.Glob(filepath.Join(dir, ".tmp-*")); tmps != nil {
		for _, f := range tmps {
			os.Remove(f)
		}
	}
	q := &outbox{dir: dir, send: send, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
	if files, _ := filepath.Glob(filepath.Join(dir, "mail-*.json")); len(files) > 0 {
		log.Printf("mail outbox: resuming %d queued mails", len(files))
	}
	go q.run()
	return q, nil
}

func (q *outbox) enqueue(m Mail) error {
	select {
	case <-q.stop:
		return errOutboxStopped
	default:
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	job := mailJob{Mail: m}
	b, err := json.Marshal(job)
	if err != nil {
		return err
	}
	var name string
	for {
		name = filepath.Join(q.dir, "mail-"+newID()+".json")
		if _, err := os.Stat(name); errors.Is(err, fs.ErrNotExist) {
			break
		} else if err != nil {
			return err
		}
	}
	if err := writeOutboxFile(q.dir, name, b); err != nil {
		return err
	}
	log.Printf("mail outbox: queued %s to=%q subject=%q", filepath.Base(name), m.To, m.Subject)
	q.signal()
	return nil
}

func writeOutboxFile(dir, path string, b []byte) error {
	f, err := os.CreateTemp(dir, ".tmp-")
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
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncOutboxDir(dir)
}

func (q *outbox) pending() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	files, _ := filepath.Glob(filepath.Join(q.dir, "mail-*.json"))
	return len(files) > 0
}

func (q *outbox) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *outbox) next(now time.Time) (string, mailJob, time.Duration, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(q.dir, "mail-*.json"))
	if err != nil {
		return "", mailJob{}, 0, err
	}
	var soonest time.Time
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", mailJob{}, 0, err
		}
		var job mailJob
		if err := json.Unmarshal(b, &job); err != nil || !validEmail(job.Mail.To) {
			log.Printf("mail outbox: invalid job %s", filepath.Base(f))
			os.Rename(f, f+".invalid")
			continue
		}
		if job.Next.IsZero() || !job.Next.After(now) {
			return f, job, 0, nil
		}
		if soonest.IsZero() || job.Next.Before(soonest) {
			soonest = job.Next
		}
	}
	if soonest.IsZero() {
		return "", mailJob{}, time.Hour, nil
	}
	return "", mailJob{}, time.Until(soonest), nil
}

func (q *outbox) finish(path string, job mailJob, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err == nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
			log.Printf("mail outbox: remove %s: %v", filepath.Base(path), removeErr)
			return
		}
		if err := syncOutboxDir(q.dir); err != nil {
			log.Printf("mail outbox: sync: %v", err)
		}
		log.Printf("mail outbox: sent %s to=%q subject=%q", filepath.Base(path), job.Mail.To, job.Mail.Subject)
		return
	}
	job.Attempts++
	job.Next = time.Now().Add(retryDelay(job.Attempts))
	b, marshalErr := json.Marshal(job)
	if marshalErr != nil {
		log.Printf("mail outbox: encode %s: %v", filepath.Base(path), marshalErr)
		return
	}
	if err := writeOutboxFile(q.dir, path, b); err != nil {
		log.Printf("mail outbox: save retry %s to=%q subject=%q: %v", filepath.Base(path), job.Mail.To, job.Mail.Subject, err)
		return
	}
	log.Printf("mail outbox: retry %s to=%q subject=%q in %s: %v", filepath.Base(path), job.Mail.To, job.Mail.Subject, retryDelay(job.Attempts), err)
}

func retryDelay(attempts int) time.Duration {
	return time.Minute * time.Duration(1<<min(max(attempts, 1)-1, 6))
}

func (q *outbox) run() {
	defer close(q.done)
	for {
		select {
		case <-q.stop:
			return
		default:
		}
		path, job, wait, err := q.next(time.Now())
		if err != nil {
			log.Printf("mail outbox: %v", err)
			wait = time.Minute
		} else if path != "" {
			q.finish(path, job, q.send(job.Mail))
			continue
		}
		timer := time.NewTimer(max(wait, time.Millisecond))
		select {
		case <-q.stop:
			timer.Stop()
			return
		case <-q.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func syncOutboxDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func (q *outbox) shutdown(ctx context.Context) error {
	q.once.Do(func() { close(q.stop) })
	select {
	case <-q.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("mail outbox: %w", ctx.Err())
	}
}
