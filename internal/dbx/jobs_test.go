package dbx

import (
	"testing"
	"time"

	"github.com/databricks/databricks-sdk-go/service/jobs"
	"github.com/databricks/databricks-sdk-go/service/pipelines"
	"github.com/stretchr/testify/assert"
)

func TestScheduleSummary(t *testing.T) {
	tests := []struct {
		name string
		in   *jobs.JobSettings
		want string
	}{
		{"nil settings", nil, ""},
		{"empty settings", &jobs.JobSettings{}, ""},
		{
			"cron",
			&jobs.JobSettings{Schedule: &jobs.CronSchedule{QuartzCronExpression: "0 0 12 * * ?"}},
			"0 0 12 * * ?",
		},
		{
			"cron paused",
			&jobs.JobSettings{Schedule: &jobs.CronSchedule{
				QuartzCronExpression: "0 0 12 * * ?",
				PauseStatus:          jobs.PauseStatusPaused,
			}},
			"0 0 12 * * ? (paused)",
		},
		{
			"cron unpaused has no suffix",
			&jobs.JobSettings{Schedule: &jobs.CronSchedule{
				QuartzCronExpression: "0 0 12 * * ?",
				PauseStatus:          jobs.PauseStatusUnpaused,
			}},
			"0 0 12 * * ?",
		},
		{
			"trigger file arrival",
			&jobs.JobSettings{Trigger: &jobs.TriggerSettings{FileArrival: &jobs.FileArrivalTriggerConfiguration{}}},
			"file arrival",
		},
		{
			"trigger periodic",
			&jobs.JobSettings{Trigger: &jobs.TriggerSettings{Periodic: &jobs.PeriodicTriggerConfiguration{}}},
			"periodic",
		},
		{
			"trigger table update",
			&jobs.JobSettings{Trigger: &jobs.TriggerSettings{TableUpdate: &jobs.TableUpdateTriggerConfiguration{}}},
			"table update",
		},
		{
			"trigger unknown",
			&jobs.JobSettings{Trigger: &jobs.TriggerSettings{}},
			"trigger",
		},
		{
			"continuous",
			&jobs.JobSettings{Continuous: &jobs.Continuous{}},
			"continuous",
		},
		{
			"schedule wins over trigger",
			&jobs.JobSettings{
				Schedule: &jobs.CronSchedule{QuartzCronExpression: "0 0 * * * ?"},
				Trigger:  &jobs.TriggerSettings{Periodic: &jobs.PeriodicTriggerConfiguration{}},
			},
			"0 0 * * * ?",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, scheduleSummary(tt.in))
		})
	}
}

func TestMapRunState(t *testing.T) {
	tests := []struct {
		name       string
		rs         *jobs.RunState
		st         *jobs.RunStatus
		wantState  string
		wantResult string
	}{
		{"both nil", nil, nil, "", ""},
		{
			"run state preferred",
			&jobs.RunState{LifeCycleState: jobs.RunLifeCycleStateTerminated, ResultState: jobs.RunResultStateSuccess},
			&jobs.RunStatus{State: jobs.RunLifecycleStateV2StateRunning},
			"TERMINATED", "SUCCESS",
		},
		{
			"run state running has empty result",
			&jobs.RunState{LifeCycleState: jobs.RunLifeCycleStateRunning},
			nil,
			"RUNNING", "",
		},
		{
			"falls back to status",
			nil,
			&jobs.RunStatus{
				State:              jobs.RunLifecycleStateV2StateTerminated,
				TerminationDetails: &jobs.TerminationDetails{Code: jobs.TerminationCodeCodeSuccess},
			},
			"TERMINATED", "SUCCESS",
		},
		{
			"status without termination details",
			nil,
			&jobs.RunStatus{State: jobs.RunLifecycleStateV2StateRunning},
			"RUNNING", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, result := mapRunState(tt.rs, tt.st)
			assert.Equal(t, tt.wantState, state)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestRunElapsed(t *testing.T) {
	now := time.UnixMilli(100_000).UTC()
	tests := []struct {
		name                   string
		runMs, execMs, startMs int64
		running                bool
		want                   time.Duration
	}{
		{"run duration preferred", 5000, 3000, 1000, false, 5 * time.Second},
		{"execution duration fallback", 0, 3000, 1000, false, 3 * time.Second},
		{"elapsed while running", 0, 0, 40_000, true, 60 * time.Second},
		{"not running, no durations", 0, 0, 40_000, false, 0},
		{"running but no start", 0, 0, 0, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, runElapsed(tt.runMs, tt.execMs, tt.startMs, tt.running, now))
		})
	}
}

func TestRenderRunOutput(t *testing.T) {
	tests := []struct {
		name string
		in   *jobs.RunOutput
		want string
	}{
		{"nil", nil, "(no output)"},
		{"empty", &jobs.RunOutput{}, "(no output)"},
		{
			"error and trace",
			&jobs.RunOutput{Error: "boom", ErrorTrace: "at line 1\nat line 2"},
			"ERROR:\nboom\nat line 1\nat line 2",
		},
		{
			"error only",
			&jobs.RunOutput{Error: "boom"},
			"ERROR:\nboom",
		},
		{
			"trace only",
			&jobs.RunOutput{ErrorTrace: "at line 1"},
			"ERROR:\nat line 1",
		},
		{
			"logs",
			&jobs.RunOutput{Logs: "hello logs"},
			"Logs:\nhello logs",
		},
		{
			"logs truncated",
			&jobs.RunOutput{Logs: "hello logs", LogsTruncated: true},
			"Logs (truncated):\nhello logs",
		},
		{
			"notebook exit value",
			&jobs.RunOutput{NotebookOutput: &jobs.NotebookOutput{Result: "42"}},
			"Notebook exit value: 42",
		},
		{
			"notebook empty result ignored",
			&jobs.RunOutput{NotebookOutput: &jobs.NotebookOutput{}},
			"(no output)",
		},
		{
			"sql marker",
			&jobs.RunOutput{SqlOutput: &jobs.SqlOutput{}},
			"[SQL task output present]",
		},
		{
			"dbt marker",
			&jobs.RunOutput{DbtOutput: &jobs.DbtOutput{}},
			"[dbt task output present]",
		},
		{
			"all sections in order",
			&jobs.RunOutput{
				Error:          "boom",
				ErrorTrace:     "trace",
				Logs:           "logline",
				LogsTruncated:  true,
				NotebookOutput: &jobs.NotebookOutput{Result: "42"},
			},
			"ERROR:\nboom\ntrace\n\nLogs (truncated):\nlogline\n\nNotebook exit value: 42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, renderRunOutput(tt.in))
		})
	}
}

func TestRenderEvents(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, "(no events)", renderEvents(nil))
		assert.Equal(t, "(no events)", renderEvents([]pipelines.PipelineEvent{}))
	})

	t.Run("reversed to oldest-first with level and message", func(t *testing.T) {
		// API returns newest-first.
		events := []pipelines.PipelineEvent{
			{Timestamp: "2026-07-15T10:02:00Z", Level: pipelines.EventLevelError, Message: "third"},
			{Timestamp: "2026-07-15T10:01:00Z", Level: pipelines.EventLevelWarn, Message: "second"},
			{Timestamp: "2026-07-15T10:00:00Z", Level: pipelines.EventLevelInfo, Message: "first"},
		}
		want := "2026-07-15T10:00:00Z  INFO  first\n" +
			"2026-07-15T10:01:00Z  WARN  second\n" +
			"2026-07-15T10:02:00Z  ERROR  third"
		assert.Equal(t, want, renderEvents(events))
	})

	t.Run("message escapes decoded", func(t *testing.T) {
		events := []pipelines.PipelineEvent{
			{Timestamp: "t", Level: pipelines.EventLevelInfo, Message: `line1\nline2`},
		}
		assert.Equal(t, "t  INFO  line1\nline2", renderEvents(events))
	})

	t.Run("exception details appended", func(t *testing.T) {
		events := []pipelines.PipelineEvent{
			{
				Timestamp: "t",
				Level:     pipelines.EventLevelError,
				Message:   "failed",
				Error: &pipelines.ErrorDetail{
					Exceptions: []pipelines.SerializedException{
						{ClassName: "java.lang.RuntimeException", Message: "kaboom"},
						{Message: "no class"},
					},
				},
			},
		}
		want := "t  ERROR  failed\n" +
			"    java.lang.RuntimeException: kaboom\n" +
			"    no class"
		assert.Equal(t, want, renderEvents(events))
	})
}
