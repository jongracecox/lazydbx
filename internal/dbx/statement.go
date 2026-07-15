package dbx

import (
	"context"
	"errors"

	"github.com/databricks/databricks-sdk-go"
)

// warehousesDAO implements WarehousesDAO against the SDK.
// STUB: implementation lands with the Phase 2 statement layer.
type warehousesDAO struct {
	w *databricks.WorkspaceClient
}

func (d warehousesDAO) List(_ context.Context) ([]Warehouse, error) {
	return nil, errors.New("not implemented")
}

// statementDAO implements StatementDAO against the SDK.
// STUB: implementation lands with the Phase 2 statement layer.
type statementDAO struct {
	w *databricks.WorkspaceClient
}

func (d statementDAO) Submit(_ context.Context, _, _ string, _ int) (string, error) {
	return "", errors.New("not implemented")
}

func (d statementDAO) Poll(_ context.Context, _ string) (StatementPoll, error) {
	return StatementPoll{}, errors.New("not implemented")
}

func (d statementDAO) Cancel(_ context.Context, _ string) error {
	return errors.New("not implemented")
}

// PickWarehouse resolves the default warehouse, serverless-first:
// explicit config ID → first RUNNING serverless → any serverless → none
// (caller opens the picker).
// STUB: implementation lands with the Phase 2 statement layer.
func PickWarehouse(configID string, warehouses []Warehouse) (Warehouse, bool) {
	_ = configID
	_ = warehouses
	return Warehouse{}, false
}
