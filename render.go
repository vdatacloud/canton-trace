package cantontrace

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Reader reads a committed transaction as a set of parties see it.
// [GRPCReader] and [JSONReader] implement it.
type Reader interface {
	Read(ctx context.Context, updateID string, parties []string) (*Trace, error)
}

// WriteText writes the trace as an indented tree, one node per line, for a
// terminal or a log a person reads. Arguments and results are left out; the
// JSON form carries them.
func WriteText(w io.Writer, t *Trace) error {
	if t == nil {
		return nil
	}
	head := t.UpdateID
	if head == "" {
		head = "(" + string(t.Source) + ")"
	}
	if _, err := fmt.Fprintf(w, "%s  %d nodes\n", head, t.Count()); err != nil {
		return err
	}
	var err error
	t.Walk(func(n *Node, depth int) bool {
		if err != nil {
			return false
		}
		_, err = fmt.Fprintf(w, "%s%s\n", strings.Repeat("  ", depth+1), n.Line())
		return true
	})
	return err
}

// Line is a one-line summary of the node: its id, kind, template entity and
// choice, and contract id.
func (n *Node) Line() string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s %s", n.NodeID, n.Kind, entity(n.TemplateID))
	if n.Choice != "" {
		b.WriteString("." + n.Choice)
		if n.InterfaceID != "" {
			b.WriteString(" via " + entity(n.InterfaceID))
		}
		if n.Consuming {
			b.WriteString(" (consuming)")
		}
	}
	if n.ContractID != "" {
		b.WriteString(" " + ShortID(n.ContractID))
	}
	return b.String()
}

// entity is the "Module:Entity" part of "package:Module:Entity".
func entity(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[i+1:]
	}
	return id
}

// ShortID shortens a contract id or a party's fingerprint for display:
// "00abcdef…" keeps 10 characters; "name::1220abcd…" keeps the name and 8
// characters of the fingerprint. JSON output always carries full ids.
func ShortID(id string) string {
	if name, fp, ok := strings.Cut(id, "::"); ok {
		if len(fp) > 8 {
			fp = fp[:8] + "…"
		}
		return name + "::" + fp
	}
	if len(id) > 10 {
		return id[:10] + "…"
	}
	return id
}
