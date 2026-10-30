package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Server is the control-plane HTTP API.
type Server struct {
	store     *Store
	guardrail Guardrail
	log       *slog.Logger
}

func NewServer(store *Store, g Guardrail, log *slog.Logger) *Server {
	return &Server{store: store, guardrail: g, log: log}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /deployments", s.createDeployment)
	mux.HandleFunc("GET /deployments/{id}", s.getDeployment)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) createDeployment(w http.ResponseWriter, r *http.Request) {
	var req DeploymentRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := req.Validate(); err != nil {
		s.log.Info("deployment invalid", "name", req.Name, "namespace", req.Namespace, "err", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	d := Deployment{DeploymentRequest: req, Status: "accepted", CreatedAt: time.Now().UTC()}
	d, err := s.store.Upsert(d, func(existing []Deployment) error {
		return s.guardrail.Check(req, existing)
	})
	var pe *PolicyError
	if errors.As(err, &pe) {
		s.log.Warn("deployment rejected", "name", req.Name, "namespace", req.Namespace, "reason", pe.Reason)
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Info("deployment accepted", "id", d.ID, "name", d.Name, "namespace", d.Namespace, "image", d.Image)
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) getDeployment(w http.ResponseWriter, r *http.Request) {
	d, ok := s.store.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	writeJSON(w, http.StatusOK, d)
}
