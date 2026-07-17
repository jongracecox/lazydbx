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
	Catalogs   CatalogsDAO
	Schemas    SchemasDAO
	Tables     TablesDAO
	Warehouses WarehousesDAO
	Statements StatementDAO
	Jobs       JobsDAO
	Pipelines  PipelinesDAO
	Apps       AppsDAO
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

// Schemas returns the schemas DAO.
func (c *Clients) Schemas() (SchemasDAO, error) {
	if c.daos.Schemas != nil {
		return c.daos.Schemas, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return schemasDAO{w: w}, nil
}

// Tables returns the tables DAO.
func (c *Clients) Tables() (TablesDAO, error) {
	if c.daos.Tables != nil {
		return c.daos.Tables, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return tablesDAO{w: w}, nil
}

// Warehouses returns the warehouses DAO.
func (c *Clients) Warehouses() (WarehousesDAO, error) {
	if c.daos.Warehouses != nil {
		return c.daos.Warehouses, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return warehousesDAO{w: w}, nil
}

// Statements returns the statement execution DAO.
func (c *Clients) Statements() (StatementDAO, error) {
	if c.daos.Statements != nil {
		return c.daos.Statements, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return statementDAO{w: w}, nil
}

// Jobs returns the jobs DAO.
func (c *Clients) Jobs() (JobsDAO, error) {
	if c.daos.Jobs != nil {
		return c.daos.Jobs, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return jobsDAO{w: w}, nil
}

// Pipelines returns the pipelines DAO.
func (c *Clients) Pipelines() (PipelinesDAO, error) {
	if c.daos.Pipelines != nil {
		return c.daos.Pipelines, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return pipelinesDAO{w: w}, nil
}

// Apps returns the apps DAO.
func (c *Clients) Apps() (AppsDAO, error) {
	if c.daos.Apps != nil {
		return c.daos.Apps, nil
	}
	w, err := c.workspace()
	if err != nil {
		return nil, err
	}
	return appsDAO{w: w}, nil
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
