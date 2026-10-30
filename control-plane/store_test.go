package main

import (
	"errors"
	"testing"
)

func allow([]Deployment) error { return nil }

func TestStoreUpsertAndGet(t *testing.T) {
	s := NewStore()
	d, err := s.Upsert(Deployment{DeploymentRequest: validRequest()}, allow)
	if err != nil || d.ID == "" {
		t.Fatalf("upsert: id=%q err=%v", d.ID, err)
	}
	got, ok := s.Get(d.ID)
	if !ok || got.Name != "hello" {
		t.Fatalf("get: ok=%v got=%+v", ok, got)
	}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("want missing id to be not found")
	}
}

func TestStoreRedeployKeepsID(t *testing.T) {
	s := NewStore()
	first, _ := s.Upsert(Deployment{DeploymentRequest: validRequest()}, allow)
	r := validRequest()
	r.Replicas = 3
	second, _ := s.Upsert(Deployment{DeploymentRequest: r}, allow)
	if first.ID != second.ID {
		t.Fatalf("redeploy got new id %q, want %q", second.ID, first.ID)
	}
	if got, _ := s.Get(first.ID); got.Replicas != 3 {
		t.Fatalf("want replicas updated to 3, got %d", got.Replicas)
	}
}

func TestStoreCheckRejects(t *testing.T) {
	s := NewStore()
	_, err := s.Upsert(Deployment{DeploymentRequest: validRequest()}, func([]Deployment) error {
		return errors.New("no")
	})
	if err == nil {
		t.Fatal("want check error")
	}
	if len(s.deployments) != 0 {
		t.Fatal("rejected deployment must not be stored")
	}
}
