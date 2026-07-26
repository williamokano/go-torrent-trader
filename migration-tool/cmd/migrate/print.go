package main

import (
	"fmt"
	"io"
	"text/tabwriter"
)

// printer writes a report to an io.Writer and remembers the first failure.
//
// Report-building code is a long sequence of writes where every error is the
// same error — the output went away. Checking each one would treble the length
// of the code and hide what it prints, so the failure is collected and checked
// once, at the end.
type printer struct {
	w   io.Writer
	err error
}

func newPrinter(w io.Writer) *printer { return &printer{w: w} }

func (p *printer) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}

func (p *printer) println(args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintln(p.w, args...)
}

// table returns a printer that aligns columns, and a flush function that must
// be called before writing anything else to the underlying writer.
func (p *printer) table() (*printer, func()) {
	tw := tabwriter.NewWriter(p.w, 0, 0, 2, ' ', 0)
	tp := &printer{w: tw}
	return tp, func() {
		if tp.err == nil {
			tp.err = tw.Flush()
		}
		if p.err == nil {
			p.err = tp.err
		}
	}
}

// err returns the first write failure, wrapped so the caller knows what was
// being written when it happened.
func (p *printer) errf(what string) error {
	if p.err == nil {
		return nil
	}
	return fmt.Errorf("writing %s: %w", what, p.err)
}
