package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"time"
)

const powTTL = 2 * time.Hour

type pow struct {
	key        []byte
	difficulty int
	now        func() time.Time
	mu         sync.Mutex
	used       map[string]time.Time
}

func newPow(difficulty int, now func() time.Time) *pow {
	key := make([]byte, 32)
	rand.Read(key)
	return &pow{key: key, difficulty: difficulty, now: now, used: map[string]time.Time{}}
}

func (p *pow) mac(msg string) string {
	h := hmac.New(sha256.New, p.key)
	h.Write([]byte(msg))
	return hex.EncodeToString(h.Sum(nil))
}

func (p *pow) issue() string {
	msg := strconv.FormatInt(p.now().Unix(), 10) + "." + rand.Text()
	return msg + "." + p.mac(msg)
}

func leadingZeroBits(b []byte) int {
	n := 0
	for _, c := range b {
		n += bits.LeadingZeros8(c)
		if c != 0 {
			break
		}
	}
	return n
}

func (p *pow) verify(challenge, nonce string) bool {
	i := strings.LastIndexByte(challenge, '.')
	if i < 0 || len(nonce) == 0 || len(nonce) > 32 || !hmac.Equal([]byte(challenge[i+1:]), []byte(p.mac(challenge[:i]))) {
		return false
	}
	ts, err := strconv.ParseInt(challenge[:strings.IndexByte(challenge, '.')], 10, 64)
	issued, now := time.Unix(ts, 0), p.now()
	if err != nil || now.Sub(issued) > powTTL || issued.After(now.Add(time.Minute)) {
		return false
	}
	sum := sha256.Sum256([]byte(challenge + nonce))
	if leadingZeroBits(sum[:]) < p.difficulty {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for c, exp := range p.used {
		if now.After(exp) {
			delete(p.used, c)
		}
	}
	if _, ok := p.used[challenge]; ok {
		return false
	}
	p.used[challenge] = issued.Add(powTTL)
	return true
}
