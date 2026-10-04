package errs

// Block is one allocated range of error-code numbers and the single category
// every code in it carries (D13 of the P7-REG Epic, the canonical allocation).
//
// Purpose: the category of a code is a function of its number; Register fills
// an empty Category from this table and rejects a mismatch.
//
// Constraints: a new block needs a line in the Epic allocation table first;
// blocks never overlap and never reach the reserved external block.
type Block struct {
	Lo, Hi   int
	Category string
}

// Owner is a sub-range of a block assigned to one Ticket, kept as data so a
// test proves the ranges are disjoint and nested inside one block.
type Owner struct {
	Lo, Hi int
	Who    string // Epic or Ticket id, e.g. "P7-PLUG-01"
}

// Reserved external block: the ci plugin keeps its own registry here
// (plugins:free/ci/internal/model). A cli fragment in this range is a
// registry problem.
const (
	reservedLo = 600
	reservedHi = 719
)

// Blocks is the ordered block table.
var Blocks = []Block{
	{1, 49, "docker"},
	{50, 99, "config"},
	{100, 149, "plugin"},
	{150, 199, "ssl"},
	{200, 249, "database"},
	{250, 299, "health"},
	{300, 349, "init"},
	{350, 399, "domain"},
	{400, 449, "cli"},
	{450, 479, "reconcile"},
	{480, 499, "deploy"},
	{500, 529, "adopt"},
	{540, 549, "secret"},
}

// Owners lists the per-Ticket sub-ranges fixed by each owner Epic. Spare and
// free numbers are deliberately absent. Sorted by Lo.
var Owners = []Owner{
	{1, 5, "existing"}, {50, 56, "existing"}, {57, 59, "P7-REG-01"},
	{60, 61, "P7-TRUTH-12"},
	{100, 105, "existing"}, {106, 110, "P7-REG-01"},
	{111, 114, "P7-PLUG-01"}, {115, 117, "P7-PLUG-09"}, {118, 120, "P7-PLUG-59"},
	{121, 124, "P7-PLUG-23"}, {125, 126, "P7-PLUG-24"}, {127, 127, "P7-PLUG-32"},
	{128, 131, "P7-PLUG-17"}, {132, 132, "P7-PLUG-64"}, {133, 134, "P7-PLUG-20"},
	{135, 136, "P7-PLUG-19"}, {137, 137, "P7-PLUG-25"}, {138, 138, "P7-PLUG-60"},
	{139, 139, "P7-PLUG-31"},
	{150, 151, "existing"},
	{200, 202, "existing"}, {203, 216, "P7-REG-01"},
	{217, 221, "P7-PROD-07"}, {222, 224, "P7-PROD-08"},
	{250, 251, "existing"}, {252, 252, "P7-REG-01"},
	{300, 301, "existing"}, {350, 351, "existing"},
	{400, 404, "P7-REG-01"},
	{405, 407, "P7-CANON-01"}, {410, 410, "P7-CANON-21"},
	{420, 423, "P7-SURF-24"}, {424, 426, "P7-SURF-25"}, {428, 432, "P7-SURF-02"},
	{433, 433, "P7-SURF-03"}, {434, 434, "P7-SURF-06"}, {435, 436, "P7-SURF-07"},
	{450, 454, "P7-LIVE-03"}, {455, 459, "P7-LIVE-09"}, {460, 464, "P7-LIVE-13"},
	{465, 469, "P7-LIVE-18"}, {470, 474, "P7-LIVE-22"},
	{480, 482, "P7-DEPL-01"}, {483, 483, "P7-DEPL-12"}, {484, 487, "P7-DEPL-13"},
	{488, 490, "P7-DEPL-15"}, {491, 491, "P7-DEPL-18"}, {492, 499, "P7-DEPL-19"},
	{500, 502, "P7-ADOPT-01"}, {503, 505, "P7-ADOPT-03"}, {506, 506, "P7-ADOPT-21"},
	{507, 508, "P7-ADOPT-06"}, {509, 512, "P7-ADOPT-22"}, {513, 514, "P7-ADOPT-07"},
	{515, 516, "P7-ADOPT-23"}, {517, 524, "P7-ADOPT-10"}, {525, 527, "P7-ADOPT-17"},
	{528, 528, "P7-ADOPT-01"}, {529, 529, "P7-ADOPT-10"},
	{540, 544, "P7-TRUST-03"},
}

// blockFor returns the block that contains code number n.
func blockFor(n int) (Block, bool) {
	for _, b := range Blocks {
		if n >= b.Lo && n <= b.Hi {
			return b, true
		}
	}
	return Block{}, false
}
