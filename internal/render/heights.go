package render

import "math"

// Scale maps a line count to 0..1 on a log scale against the largest value
// in the history, so small folders stay visible next to large ones.
func Scale(v, max int64) float64 {
	if v <= 0 || max <= 0 {
		return 0
	}
	return math.Min(math.Log1p(float64(v))/math.Log1p(float64(max)), 1)
}

// peakFill is how much of the available height the tallest mountain uses.
const peakFill = 0.95

// neighbourShare is how much the smaller overlapping slopes add on top of
// the largest one: enough to join peaks into ridges, little enough to keep
// valleys between them.
const neighbourShare = 0.3

// Heights returns the silhouette of the range across width columns, in
// rows (0..rows), and which slot dominates each column (-1 where none).
//
// Each slot is a Gaussian hill spreading (width/slots)*0.55+1 columns. A
// column's height is its largest contribution plus neighbourShare of the
// others, then a small fixed texture so slopes look weathered instead of
// perfectly smooth.
//
// Slot (n-1)/2, where centreOut puts the largest folder, stands exactly in
// the middle and the others follow at equal gaps. With an even count that
// leaves one extra gap on the left, instead of pushing the largest mountain
// off centre.
func Heights(values []int64, max int64, width int, rows float64) ([]float64, []int) {
	h := make([]float64, width)
	owner := make([]int, width)
	for x := range owner {
		owner[x] = -1
	}
	n := len(values)
	if n == 0 || width == 0 {
		return h, owner
	}

	spread := float64(width)/float64(n)*0.55 + 1
	centres := slotCentres(n, width)
	peak := make([]float64, n)
	for i, v := range values {
		peak[i] = Scale(v, max) * rows * peakFill
	}

	for x := range width {
		var top, sum float64
		for i, p := range peak {
			if p == 0 {
				continue
			}
			d := (float64(x) + 0.5 - centres[i]) / spread
			c := p * math.Exp(-d*d)
			sum += c
			if c > top {
				top, owner[x] = c, i
			}
		}
		v := top + neighbourShare*(sum-top)
		v *= 1 + texture(x, spread)
		h[x] = math.Min(math.Max(v, 0), rows)
	}
	return h, owner
}

// slotCentres returns the column each of n slots is centred on: slot
// (n-1)/2 in the middle, the others at equal gaps (see Heights).
func slotCentres(n, width int) []float64 {
	gap := slotGap(n, width)
	mid := (n - 1) / 2
	out := make([]float64, n)
	for i := range out {
		out[i] = float64(width)/2 + float64(i-mid)*gap
	}
	return out
}

// slotGap is the distance between neighbouring slot centres.
func slotGap(n, width int) float64 {
	if n%2 == 0 {
		return float64(width) / float64(n+1)
	}
	return float64(width) / float64(n)
}

// textureSpread is the hill width the texture's wavelengths are tuned for.
// Wider hills get proportionally longer ripples: at a fixed 3–4 column
// period a broad summit would look like battlements rather than rock.
const textureSpread = 6.0

// texture is a fixed ±6% ripple; it depends only on the column and the
// layout, so it does not flicker between frames.
func texture(x int, spread float64) float64 {
	u := float64(x) * textureSpread / math.Max(spread, textureSpread)
	return 0.035*math.Sin(u*1.7) + 0.025*math.Sin(u*0.63)
}
