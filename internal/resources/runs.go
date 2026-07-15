package resources

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

const runsListLimit = 100

var runCols = []resource.ColSpec[dbx.Run]{
	{Column: resource.Column{Title: "RUN ID", Width: 14}, Extract: func(r dbx.Run) string { return strconv.FormatInt(r.ID, 10) }},
	{Column: resource.Column{Title: "STATE", Width: 14}, Extract: func(r dbx.Run) string { return r.State }},
	{Column: resource.Column{Title: "RESULT", Width: 12}, Extract: func(r dbx.Run) string { return r.Result }},
	{Column: resource.Column{Title: "TRIGGER", Width: 14, Wide: true}, Extract: func(r dbx.Run) string { return r.Trigger }},
	{Column: resource.Column{Title: "STARTED", Width: 12}, Extract: func(r dbx.Run) string { return relTime(r.StartedAt) }},
	{Column: resource.Column{Title: "DURATION", Width: 10}, Extract: func(r dbx.Run) string { return fmtDuration(r.Duration) }},
}

// runStateCol and runResultCol index into runCols for Styler.
const (
	runStateCol  = 1
	runResultCol = 2
)

// RunsDef browses the runs of one job.
type RunsDef struct{}

func (RunsDef) Name() string                { return "runs" }
func (RunsDef) Aliases() []string           { return []string{"run"} }
func (RunsDef) Args() []string              { return []string{"job"} }
func (RunsDef) Columns() []resource.Column  { return resource.Cols(runCols) }
func (RunsDef) PollInterval() time.Duration { return 10 * time.Second }
func (RunsDef) Child() string               { return "taskruns" }

func (RunsDef) ChildScope(parent resource.Scope, row resource.Row) resource.Scope {
	return parent.Merge("run", row.ID)
}

func (RunsDef) Actions() []resource.Action { return nil }

// CellClass colors the STATE and RESULT columns semantically.
func (RunsDef) CellClass(col int, value string) resource.CellClass {
	switch col {
	case runStateCol, runResultCol:
		return stateClass(value)
	default:
		return resource.CellDefault
	}
}

func (RunsDef) List(ctx context.Context, c *dbx.Clients, scope resource.Scope) ([]resource.Row, error) {
	jobID, err := parseScopeInt(scope, "job")
	if err != nil {
		return nil, err
	}
	dao, err := c.Jobs()
	if err != nil {
		return nil, err
	}
	items, err := dao.ListRuns(ctx, jobID, runsListLimit)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(r dbx.Run) string { return strconv.FormatInt(r.ID, 10) }, runCols), nil
}

func (RunsDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}

// parseScopeInt parses a scope value as int64, erroring clearly when it
// isn't a valid ID.
func parseScopeInt(scope resource.Scope, key string) (int64, error) {
	raw := scope[key]
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s id %q: %w", key, raw, err)
	}
	return v, nil
}

// fmtDuration renders a duration like "1m3s", truncated to whole seconds.
// Zero renders empty (run hasn't started/finished timing yet).
func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.Truncate(time.Second).String()
}
