package main

import (
	"fmt"
	"io"
)

type report struct {
	out io.Writer
}

func (r report) intro(c counter) {
	fmt.Fprintf(r.out, "Increase concurrently without ifVersion: %d clients × %d increments on %s\n", c.cfg.clients, c.cfg.increments, c.keyURL)
	fmt.Fprintln(r.out, "each client: GET → PUT (no ifVersion), so there is never a 409 to retry")
	fmt.Fprintln(r.out)
}

func (r report) increased(c counter) {
	writes := c.cfg.clients * c.cfg.increments
	fmt.Fprintf(r.out, "done: %d writes, every one answered 200\n", writes)
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "Verify:")
	fmt.Fprintf(r.out, "   curl %s\n", c.keyURL)
	fmt.Fprintf(r.out, "   expect version=%d (every write was accepted) but count < %d (lost updates)\n", writes+1, writes)
}
