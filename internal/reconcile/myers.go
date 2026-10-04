package reconcile

import "strings"

// maxTraceDistance bounds the memory of an edit script: the Myers trace holds
// about D*D ints for D inserted+deleted lines. Past it, only the distance is
// computed (O(N) memory) and Unified prints a placeholder instead of hunks.
const maxTraceDistance = 2000

// maxEditDistance bounds the time of a diff: the search is O((N+M)*D) and D can
// reach N+M for two unrelated files. Past this many inserted plus deleted lines
// myers gives up and reports -1, so a 20,000-line file replaced wholesale costs
// about 0.1 s instead of seconds. Common prefix and suffix lines are trimmed
// first, so a large file with a small change is cheap regardless.
const maxEditDistance = 5000

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
// number of deleted plus inserted lines, or -1 when D exceeds maxEditDistance,
// and (when wantScript and D is within maxTraceDistance) the script in forward
// order. The script is nil, with D still exact, when the trace would exceed
// its bound. Common leading and trailing lines are trimmed before the search.
func myers(a, b []int, wantScript bool) (int, []editOp) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	d, mid := myersCore(a[p:len(a)-s], b[p:len(b)-s], wantScript)
	if d < 0 || (wantScript && mid == nil && d > 0) {
		return d, nil
	}
	if !wantScript {
		return d, nil
	}
	ops := make([]editOp, 0, p+len(mid)+s)
	for i := 0; i < p; i++ {
		ops = append(ops, editOp{'=', i, i})
	}
	for _, op := range mid {
		ops = append(ops, editOp{op.Tag, op.A + p, op.B + p})
	}
	for i := 0; i < s; i++ {
		ops = append(ops, editOp{'=', len(a) - s + i, len(b) - s + i})
	}
	return d, ops
}

// myersCore is the search proper, over already-trimmed slices.
func myersCore(a, b []int, wantScript bool) (int, []editOp) {
	n, m := len(a), len(b)
	max := n + m
	limit := min(max, maxEditDistance)
	off := max + 1
	v := make([]int, 2*max+3)
	var trace [][]int
	for d := 0; d <= limit; d++ {
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
	if limit < max {
		return -1, nil
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
