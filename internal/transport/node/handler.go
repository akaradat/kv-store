package node

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/akaradat/kv-store/internal/core/kv"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

type Store interface {
	Get(key string) (kv.Entry, error)
	Put(key string, ifVersion uint64, value []byte) (kv.Entry, error)
	Patch(key string, ifVersion uint64, delta []byte) (kv.Entry, error)
	Range(fn func(key string) bool)
}

type NewHandlerInput struct {
	Store  Store
	NodeID string
	Log    *slog.Logger
}

type handler struct {
	store  Store
	nodeID string
}

func NewHandler(in NewHandlerInput) http.Handler {
	h := &handler{store: in.Store, nodeID: in.NodeID}
	r := chi.NewRouter()
	r.Use(httpapi.RequestLog(in.Log))
	r.Use(middleware.RequestSize(httpapi.MaxBodyBytes))
	r.NotFound(httpapi.NotFound)
	r.Get("/kv", h.list)
	r.Get("/kv/{key}", h.get)
	r.Put("/kv/{key}", h.put)
	r.Patch("/kv/{key}", h.patch)
	return r
}
