package resources

import (
	"context"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

const pipelineEventsLimit = 200

var pipelineCols = []resource.ColSpec[dbx.Pipeline]{
	{Column: resource.Column{Title: "NAME"}, Extract: func(p dbx.Pipeline) string { return p.Name }},
	{Column: resource.Column{Title: "STATE", Width: 14}, Extract: func(p dbx.Pipeline) string { return p.State }},
	{Column: resource.Column{Title: "HEALTH", Width: 10}, Extract: func(p dbx.Pipeline) string { return p.Health }},
	{Column: resource.Column{Title: "ID", Width: 38, Wide: true}, Extract: func(p dbx.Pipeline) string { return p.ID }},
}

const (
	pipelineStateCol  = 1
	pipelineHealthCol = 2
)

// PipelinesDef browses Lakeflow/DLT pipelines.
type PipelinesDef struct{}

func (PipelinesDef) Name() string                { return "pipelines" }
func (PipelinesDef) Aliases() []string           { return []string{"pipeline", "pl"} }
func (PipelinesDef) Args() []string              { return nil }
func (PipelinesDef) Columns() []resource.Column  { return resource.Cols(pipelineCols) }
func (PipelinesDef) PollInterval() time.Duration { return 15 * time.Second }
func (PipelinesDef) Child() string               { return "updates" }

func (PipelinesDef) ChildScope(parent resource.Scope, row resource.Row) resource.Scope {
	return parent.Merge("pipeline", row.ID)
}

// CellClass colors the STATE and HEALTH columns semantically.
func (PipelinesDef) CellClass(col int, value string) resource.CellClass {
	switch col {
	case pipelineStateCol, pipelineHealthCol:
		return stateClass(value)
	default:
		return resource.CellDefault
	}
}

func (PipelinesDef) Actions() []resource.Action {
	return []resource.Action{
		{
			Key:      "l",
			Name:     "events",
			NeedsRow: true,
			Run: func(_ context.Context, c *dbx.Clients, _ resource.Scope, row resource.Row) any {
				p := row.Data.(dbx.Pipeline)
				return view.OpenLogMsg{
					Title:  "events/" + p.Name,
					Follow: isRunningState(p.State),
					Fetch: func(ctx context.Context) (string, error) {
						dao, err := c.Pipelines()
						if err != nil {
							return "", err
						}
						return dao.Events(ctx, p.ID, pipelineEventsLimit)
					},
				}
			},
		},
	}
}

func (PipelinesDef) List(ctx context.Context, c *dbx.Clients, _ resource.Scope) ([]resource.Row, error) {
	dao, err := c.Pipelines()
	if err != nil {
		return nil, err
	}
	items, err := dao.List(ctx)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(p dbx.Pipeline) string { return p.ID }, pipelineCols), nil
}

func (PipelinesDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}
