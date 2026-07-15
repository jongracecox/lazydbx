package dbx

import (
	"context"
	"errors"

	"github.com/databricks/databricks-sdk-go"
)

// jobsDAO implements JobsDAO against the SDK.
// STUB: implementation lands with the Phase 3 jobs layer.
type jobsDAO struct {
	w *databricks.WorkspaceClient
}

func (d jobsDAO) List(_ context.Context) ([]Job, error) {
	return nil, errors.New("not implemented")
}

func (d jobsDAO) ListRuns(_ context.Context, _ int64, _ int) ([]Run, error) {
	return nil, errors.New("not implemented")
}

func (d jobsDAO) GetRunTasks(_ context.Context, _ int64) ([]TaskRun, error) {
	return nil, errors.New("not implemented")
}

func (d jobsDAO) GetRunOutput(_ context.Context, _ int64) (string, error) {
	return "", errors.New("not implemented")
}

// pipelinesDAO implements PipelinesDAO against the SDK.
// STUB: implementation lands with the Phase 3 pipelines layer.
type pipelinesDAO struct {
	w *databricks.WorkspaceClient
}

func (d pipelinesDAO) List(_ context.Context) ([]Pipeline, error) {
	return nil, errors.New("not implemented")
}

func (d pipelinesDAO) ListUpdates(_ context.Context, _ string, _ int) ([]PipelineUpdate, error) {
	return nil, errors.New("not implemented")
}

func (d pipelinesDAO) Events(_ context.Context, _ string, _ int) (string, error) {
	return "", errors.New("not implemented")
}
