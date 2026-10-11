package plan

// render.go — the human form of a Plan.
//
// Purpose: print the old to new table and the step list.
// Inputs: a Plan and a writer.
// Outputs: text; an unresolvable pin shows "unresolvable" and its reason.
// Constraints: no colour and no terminal probing, so output is stable in logs.

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// Render writes the pin table and the step list to w.
func Render(w io.Writer, p Plan) error {
	if len(p.Pins) == 0 {
		if _, err := fmt.Fprintln(w, "No pin changes: the CLI, images and plugins match what is on offer."); err != nil {
			return err
		}
	} else {
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "KIND\tNAME\tFROM\tTO\tNOTE")
		for _, pin := range p.Pins {
			to, note := pin.To, ""
			if !pin.Resolvable {
				to, note = "unresolvable", pin.Reason
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", pin.Kind, pin.Name, pin.From, to, note)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nSteps:"); err != nil {
		return err
	}
	for i, s := range p.Steps {
		if _, err := fmt.Fprintf(w, "  %d. %-9s %-5s %s", i+1, s.Name, s.Kind, s.Command); err != nil {
			return err
		}
		if len(s.Pins) > 0 {
			if _, err := fmt.Fprintf(w, " (%d pin(s))", len(s.Pins)); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}
