package dbx

import (
	"context"
	"fmt"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/sql"
)

// warehousesDAO implements WarehousesDAO against the SDK.
type warehousesDAO struct {
	w *databricks.WorkspaceClient
}

func (d warehousesDAO) List(ctx context.Context) ([]Warehouse, error) {
	infos, err := d.w.Warehouses.ListAll(ctx, sql.ListWarehousesRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing warehouses: %w", err)
	}
	out := make([]Warehouse, 0, len(infos))
	for i := range infos {
		ei := &infos[i]
		out = append(out, Warehouse{
			ID:         ei.Id,
			Name:       ei.Name,
			State:      string(ei.State),
			Size:       ei.ClusterSize,
			Serverless: ei.EnableServerlessCompute,
		})
	}
	sortByName(out, func(wh Warehouse) string { return wh.Name })
	return out, nil
}

// statementDAO implements StatementDAO against the SDK, driving the async
// statement-execution API: Submit returns immediately (WaitTimeout 0s), the
// engine polls until a terminal state, then decodes the first inline chunk.
type statementDAO struct {
	w *databricks.WorkspaceClient
}

func (d statementDAO) Submit(ctx context.Context, warehouseID, statement string, rowLimit int) (string, error) {
	resp, err := d.w.StatementExecution.ExecuteStatement(ctx, sql.ExecuteStatementRequest{
		WarehouseId: warehouseID,
		Statement:   statement,
		WaitTimeout: "0s",
		Format:      sql.FormatJsonArray,
		Disposition: sql.DispositionInline,
		RowLimit:    int64(rowLimit),
	})
	if err != nil {
		return "", fmt.Errorf("submitting statement: %w", err)
	}
	return resp.StatementId, nil
}

func (d statementDAO) Poll(ctx context.Context, statementID string) (StatementPoll, error) {
	resp, err := d.w.StatementExecution.GetStatementByStatementId(ctx, statementID)
	if err != nil {
		return StatementPoll{}, fmt.Errorf("polling statement %s: %w", statementID, err)
	}

	poll := StatementPoll{}
	if resp.Status != nil {
		poll.State = string(resp.Status.State)
	}

	switch poll.State {
	case StmtFailed:
		if resp.Status != nil && resp.Status.Error != nil {
			poll.Message = resp.Status.Error.Message
		}
	case StmtSucceeded:
		poll.Result = decodeResult(resp.Manifest, resp.Result)
	}
	return poll, nil
}

func (d statementDAO) Cancel(ctx context.Context, statementID string) error {
	if err := d.w.StatementExecution.CancelExecution(ctx, sql.CancelExecutionRequest{
		StatementId: statementID,
	}); err != nil {
		return fmt.Errorf("canceling statement %s: %w", statementID, err)
	}
	return nil
}

// decodeResult reduces the first inline chunk of a statement result to a
// StmtResult. It is a pure function (no SDK calls) so it can be unit-tested.
// A nil manifest or result — e.g. a DDL statement that returns no rows —
// yields an empty, non-nil StmtResult. v1 renders only the first chunk; when
// more data exists (manifest truncation or a further chunk) Truncated is set
// and the caller shows a "first N rows" banner.
func decodeResult(manifest *sql.ResultManifest, result *sql.ResultData) *StmtResult {
	out := &StmtResult{}
	if manifest != nil {
		if manifest.Schema != nil {
			cols := make([]sql.ColumnInfo, len(manifest.Schema.Columns))
			copy(cols, manifest.Schema.Columns)
			sortByPosition(cols)
			out.Columns = make([]StmtColumn, 0, len(cols))
			for i := range cols {
				col := &cols[i]
				typ := col.TypeText
				if typ == "" {
					typ = string(col.TypeName)
				}
				out.Columns = append(out.Columns, StmtColumn{
					Name: col.Name,
					Type: typ,
				})
			}
		}
		if manifest.Truncated {
			out.Truncated = true
		}
	}
	if result != nil {
		out.Rows = result.DataArray
		if result.NextChunkIndex != 0 {
			out.Truncated = true
		}
	}
	return out
}

func sortByPosition(cols []sql.ColumnInfo) {
	// insertion sort keeps it dependency-free and stable for the tiny column
	// counts a result schema carries.
	for i := 1; i < len(cols); i++ {
		for j := i; j > 0 && cols[j].Position < cols[j-1].Position; j-- {
			cols[j], cols[j-1] = cols[j-1], cols[j]
		}
	}
}

// PickWarehouse resolves the default warehouse, serverless-first. The input
// is already name-sorted, so "first" means first in slice order. Resolution
// order: explicit config ID → first RUNNING serverless → first serverless in
// any state → first RUNNING warehouse of any kind → none (caller opens the
// picker).
func PickWarehouse(configID string, warehouses []Warehouse) (Warehouse, bool) {
	if configID != "" {
		for i := range warehouses {
			if warehouses[i].ID == configID {
				return warehouses[i], true
			}
		}
	}
	for i := range warehouses {
		if warehouses[i].Serverless && warehouses[i].State == "RUNNING" {
			return warehouses[i], true
		}
	}
	for i := range warehouses {
		if warehouses[i].Serverless {
			return warehouses[i], true
		}
	}
	for i := range warehouses {
		if warehouses[i].State == "RUNNING" {
			return warehouses[i], true
		}
	}
	return Warehouse{}, false
}
