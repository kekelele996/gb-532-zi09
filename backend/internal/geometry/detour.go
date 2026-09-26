package geometry

import (
	"fmt"
	"math"

	"github.com/paulmach/orb"
)

// boundaryToleranceM 是折点落在测区边界上时允许的米级误差。
const boundaryToleranceM = 1e-6

// PointInsideOrOnBoundary 判断平面投影点是否落在多边形内部或边界上。
// 坐标必须为项目约定的米制投影坐标；孔洞区域视为测区外。
func PointInsideOrOnBoundary(polygon orb.Polygon, point orb.Point) bool {
	if len(polygon) == 0 {
		return false
	}
	onExterior := PointOnRing(polygon[0], point, boundaryToleranceM)
	if !ringContains(polygon[0], point) && !onExterior {
		return false
	}
	for _, hole := range polygon[1:] {
		// 落在孔洞内部（不含孔洞边界）才算测区外；内环边界仍属于多边形。
		if ringContains(hole, point) && !PointOnRing(hole, point, boundaryToleranceM) {
			return false
		}
	}
	return true
}

// PointOnRing 判断点是否压在环的任意一条线段上，toleranceM 为米级容差。
func PointOnRing(ring orb.Ring, point orb.Point, toleranceM float64) bool {
	for index := 1; index < len(ring); index++ {
		if pointSegmentDistance(point, ring[index-1], ring[index]) <= toleranceM {
			return true
		}
	}
	return false
}

// SegmentIntersection 返回两条线段是否相交（含端点相接与共线重叠），
// 并在相交时返回一个代表性交点。平行不相交时交点无意义。
func SegmentIntersection(aStart, aEnd, bStart, bEnd orb.Point) (orb.Point, bool) {
	ar, br := aEnd[0]-aStart[0], aEnd[1]-aStart[1]
	cr, ds := bEnd[0]-bStart[0], bEnd[1]-bStart[1]
	denominator := ar*ds - br*cr
	deltaX, deltaY := bStart[0]-aStart[0], bStart[1]-aStart[1]
	if math.Abs(denominator) > boundaryToleranceM {
		t := (deltaX*ds - deltaY*cr) / denominator
		u := (deltaX*br - deltaY*ar) / denominator
		if t >= -boundaryToleranceM && t <= 1+boundaryToleranceM && u >= -boundaryToleranceM && u <= 1+boundaryToleranceM {
			return orb.Point{aStart[0] + clamp(t)*ar, aStart[1] + clamp(t)*br}, true
		}
		return orb.Point{}, false
	}
	// 两线方向共线（或均退化为点）：任一端点落在另一线段上即视为相交。
	if pointSegmentDistance(bStart, aStart, aEnd) <= boundaryToleranceM {
		return bStart, true
	}
	if pointSegmentDistance(bEnd, aStart, aEnd) <= boundaryToleranceM {
		return bEnd, true
	}
	if pointSegmentDistance(aStart, bStart, bEnd) <= boundaryToleranceM {
		return aStart, true
	}
	if pointSegmentDistance(aEnd, bStart, bEnd) <= boundaryToleranceM {
		return aEnd, true
	}
	return orb.Point{}, false
}

// OrderedDetourLine 用两处折点替换原始直线上的对应路径，
// 得到 start ... P1 ... P2 ... end 的绕行折线。
// 折点按其在原始折线投影轴上的位置（弧长参数）排序，
// 因此输入两处折点时无需保证顺序；原始顶点中不被折点取代的拐点保留。
func OrderedDetourLine(line orb.LineString, first, second orb.Point) (orb.LineString, error) {
	if len(line) < 2 {
		return nil, fmt.Errorf("%w: detour target line needs at least two positions", ErrInvalidGeometry)
	}
	type candidate struct {
		point orb.Point
		t     float64
		order int
	}
	candidates := make([]candidate, 0, len(line))
	for index, point := range line {
		t, err := lineParameter(line, index)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate{point: point, t: t, order: index})
	}
	for index, point := range []orb.Point{first, second} {
		t, err := projectedParameter(line, point)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate{point: point, t: t, order: len(line) + index})
	}
	// 稳定排序保证投影重合时保持输入先后。
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0; j-- {
			if candidates[j].t < candidates[j-1].t ||
				math.Abs(candidates[j].t-candidates[j-1].t) <= boundaryToleranceM && candidates[j].order < candidates[j-1].order {
				candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
			} else {
				break
			}
		}
	}
	result := make(orb.LineString, 0, len(candidates))
	for _, item := range candidates {
		result = append(result, item.point)
	}
	return result, nil
}

// lineParameter 返回原折线顶点沿折线累计弧长归一化后的参数。
func lineParameter(line orb.LineString, index int) (float64, error) {
	total := 0.0
	lengths := make([]float64, len(line))
	for i := 1; i < len(line); i++ {
		segmentLength := distance(line[i-1], line[i])
		if segmentLength <= 0 {
			return 0, fmt.Errorf("%w: zero-length segment in target line", ErrInvalidGeometry)
		}
		total += segmentLength
		lengths[i] = total
	}
	if total <= 0 {
		return 0, fmt.Errorf("%w: target line has no length", ErrInvalidGeometry)
	}
	return lengths[index] / total, nil
}

// projectedParameter 返回点在折线上的最近投影位置的弧长参数。
func projectedParameter(line orb.LineString, point orb.Point) (float64, error) {
	total := 0.0
	bestDistance := math.Inf(1)
	bestOffset := 0.0
	for index := 1; index < len(line); index++ {
		start, end := line[index-1], line[index]
		segmentLength := distance(start, end)
		if segmentLength <= 0 {
			return 0, fmt.Errorf("%w: zero-length segment in target line", ErrInvalidGeometry)
		}
		projection := nearestProjectionParameter(point, start, end)
		projectedPoint := orb.Point{start[0] + projection*(end[0]-start[0]), start[1] + projection*(end[1]-start[1])}
		gap := distance(point, projectedPoint)
		if gap < bestDistance {
			bestDistance = gap
			bestOffset = total + projection*segmentLength
		}
		total += segmentLength
	}
	if total <= 0 {
		return 0, fmt.Errorf("%w: target line has no length", ErrInvalidGeometry)
	}
	return clamp(bestOffset / total), nil
}

// nearestProjectionParameter 返回点在线段上的最近投影参数（夹到 [0,1]）。
func nearestProjectionParameter(point, start, end orb.Point) float64 {
	dx, dy := end[0]-start[0], end[1]-start[1]
	lengthSquared := dx*dx + dy*dy
	if lengthSquared <= 0 {
		return 0
	}
	return clamp(((point[0]-start[0])*dx + (point[1]-start[1])*dy) / lengthSquared)
}

func clamp(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
