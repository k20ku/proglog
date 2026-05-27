package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewLogServer() *http.Server {
	httpsv := newLogServer()
	mux := chi.NewRouter()
	mux.Post("/", httpsv.handleProduce)
	mux.Get("/", httpsv.handleConsume)

	return &http.Server{
		Handler: mux,
	}
}

type logServer struct {
	Log *Log
}

func newLogServer() *logServer {
	return &logServer{
		Log: NewLog(),
	}
}

type ProduceRequest struct {
	Record Record `json:"record"`
}

type ProduceResponse struct {
	Offset uint64 `json:"offset"`
}

type ConsumeRequest struct {
	Offset uint64 `json:"offset"`
}

type ConsumeResponse struct {
	Record Record `json:"record"`
}

func (s *logServer) handleProduce(w http.ResponseWriter, r *http.Request) {
	// Request
	var req ProduceRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log
	off, err := s.Log.Append(req.Record)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Response
	res := ProduceResponse{Offset: off}
	err = json.NewEncoder(w).Encode(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}
}

func (s *logServer) handleConsume(w http.ResponseWriter, r *http.Request) {
	// Request
	var req ConsumeRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// handle
	record, err := s.Log.Read(req.Offset)
	if err != nil {
		if err == ErrOffsetNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Response
	res := ConsumeResponse{Record: record}
	err = json.NewEncoder(w).Encode(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
