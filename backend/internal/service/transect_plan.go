package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"gorm.io/datatypes"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/geometry"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

type TransectPlanService struct {
	repository *repository.TransectPlanRepository
	areas      *repository.SurveyAreaRepository
	audit      *AuditService
}

func NewTransectPlanService(repository *repository.TransectPlanRepository, areas *repository.SurveyAreaRepository, audit *AuditService) *TransectPlanService {
	return &TransectPlanService{repository: repository, areas: areas, audit: audit}
}

func (s *TransectPlanService) List(query dto.TransectPlanQuery) ([]model.TransectPlan, int64, error) {
	return s.repository.List(query)
}
func (s *TransectPlanService) Get(id uint) (model.TransectPlan, error) {
	item, err := s.repository.Get(id)
	if err != nil {
		return item, mapDatabaseError(err, "测线规划")
	}
	return item, nil
}

func (s *TransectPlanService) Create(request dto.CreateTransectPlanRequest, actor Actor) (model.TransectPlan, error) {
	area, err := s.areas.Get(request.SurveyAreaID)
	if err != nil {
		return model.TransectPlan{}, mapDatabaseError(err, "测区")
	}
	if area.Status == constants.AreaArchived {
		return model.TransectPlan{}, api.Conflict("AREA_ARCHIVED", "归档测区不能新增规划", nil)
	}
	if _, err := geometry.ParseLines(request.LineGeoJSON); err != nil {
		return model.TransectPlan{}, api.Unprocessable("GEOJSON_INVALID", "测线 GeoJSON 无效", err)
	}
	item := model.TransectPlan{SurveyAreaID: request.SurveyAreaID, Name: request.Name, LineGeoJSON: datatypes.JSON(request.LineGeoJSON), PlannedHeading: request.PlannedHeading, PlannedSwathM: request.PlannedSwathM, LineSpacingM: request.LineSpacingM, PlanState: constants.PlanDraft, Version: 1, CreatedBy: actor.UserID}
	if err := s.repository.Create(&item); err != nil {
		return item, err
	}
	if err := s.audit.Record(actor, "plan.create", "transect_plan", item.ID, nil, item, map[string]any{"source": "manual"}); err != nil {
		return item, err
	}
	return s.Get(item.ID)
}

func (s *TransectPlanService) Generate(request dto.GenerateLinesRequest, actor Actor) (model.TransectPlan, error) {
	area, err := s.areas.Get(request.SurveyAreaID)
	if err != nil {
		return model.TransectPlan{}, mapDatabaseError(err, "测区")
	}
	polygon, err := geometry.ParsePolygon(area.BoundaryGeoJSON)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("GEOJSON_INVALID", "测区边界无法生成测线", err)
	}
	minX, minY, maxX, maxY := bounds(polygon)
	if request.LineSpacingM > math.Max(maxX-minX, maxY-minY) {
		return model.TransectPlan{}, api.Unprocessable("LINE_SPACING_INVALID", "测线间距超过测区跨度", nil)
	}
	lines := orb.MultiLineString{}
	horizontal := request.Heading >= 45 && request.Heading < 135 || request.Heading >= 225 && request.Heading < 315
	if horizontal {
		for y := minY + request.LineSpacingM/2; y < maxY; y += request.LineSpacingM {
			lines = append(lines, orb.LineString{{minX, y}, {maxX, y}})
		}
	} else {
		for x := minX + request.LineSpacingM/2; x < maxX; x += request.LineSpacingM {
			lines = append(lines, orb.LineString{{x, minY}, {x, maxY}})
		}
	}
	if len(lines) == 0 {
		return model.TransectPlan{}, api.Unprocessable("NO_TRANSECTS", "当前参数无法在测区内生成测线", nil)
	}
	encoded, err := geometry.MarshalFeature(lines)
	if err != nil {
		return model.TransectPlan{}, fmt.Errorf("encode generated transects: %w", err)
	}
	return s.Create(dto.CreateTransectPlanRequest{SurveyAreaID: request.SurveyAreaID, Name: request.Name, LineGeoJSON: encoded, PlannedHeading: request.Heading, PlannedSwathM: request.PlannedSwathM, LineSpacingM: request.LineSpacingM}, actor)
}

func (s *TransectPlanService) Update(id uint, request dto.UpdateTransectPlanRequest, actor Actor) (model.TransectPlan, error) {
	before, err := s.Get(id)
	if err != nil {
		return model.TransectPlan{}, err
	}
	if before.PlanState != constants.PlanDraft {
		return model.TransectPlan{}, api.Conflict("PLAN_LOCKED", "已锁定规划只能复制新版本", nil)
	}
	if _, err := geometry.ParseLines(request.LineGeoJSON); err != nil {
		return model.TransectPlan{}, api.Unprocessable("GEOJSON_INVALID", "测线 GeoJSON 无效", err)
	}
	updated, err := s.repository.Update(id, request.ExpectedVersion, map[string]any{"name": request.Name, "line_geojson": request.LineGeoJSON, "planned_heading": request.PlannedHeading, "planned_swath_m": request.PlannedSwathM, "line_spacing_m": request.LineSpacingM})
	if err != nil {
		return model.TransectPlan{}, mapDatabaseError(err, "测线规划")
	}
	if err := s.audit.Record(actor, "plan.update", "transect_plan", id, before, updated, map[string]any{"expected_version": request.ExpectedVersion}); err != nil {
		return updated, err
	}
	return updated, nil
}

func (s *TransectPlanService) Lock(id uint, request dto.PlanTransitionRequest, actor Actor) (model.TransectPlan, error) {
	before, err := s.Get(id)
	if err != nil {
		return model.TransectPlan{}, err
	}
	if before.PlanState != constants.PlanDraft || request.TargetState != constants.PlanLocked {
		return model.TransectPlan{}, api.Conflict("PLAN_STATE_INVALID", "规划状态迁移无效", nil)
	}
	updated, err := s.repository.Transition(id, request.ExpectedVersion, constants.PlanDraft, constants.PlanLocked)
	if err != nil {
		return updated, mapDatabaseError(err, "测线规划")
	}
	if err := s.audit.Record(actor, "plan.lock", "transect_plan", id, before, updated, nil); err != nil {
		return updated, err
	}
	return updated, nil
}

func (s *TransectPlanService) Copy(id uint, actor Actor) (model.TransectPlan, error) {
	before, err := s.Get(id)
	if err != nil {
		return model.TransectPlan{}, err
	}
	copy, err := s.repository.Copy(before, actor.UserID)
	if err != nil {
		return copy, err
	}
	copy.CreatedAt = time.Now().UTC()
	copy.UpdatedAt = copy.CreatedAt
	if err := s.audit.Record(actor, "plan.copy", "transect_plan", copy.ID, before, copy, map[string]any{"source_plan_id": id}); err != nil {
		return copy, err
	}
	return copy, nil
}

// ApplyDetour 在草稿规划的指定测线上用两处折点替换直线段。
// 仅更新 line_geojson 与版本号，其他测线、线间距和计划扫幅保持不变；
// 折点必须落在测区内，绕行段触碰相邻测线时拒绝保存并保留原几何。
func (s *TransectPlanService) ApplyDetour(id uint, request dto.ApplyDetourRequest, actor Actor) (model.TransectPlan, error) {
	before, err := s.Get(id)
	if err != nil {
		return model.TransectPlan{}, err
	}
	if before.PlanState != constants.PlanDraft {
		return model.TransectPlan{}, api.Conflict("PLAN_LOCKED", "已锁定规划不能安排绕行，请先复制为新草稿", nil)
	}
	first, err := parseDetourPoint(request.FirstVertex)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("DETOUR_POINT_INVALID", "第一处折点坐标无效", err)
	}
	second, err := parseDetourPoint(request.SecondVertex)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("DETOUR_POINT_INVALID", "第二处折点坐标无效", err)
	}
	area := before.SurveyArea
	if area == nil {
		loaded, loadErr := s.areas.Get(before.SurveyAreaID)
		if loadErr != nil {
			return model.TransectPlan{}, mapDatabaseError(loadErr, "测区")
		}
		area = &loaded
	}
	boundary, err := geometry.ParsePolygon(area.BoundaryGeoJSON)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("GEOJSON_INVALID", "测区边界无法校验绕行折点", err)
	}
	for _, candidate := range []struct {
		label string
		point orb.Point
	}{
		{label: "第一处折点", point: first},
		{label: "第二处折点", point: second},
	} {
		if !geometry.PointInsideOrOnBoundary(boundary, candidate.point) {
			return model.TransectPlan{}, unprocessableWithDetails("DETOUR_OUTSIDE_AREA", candidate.label+"必须落在测区边界内", map[string]any{"vertex": candidate.point})
		}
	}
	if distance2D(first, second) == 0 {
		return model.TransectPlan{}, unprocessableWithDetails("DETOUR_VERTEX_DUPLICATE", "两处折点不能重合", map[string]any{"vertex": first})
	}

	feature, err := geojson.UnmarshalFeature(before.LineGeoJSON)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("GEOJSON_INVALID", "当前测线几何无法解析", err)
	}
	lines, err := geometry.ParseLines(before.LineGeoJSON)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("GEOJSON_INVALID", "当前测线几何无效", err)
	}
	if request.LineIndex < 0 || request.LineIndex >= len(lines) {
		return model.TransectPlan{}, api.BadRequest("DETOUR_LINE_NOT_FOUND",
			fmt.Sprintf("测线索引 %d 不存在，当前规划共有 %d 条测线", request.LineIndex, len(lines)), nil)
	}
	target := lines[request.LineIndex]
	replaced, err := geometry.OrderedDetourLine(target, first, second)
	if err != nil {
		return model.TransectPlan{}, api.Unprocessable("DETOUR_LINE_INVALID", "所选测线无法安排绕行", err)
	}

	originalSegments := segmentSet(target)
	detourSegments := make([][2]orb.Point, 0, len(replaced)-1)
	for index := 1; index < len(replaced); index++ {
		segment := [2]orb.Point{replaced[index-1], replaced[index]}
		if _, exists := originalSegments[segmentKey(segment[0], segment[1])]; !exists {
			detourSegments = append(detourSegments, segment)
		}
	}
	for otherIndex, other := range lines {
		if otherIndex == request.LineIndex {
			continue
		}
		for segmentIndex := 1; segmentIndex < len(other); segmentIndex++ {
			for detourIndex, detourSegment := range detourSegments {
				if intersection, intersects := geometry.SegmentIntersection(detourSegment[0], detourSegment[1], other[segmentIndex-1], other[segmentIndex]); intersects {
					return model.TransectPlan{}, unprocessableWithDetails("DETOUR_CONFLICT",
						fmt.Sprintf("绕行段与第 %d 条相邻测线相交，已拒绝保存", otherIndex+1),
						map[string]any{
							"target_line_index":   request.LineIndex,
							"conflict_line_index": otherIndex,
							"detour_segment":      detourIndex + 1,
							"intersection":        intersection,
						})
				}
			}
		}
	}

	updatedLines := make(orb.MultiLineString, len(lines))
	copy(updatedLines, lines)
	updatedLines[request.LineIndex] = replaced
	feature.Geometry = updatedLines
	if feature.Properties == nil {
		feature.Properties = geojson.Properties{}
	}
	detours := readDetourProperties(feature.Properties)
	orderedVertices := orderedDetourVertices(replaced, target, first, second)
	detours[request.LineIndex] = orderedVertices
	feature.Properties["detours"] = detourMapForJSON(detours)
	encoded, err := feature.MarshalJSON()
	if err != nil {
		return model.TransectPlan{}, fmt.Errorf("encode detour feature: %w", err)
	}
	updated, err := s.repository.Update(id, request.ExpectedVersion, map[string]any{"line_geojson": json.RawMessage(encoded)})
	if err != nil {
		return model.TransectPlan{}, mapDatabaseError(err, "测线规划")
	}
	if err := s.audit.Record(actor, "plan.detour", "transect_plan", id, before, updated, map[string]any{
		"line_index":       request.LineIndex,
		"detour_vertices":  orderedVertices,
		"expected_version": request.ExpectedVersion,
	}); err != nil {
		return updated, err
	}
	return updated, nil
}

func parseDetourPoint(point *dto.DetourPoint) (orb.Point, error) {
	if point == nil || point.X == nil || point.Y == nil {
		return orb.Point{}, fmt.Errorf("x and y are required")
	}
	if math.IsNaN(*point.X) || math.IsInf(*point.X, 0) || math.IsNaN(*point.Y) || math.IsInf(*point.Y, 0) {
		return orb.Point{}, fmt.Errorf("coordinates must be finite")
	}
	return orb.Point{*point.X, *point.Y}, nil
}

// segmentSet 记录原线段的无向键，用于区分绕行新增段与原直线段。
func segmentSet(line orb.LineString) map[string]struct{} {
	segments := make(map[string]struct{}, len(line)-1)
	for index := 1; index < len(line); index++ {
		segments[segmentKey(line[index-1], line[index])] = struct{}{}
	}
	return segments
}

func segmentKey(start, end orb.Point) string {
	first, second := start, end
	if start[0] > end[0] || start[0] == end[0] && start[1] > end[1] {
		first, second = end, start
	}
	return fmt.Sprintf("%.6f:%.6f|%.6f:%.6f", first[0], first[1], second[0], second[1])
}

// readDetourProperties 读取已保存的折点信息，键为测线索引。
func readDetourProperties(properties geojson.Properties) map[int][]orb.Point {
	detours := map[int][]orb.Point{}
	raw, ok := properties["detours"]
	if !ok {
		return detours
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return detours
	}
	indices := make([]int, 0, len(entries))
	for key := range entries {
		var index int
		if _, err := fmt.Sscanf(key, "%d", &index); err == nil {
			indices = append(indices, index)
		}
	}
	sort.Ints(indices)
	for _, index := range indices {
		points, ok := entries[fmt.Sprintf("%d", index)].([]any)
		if !ok || len(points) != 2 {
			continue
		}
		vertices := make([]orb.Point, 0, 2)
		for _, candidate := range points {
			coordinates, ok := candidate.([]any)
			if !ok || len(coordinates) < 2 {
				vertices = nil
				break
			}
			x, xOK := coordinates[0].(float64)
			y, yOK := coordinates[1].(float64)
			if !xOK || !yOK {
				vertices = nil
				break
			}
			vertices = append(vertices, orb.Point{x, y})
		}
		if len(vertices) == 2 {
			detours[index] = vertices
		}
	}
	return detours
}

func detourMapForJSON(detours map[int][]orb.Point) map[string][][2]float64 {
	result := make(map[string][][2]float64, len(detours))
	for index, vertices := range detours {
		pairs := make([][2]float64, 0, len(vertices))
		for _, vertex := range vertices {
			pairs = append(pairs, [2]float64{vertex[0], vertex[1]})
		}
		result[fmt.Sprintf("%d", index)] = pairs
	}
	return result
}

func distance2D(a, b orb.Point) float64 {
	return math.Hypot(a[0]-b[0], a[1]-b[1])
}

// orderedDetourVertices 从替换折线中提取两处折点并保持沿测线方向的顺序；
// 提取失败时回退为按投影排序的输入折点。
func orderedDetourVertices(replaced, target orb.LineString, first, second orb.Point) []orb.Point {
	original := make(map[orb.Point]struct{}, len(target))
	for _, point := range target {
		original[point] = struct{}{}
	}
	vertices := make([]orb.Point, 0, 2)
	for _, point := range replaced {
		if _, exists := original[point]; !exists {
			vertices = append(vertices, point)
		}
	}
	if len(vertices) == 2 {
		return vertices
	}
	return []orb.Point{first, second}
}

// unprocessableWithDetails 构造 422 业务错误并附带结构化细节（如冲突交点）。
func unprocessableWithDetails(code, message string, details map[string]any) error {
	appErr := api.Unprocessable(code, message, nil)
	appErr.Details = details
	return appErr
}

func bounds(polygon orb.Polygon) (float64, float64, float64, float64) {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, ring := range polygon {
		for _, point := range ring {
			minX = math.Min(minX, point[0])
			minY = math.Min(minY, point[1])
			maxX = math.Max(maxX, point[0])
			maxY = math.Max(maxY, point[1])
		}
	}
	return minX, minY, maxX, maxY
}
