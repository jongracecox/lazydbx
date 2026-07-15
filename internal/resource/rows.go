package resource

// ColSpec pairs a column definition with a typed cell extractor. Concrete
// defs declare a []ColSpec[T] once; Cols and BuildRows derive everything
// else, keeping type assertions out of view code entirely.
type ColSpec[T any] struct {
	Column
	Extract func(T) string
}

// Cols projects the Column definitions out of a spec list.
func Cols[T any](specs []ColSpec[T]) []Column {
	cols := make([]Column, len(specs))
	for i, s := range specs {
		cols[i] = s.Column
	}
	return cols
}

// BuildRows renders API objects into rows using the given specs.
func BuildRows[T any](items []T, id func(T) string, specs []ColSpec[T]) []Row {
	rows := make([]Row, 0, len(items))
	for _, item := range items {
		cells := make([]string, len(specs))
		for i, s := range specs {
			cells[i] = s.Extract(item)
		}
		rows = append(rows, Row{ID: id(item), Cells: cells, Data: item})
	}
	return rows
}

// MatchesFilter reports whether any cell contains the filter text
// (case-insensitive substring). An empty filter matches everything.
func (r Row) MatchesFilter(filter string) bool {
	if filter == "" {
		return true
	}
	for _, cell := range r.Cells {
		if containsFold(cell, filter) {
			return true
		}
	}
	return false
}

// containsFold is a case-insensitive strings.Contains without allocating
// lowered copies on the hot path for the common ASCII case.
func containsFold(s, substr string) bool {
	n := len(substr)
	if n == 0 {
		return true
	}
	if len(s) < n {
		return false
	}
	for i := 0; i+n <= len(s); i++ {
		if equalFoldASCII(s[i:i+n], substr) {
			return true
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
