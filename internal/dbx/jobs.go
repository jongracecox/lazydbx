package dbx

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/listing"
	"github.com/databricks/databricks-sdk-go/service/jobs"
	"github.com/databricks/databricks-sdk-go/service/pipelines"
)

// jobsDAO implements JobsDAO against the SDK. Implementations stay thin:
// pagination and type mapping only, no business logic.
type jobsDAO struct {
	w *databricks.WorkspaceClient
}

func (d jobsDAO) List(ctx context.Context) ([]Job, error) {
	bases, err := listing.ToSlice(ctx, d.w.Jobs.List(ctx, jobs.ListJobsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}
	out := make([]Job, 0, len(bases))
	for i := range bases {
		bj := &bases[i]
		name := ""
		if bj.Settings != nil {
			name = DecodeEscapes(bj.Settings.Name)
		}
		out = append(out, Job{
			ID:        bj.JobId,
			Name:      name,
			Schedule:  scheduleSummary(bj.Settings),
			Creator:   bj.CreatorUserName,
			CreatedAt: millisToTime(bj.CreatedTime),
		})
	}
	sortByName(out, func(j Job) string { return j.Name })
	return out, nil
}

func (d jobsDAO) ListRuns(ctx context.Context, jobID int64, limit int) ([]Run, error) {
	it := d.w.Jobs.ListRuns(ctx, jobs.ListRunsRequest{JobId: jobID})
	bases, err := listing.ToSliceN(ctx, it, limit)
	if err != nil {
		return nil, fmt.Errorf("listing runs for job %d: %w", jobID, err)
	}
	now := time.Now().UTC()
	out := make([]Run, 0, len(bases))
	for i := range bases {
		br := &bases[i]
		state, result := mapRunState(br.State, br.Status)
		out = append(out, Run{
			ID:        br.RunId,
			State:     state,
			Result:    result,
			Trigger:   string(br.Trigger),
			StartedAt: millisToTime(br.StartTime),
			Duration:  runElapsed(br.RunDuration, br.ExecutionDuration, br.StartTime, result == "", now),
		})
	}
	// Keep API order (newest first) — do not name-sort.
	return out, nil
}

func (d jobsDAO) GetRunTasks(ctx context.Context, runID int64) ([]TaskRun, error) {
	run, err := d.w.Jobs.GetRun(ctx, jobs.GetRunRequest{RunId: runID})
	if err != nil {
		return nil, fmt.Errorf("getting run %d: %w", runID, err)
	}
	now := time.Now().UTC()
	out := make([]TaskRun, 0, len(run.Tasks))
	for i := range run.Tasks {
		t := &run.Tasks[i]
		state, result := mapRunState(t.State, t.Status)
		out = append(out, TaskRun{
			RunID:     t.RunId,
			Key:       t.TaskKey,
			State:     state,
			Result:    result,
			StartedAt: millisToTime(t.StartTime),
			Duration:  runElapsed(t.RunDuration, t.ExecutionDuration, t.StartTime, result == "", now),
		})
	}
	// Preserve API task order.
	return out, nil
}

func (d jobsDAO) GetRunOutput(ctx context.Context, taskRunID int64) (string, error) {
	out, err := d.w.Jobs.GetRunOutput(ctx, jobs.GetRunOutputRequest{RunId: taskRunID})
	if err != nil {
		return "", fmt.Errorf("getting run output for %d: %w", taskRunID, err)
	}
	return renderRunOutput(out), nil
}

// scheduleSummary derives a human-readable schedule summary from a job's
// settings: cron expression (with "(paused)" suffix), a short trigger word,
// "continuous", or "".
func scheduleSummary(s *jobs.JobSettings) string {
	if s == nil {
		return ""
	}
	switch {
	case s.Schedule != nil:
		summary := s.Schedule.QuartzCronExpression
		if s.Schedule.PauseStatus == jobs.PauseStatusPaused {
			summary += " (paused)"
		}
		return summary
	case s.Trigger != nil:
		return triggerWord(s.Trigger)
	case s.Continuous != nil:
		return "continuous"
	default:
		return ""
	}
}

// triggerWord picks a short label for the configured trigger type.
func triggerWord(t *jobs.TriggerSettings) string {
	switch {
	case t.FileArrival != nil:
		return "file arrival"
	case t.Periodic != nil:
		return "periodic"
	case t.TableUpdate != nil:
		return "table update"
	case t.Model != nil:
		return "model"
	default:
		return "trigger"
	}
}

// mapRunState reduces the SDK's run state representations to our two string
// fields (lifecycle-ish state, terminal result). It prefers the (deprecated
// but simpler) RunState and falls back to the newer RunStatus when only that
// is populated.
func mapRunState(rs *jobs.RunState, st *jobs.RunStatus) (state, result string) {
	if rs != nil {
		return string(rs.LifeCycleState), string(rs.ResultState)
	}
	if st != nil {
		state = string(st.State)
		if st.TerminationDetails != nil {
			result = string(st.TerminationDetails.Code)
		}
	}
	return state, result
}

// runElapsed computes a run/task duration: RunDuration when set, else
// ExecutionDuration, else elapsed since start while still running, else zero.
func runElapsed(runDurationMs, execDurationMs, startMs int64, running bool, now time.Time) time.Duration {
	switch {
	case runDurationMs > 0:
		return time.Duration(runDurationMs) * time.Millisecond
	case execDurationMs > 0:
		return time.Duration(execDurationMs) * time.Millisecond
	case running && startMs > 0:
		return now.Sub(millisToTime(startMs))
	default:
		return 0
	}
}

// renderRunOutput composes a task run's output into display text: error +
// trace, logs (with a truncation marker), notebook exit value, and short
// markers for SQL/dbt task output. Sections appear only when non-empty;
// "(no output)" when everything is empty.
func renderRunOutput(o *jobs.RunOutput) string {
	if o == nil {
		return "(no output)"
	}
	var sections []string
	if o.Error != "" || o.ErrorTrace != "" {
		s := "ERROR:\n" + o.Error
		if o.ErrorTrace != "" {
			if o.Error != "" {
				s += "\n"
			}
			s += o.ErrorTrace
		}
		sections = append(sections, s)
	}
	if o.Logs != "" {
		header := "Logs:"
		if o.LogsTruncated {
			header = "Logs (truncated):"
		}
		sections = append(sections, header+"\n"+o.Logs)
	}
	if o.NotebookOutput != nil && o.NotebookOutput.Result != "" {
		sections = append(sections, "Notebook exit value: "+o.NotebookOutput.Result)
	}
	if o.SqlOutput != nil {
		sections = append(sections, "[SQL task output present]")
	}
	if o.DbtOutput != nil {
		sections = append(sections, "[dbt task output present]")
	}
	if len(sections) == 0 {
		return "(no output)"
	}
	return strings.Join(sections, "\n\n")
}

// pipelinesDAO implements PipelinesDAO against the SDK.
type pipelinesDAO struct {
	w *databricks.WorkspaceClient
}

func (d pipelinesDAO) List(ctx context.Context) ([]Pipeline, error) {
	infos, err := d.w.Pipelines.ListPipelinesAll(ctx, pipelines.ListPipelinesRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing pipelines: %w", err)
	}
	out := make([]Pipeline, 0, len(infos))
	for i := range infos {
		p := &infos[i]
		out = append(out, Pipeline{
			ID:     p.PipelineId,
			Name:   DecodeEscapes(p.Name),
			State:  string(p.State),
			Health: string(p.Health),
		})
	}
	sortByName(out, func(p Pipeline) string { return p.Name })
	return out, nil
}

func (d pipelinesDAO) ListUpdates(ctx context.Context, pipelineID string, limit int) ([]PipelineUpdate, error) {
	resp, err := d.w.Pipelines.ListUpdates(ctx, pipelines.ListUpdatesRequest{
		PipelineId: pipelineID,
		MaxResults: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing updates for pipeline %s: %w", pipelineID, err)
	}
	out := make([]PipelineUpdate, 0, len(resp.Updates))
	for i := range resp.Updates {
		u := &resp.Updates[i]
		out = append(out, PipelineUpdate{
			ID:        u.UpdateId,
			State:     string(u.State),
			Cause:     string(u.Cause),
			CreatedAt: millisToTime(u.CreationTime),
		})
	}
	// Keep API order (newest first).
	return out, nil
}

func (d pipelinesDAO) Events(ctx context.Context, pipelineID string, maxResults int) (string, error) {
	// MaxResults is a per-page cap (server max 100); ToSliceN bounds the total
	// and handles paging.
	pageSize := maxResults
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 100
	}
	it := d.w.Pipelines.ListPipelineEvents(ctx, pipelines.ListPipelineEventsRequest{
		PipelineId: pipelineID,
		MaxResults: pageSize,
	})
	events, err := listing.ToSliceN(ctx, it, maxResults)
	if err != nil {
		return "", fmt.Errorf("listing events for pipeline %s: %w", pipelineID, err)
	}
	return renderEvents(events), nil
}

// renderEvents renders pipeline events as display text, one line per event.
// The API returns newest-first; output is reversed to oldest-first. Each line
// is "timestamp  LEVEL  message", with any exception details indented beneath.
// An empty slice renders "(no events)".
func renderEvents(events []pipelines.PipelineEvent) string {
	if len(events) == 0 {
		return "(no events)"
	}
	lines := make([]string, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		e := &events[i]
		line := fmt.Sprintf("%s  %s  %s", e.Timestamp, string(e.Level), DecodeEscapes(e.Message))
		if e.Error != nil {
			for j := range e.Error.Exceptions {
				ex := &e.Error.Exceptions[j]
				detail := ex.Message
				if ex.ClassName != "" {
					detail = ex.ClassName + ": " + ex.Message
				}
				line += "\n    " + DecodeEscapes(detail)
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
