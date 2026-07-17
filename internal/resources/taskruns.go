package resources

import (
	"context"
	"strconv"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

var taskRunCols = []resource.ColSpec[dbx.TaskRun]{
	{Column: resource.Column{Title: "TASK"}, Extract: func(t dbx.TaskRun) string { return t.Key }},
	{Column: resource.Column{Title: "STATE", Width: 14}, Extract: func(t dbx.TaskRun) string { return t.State }},
	{Column: resource.Column{Title: "RESULT", Width: 12}, Extract: func(t dbx.TaskRun) string { return t.Result }},
	{Column: resource.Column{Title: "STARTED", Width: 12}, Extract: func(t dbx.TaskRun) string { return relTime(t.StartedAt) }},
	{Column: resource.Column{Title: "DURATION", Width: 10}, Extract: func(t dbx.TaskRun) string { return fmtDuration(t.Duration) }},
}

const (
	taskRunStateCol  = 1
	taskRunResultCol = 2
)

// TaskRunsDef browses the task-level runs within one job run — the leaf of
// the jobs drill-down.
type TaskRunsDef struct{}

func (TaskRunsDef) Name() string                { return "taskruns" }
func (TaskRunsDef) Aliases() []string           { return []string{"taskrun", "tasks"} }
func (TaskRunsDef) Args() []string              { return []string{"job", "run"} }
func (TaskRunsDef) Columns() []resource.Column  { return resource.Cols(taskRunCols) }
func (TaskRunsDef) PollInterval() time.Duration { return 10 * time.Second }
func (TaskRunsDef) Child() string               { return "" }

func (TaskRunsDef) ChildScope(parent resource.Scope, _ resource.Row) resource.Scope {
	return parent
}

// CellClass colors the STATE and RESULT columns semantically.
func (TaskRunsDef) CellClass(col int, value string) resource.CellClass {
	switch col {
	case taskRunStateCol, taskRunResultCol:
		return stateClass(value)
	default:
		return resource.CellDefault
	}
}

func (TaskRunsDef) Actions() []resource.Action {
	return []resource.Action{
		{
			Key:      "l",
			Name:     "logs",
			NeedsRow: true,
			Run: func(_ context.Context, c *dbx.Clients, _ resource.Scope, row resource.Row) any {
				task := row.Data.(dbx.TaskRun)
				return view.OpenLogMsg{
					Title:  "logs/" + task.Key,
					Follow: isRunningState(task.State),
					Fetch: func(ctx context.Context) (string, error) {
						dao, err := c.Jobs()
						if err != nil {
							return "", err
						}
						return dao.GetRunOutput(ctx, task.RunID)
					},
				}
			},
		},
	}
}

// Tabs implements resource.Tabber: the tab names EnterMsg produces, in order.
func (TaskRunsDef) Tabs() []string { return []string{"logs", "details"} }

// EnterMsg implements resource.Opener: selecting a task run opens tabs —
// its logs beside its metadata — instead of bare describe.
func (TaskRunsDef) EnterMsg(c *dbx.Clients, _ resource.Scope, row resource.Row) any {
	task, ok := row.Data.(dbx.TaskRun)
	if !ok {
		return view.FlashMsg{Level: view.FlashWarn, Text: "task details unavailable (stale cache) — refresh with r"}
	}
	return view.OpenTabsMsg{
		Title: task.Key,
		Tabs: []view.TabSpec{
			{Name: "logs", Log: &view.LogTabSpec{
				Follow: isRunningState(task.State),
				Fetch: func(ctx context.Context) (string, error) {
					dao, err := c.Jobs()
					if err != nil {
						return "", err
					}
					return dao.GetRunOutput(ctx, task.RunID)
				},
			}},
			{Name: "details", Detail: func(context.Context) (any, error) { return task, nil }},
		},
	}
}

func (TaskRunsDef) List(ctx context.Context, c *dbx.Clients, scope resource.Scope) ([]resource.Row, error) {
	runID, err := parseScopeInt(scope, "run")
	if err != nil {
		return nil, err
	}
	dao, err := c.Jobs()
	if err != nil {
		return nil, err
	}
	items, err := dao.GetRunTasks(ctx, runID)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(t dbx.TaskRun) string { return strconv.FormatInt(t.RunID, 10) }, taskRunCols), nil
}

func (TaskRunsDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}

// isRunningState reports whether a lifecycle state is still in flight.
func isRunningState(state string) bool {
	return stateClass(state) == resource.CellRunning
}
