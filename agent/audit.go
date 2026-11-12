package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// AuditEvent is one line of the audit log. Every alert, triage, proposal, approval,
// rejection and execution is recorded, so any action can be traced back to its cause.
type AuditEvent struct {
	Time       time.Time `json:"time"`
	Kind       string    `json:"kind"` // alert_received | triage | proposal | approved | rejected | executed | execution_failed
	ProposalID string    `json:"proposal_id,omitempty"`
	Actor      string    `json:"actor"` // "agent" or the approving human
	Data       any       `json:"data,omitempty"`
	PrevHash   string    `json:"prev_hash"`
	Hash       string    `json:"hash"`
}

// AuditLog is an append-only JSONL file. Each event's hash covers the previous hash,
// so editing or deleting a past line breaks the chain and Verify catches it.
type AuditLog struct {
	mu   sync.Mutex
	path string
	last string
}

func OpenAuditLog(path string) (*AuditLog, error) {
	l := &AuditLog{path: path}
	events, err := ReadAudit(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if n := len(events); n > 0 {
		l.last = events[n-1].Hash
	}
	return l, nil
}

func hashEvent(e AuditEvent) string {
	e.Hash = ""
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (l *AuditLog) Record(kind, proposalID, actor string, data any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := AuditEvent{Time: time.Now().UTC(), Kind: kind, ProposalID: proposalID, Actor: actor, Data: data, PrevHash: l.last}
	// Round-trip Data through JSON so the hash is computed on exactly what gets written.
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	json.Unmarshal(raw, &e)
	e.Hash = hashEvent(e)

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, _ := json.Marshal(e)
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	l.last = e.Hash
	return nil
}

func ReadAudit(path string) ([]AuditEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []AuditEvent
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e AuditEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("audit line %d: %w", len(out)+1, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// VerifyAudit checks the hash chain; it returns the index of the first bad event, or -1.
func VerifyAudit(events []AuditEvent) int {
	prev := ""
	for i, e := range events {
		if e.PrevHash != prev || hashEvent(e) != e.Hash {
			return i
		}
		prev = e.Hash
	}
	return -1
}
