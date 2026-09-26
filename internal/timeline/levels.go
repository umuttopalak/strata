package timeline

// MinPeaks is how many mountains automatic depth aims for.
const MinPeaks = 4

// maxAutoDepth is the deepest folder level automatic depth considers.
const maxAutoDepth = 4

// coarse returns the key of a collected column at a folder level. At level
// 1 root-level files merge into RootKey; deeper, each is its own column.
func (c *columns) coarse(id, level int) string {
	if c.rootFile[id] {
		if level <= 1 {
			return RootKey
		}
		return c.names[id]
	}
	return KeyFor(c.names[id]+"/", level) // names are folders
}

// pickLevel returns the shallowest level with at least MinPeaks non-empty
// columns at the end of the history, or the level with the most.
func (c *columns) pickLevel(final []int64) int {
	best, bestN := 1, -1
	for level := 1; level <= maxAutoDepth; level++ {
		sums := map[string]int64{}
		for id := range c.names {
			sums[c.coarse(id, level)] += final[id]
		}
		n := 0
		for _, v := range sums {
			if v > 0 {
				n++
			}
		}
		if n >= MinPeaks {
			return level
		}
		if n > bestN {
			best, bestN = level, n
		}
	}
	return best
}

// coarsen merges collected columns into their keys at level, keeping the
// order of first appearance. remap[collected id] is the merged column id.
func (c *columns) coarsen(level int) (names []string, remap []int) {
	ids := map[string]int{}
	remap = make([]int, len(c.names))
	for id := range c.names {
		key := c.coarse(id, level)
		out, ok := ids[key]
		if !ok {
			out = len(names)
			ids[key] = out
			names = append(names, key)
		}
		remap[id] = out
	}
	return names, remap
}

// netTotals sums every change per collected column.
func netTotals(n int, seed []delta, records []record) []int64 {
	t := make([]int64, n)
	for _, d := range seed {
		t[d.col] += d.n
	}
	for _, r := range records {
		for _, d := range r.deltas {
			t[d.col] += d.n
		}
	}
	return t
}

func identity(n int) []int {
	r := make([]int, n)
	for i := range r {
		r[i] = i
	}
	return r
}
