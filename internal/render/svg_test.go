package render

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func svgFor(t *testing.T, o SVGOptions) string {
	t.Helper()
	cols := []string{"src", "docs", "(root)"}
	tl := timelineOf(cols,
		[]int64{0, 0, 0},
		[]int64{100, 10, 5},
		[]int64{2_000, 40, 5},
		[]int64{9_000, 300, 20},
	)
	tl.Frames[3].Caption.Subject = `fix <script> & "quotes"`
	var b bytes.Buffer
	if err := WriteSVG(&b, tl, NewLayout(tl), o); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestSVGIsWellFormed(t *testing.T) {
	s := svgFor(t, DefaultSVGOptions)
	d := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid XML: %v", err)
		}
	}
	if strings.Contains(s, "<script") {
		t.Error("commit text must be escaped, found a raw <script>")
	}
	if !strings.Contains(s, "fix &lt;script&gt; &amp; &#34;quotes&#34;") {
		t.Error("escaped caption missing")
	}
	if !strings.Contains(s, `width="832" height="540"`) {
		t.Errorf("unexpected size: %.200s", s)
	}
}

func TestSVGAnimationTiming(t *testing.T) {
	o := DefaultSVGOptions
	o.FrameDuration = time.Second
	o.MinPlay = 0
	o.Hold = 2 * time.Second
	s := svgFor(t, o)

	// 4 frames, 1 s each → 3 s of play in 3 steps, plus 2 hold steps.
	durs := regexp.MustCompile(`dur="([^"]+)"`).FindAllStringSubmatch(s, -1)
	if len(durs) == 0 {
		t.Fatal("no animations")
	}
	for _, d := range durs {
		if d[1] != "5.00s" {
			t.Fatalf("dur = %s, want 5.00s", d[1])
		}
	}
	for _, m := range regexp.MustCompile(`type="translate" values="([^"]+)"`).FindAllStringSubmatch(s, -1) {
		if n := strings.Count(m[1], ";") + 1; n != 6 {
			t.Fatalf("translate has %d values, want 4 keyframes + 2 hold", n)
		}
	}
	// No scripts: GitHub strips them and they would not run in <img> anyway.
	if strings.Contains(s, "<script") || strings.Contains(s, "onload") {
		t.Error("SVG must not contain scripts")
	}
}

func TestSVGShortHistoryPlaysAtLeastMinPlay(t *testing.T) {
	o := DefaultSVGOptions
	o.Hold = 0
	s := svgFor(t, o) // 3 steps × 75 ms would be a blink
	if !strings.Contains(s, `dur="6.00s"`) {
		t.Errorf("want the 6 s minimum play time, got %v",
			regexp.MustCompile(`dur="[^"]+"`).FindString(s))
	}
	if got := o.PlayTime(4); got != 6*time.Second {
		t.Errorf("PlayTime(4) = %v", got)
	}
}

func TestSVGStaticFallbackShowsFinalFrame(t *testing.T) {
	s := svgFor(t, DefaultSVGOptions)
	visible := regexp.MustCompile(`<g visibility="visible">`).FindAllStringIndex(s, -1)
	if len(visible) != 1 {
		t.Fatalf("%d caption groups visible without animation, want exactly the last", len(visible))
	}
	if !strings.Contains(s[visible[0][0]:], "commit 3/3") {
		t.Error("the visible caption group should be the final commit")
	}
}

func TestSampleFrames(t *testing.T) {
	if got := sampleFrames(5, 10); !slices.Equal(got, []int{0, 1, 2, 3, 4}) {
		t.Errorf("short: %v", got)
	}
	got := sampleFrames(301, 5)
	if !slices.Equal(got, []int{0, 75, 150, 225, 300}) {
		t.Errorf("sampled: %v", got)
	}
}

func TestSVGRejectsTinySize(t *testing.T) {
	o := DefaultSVGOptions
	o.Height = 3
	tl := timelineOf([]string{"a"}, []int64{0}, []int64{1})
	if err := WriteSVG(io.Discard, tl, NewLayout(tl), o); err == nil {
		t.Error("expected an error for a size with no room for mountains")
	}
}

func TestFinishedText(t *testing.T) {
	one := timelineOf([]string{"a"}, []int64{0}, []int64{1})
	if got := FinishedText(one); got != "✓ done · 1 commit" {
		t.Errorf("one commit: %q", got)
	}
	three := timelineOf([]string{"a"}, []int64{0}, []int64{1}, []int64{2}, []int64{3})
	if got := FinishedText(three); got != "✓ done · 3 commits" {
		t.Errorf("three commits: %q", got)
	}
}

func TestMessageSVG(t *testing.T) {
	var b bytes.Buffer
	if err := WriteMessageSVG(&b, "owner/<repo>", "repository not found", "or it is private"); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	if err := xml.Unmarshal([]byte(s), new(struct{})); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	if !strings.Contains(s, "owner/&lt;repo&gt;") || !strings.Contains(s, "or it is private") {
		t.Errorf("unexpected content: %s", s)
	}
}
