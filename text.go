// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"fmt"
	"regexp"
	"strings"
)

var errorLine = regexp.MustCompile(`(?i)\b(error|fatal|panic|failed|failure|exception|traceback)\b`)

// compressText dedups repeated lines, then keeps head and tail lines
func compressText(s string, c *pruneCtx) string {
	lines := strings.Split(s, "\n")
	if c.lim.Dedup {
		lines = dedupLines(lines)
	}
	if c.lim.MaxLines > 0 && len(lines) > c.lim.MaxLines {
		// a trailing newline is not a line: keep it out of the budget
		trailing := lines[len(lines)-1] == ""
		if trailing {
			lines = lines[:len(lines)-1]
		}
		if len(lines) > c.lim.MaxLines {
			lines = capLines(lines, c)
		}
		if trailing {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

// dedupLines collapses runs of 3+ identical lines, losslessly labelled
func dedupLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		j := i
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		out = append(out, lines[i])
		if run := j - i; run >= 3 {
			out = append(out, fmt.Sprintf("[isthmos: previous line repeated %d more times]", run-1))
		} else {
			out = append(out, lines[i+1:j]...)
		}
		i = j
	}
	return out
}

// capLines keeps head, tail, and error-looking lines, marking the cut
func capLines(t []string, c *pruneCtx) []string {
	lim := c.lim
	keepLast := lim.KeepLast
	if keepLast >= lim.MaxLines {
		keepLast = lim.MaxLines - 1
	}
	tailStart := len(t) - keepLast
	keep := make([]bool, len(t))
	for i := 0; i < lim.MaxLines-keepLast; i++ {
		keep[i] = true
	}
	for i := tailStart; i < len(t); i++ {
		keep[i] = true
	}
	dropped := 0
	for i, ln := range t {
		if !keep[i] && errorLine.MatchString(ln) {
			keep[i] = true
		} else if !keep[i] {
			dropped++
		}
	}
	if dropped == 0 {
		return t
	}
	c.truncated = true
	out := make([]string, 0, len(t)-dropped+1)
	for i := 0; i < tailStart; i++ {
		if keep[i] {
			out = append(out, t[i])
		}
	}
	out = append(out, fmt.Sprintf("[isthmos: %d of %d lines truncated%s]", dropped, len(t), c.hint()))
	out = append(out, t[tailStart:]...)
	return out
}
