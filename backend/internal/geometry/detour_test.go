package geometry

import (
	"testing"

	"github.com/paulmach/orb"
)

func TestPolygonContainsStrictInterior(t *testing.T) {
	boundary := orb.Polygon{orb.Ring{{0, 0}, {100, 0}, {100, 100}, {0, 100}, {0, 0}}}
	if !PolygonContains(boundary, orb.Point{50, 50}) {
		t.Fatal("interior point must be inside")
	}
	if PolygonContains(boundary, orb.Point{50, 0}) {
		t.Fatal("point on the boundary must be outside")
	}
	if PolygonContains(boundary, orb.Point{101, 50}) {
		t.Fatal("exterior point must be outside")
	}
}

func TestApplyDetourOrdersVerticesAndKeepsNeighbours(t *testing.T) {
	lines := []orb.LineString{{{0, 20}, {100, 20}}, {{0, 80}, {100, 80}}}
	// Vertices submitted in reverse projection order.
	result, err := ApplyDetour(lines, 0, [2]orb.Point{{80, 50}, {20, 50}})
	if err != nil {
		t.Fatalf("apply detour: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("line count = %d, want 2", len(result))
	}
	want := orb.LineString{{0, 20}, {20, 50}, {80, 50}, {100, 20}}
	for index, point := range want {
		if result[0][index] != point {
			t.Fatalf("detoured line[%d] = %v, want %v", index, result[0][index], point)
		}
	}
	if result[1][0] != (orb.Point{0, 80}) || result[1][1] != (orb.Point{100, 80}) {
		t.Fatalf("neighbour line changed: %v", result[1])
	}
}

func TestApplyDetourRejectsDuplicateAndEndpointVertices(t *testing.T) {
	lines := []orb.LineString{{{0, 20}, {100, 20}}}
	if _, err := ApplyDetour(lines, 0, [2]orb.Point{{40, 50}, {40, 50}}); err == nil {
		t.Fatal("duplicate vertices must be rejected")
	}
	if _, err := ApplyDetour(lines, 0, [2]orb.Point{{0, 20}, {40, 50}}); err == nil {
		t.Fatal("vertex coinciding with line endpoint must be rejected")
	}
	if _, err := ApplyDetour(lines, 4, [2]orb.Point{{40, 50}, {60, 50}}); err == nil {
		t.Fatal("out-of-range line index must be rejected")
	}
}

func TestFindDetourConflict(t *testing.T) {
	boundary := orb.Polygon{orb.Ring{{0, 0}, {100, 0}, {100, 100}, {0, 100}, {0, 0}}}
	// Two horizontal neighbours at y=20 and y=80. A bow around y=50 is safe.
	safe := []orb.LineString{{{0, 20}, {100, 20}}, {{0, 80}, {100, 80}}}
	safeVertices := [2]orb.Point{{30, 50}, {70, 50}}
	if conflict := FindDetourConflict(safe, 0, OrderedDetourVertices(safe[0], safeVertices)); conflict != nil {
		t.Fatalf("safe detour reported conflict at %v", conflict.At)
	}
	// A vertex at y=80 makes the middle leg touch the neighbouring line.
	collision := FindDetourConflict(safe, 0, [2]orb.Point{{30, 80}, {70, 50}})
	if collision == nil {
		t.Fatal("leg crossing the neighbouring line must be rejected")
	}
	if collision.NeighbourIndex != 1 {
		t.Fatalf("neighbour index = %d, want 1", collision.NeighbourIndex)
	}
	if !PolygonContains(boundary, collision.At) && collision.At != (orb.Point{30, 80}) {
		t.Fatalf("conflict point %v must locate the touch", collision.At)
	}
	// Collinear overlap with the neighbour must also be a conflict.
	collinear := []orb.LineString{{{0, 20}, {100, 20}}, {{0, 80}, {100, 80}}}
	if FindDetourConflict(collinear, 0, [2]orb.Point{{30, 80}, {70, 80}}) == nil {
		t.Fatal("collinear detour leg must conflict with the neighbouring line")
	}
}

func TestSegmentsIntersectCases(t *testing.T) {
	if _, ok := segmentsIntersect(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{5, -5}, orb.Point{5, 5}); !ok {
		t.Fatal("perpendicular segments must intersect")
	}
	if _, ok := segmentsIntersect(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{5, 5}, orb.Point{5, 10}); ok {
		t.Fatal("disjoint segments must not intersect")
	}
	if _, ok := segmentsIntersect(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{4, 0}, orb.Point{6, 0}); !ok {
		t.Fatal("collinear overlapping segments must intersect")
	}
	if _, ok := segmentsIntersect(orb.Point{0, 0}, orb.Point{10, 0}, orb.Point{10, 0}, orb.Point{20, 0}); !ok {
		t.Fatal("segments sharing an endpoint must intersect (touch rejected)")
	}
}
