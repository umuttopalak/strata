package player

// key is a playback command read from the keyboard.
type key int

const (
	keyNone key = iota
	keyPause
	keyBack
	keyForward
	keyFaster
	keySlower
	keyQuit
	keyInterrupt // Ctrl+C: raw mode delivers it as a byte, not a signal
)

// parseKeys turns raw terminal input into commands. Arrow keys arrive as
// ESC [ C / ESC [ D (or ESC O C / ESC O D in application mode); anything
// unrecognised is skipped.
func parseKeys(b []byte) []key {
	var keys []key
	for i := 0; i < len(b); i++ {
		switch c := b[i]; c {
		case ' ', 'p':
			keys = append(keys, keyPause)
		case '+', '=':
			keys = append(keys, keyFaster)
		case '-', '_':
			keys = append(keys, keySlower)
		case 'q', 'Q':
			keys = append(keys, keyQuit)
		case 3:
			keys = append(keys, keyInterrupt)
		case 'h':
			keys = append(keys, keyBack)
		case 'l':
			keys = append(keys, keyForward)
		case 0x1b:
			if i+2 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
				switch b[i+2] {
				case 'D':
					keys = append(keys, keyBack)
				case 'C':
					keys = append(keys, keyForward)
				}
				i += 2
			}
		}
	}
	return keys
}

// speeds are the steps + and - move through.
var speeds = []float64{0.25, 0.5, 1, 2, 4, 8, 16}

// nextSpeed returns the speed one step faster (dir 1) or slower (dir -1).
func nextSpeed(cur float64, dir int) float64 {
	i := 0
	for i < len(speeds)-1 && speeds[i] < cur {
		i++
	}
	// cur may sit between two steps (a --speed of 3): snap towards dir.
	if speeds[i] != cur && dir < 0 {
		return speeds[max(i-1, 0)]
	}
	if speeds[i] != cur && dir > 0 {
		return speeds[i]
	}
	return speeds[min(max(i+dir, 0), len(speeds)-1)]
}
