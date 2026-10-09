package main

import (
	"fmt"
	"io"
)

type report struct {
	out io.Writer
}

func (r report) intro(c counter) {
	fmt.Fprintf(r.out, "Increase concurrently: %d clients × %d increments on %s\n", c.cfg.clients, c.cfg.increments, c.keyURL)
	fmt.Fprintln(r.out, "each client: GET → PUT ?ifVersion=<version read>, retry on 409")
	fmt.Fprintln(r.out)
}

func (r report) increased(c counter, results []clientResult) {
	ok := 0
	for i, res := range results {
		ok += res.ok
		fmt.Fprintf(r.out, "client %d: %d ok, %d conflicts (409, retried)\n", i+1, res.ok, res.conflicts)
	}
	conflicts := totalConflicts(results)
	fmt.Fprintf(r.out, "total:    %d ok, %d conflicts\n", ok, conflicts)
	if conflicts == 0 {
		fmt.Fprintln(r.out, "warning: 0 conflicts, so the clients never collided; rerun or raise -clients")
	}
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "Verify:")
	fmt.Fprintf(r.out, "   curl %s\n", c.keyURL)
	fmt.Fprintf(r.out, "   expect count=%d, version=%d (seed + %d increments)\n", ok, ok+1, ok)
}
