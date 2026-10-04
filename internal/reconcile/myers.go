package reconcile

import "strings"

// maxTraceDistance bounds the memory of an edit script: the Myers trace holds
// about D*D ints for D inserted+deleted lines. Past it, only the distance is
// computed (O(N) memory) and Unified prints a placeholder instead of hunks.
const maxTraceDistance = 2000

// editOp is one step of an edit script. Tag is '=', '-' or '+'; A and B are
// the count of a-lines and b-lines consumed before this op (0-based indexes
// of the line it refers to on the side(s) it touches).
type editOp struct {
	Tag  byte
	A, B int
}

// splitLines splits data into lines that keep their trailing "\n", so a file
// that differs only by a missing final newline still differs. Empty data is
// zero lines.
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// countLines returns the number of lines splitLines yields.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := 0
	for _, c := range data {
		if c == '\n' {
			n++
		}
	}
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// intern maps both line slices to int ids so the diff compares ints.
func intern(a, b []string) ([]int, []int) {
	ids := make(map[string]int, len(a)+len(b))
	conv := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, l := range lines {
			id, ok := ids[l]
			if !ok {
				id = len(ids)
				ids[l] = id
			}
			out[i] = id
		}
		return out
	}
	return conv(a), conv(b)
}

// myers runs the O(ND) Myers shortest-edit search over a and b.
//
// Inputs: id slices; wantScript asks for the edit script too. Outputs: D, the
// number of deleted plus inserted lines, and (when wantScript and D is within
// maxTraceDistance) the script in forward order. The script is nil, with D
// still exact, when the trace would exceed the bound.
func myers(a, b []int, wantScript bool) (int, []editOp) {
	n, m := len(a), len(b)
	max := n + m
	off := max + 1
	v := make([]int, 2*max+3)
	var trace [][]int
	for d := 0; d <= max; d++ {
		if wantScript && d <= maxTraceDistance {
			lo := off - d - 1
			trace = append(trace, append([]int(nil), v[lo:off+d+2]...))
		} else if wantScript {
			wantScript, trace = false, nil
		}
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				if !wantScript {
					return d, nil
				}
				return d, backtrack(trace, n, m)
			}
		}
	}
	return max, nil
}

// backtrack rebuilds the forward edit script from the saved per-round states.
func backtrack(trace [][]int, n, m int) []editOp {
	var rev []editOp
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		get := func(k int) int { return v[k+d+1] } // snapshot base is -d-1
		k := x - y
		var pk int
		if k == -d || (k != d && get(k-1) < get(k+1)) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := get(pk)
		py := px - pk
		if d == 0 {
			px, py = 0, 0
		}
		for x > px && y > py {
			x--
			y--
			rev = append(rev, editOp{'=', x, y})
		}
		if d > 0 {
			if x == px {
				y--
				rev = append(rev, editOp{'+', x, y})
			} else {
				x--
				rev = append(rev, editOp{'-', x, y})
			}
		}
		x, y = px, py
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}
