// Package render provides token-efficient text formats for CLI output.
//
// The CLI's primary consumer is now the slash-command layer in AI tools, not
// a human at a TTY. The functions here emit compact, parseable text that:
//
//   - has stable field names so the model can grep for `id:` / `title:` etc.
//   - omits decorative borders, color codes, spinners, and progress noise
//   - omits null/empty fields rather than rendering `null`
//   - truncates long lines to a fixed width (default 80) without breaking columns
//
// For interactive TTY use, callers should branch on ui.IsInteractive() and use
// the richer renderers in internal/ui (panels, spinners, ASCII art).
package render

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// DefaultWidth is the assumed column width when stdout is not a TTY.
// Wider terminals can override via WidthFromTerminal.
const DefaultWidth = 80

// Field is one row in a Record.
type Field struct {
	Key   string // lowercase, single-word ideally
	Value string // multi-line allowed; rendered verbatim
}

// Record renders a single domain object as a key:value block.
//
// Output shape:
//
//	id        task-a1b2
//	title     Add Stripe billing integration
//	scope     billing
//	priority  1
//	status    pending
//
//	description
//	  Integrate Stripe webhooks for subscription events.
//
//	ac
//	  - Webhook endpoint live
//	  - Idempotency keys prevent dupes
//
// Inline fields (single-line values) align in two columns. Block fields
// (Key with empty Value followed by lines via WriteBlock) span the full row.
type Record struct {
	w io.Writer
	// inlineKeyWidth is computed lazily from the longest inline key in Inline.
	inline []Field
}

// NewRecord creates a Record writing to w.
func NewRecord(w io.Writer) *Record { return &Record{w: w} }

// Inline appends a one-line field. Empty values are dropped.
func (r *Record) Inline(key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	r.inline = append(r.inline, Field{Key: key, Value: value})
}

// Inlinef appends a one-line field formatted via fmt.Sprintf. Empty values dropped.
func (r *Record) Inlinef(key, format string, args ...any) {
	r.Inline(key, fmt.Sprintf(format, args...))
}

// flushInline writes the buffered inline fields with aligned key column.
func (r *Record) flushInline() {
	if len(r.inline) == 0 {
		return
	}
	keyWidth := 0
	for _, f := range r.inline {
		if len(f.Key) > keyWidth {
			keyWidth = len(f.Key)
		}
	}
	for _, f := range r.inline {
		fmt.Fprintf(r.w, "%-*s  %s\n", keyWidth, f.Key, f.Value)
	}
	r.inline = nil
}

// Block writes a block-style field: a key on its own line, then indented body.
// Empty body is dropped. The body is indented by two spaces; existing newlines
// are preserved.
func (r *Record) Block(key, body string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return
	}
	r.flushInline()
	fmt.Fprintln(r.w)
	fmt.Fprintln(r.w, key)
	for _, line := range strings.Split(body, "\n") {
		fmt.Fprintf(r.w, "  %s\n", line)
	}
}

// List writes a key followed by an indented bullet list. Skips empty input.
func (r *Record) List(key string, items []string) {
	if len(items) == 0 {
		return
	}
	r.flushInline()
	fmt.Fprintln(r.w)
	fmt.Fprintln(r.w, key)
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fmt.Fprintf(r.w, "  - %s\n", item)
	}
}

// End flushes any pending inline fields. Call once at the end.
func (r *Record) End() {
	r.flushInline()
}

// Table renders rows aligned in fixed-width columns. Header is mandatory; data
// rows are added via Row(). Cells are truncated to fit within the total width.
//
// Output shape:
//
//	ID         P  STATUS    SCOPE     TITLE
//	task-a1b2  1  pending   billing   Add Stripe billing
//	task-c3d4  2  pending   auth      Rotate JWT keys
type Table struct {
	w           io.Writer
	header      []string
	rows        [][]string
	totalWidth  int
	separator   string
	rightAlign  map[int]bool // column index -> right-align
	maxColWidth int          // 0 = no per-column cap
}

// NewTable creates a table with the given header. totalWidth is the soft
// maximum row width; longer cells are truncated. Use 0 for DefaultWidth.
func NewTable(w io.Writer, header []string) *Table {
	return &Table{
		w:          w,
		header:     header,
		totalWidth: DefaultWidth,
		separator:  "  ",
		rightAlign: make(map[int]bool),
	}
}

// Width sets the soft maximum total width. Rows longer than this have the
// last column truncated.
func (t *Table) Width(w int) *Table {
	t.totalWidth = w
	return t
}

// RightAlign marks a column index as right-aligned (useful for numbers).
func (t *Table) RightAlign(col int) *Table {
	t.rightAlign[col] = true
	return t
}

// Row appends a data row. Pass strings in the same order as the header.
func (t *Table) Row(cells ...string) {
	t.rows = append(t.rows, cells)
}

// Render writes the table to the writer.
func (t *Table) Render() {
	if len(t.header) == 0 {
		return
	}

	// Compute natural column widths.
	cols := len(t.header)
	width := make([]int, cols)
	for i, h := range t.header {
		width[i] = len(h)
	}
	for _, row := range t.rows {
		for i := 0; i < cols && i < len(row); i++ {
			if n := visualWidth(row[i]); n > width[i] {
				width[i] = n
			}
		}
	}

	// If total exceeds totalWidth, truncate the last column.
	separators := len(t.separator) * (cols - 1)
	total := separators
	for _, w := range width {
		total += w
	}
	if t.totalWidth > 0 && total > t.totalWidth {
		overflow := total - t.totalWidth
		if width[cols-1]-overflow >= 6 {
			width[cols-1] -= overflow
		}
	}

	// Render header.
	t.writeRow(t.header, width)
	// Render rows.
	for _, row := range t.rows {
		t.writeRow(row, width)
	}
}

func (t *Table) writeRow(cells []string, width []int) {
	parts := make([]string, len(width))
	for i := range width {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		// Truncate to column width.
		if visualWidth(cell) > width[i] {
			cell = truncate(cell, width[i])
		}
		if t.rightAlign[i] {
			parts[i] = fmt.Sprintf("%*s", width[i], cell)
		} else {
			parts[i] = fmt.Sprintf("%-*s", width[i], cell)
		}
	}
	fmt.Fprintln(t.w, strings.TrimRight(strings.Join(parts, t.separator), " "))
}

// Hits renders a list of search/knowledge hits as compact citations.
//
// Output shape:
//
//	[D] AWS SDK v2 (replaced v1)
//	    internal/llm/aws.go · used by agents only
//
//	[D] UUID generation
//	    pkg/idgen/ · google/uuid for stringable IDs
type Hit struct {
	Type    string // single-letter or short label, e.g. "D" "F" "C" "P" "DOC"
	Title   string
	Path    string // optional source path
	Summary string // optional one-line summary
}

// RenderHits writes a list of hits to w in compact form.
func RenderHits(w io.Writer, hits []Hit) {
	for i, h := range hits {
		if i > 0 {
			fmt.Fprintln(w)
		}
		typ := h.Type
		if typ == "" {
			typ = "?"
		}
		fmt.Fprintf(w, "[%s] %s\n", typ, h.Title)
		var detail []string
		if h.Path != "" {
			detail = append(detail, h.Path)
		}
		if h.Summary != "" {
			detail = append(detail, h.Summary)
		}
		if len(detail) > 0 {
			fmt.Fprintf(w, "    %s\n", strings.Join(detail, " · "))
		}
	}
}

// Group renders a heading + list of items grouped by category. Used by
// `taskwing knowledge` to produce the same shape as the SessionStart brief.
//
// Output shape:
//
//	D Decisions (56)
//	- title 1
//	- title 2
//
//	F Features (7)
//	- title 1
type Group struct {
	Code  string // e.g. "D"
	Label string // e.g. "Decisions"
	Items []string
}

// RenderGroups writes the grouped list to w. Empty groups are skipped.
// If sortItems is true, items within each group are sorted alphabetically.
func RenderGroups(w io.Writer, groups []Group, sortItems bool) {
	for i, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "%s %s (%d)\n", g.Code, g.Label, len(g.Items))
		items := g.Items
		if sortItems {
			items = append([]string(nil), g.Items...)
			sort.Strings(items)
		}
		for _, item := range items {
			fmt.Fprintf(w, "- %s\n", item)
		}
	}
}

// truncate trims s to width, adding an ellipsis if truncation occurred.
// Width is measured in runes.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// visualWidth approximates the printed width of s in cells. ASCII-only for
// now; good enough for the slugs/titles/paths we render.
func visualWidth(s string) int {
	return len([]rune(s))
}
