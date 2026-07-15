package dbx

import (
	"testing"

	"github.com/databricks/databricks-sdk-go/service/sql"
	"github.com/stretchr/testify/assert"
)

func TestPickWarehouse(t *testing.T) {
	// Warehouses arrive name-sorted; "first" means first in slice order.
	warehouses := []Warehouse{
		{ID: "cls-stopped", Name: "a-classic-stopped", State: "STOPPED", Serverless: false},
		{ID: "cls-running", Name: "b-classic-running", State: "RUNNING", Serverless: false},
		{ID: "srv-stopped", Name: "c-serverless-stopped", State: "STOPPED", Serverless: true},
		{ID: "srv-running", Name: "d-serverless-running", State: "RUNNING", Serverless: true},
	}

	tests := []struct {
		name       string
		configID   string
		warehouses []Warehouse
		wantID     string
		wantOK     bool
	}{
		{
			name:       "explicit config ID wins",
			configID:   "cls-stopped",
			warehouses: warehouses,
			wantID:     "cls-stopped",
			wantOK:     true,
		},
		{
			name:       "first running serverless",
			configID:   "",
			warehouses: warehouses,
			wantID:     "srv-running",
			wantOK:     true,
		},
		{
			name:     "first serverless in any state",
			configID: "",
			warehouses: []Warehouse{
				{ID: "cls-running", Name: "b", State: "RUNNING", Serverless: false},
				{ID: "srv-stopped", Name: "c", State: "STOPPED", Serverless: true},
			},
			wantID: "srv-stopped",
			wantOK: true,
		},
		{
			name:     "first running of any kind",
			configID: "",
			warehouses: []Warehouse{
				{ID: "cls-stopped", Name: "a", State: "STOPPED", Serverless: false},
				{ID: "cls-running", Name: "b", State: "RUNNING", Serverless: false},
			},
			wantID: "cls-running",
			wantOK: true,
		},
		{
			name:     "none found",
			configID: "",
			warehouses: []Warehouse{
				{ID: "cls-stopped", Name: "a", State: "STOPPED", Serverless: false},
			},
			wantID: "",
			wantOK: false,
		},
		{
			name:       "unknown config ID falls through to serverless",
			configID:   "does-not-exist",
			warehouses: warehouses,
			wantID:     "srv-running",
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PickWarehouse(tt.configID, tt.warehouses)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestDecodeResult(t *testing.T) {
	tests := []struct {
		name          string
		manifest      *sql.ResultManifest
		result        *sql.ResultData
		wantColumns   []StmtColumn
		wantRows      [][]string
		wantTruncated bool
	}{
		{
			name: "normal result ordered by position",
			manifest: &sql.ResultManifest{
				Schema: &sql.ResultSchema{
					Columns: []sql.ColumnInfo{
						{Name: "b", TypeText: "STRING", Position: 1},
						{Name: "a", TypeText: "INT", Position: 0},
					},
				},
			},
			result: &sql.ResultData{
				DataArray: [][]string{{"1", "x"}, {"2", "y"}},
			},
			wantColumns: []StmtColumn{
				{Name: "a", Type: "INT"},
				{Name: "b", Type: "STRING"},
			},
			wantRows:      [][]string{{"1", "x"}, {"2", "y"}},
			wantTruncated: false,
		},
		{
			name: "truncated via manifest",
			manifest: &sql.ResultManifest{
				Truncated: true,
				Schema: &sql.ResultSchema{
					Columns: []sql.ColumnInfo{{Name: "a", TypeText: "INT", Position: 0}},
				},
			},
			result: &sql.ResultData{
				DataArray: [][]string{{"1"}},
			},
			wantColumns:   []StmtColumn{{Name: "a", Type: "INT"}},
			wantRows:      [][]string{{"1"}},
			wantTruncated: true,
		},
		{
			name: "truncated via next chunk index",
			manifest: &sql.ResultManifest{
				Schema: &sql.ResultSchema{
					Columns: []sql.ColumnInfo{{Name: "a", TypeText: "INT", Position: 0}},
				},
			},
			result: &sql.ResultData{
				DataArray:      [][]string{{"1"}},
				NextChunkIndex: 1,
			},
			wantColumns:   []StmtColumn{{Name: "a", Type: "INT"}},
			wantRows:      [][]string{{"1"}},
			wantTruncated: true,
		},
		{
			name:          "nil manifest and result yields empty non-nil",
			manifest:      nil,
			result:        nil,
			wantColumns:   nil,
			wantRows:      nil,
			wantTruncated: false,
		},
		{
			name: "empty type text falls back to type name",
			manifest: &sql.ResultManifest{
				Schema: &sql.ResultSchema{
					Columns: []sql.ColumnInfo{
						{Name: "a", TypeText: "", TypeName: sql.ColumnInfoTypeNameInt, Position: 0},
					},
				},
			},
			result: &sql.ResultData{
				DataArray: [][]string{{"1"}},
			},
			wantColumns:   []StmtColumn{{Name: "a", Type: string(sql.ColumnInfoTypeNameInt)}},
			wantRows:      [][]string{{"1"}},
			wantTruncated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeResult(tt.manifest, tt.result)
			assert.NotNil(t, got)
			assert.Equal(t, tt.wantColumns, got.Columns)
			assert.Equal(t, tt.wantRows, got.Rows)
			assert.Equal(t, tt.wantTruncated, got.Truncated)
		})
	}
}
