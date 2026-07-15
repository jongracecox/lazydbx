package dbx

import (
	"fmt"
	"sync"

	"github.com/databricks/databricks-sdk-go"

	"github.com/jongracecox/lazydbx/internal/version"
)

// registerUserAgent tags SDK traffic with our product name, once. Called
// from client construction rather than init() because WithProduct panics on
// invalid input — that must surface at a recoverable point, not at import.
var registerUserAgent = sync.OnceFunc(func() {
	databricks.WithProduct("lazydbx", version.Version)
})

// Clients bundles the lazily-constructed SDK clients and DAOs for one
// profile. SDK clients are immutable with respect to auth, so switching
// profiles means constructing a new Clients (cache them via Pool).
type Clients struct {
	profile Profile
	daos    DAOs

	mu sync.Mutex
	ws *databricks.WorkspaceClient
}

// DAOs carries injected DAO implementations. Production code leaves fields
// nil (SDK-backed DAOs are built lazily); tests inject fakes so resource
// defs run without any SDK or network involvement.
type DAOs struct {
	Catalogs CatalogsDAO
}

// NewClients wraps a profile; no network I/O happens until first use.
func NewClients(p Profile) *Clients {
	return &Clients{profile: p}
}

// NewClientsWithDAOs builds Clients with injected DAOs, for tests.
func NewClientsWithDAOs(p Profile, daos DAOs) *Clients {
	return &Clients{profile: p, daos: daos}
}

// Profile returns the profile these clients authenticate as.
func (c *Clients) Profile() Profile { return c.profile }

// workspace lazily builds the WorkspaceClient. Auth resolution (PAT, OAuth,
// CLI token cache...) is entirely the SDK's unified auth.
func (c *Clients) workspace() (*databricks.WorkspaceClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws != nil {
		return c.ws, nil
	}
	registerUserAgent()
	w, err := databricks.NewWorkspaceClient(&databricks.Config{Profile: c.profile.Name})
	if err != nil {
		return nil, fmt.Errorf("connecting profile %s: %w", c.profile.Name, err)
	}
	c.ws = w
	return w, nil
}

// Catalogs returns the catalogs DAO.
func (c *Clients) Catalogs() (CatalogsDAO, error) {
	if c.daos.Catalogs != nil {
		return c.daos.Catalogs, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return catalogsDAO{w: w}, nil
}

// Pool caches one Clients per profile name.
type Pool struct {
	mu      sync.Mutex
	clients map[string]*Clients
}

// NewPool returns an empty client pool.
func NewPool() *Pool {
	return &Pool{clients: map[string]*Clients{}}
}

// Get returns the cached Clients for a profile, constructing on first use.
func (p *Pool) Get(profile Profile) *Clients {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[profile.Name]; ok {
		return c
	}
	c := NewClients(profile)
	p.clients[profile.Name] = c
	return c
}
