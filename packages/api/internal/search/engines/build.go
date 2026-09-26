package engines

import "context"

// build.go wires registry specs into runnable Engine implementations.

type specEngine struct {
	spec *Spec
	http *engineHTTP
}

func (e *specEngine) Name() string         { return e.spec.Name }
func (e *specEngine) Categories() []string { return e.spec.Categories }

func (e *specEngine) Search(ctx context.Context, q Query) ([]Result, error) {
	return e.http.fetch(ctx, q)
}

// NewEngines materializes the registry's enabled roster in merge order
// (weight desc, then name), sharing one transport pool across engines.
func NewEngines(reg *Registry, pool *ProxyPool) []Engine {
	if reg == nil {
		return nil
	}
	return newEngines(reg.EnabledSpecs(), pool)
}

// newEngines builds engines for an arbitrary spec list — the canary uses it
// to probe disabled opt-ins (google, startpage) that NewEngines skips.
func newEngines(specs []*Spec, pool *ProxyPool) []Engine {
	tp := newTransportPool(pool)
	out := make([]Engine, 0, len(specs))
	for _, s := range specs {
		out = append(out, &specEngine{spec: s, http: newEngineHTTP(s, tp)})
	}
	return out
}
