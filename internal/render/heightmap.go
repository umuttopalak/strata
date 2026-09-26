package render

import (
	"hash/fnv"
	"math"
)

// Terrain is the silhouette of a mountain range sampled at evenly spaced
// points. It knows nothing about terminals or pixels.
type Terrain struct {
	Heights []float64 // 0..1 at each sample
	Owner   []int     // index into the peaks of the mountain on top, -1 if none
	Centers []float64 // sample position of each peak's summit
}

// decades is how many orders of magnitude below the tallest mountain are
// still drawn above the minimum: at 4, a folder with a tenth of the lines
// stands at 3/4 of the height and one with a hundredth at half.
const decades = 4

// minHeight keeps tiny folders visible as foothills.
const minHeight = 0.04

// Scale maps a line count to a height in 0..1 on a log scale relative to
// ceiling, so folders that differ by orders of magnitude all stay readable.
func Scale(v, ceiling int64) float64 {
	if v <= 0 || ceiling <= 0 {
		return 0
	}
	h := 1 + math.Log10(float64(v)/float64(ceiling))/decades
	return min(max(h, minHeight), 1)
}

// Shape lays the peaks out left to right in equal slots and raises a
// mountain in each one. Where mountains overlap, the taller slope wins.
//
// height is the drawing's full height measured in the same units as one
// sample's width. It keeps slopes from getting steeper than a real mountain
// (a tall peak in a narrow slot would otherwise look like a tower), so tall
// mountains spread over their neighbours' slots, up to maxSpread.
func Shape(peaks []Peak, ceiling int64, samples int, height float64) Terrain {
	t := Terrain{
		Heights: make([]float64, samples),
		Owner:   make([]int, samples),
		Centers: make([]float64, len(peaks)),
	}
	for x := range t.Owner {
		t.Owner[x] = -1
	}
	if len(peaks) == 0 || samples == 0 {
		return t
	}

	slot := float64(samples) / float64(len(peaks))
	for i, p := range peaks {
		h := Scale(p.Value, ceiling)
		c := (float64(i) + 0.5) * slot
		t.Centers[i] = c
		if h == 0 {
			continue
		}
		half := max(slot*0.4, min(h*height*slope, slot*maxSpread))
		seed := ridgeSeed(p.Name)
		lo := max(0, int(math.Floor(c-half)))
		hi := min(samples-1, int(math.Ceil(c+half)))
		for x := lo; x <= hi; x++ {
			// Sample at the middle of each cell so the profile is symmetric.
			d := (float64(x) + 0.5 - c) / half
			v := h * hill(d) * (1 + ridge(float64(x), seed)*math.Abs(d))
			if v > t.Heights[x] {
				t.Heights[x], t.Owner[x] = min(v, 1), i
			}
		}
	}
	return t
}

// slope is a mountain's half-width relative to its height.
const slope = 0.55

// maxSpread caps a mountain's half-width in slots, so neighbours of similar
// height still leave a valley between them instead of fusing into a wall.
const maxSpread = 0.9

// hill is a smooth bump: 1 at d=0 falling to 0 at |d|=1, steeper than a
// cosine bell near the base so it reads as a mountain rather than a dune.
func hill(d float64) float64 {
	d = math.Abs(d)
	if d >= 1 {
		return 0
	}
	c := (1 + math.Cos(math.Pi*d)) / 2
	return c * math.Sqrt(c)
}

// ridge adds a little deterministic unevenness to the slopes. It is zero at
// the summit (scaled by |d| in Shape), so peak heights stay exact, and it
// depends only on the peak's name and position, so it does not flicker.
func ridge(x, seed float64) float64 {
	return 0.10*math.Sin(x*0.9+seed) + 0.05*math.Sin(x*2.3+seed*1.7)
}

func ridgeSeed(name string) float64 {
	h := fnv.New32a()
	h.Write([]byte(name))
	return float64(h.Sum32()%6283) / 1000
}
