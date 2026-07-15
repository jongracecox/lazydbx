package resources

import "github.com/jongracecox/lazydbx/internal/resource"

// NewRegistry builds the registry of all built-in resources. Every new
// resource def gets its one registration line here.
func NewRegistry() *resource.Registry {
	reg := resource.NewRegistry()
	reg.MustRegister(CatalogsDef{})
	return reg
}
