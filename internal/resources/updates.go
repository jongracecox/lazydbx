package resources

import (
	"context"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

const updatesListLimit = 50

var updateCols = []resource.ColSpec[dbx.PipelineUpdate]{
	{Column: resource.Column{Title: "UPDATE ID", Width: 38}, Extract: func(u dbx.PipelineUpdate) string { return u.ID }},
	{Column: resource.Column{Title: "STATE", Width: 14}, Extract: func(u dbx.PipelineUpdate) string { return u.State }},
	{Column: resource.Column{Title: "CAUSE"}, Extract: func(u dbx.PipelineUpdate) string { return u.Cause }},
	{Column: resource.Column{Title: "CREATED", Width: 12}, Extract: func(u dbx.PipelineUpdate) string { return relTime(u.CreatedAt) }},
}

const updateStateCol = 1

// UpdatesDef browses the updates (executions) of one pipeline — the leaf of
// the pipelines drill-down.
type UpdatesDef struct{}

func (UpdatesDef) Name() string                { return "updates" }
func (UpdatesDef) Aliases() []string           { return []string{"update"} }
func (UpdatesDef) Args() []string              { return []string{"pipeline"} }
func (UpdatesDef) Columns() []resource.Column  { return resource.Cols(updateCols) }
func (UpdatesDef) PollInterval() time.Duration { return 10 * time.Second }
func (UpdatesDef) Child() string               { return "" }

func (UpdatesDef) ChildScope(parent resource.Scope, _ resource.Row) resource.Scope {
	return parent
}

// CellClass colors the STATE column semantically.
func (UpdatesDef) CellClass(col int, value string) resource.CellClass {
	if col == updateStateCol {
		return stateClass(value)
	}
	return resource.CellDefault
}

func (UpdatesDef) Actions() []resource.Action {
	return []resource.Action{
		{
			Key:      "l",
			Name:     "events",
			NeedsRow: true,
			Run: func(_ context.Context, c *dbx.Clients, scope resource.Scope, _ resource.Row) any {
				pipelineID := scope["pipeline"]
				return view.OpenLogMsg{
					Title:  "events/" + pipelineID,
					Follow: false,
					Fetch: func(ctx context.Context) (string, error) {
						dao, err := c.Pipelines()
						if err != nil {
							return "", err
						}
						return dao.Events(ctx, pipelineID, pipelineEventsLimit)
					},
				}
			},
		},
	}
}

func (UpdatesDef) List(ctx context.Context, c *dbx.Clients, scope resource.Scope) ([]resource.Row, error) {
	dao, err := c.Pipelines()
	if err != nil {
		return nil, err
	}
	items, err := dao.ListUpdates(ctx, scope["pipeline"], updatesListLimit)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(u dbx.PipelineUpdate) string { return u.ID }, updateCols), nil
}

func (UpdatesDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}
