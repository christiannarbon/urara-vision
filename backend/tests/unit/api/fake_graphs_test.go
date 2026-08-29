// A fake graph projection, recording what each query was asked for.
package api_test

import (
	"context"

	"urara-vision/backend/internal/graph"
	"urara-vision/backend/internal/model"
	neostore "urara-vision/backend/internal/store/neo4j"
)

// fakeGraphs is the same idea for the graph projection.
type fakeGraphs struct {
	graph      *neostore.Graph
	paths      []neostore.JoinPath
	upstream   []neostore.LineageEntry
	downstream []neostore.LineageEntry
	siblings   []neostore.LineageEntry

	errProject error
	errPing    error

	// Recorded calls.
	projected  *model.Model
	edgeCount  int
	deleted    []string
	graphOpt   neostore.GraphOptions
	nbrDepth   int
	nbrSources bool
	pathFrom   string
	pathTo     string
	pathDepth  int
	pathLimit  int
	upstreamID string
	downID     string
}

func (f *fakeGraphs) Project(_ context.Context, m *model.Model, edges []graph.Edge) error {
	f.projected, f.edgeCount = m, len(edges)
	return f.errProject
}

func (f *fakeGraphs) DeleteSnapshot(_ context.Context, sid string) error {
	f.deleted = append(f.deleted, sid)
	return nil
}

func (f *fakeGraphs) GetGraph(_ context.Context, _ string, opt neostore.GraphOptions) (*neostore.Graph, error) {
	f.graphOpt = opt
	return f.emptyIfNil(), nil
}

func (f *fakeGraphs) Neighborhood(_ context.Context, _, _ string, depth int, sources bool) (*neostore.Graph, error) {
	f.nbrDepth, f.nbrSources = depth, sources
	return f.emptyIfNil(), nil
}

func (f *fakeGraphs) FindPaths(_ context.Context, _, from, to string, maxDepth, limit int) ([]neostore.JoinPath, error) {
	f.pathFrom, f.pathTo, f.pathDepth, f.pathLimit = from, to, maxDepth, limit
	return f.paths, nil
}

func (f *fakeGraphs) Upstream(_ context.Context, _, tableID string) ([]neostore.LineageEntry, error) {
	f.upstreamID = tableID
	return f.upstream, nil
}

func (f *fakeGraphs) Downstream(_ context.Context, _, sourceID string) ([]neostore.LineageEntry, error) {
	f.downID = sourceID
	return f.downstream, nil
}

func (f *fakeGraphs) SiblingsBySource(context.Context, string, string) ([]neostore.LineageEntry, error) {
	return f.siblings, nil
}

func (f *fakeGraphs) Ping(context.Context) error { return f.errPing }

func (f *fakeGraphs) emptyIfNil() *neostore.Graph {
	if f.graph == nil {
		return &neostore.Graph{}
	}
	return f.graph
}
