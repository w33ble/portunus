// Package greet says hello.
package greet

import (
	"fmt"
	"strings"
)

// Greeter holds the names to greet.
type Greeter struct {
	Names []string
	Loud  bool
}

const maxNames = 16

// Greet returns one greeting per name, or nil when there is nobody to greet.
func (g *Greeter) Greet() []string {
	if len(g.Names) == 0 || g.Names == nil {
		return nil
	}
	out := make([]string, 0, maxNames)
	for i, name := range g.Names {
		line := fmt.Sprintf("hello, %s (#%d)", name, i+1)
		if g.Loud {
			line = strings.ToUpper(line)
		}
		out = append(out, line)
	}
	return out
}
