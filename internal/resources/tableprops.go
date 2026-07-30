package resources

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// propTree turns the flat TBLPROPERTIES map into a tree for the properties tab.
// Property keys are dot-paths, so `delta.feature.appendOnly` nests under
// delta → feature and the whole analyze-generated
// `spark.sql.statistics.colStats.*` block — hundreds of entries on a wide table
// — collapses to a single line. Siblings sort alphabetically at every level,
// which also sinks that `spark.*` block below the properties a human set.
func propTree(props map[string]string) []view.TreeNode {
	root := &propNode{kids: map[string]*propNode{}}
	for key, value := range props {
		n := root
		for _, part := range strings.Split(key, ".") {
			kid, ok := n.kids[part]
			if !ok {
				kid = &propNode{kids: map[string]*propNode{}}
				n.kids[part] = kid
			}
			n = kid
		}
		n.value = value
	}
	return root.nodes()
}

// epochTimeFormat is how a decoded epoch value is glossed.
const epochTimeFormat = "2006-01-02 15:04:05.000"

// epochWords are the name fragments that suggest a property holds a time.
// `_at`/`At` suffixes are handled separately — a bare "at" suffix test would
// also match innocent names like "format".
var epochWords = []string{"created", "updated", "modified", "deleted", "expire", "timestamp", "time", "date"}

// epochNote decodes a likely epoch timestamp into a readable local time, or
// returns "" to leave the value alone. Nothing about a bare integer proves it is
// a timestamp, so this needs *both* halves of the guess to hold: the property
// name has to read like a time, and the digits have to land in a plausible range
// once interpreted at the precision their count implies. `lastUpdateVersion:
// 6215` therefore stays untouched, as does a non-numeric value like
// `deltaFileStatistics: load_date`.
func epochNote(label, value string) string {
	if !looksLikeTimeName(label) {
		return ""
	}
	t, ok := parseEpoch(value)
	if !ok {
		return ""
	}
	return t.Local().Format(epochTimeFormat)
}

// looksLikeTimeName reports whether a property name reads like a timestamp.
func looksLikeTimeName(label string) bool {
	if strings.HasSuffix(label, "_at") || strings.HasSuffix(label, "At") {
		return true
	}
	lower := strings.ToLower(label)
	for _, word := range epochWords {
		if strings.Contains(lower, word) {
			return true
		}
	}
	return false
}

// epochPrecision maps a digit count to the unit those digits must be in for the
// value to be a time in the plausible range — 10 digits can only be seconds, 13
// only milliseconds, and so on. Counts outside this table (a version number, a
// row count) decode to nothing.
var epochPrecision = map[int]time.Duration{
	10: time.Second, 13: time.Millisecond, 16: time.Microsecond, 19: time.Nanosecond,
}

// Plausible bounds for a decoded timestamp: no Databricks table property
// predates Unity Catalog, and nothing legitimately sits a century out.
var (
	epochMin = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	epochMax = time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
)

// parseEpoch decodes an all-digit value as an epoch timestamp at the precision
// its length implies, rejecting anything outside the plausible range.
func parseEpoch(value string) (time.Time, bool) {
	unit, ok := epochPrecision[len(value)]
	if !ok {
		return time.Time{}, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	t := time.Unix(0, n*int64(unit))
	if t.Before(epochMin) || t.After(epochMax) {
		return time.Time{}, false
	}
	return t, true
}

// propNode is the intermediate trie used to group dotted keys.
type propNode struct {
	value string
	kids  map[string]*propNode
}

// nodes converts the trie's children into sorted view nodes.
func (n *propNode) nodes() []view.TreeNode {
	if len(n.kids) == 0 {
		return nil
	}
	labels := make([]string, 0, len(n.kids))
	for label := range n.kids {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	out := make([]view.TreeNode, 0, len(labels))
	for _, label := range labels {
		kid := n.kids[label]
		out = append(out, view.TreeNode{
			Label:    label,
			Value:    kid.value,
			Note:     epochNote(label, kid.value),
			Children: kid.nodes(),
		})
	}
	return out
}
