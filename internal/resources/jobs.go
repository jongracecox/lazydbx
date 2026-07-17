package resources

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

var jobCols = []resource.ColSpec[dbx.Job]{
	{Column: resource.Column{Title: "ID", Width: 14}, Extract: func(j dbx.Job) string { return strconv.FormatInt(j.ID, 10) }},
	{Column: resource.Column{Title: "NAME"}, Extract: func(j dbx.Job) string { return j.Name }},
	{Column: resource.Column{Title: "LAST RUN", Width: 10}, Extract: func(j dbx.Job) string { return relTime(j.LastRunAt) }},
	{Column: resource.Column{Title: "STATUS", Width: 14}, Extract: lastStatus},
	{Column: resource.Column{Title: "SCHEDULE", Width: 22}, Extract: func(j dbx.Job) string { return j.Schedule }},
	{Column: resource.Column{Title: "CREATOR", Width: 28, Wide: true}, Extract: func(j dbx.Job) string { return j.Creator }},
	{Column: resource.Column{Title: "CREATED", Width: 12, Wide: true}, Extract: func(j dbx.Job) string { return relTime(j.CreatedAt) }},
}

// jobStatusCol is the index of the STATUS column, colored via stateClass.
const jobStatusCol = 3

// lastStatus prefers the terminal result (SUCCESS/FAILED); an in-flight run
// shows its lifecycle state instead.
func lastStatus(j dbx.Job) string {
	if j.LastRunResult != "" {
		return j.LastRunResult
	}
	return j.LastRunState
}

// JobsDef browses Databricks jobs.
type JobsDef struct{}

func (JobsDef) Name() string               { return "jobs" }
func (JobsDef) Aliases() []string          { return []string{"job", "j"} }
func (JobsDef) Args() []string             { return nil }
func (JobsDef) Columns() []resource.Column { return resource.Cols(jobCols) }

// PollInterval is 30s — the jobs list API is rate-limited to 20/s.
func (JobsDef) PollInterval() time.Duration { return 30 * time.Second }

func (JobsDef) Child() string { return "runs" }

func (JobsDef) ChildScope(parent resource.Scope, row resource.Row) resource.Scope {
	return parent.Merge("job", row.ID)
}

func (JobsDef) Actions() []resource.Action { return nil }

// CellClass implements resource.Styler: the STATUS column carries the last
// run's verdict color.
func (JobsDef) CellClass(col int, value string) resource.CellClass {
	if col == jobStatusCol {
		return stateClass(value)
	}
	return resource.CellDefault
}

// RowTags implements resource.Tagger from the job's custom tags, rendered
// "key=value" (bare "key" when the value is empty), sorted.
func (JobsDef) RowTags(row resource.Row) []string {
	job, ok := row.Data.(dbx.Job)
	if !ok || len(job.Tags) == 0 {
		return nil
	}
	tags := make([]string, 0, len(job.Tags))
	for k, v := range job.Tags {
		if v == "" {
			tags = append(tags, k)
		} else {
			tags = append(tags, k+"="+v)
		}
	}
	sort.Strings(tags)
	return tags
}

func (JobsDef) List(ctx context.Context, c *dbx.Clients, _ resource.Scope) ([]resource.Row, error) {
	dao, err := c.Jobs()
	if err != nil {
		return nil, err
	}
	items, err := dao.List(ctx)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(j dbx.Job) string { return strconv.FormatInt(j.ID, 10) }, jobCols), nil
}

func (JobsDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}

// stateClass maps a lifecycle/result/health value to a semantic CellClass,
// shared by runs, taskruns, pipelines, updates, and apps. Case-insensitive.
//
// TERMINATED (job/run lifecycle state) is intentionally CellDefault, not
// CellWarn: it's a neutral terminal state whose Result column (SUCCESS,
// FAILED, CANCELED, ...) already carries the verdict — coloring the state
// column too would be redundant and could clash with the result color.
//
// App states share this map: ACTIVE (compute healthy) is Good; CRASHED is Bad;
// DEPLOYING/UPDATING/IN_PROGRESS are in-flight; STOPPED/STOPPING/UNAVAILABLE/
// DELETING are Warn. An app's RUNNING app-state maps to CellRunning like a job
// run — a running app reads as "running" rather than green.
func stateClass(value string) resource.CellClass {
	switch strings.ToUpper(value) {
	case "SUCCESS", "SUCCEEDED", "COMPLETED", "IDLE", "HEALTHY", "ACTIVE":
		return resource.CellGood
	case "FAILED", "INTERNAL_ERROR", "ERROR", "UNHEALTHY", "CRASHED":
		return resource.CellBad
	case "CANCELED", "CANCELLED", "TIMEDOUT", "TIMED_OUT", "SKIPPED", "TERMINATING",
		"STOPPED", "STOPPING", "UNAVAILABLE", "DELETING":
		return resource.CellWarn
	case "RUNNING", "PENDING", "STARTING", "INITIALIZING", "SETTING_UP_TABLES",
		"WAITING_FOR_RESOURCES", "QUEUED", "CREATED", "DEPLOYING", "UPDATING", "IN_PROGRESS":
		return resource.CellRunning
	default:
		return resource.CellDefault
	}
}
