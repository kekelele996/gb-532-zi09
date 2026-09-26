package geometry

import (
	"math"
	"testing"

	"github.com/paulmach/orb"
)

func TestPointInsideOrOnBoundary(t *testing.T) {
	boundary := orb.Polygon{orb.Ring{{0, 0}, {100, 0}, {100, 100}, {0, 100}, {0, 0}}}
	cases := []struct {
		name  string
		point orb.Point
		want  bool
	}{
		{"interior", orb.Point{50, 50}, true},
		{"corner", orb.Point{0, 0}, true},
		{"on edge", orb.Point{100, 30}, true},
		{"outside x", orb.Point{101, 50}, false},
		{"outside y", orb.Point{50, -1}, false},
		{"far outside", orb.Point{500, 500}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointInsideOrOnBoundary(boundary, tc.point); got != tc.want {
				t.Fatalf("PointInsideOrOnBoundary(%v) = %v, want %v", tc.point, got, tc.want)
			}
		})
	}
}

func TestPointInsideExcludesHole(t *testing.T) {
	boundary := orb.Polygon{
		orb.Ring{{0, 0}, {100, 0}, {100, 100}, {0, 100}, {0, 0}},
		orb.Ring{{40, 40}, {60, 40}, {60, 60}, {40, 60}, {40, 40}},
	}
	if PointInsideOrOnBoundary(boundary, orb.Point{50, 50}) {
		t.Fatal("point inside hole must be outside survey area")
	}
	if !PointInsideOrOnBoundary(boundary, orb.Point{40, 50}) {
		t.Fatal("point on hole boundary is not part of the area")
	}
	if !PointInsideOrOnBoundary(boundary, orb.Point{20, 50}) {
		t.Fatal("point between boundary and hole must be inside")
	}
}

func TestSegmentIntersection(t *testing.T) {
	cross, ok := SegmentIntersection(orb.Point{0, 0}, orb.Point{100, 100}, orb.Point{0, 100}, orb.Point{100, 0})
	if !ok {
		t.Fatal("crossing segments must intersect")
	}
	if math.Abs(cross[0]-50) > 1e-6 || math.Abs(cross[1]-50) > 1e-6 {
		t.Fatalf("intersection = %v, want (50,50)", cross)
	}
	if _, ok := SegmentIntersection(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{0, 10}, orb.Point{10, 10}); ok {
		t.Fatal("parallel disjoint segments must not intersect")
	}
	if _, ok := SegmentIntersection(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{10, 0}, orb.Point{20, 0}); !ok {
		t.Fatal("segments sharing an endpoint must intersect")
	}
	if _, ok := SegmentIntersection(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{4, 0}, orb.Point{6, 0}); !ok {
		t.Fatal("collinear overlapping segments must intersect")
	}
}

func TestOrderedDetourLineReplacesStraightSection(t *testing.T) {
	line := orb.LineString{{0, 50}, {100, 50}}
	replaced, err := OrderedDetourLine(line, orb.Point{40, 20}, orb.Point{60, 20})
	if err != nil {
		t.Fatalf("ordered detour: %v", err)
	}
	want := orb.LineString{{0, 50}, {40, 20}, {60, 20}, {100, 50}}
	if len(replaced) != len(want) {
		t.Fatalf("detour length = %d, want %d: %v", len(replaced), len(want), replaced)
	}
	for index := range want {
		if replaced[index] != want[index] {
			t.Fatalf("vertex %d = %v, want %v; full %v", index, replaced[index], want[index], replaced)
		}
	}
}

func TestOrderedDetourLineSortsVerticesByProjection(t *testing.T) {
	line := orb.LineString{{0, 50}, {100, 50}}
	// 折点输入顺序与沿测线顺序相反时，结果仍按测线方向排列。
	replaced, err := OrderedDetourLine(line, orb.Point{60, 80}, orb.Point{40, 80})
	if err != nil {
		t.Fatalf("ordered detour reversed: %v", err)
	}
	want := orb.LineString{{0, 50}, {40, 80}, {60, 80}, {100, 50}}
	if len(replaced) != len(want) {
		t.Fatalf("detour length = %d, want %d", len(replaced), len(want))
	}
	for index := range want {
		if replaced[index] != want[index] {
			t.Fatalf("vertex %d = %v, want %v; full %v", index, replaced[index], want[index], replaced)
		}
	}
}

func TestOrderedDetourLineKeepsIntermediateVertices(t *testing.T) {
	line := orb.LineString{{0, 0}, {50, 0}, {100, 0}}
	replaced, err := OrderedDetourLine(line, orb.Point{60, 20}, orb.Point{80, 20})
	if err != nil {
		t.Fatalf("ordered detour: %v", err)
	}
	if len(replaced) != 5 {
		t.Fatalf("detour length = %d, want 5 (endpoints, kink and two vertices): %v", len(replaced), replaced)
	}
	if replaced[1] != (orb.Point{50, 0}) {
		t.Fatalf("original intermediate vertex must be retained at index 1, got %v", replaced[1])
	}
	if replaced[2] != (orb.Point{60, 20}) || replaced[3] != (orb.Point{80, 20}) {
		t.Fatalf("detour vertices ordered incorrectly: %v", replaced)
	}
}
