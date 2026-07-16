package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jongracecox/lazydbx/internal/resource"
)

const host = "https://acme.cloud.databricks.com"

func TestWebURL(t *testing.T) {
	tests := []struct {
		name   string
		linker resource.WebLinker
		scope  resource.Scope
		row    resource.Row
		want   string
		wantOK bool
	}{
		{
			name:   "catalog",
			linker: CatalogsDef{},
			row:    resource.Row{ID: "main"},
			want:   host + "/explore/data/main",
			wantOK: true,
		},
		{
			name:   "schema",
			linker: SchemasDef{},
			scope:  resource.Scope{"catalog": "main"},
			row:    resource.Row{ID: "silver"},
			want:   host + "/explore/data/main/silver",
			wantOK: true,
		},
		{
			name:   "table",
			linker: TablesDef{},
			scope:  resource.Scope{"catalog": "main", "schema": "silver"},
			row:    resource.Row{ID: "events"},
			want:   host + "/explore/data/main/silver/events",
			wantOK: true,
		},
		{
			name:   "columns open parent table",
			linker: ColumnsDef{},
			scope:  resource.Scope{"catalog": "main", "schema": "silver", "table": "events"},
			row:    resource.Row{ID: "user_id"},
			want:   host + "/explore/data/main/silver/events",
			wantOK: true,
		},
		{
			name:   "job",
			linker: JobsDef{},
			row:    resource.Row{ID: "123"},
			want:   host + "/jobs/123",
			wantOK: true,
		},
		{
			name:   "run",
			linker: RunsDef{},
			scope:  resource.Scope{"job": "123"},
			row:    resource.Row{ID: "456"},
			want:   host + "/jobs/123/runs/456",
			wantOK: true,
		},
		{
			name:   "taskrun opens parent run",
			linker: TaskRunsDef{},
			scope:  resource.Scope{"job": "123", "run": "456"},
			row:    resource.Row{ID: "789"},
			want:   host + "/jobs/123/runs/456",
			wantOK: true,
		},
		{
			name:   "pipeline",
			linker: PipelinesDef{},
			row:    resource.Row{ID: "abc-def"},
			want:   host + "/pipelines/abc-def",
			wantOK: true,
		},
		{
			name:   "update",
			linker: UpdatesDef{},
			scope:  resource.Scope{"pipeline": "abc-def"},
			row:    resource.Row{ID: "upd-1"},
			want:   host + "/pipelines/abc-def/updates/upd-1",
			wantOK: true,
		},
		{
			name:   "empty host is not linkable",
			linker: CatalogsDef{},
			row:    resource.Row{ID: "main"},
			want:   "",
			wantOK: false,
		},
		{
			name:   "missing scope segment is not linkable",
			linker: TablesDef{},
			scope:  resource.Scope{"catalog": "main"}, // no schema
			row:    resource.Row{ID: "events"},
			want:   "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := host
			if tt.name == "empty host is not linkable" {
				h = ""
			}
			got, ok := tt.linker.WebURL(h, tt.scope, tt.row)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWebURLEscapesSegments(t *testing.T) {
	got, ok := TablesDef{}.WebURL(host,
		resource.Scope{"catalog": "my catalog", "schema": "silver"},
		resource.Row{ID: "events/raw"})
	assert.True(t, ok)
	assert.Equal(t, host+"/explore/data/my%20catalog/silver/events%2Fraw", got)
}
