package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// fakeJobsDAO is a struct of func fields — the house pattern for faking DAOs.
type fakeJobsDAO struct {
	ListFn         func(ctx context.Context) ([]dbx.Job, error)
	ListRunsFn     func(ctx context.Context, jobID int64, limit int) ([]dbx.Run, error)
	GetRunTasksFn  func(ctx context.Context, runID int64) ([]dbx.TaskRun, error)
	GetRunOutputFn func(ctx context.Context, taskRunID int64) (string, error)
}

func (f fakeJobsDAO) List(ctx context.Context) ([]dbx.Job, error) { return f.ListFn(ctx) }

func (f fakeJobsDAO) ListRuns(ctx context.Context, jobID int64, limit int) ([]dbx.Run, error) {
	return f.ListRunsFn(ctx, jobID, limit)
}

func (f fakeJobsDAO) GetRunTasks(ctx context.Context, runID int64) ([]dbx.TaskRun, error) {
	return f.GetRunTasksFn(ctx, runID)
}

func (f fakeJobsDAO) GetRunOutput(ctx context.Context, taskRunID int64) (string, error) {
	return f.GetRunOutputFn(ctx, taskRunID)
}

// fakePipelinesDAO is a struct of func fields.
type fakePipelinesDAO struct {
	ListFn        func(ctx context.Context) ([]dbx.Pipeline, error)
	ListUpdatesFn func(ctx context.Context, pipelineID string, limit int) ([]dbx.PipelineUpdate, error)
	EventsFn      func(ctx context.Context, pipelineID string, maxResults int) (string, error)
}

func (f fakePipelinesDAO) List(ctx context.Context) ([]dbx.Pipeline, error) { return f.ListFn(ctx) }

func (f fakePipelinesDAO) ListUpdates(ctx context.Context, pipelineID string, limit int) ([]dbx.PipelineUpdate, error) {
	return f.ListUpdatesFn(ctx, pipelineID, limit)
}

func (f fakePipelinesDAO) Events(ctx context.Context, pipelineID string, maxResults int) (string, error) {
	return f.EventsFn(ctx, pipelineID, maxResults)
}

func clientsWithJobs(dao dbx.JobsDAO) *dbx.Clients {
	return dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{Jobs: dao})
}

func clientsWithPipelines(dao dbx.PipelinesDAO) *dbx.Clients {
	return dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{Pipelines: dao})
}

// --- jobs ---

func TestJobsDefList(t *testing.T) {
	created := time.Now().Add(-2 * time.Hour)
	c := clientsWithJobs(fakeJobsDAO{
		ListFn: func(context.Context) ([]dbx.Job, error) {
			return []dbx.Job{
				{ID: 123, Name: "etl", Schedule: "0 0 * * *", Creator: "jon@example.com", CreatedAt: created},
				{ID: 456, Name: "reports"},
			}, nil
		},
	})

	rows, err := JobsDef{}.List(context.Background(), c, resource.Scope{})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "123", rows[0].ID)
	assert.Equal(t, []string{"123", "etl", "0 0 * * *", "jon@example.com", "2h"}, rows[0].Cells)
	assert.Equal(t, "456", rows[1].ID)
}

func TestJobsDefShape(t *testing.T) {
	d := JobsDef{}
	assert.Equal(t, "jobs", d.Name())
	assert.Contains(t, d.Aliases(), "j")
	assert.Empty(t, d.Args())
	assert.Equal(t, "runs", d.Child())
	assert.Equal(t, resource.Scope{"job": "123"}, d.ChildScope(resource.Scope{}, resource.Row{ID: "123"}))
	assert.Equal(t, 30*time.Second, d.PollInterval())
}

// --- runs ---

func TestRunsDefList(t *testing.T) {
	var gotJobID int64
	c := clientsWithJobs(fakeJobsDAO{
		ListRunsFn: func(_ context.Context, jobID int64, limit int) ([]dbx.Run, error) {
			gotJobID = jobID
			assert.Equal(t, 100, limit)
			return []dbx.Run{
				{ID: 1, State: "TERMINATED", Result: "SUCCESS", Trigger: "SCHEDULED", Duration: 63 * time.Second},
				{ID: 2, State: "RUNNING", Trigger: "MANUAL"},
			}, nil
		},
	})

	rows, err := RunsDef{}.List(context.Background(), c, resource.Scope{"job": "123"})
	require.NoError(t, err)
	assert.Equal(t, int64(123), gotJobID)
	require.Len(t, rows, 2)
	assert.Equal(t, "1", rows[0].ID)
	assert.Equal(t, []string{"1", "TERMINATED", "SUCCESS", "SCHEDULED", "", "1m3s"}, rows[0].Cells)
	assert.Equal(t, "2", rows[1].ID)
	assert.Empty(t, rows[1].Cells[5], "zero duration renders empty")
}

func TestRunsDefListBadJobID(t *testing.T) {
	c := clientsWithJobs(fakeJobsDAO{})
	_, err := RunsDef{}.List(context.Background(), c, resource.Scope{"job": "not-a-number"})
	assert.ErrorContains(t, err, "not-a-number")
}

func TestRunsDefShape(t *testing.T) {
	d := RunsDef{}
	assert.Equal(t, []string{"job"}, d.Args())
	assert.Equal(t, "taskruns", d.Child())
	assert.Equal(t, resource.Scope{"job": "123", "run": "1"},
		d.ChildScope(resource.Scope{"job": "123"}, resource.Row{ID: "1"}))
}

func TestRunsDefCellClass(t *testing.T) {
	d := RunsDef{}
	assert.Equal(t, resource.CellRunning, d.CellClass(runStateCol, "RUNNING"))
	assert.Equal(t, resource.CellGood, d.CellClass(runResultCol, "SUCCESS"))
	assert.Equal(t, resource.CellBad, d.CellClass(runResultCol, "FAILED"))
	assert.Equal(t, resource.CellDefault, d.CellClass(0, "1"), "non-state column is uncolored")
}

// --- taskruns ---

func TestTaskRunsDefList(t *testing.T) {
	var gotRunID int64
	c := clientsWithJobs(fakeJobsDAO{
		GetRunTasksFn: func(_ context.Context, runID int64) ([]dbx.TaskRun, error) {
			gotRunID = runID
			return []dbx.TaskRun{
				{RunID: 999, Key: "extract", State: "TERMINATED", Result: "SUCCESS", Duration: 5 * time.Second},
			}, nil
		},
	})

	rows, err := TaskRunsDef{}.List(context.Background(), c, resource.Scope{"job": "123", "run": "1"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), gotRunID)
	require.Len(t, rows, 1)
	assert.Equal(t, "999", rows[0].ID)
	assert.Equal(t, "extract", rows[0].Cells[0])
	assert.Empty(t, TaskRunsDef{}.Child(), "taskruns is the leaf")
}

func TestTaskRunsDefLogsAction(t *testing.T) {
	var gotTaskRunID int64
	c := clientsWithJobs(fakeJobsDAO{
		GetRunOutputFn: func(_ context.Context, taskRunID int64) (string, error) {
			gotTaskRunID = taskRunID
			return "log output", nil
		},
	})

	actions := TaskRunsDef{}.Actions()
	require.Len(t, actions, 1)
	action := actions[0]
	assert.Equal(t, "l", action.Key)
	assert.True(t, action.NeedsRow)

	row := resource.Row{ID: "999", Data: dbx.TaskRun{RunID: 999, Key: "extract", State: "RUNNING"}}
	msg := action.Run(context.Background(), c, resource.Scope{}, row)
	open, ok := msg.(view.OpenLogMsg)
	require.True(t, ok)
	assert.Equal(t, "logs/extract", open.Title)
	assert.True(t, open.Follow, "RUNNING task follows")

	out, err := open.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "log output", out)
	assert.Equal(t, int64(999), gotTaskRunID)
}

func TestTaskRunsDefLogsActionNotFollowingWhenTerminated(t *testing.T) {
	c := clientsWithJobs(fakeJobsDAO{
		GetRunOutputFn: func(context.Context, int64) (string, error) { return "", nil },
	})
	row := resource.Row{ID: "999", Data: dbx.TaskRun{RunID: 999, Key: "extract", State: "TERMINATED"}}
	msg := TaskRunsDef{}.Actions()[0].Run(context.Background(), c, resource.Scope{}, row)
	assert.False(t, msg.(view.OpenLogMsg).Follow)
}

// --- pipelines ---

func TestPipelinesDefList(t *testing.T) {
	c := clientsWithPipelines(fakePipelinesDAO{
		ListFn: func(context.Context) ([]dbx.Pipeline, error) {
			return []dbx.Pipeline{
				{ID: "abc-123", Name: "silver-etl", State: "RUNNING", Health: "HEALTHY"},
			}, nil
		},
	})

	rows, err := PipelinesDef{}.List(context.Background(), c, resource.Scope{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "abc-123", rows[0].ID)
	assert.Equal(t, []string{"silver-etl", "RUNNING", "HEALTHY", "abc-123"}, rows[0].Cells)
}

func TestPipelinesDefShape(t *testing.T) {
	d := PipelinesDef{}
	assert.Equal(t, "updates", d.Child())
	assert.Equal(t, resource.Scope{"pipeline": "abc-123"},
		d.ChildScope(resource.Scope{}, resource.Row{ID: "abc-123"}))
	assert.Equal(t, resource.CellGood, d.CellClass(pipelineHealthCol, "HEALTHY"))
	assert.Equal(t, resource.CellRunning, d.CellClass(pipelineStateCol, "RUNNING"))
}

func TestPipelinesDefEventsAction(t *testing.T) {
	var gotID string
	c := clientsWithPipelines(fakePipelinesDAO{
		EventsFn: func(_ context.Context, id string, maxResults int) (string, error) {
			gotID = id
			assert.Equal(t, 200, maxResults)
			return "events", nil
		},
	})

	actions := PipelinesDef{}.Actions()
	require.Len(t, actions, 1)
	row := resource.Row{ID: "abc-123", Data: dbx.Pipeline{ID: "abc-123", Name: "silver-etl", State: "RUNNING"}}
	msg := actions[0].Run(context.Background(), c, resource.Scope{}, row)
	open, ok := msg.(view.OpenLogMsg)
	require.True(t, ok)
	assert.Equal(t, "events/silver-etl", open.Title)
	assert.True(t, open.Follow)

	out, err := open.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "events", out)
	assert.Equal(t, "abc-123", gotID)
}

// --- updates ---

func TestUpdatesDefList(t *testing.T) {
	var gotPipelineID string
	c := clientsWithPipelines(fakePipelinesDAO{
		ListUpdatesFn: func(_ context.Context, pipelineID string, limit int) ([]dbx.PipelineUpdate, error) {
			gotPipelineID = pipelineID
			assert.Equal(t, 50, limit)
			return []dbx.PipelineUpdate{
				{ID: "upd-1", State: "COMPLETED", Cause: "SCHEDULE"},
			}, nil
		},
	})

	rows, err := UpdatesDef{}.List(context.Background(), c, resource.Scope{"pipeline": "abc-123"})
	require.NoError(t, err)
	assert.Equal(t, "abc-123", gotPipelineID)
	require.Len(t, rows, 1)
	assert.Equal(t, "upd-1", rows[0].ID)
	assert.Empty(t, UpdatesDef{}.Child(), "updates is the leaf")
}

func TestUpdatesDefEventsAction(t *testing.T) {
	var gotID string
	c := clientsWithPipelines(fakePipelinesDAO{
		EventsFn: func(_ context.Context, id string, maxResults int) (string, error) {
			gotID = id
			return "events", nil
		},
	})

	actions := UpdatesDef{}.Actions()
	require.Len(t, actions, 1)
	msg := actions[0].Run(context.Background(), c, resource.Scope{"pipeline": "abc-123"}, resource.Row{ID: "upd-1"})
	open, ok := msg.(view.OpenLogMsg)
	require.True(t, ok)
	assert.Equal(t, "events/abc-123", open.Title)

	_, err := open.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "abc-123", gotID)
}

func TestUpdatesDefCellClass(t *testing.T) {
	d := UpdatesDef{}
	assert.Equal(t, resource.CellGood, d.CellClass(updateStateCol, "COMPLETED"))
	assert.Equal(t, resource.CellDefault, d.CellClass(0, "upd-1"))
}

// --- stateClass ---

func TestStateClass(t *testing.T) {
	tests := []struct {
		value string
		want  resource.CellClass
	}{
		{"SUCCESS", resource.CellGood},
		{"succeeded", resource.CellGood},
		{"COMPLETED", resource.CellGood},
		{"IDLE", resource.CellGood},
		{"HEALTHY", resource.CellGood},
		{"FAILED", resource.CellBad},
		{"INTERNAL_ERROR", resource.CellBad},
		{"ERROR", resource.CellBad},
		{"UNHEALTHY", resource.CellBad},
		{"CANCELED", resource.CellWarn},
		{"CANCELLED", resource.CellWarn},
		{"TIMEDOUT", resource.CellWarn},
		{"TIMED_OUT", resource.CellWarn},
		{"SKIPPED", resource.CellWarn},
		{"RUNNING", resource.CellRunning},
		{"PENDING", resource.CellRunning},
		{"STARTING", resource.CellRunning},
		{"INITIALIZING", resource.CellRunning},
		{"SETTING_UP_TABLES", resource.CellRunning},
		{"WAITING_FOR_RESOURCES", resource.CellRunning},
		{"QUEUED", resource.CellRunning},
		{"CREATED", resource.CellRunning},
		{"TERMINATED", resource.CellDefault},
		{"UNKNOWN_THING", resource.CellDefault},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.want, stateClass(tt.value))
		})
	}
}

// --- drill chain + registry ---

func TestJobsDrillChainRegistered(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"jobs", "runs", "taskruns", "pipelines", "updates"} {
		_, ok := reg.Get(name)
		assert.True(t, ok, name)
	}
	for _, alias := range []string{"job", "j", "run", "taskrun", "tasks", "pipeline", "pl", "update"} {
		_, ok := reg.Get(alias)
		assert.True(t, ok, alias)
	}

	assert.Equal(t, "runs", JobsDef{}.Child())
	assert.Equal(t, "taskruns", RunsDef{}.Child())
	assert.Equal(t, "updates", PipelinesDef{}.Child())

	cmd, err := reg.Parse("runs 123")
	require.NoError(t, err)
	assert.Equal(t, resource.Scope{"job": "123"}, cmd.Scope)
	assert.Equal(t, "runs", cmd.Def.Name())
}

func TestJobsDefListError(t *testing.T) {
	c := clientsWithJobs(fakeJobsDAO{
		ListFn: func(context.Context) ([]dbx.Job, error) { return nil, errors.New("boom") },
	})
	_, err := JobsDef{}.List(context.Background(), c, resource.Scope{})
	assert.ErrorContains(t, err, "boom")
}
