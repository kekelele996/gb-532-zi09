package geometry

import (
	"fmt"
	"math"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
)

// DetourConflict describes the first place where a detour leg touches a
// neighbouring transect line. Coordinates stay in the projected metre CRS.
type DetourConflict struct {
	NeighbourIndex int       `json:"neighbour_index"`
	At             orb.Point `json:"at"`
	OnLeg          bool      `json:"on_detour_leg"`
}

// PolygonContains reports whether the point is strictly inside the (possibly
// holed) projected polygon. Points on a ring edge count as outside because a
// detour vertex must fall inside the survey area.
func PolygonContains(polygon orb.Polygon, point orb.Point) bool {
	if len(polygon) == 0 || !ringContains(polygon[0], point) || pointOnRing(polygon[0], point) {
		return false
	}
	for _, hole := range polygon[1:] {
		if ringContains(hole, point) || pointOnRing(hole, point) {
			return false
		}
	}
	return true
}

func pointOnRing(ring orb.Ring, point orb.Point) bool {
	for index := 1; index < len(ring); index++ {
		if pointSegmentDistance(point, ring[index-1], ring[index]) <= geometryEpsilon {
			return true
		}
	}
	return false
}

// ApplyDetour replaces the straight base segment at lineIndex with a polyline
// that visits the two detour vertices in their order of projection onto the
// base segment. Neighbouring lines are returned unchanged.
func ApplyDetour(lines []orb.LineString, lineIndex int, vertices [2]orb.Point) ([]orb.LineString, error) {
	if lineIndex < 0 || lineIndex >= len(lines) {
		return nil, fmt.Errorf("%w: detour target line index %d does not exist", ErrInvalidGeometry, lineIndex)
	}
	base := lines[lineIndex]
	if len(base) < 2 {
		return nil, fmt.Errorf("%w: detour target line needs two endpoints", ErrInvalidGeometry)
	}
	for _, vertex := range vertices {
		if !finite(vertex[0]) || !finite(vertex[1]) {
			return nil, fmt.Errorf("%w: detour vertex coordinate is not finite", ErrInvalidGeometry)
		}
	}
	start, end := base[0], base[len(base)-1]
	if distance(start, end) == 0 {
		return nil, fmt.Errorf("%w: detour target line has zero length", ErrInvalidGeometry)
	}
	ordered := OrderedDetourVertices(base, vertices)
	for _, vertex := range ordered {
		if distance(vertex, start) == 0 || distance(vertex, end) == 0 {
			return nil, fmt.Errorf("%w: detour vertex must not coincide with a line endpoint", ErrInvalidGeometry)
		}
	}
	if ordered[0] == ordered[1] {
		return nil, fmt.Errorf("%w: detour vertices must be distinct", ErrInvalidGeometry)
	}
	detoured := make(orb.LineString, 0, len(base)+2)
	detoured = append(detoured, start)
	detoured = append(detoured, ordered[0], ordered[1])
	detoured = append(detoured, end)

	result := make([]orb.LineString, len(lines))
	copy(result, lines)
	result[lineIndex] = detoured
	return result, nil
}

// OrderedDetourVertices returns the vertices in their order of projection on
// the base line so detour legs follow the transect direction regardless of
// the request order.
func OrderedDetourVertices(base orb.LineString, vertices [2]orb.Point) [2]orb.Point {
	if len(base) < 2 {
		return vertices
	}
	start, end := base[0], base[len(base)-1]
	dx, dy := end[0]-start[0], end[1]-start[1]
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return vertices
	}
	projection := func(vertex orb.Point) float64 {
		return ((vertex[0]-start[0])*dx + (vertex[1]-start[1])*dy) / lengthSquared
	}
	if projection(vertices[1]) < projection(vertices[0]) {
		return [2]orb.Point{vertices[1], vertices[0]}
	}
	return vertices
}

// FindDetourConflict checks the two new detour legs against every other line.
// Shared endpoints and crossings with the untouched base portion are not
// possible, so any intersection (including a touch or collinear overlap) is a
// collision with a neighbouring transect.
func FindDetourConflict(lines []orb.LineString, lineIndex int, vertices [2]orb.Point) *DetourConflict {
	if lineIndex < 0 || lineIndex >= len(lines) {
		return nil
	}
	base := lines[lineIndex]
	if len(base) < 2 {
		return nil
	}
	legs := [][2]orb.Point{{base[0], vertices[0]}, {vertices[0], vertices[1]}, {vertices[1], base[len(base)-1]}}
	for neighbourIndex, neighbour := range lines {
		if neighbourIndex == lineIndex {
			continue
		}
		for segmentIndex := 1; segmentIndex < len(neighbour); segmentIndex++ {
			candidate := [2]orb.Point{neighbour[segmentIndex-1], neighbour[segmentIndex]}
			for legIndex, leg := range legs {
				if point, intersects := segmentsIntersect(leg[0], leg[1], candidate[0], candidate[1]); intersects {
					return &DetourConflict{NeighbourIndex: neighbourIndex, At: point, OnLeg: legIndex == 1}
				}
			}
		}
	}
	return nil
}

// segmentsIntersect returns the intersection point for two planar segments.
// Collinear overlap reports the midpoint of the shared run so the planner still
// receives a concrete conflict coordinate.
func segmentsIntersect(p1, p2, p3, p4 orb.Point) (orb.Point, bool) {
	rx, ry := p2[0]-p1[0], p2[1]-p1[1]
	sx, sy := p4[0]-p3[0], p4[1]-p3[1]
	denominator := rx*sy - ry*sx
	qpX, qpY := p3[0]-p1[0], p3[1]-p1[1]
	if math.Abs(denominator) > geometryEpsilon {
		t := (qpX*sy - qpY*sx) / denominator
		u := (qpX*ry - qpY*rx) / denominator
		if t >= -geometryEpsilon && t <= 1+geometryEpsilon && u >= -geometryEpsilon && u <= 1+geometryEpsilon {
			return orb.Point{p1[0] + clamp01(t)*rx, p1[1] + clamp01(t)*ry}, true
		}
		return orb.Point{}, false
	}
	if math.Abs(qpX*ry-qpY*rx) > geometryEpsilon {
		return orb.Point{}, false
	}
	// Collinear segments: find the overlap of their parameters on p1->p2.
	lengthSquared := rx*rx + ry*ry
	if lengthSquared == 0 {
		if pointSegmentDistance(p1, p3, p4) <= geometryEpsilon {
			return p1, true
		}
		return orb.Point{}, false
	}
	t3 := ((p3[0]-p1[0])*rx + (p3[1]-p1[1])*ry) / lengthSquared
	t4 := ((p4[0]-p1[0])*rx + (p4[1]-p1[1])*ry) / lengthSquared
	lo, hi := math.Min(t3, t4), math.Max(t3, t4)
	overlapLo, overlapHi := math.Max(lo, 0), math.Min(hi, 1)
	if overlapHi < overlapLo-geometryEpsilon {
		return orb.Point{}, false
	}
	mid := (overlapLo + overlapHi) / 2
	return orb.Point{p1[0] + mid*rx, p1[1] + mid*ry}, true
}

const geometryEpsilon = 1e-9

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

// MarshalLines keeps the LineString/MultiLineString shape of the original
// feature so a detour never changes the plan's geometry type, only a line's
// coordinates.
func MarshalLines(lines []orb.LineString, original []byte) ([]byte, error) {
	var geometryValue orb.Geometry = orb.MultiLineString(lines)
	feature, err := geojson.UnmarshalFeature(original)
	if err == nil {
		if _, single := feature.Geometry.(orb.LineString); single {
			if len(lines) == 1 {
				geometryValue = lines[0]
			}
		}
	}
	return MarshalFeature(geometryValue)
}
