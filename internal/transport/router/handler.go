package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/akaradat/kv-store/internal/core/cluster"
	"github.com/akaradat/kv-store/internal/transport/httpapi"
)

const idleConnsPerNode = 128

type NewHandlerInput struct {
	Cluster     *cluster.Cluster
	NodeTimeout time.Duration
	Log         *slog.Logger
}

type handler struct {
	cluster *cluster.Cluster
	client  *http.Client
	timeout time.Duration
	log     *slog.Logger
}

func NewHandler(in NewHandlerInput) http.Handler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Go's default of 2 idle connections per host makes the counter test open and close connections.
	transport.MaxIdleConnsPerHost = idleConnsPerNode
	h := &handler{cluster: in.Cluster, client: &http.Client{Transport: transport}, timeout: in.NodeTimeout, log: in.Log}
	r := chi.NewRouter()
	r.Use(httpapi.RequestLog(in.Log))
	r.Use(middleware.RequestSize(httpapi.MaxBodyBytes))
	r.NotFound(httpapi.NotFound)
	r.Get("/kv", h.list)
	r.Get("/kv/{key}", h.get)
	r.Put("/kv/{key}", h.write)
	r.Patch("/kv/{key}", h.write)
	return r
}
