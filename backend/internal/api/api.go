// Package api exposes the HTTP surface: ingest, snapshot metadata, graph
// queries, table detail and search.
//
// Both datastores are reached through interfaces, so the handlers' own work --
// validating parameters, resolving the "latest" alias, mapping a store outcome
// to a status code -- is testable without a database.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"urara-vision/backend/internal/config"
	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	neostore "urara-vision/backend/internal/store/neo4j"
	"urara-vision/backend/internal/store/postgres"
)

// GraphStore is the graph projection the API reads from. It is an interface so
// the server can run with Neo4j disabled, falling back to an in-process
// projection built from Postgres.
type GraphStore interface {
	Project(ctx context.Context, m *model.Model, edges []graph.Edge) error
	DeleteSnapshot(ctx context.Context, sid string) error
	GetGraph(ctx context.Context, sid string, opt neostore.GraphOptions) (*neostore.Graph, error)
	Neighborhood(ctx context.Context, sid, tableID string, depth int, includeSources bool) (*neostore.Graph, error)
	FindPaths(ctx context.Context, sid, from, to string, maxDepth, limit int) ([]neostore.JoinPath, error)
	Upstream(ctx context.Context, sid, tableID string) ([]neostore.LineageEntry, error)
	Downstream(ctx context.Context, sid, sourceID string) ([]neostore.LineageEntry, error)
	SiblingsBySource(ctx context.Context, sid, tableID string) ([]neostore.LineageEntry, error)
	Ping(ctx context.Context) error
}

// MetaStore is the record of truth the API reads documents back out of. It is
// an interface for the same reason GraphStore is: the handlers' own logic --
// parameter validation, the "latest" alias, error-to-status mapping -- is worth
// testing without a database standing behind it.
//
// It is deliberately the exact set of methods the handlers call rather than
// everything *postgres.Store offers, so a fake only has to implement what the
// HTTP surface actually uses.
type MetaStore interface {
	SaveSnapshot(ctx context.Context, m *model.Model) error
	ListSnapshots(ctx context.Context) ([]model.Snapshot, error)
	GetSnapshot(ctx context.Context, id string) (*model.Snapshot, error)
	DeleteSnapshot(ctx context.Context, id string) error
	LatestSnapshotID(ctx context.Context) (string, error)
	ListDomains(ctx context.Context, sid string) ([]model.Domain, error)
	ListTables(ctx context.Context, sid, domainID string) ([]postgres.TableSummary, error)
	GetTable(ctx context.Context, sid, tableID string) (*model.Table, error)
	IncomingRelationships(ctx context.Context, sid, tableID string) ([]postgres.Referrer, error)
	Search(ctx context.Context, sid, query string, limit int) ([]postgres.SearchHit, error)
	ListDiagnostics(ctx context.Context, sid, severity string) ([]model.Diagnostic, error)
	ListSourceTables(ctx context.Context, sid string) ([]model.SourceTable, error)
	Ping(ctx context.Context) error
}

// Server holds the API dependencies.
type Server struct {
	cfg    *config.Config
	pg     MetaStore
	graphs GraphStore
	log    *slog.Logger
}

// New builds a Server.
func New(cfg *config.Config, pg MetaStore, graphs GraphStore, log *slog.Logger) *Server {
	return &Server{cfg: cfg, pg: pg, graphs: graphs, log: log}
}

// Compile-time proof that the real store still satisfies the interface, so a
// signature change on the store is caught here rather than in main.
var _ MetaStore = (*postgres.Store)(nil)

// Routes returns the configured router.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * 1e9))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)

	// Everything under /api/v1 is behind the bearer token. The probes above
	// deliberately are not: kubelet cannot carry a credential.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.requireToken)

		r.Post("/ingest", s.handleIngest)
		r.Get("/snapshots", s.handleListSnapshots)

		r.Route("/snapshots/{sid}", func(r chi.Router) {
			r.Get("/", s.handleGetSnapshot)
			r.Delete("/", s.handleDeleteSnapshot)
			r.Get("/context", s.handleContext)
			r.Get("/domains", s.handleListDomains)
			r.Get("/tables", s.handleListTables)
			r.Get("/tables/detail", s.handleTablesDetail)
			r.Get("/table", s.handleGetTable)
			r.Get("/graph", s.handleGraph)
			r.Get("/neighborhood", s.handleNeighborhood)
			r.Get("/paths", s.handlePaths)
			r.Get("/lineage", s.handleLineage)
			r.Get("/search", s.handleSearch)
			r.Get("/diagnostics", s.handleDiagnostics)
			r.Get("/sources", s.handleSources)
		})
	})

	return r
}
