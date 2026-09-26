package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/geometry"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

func newDetourTestStore(t *testing.T) (*TransectPlanService, model.SurveyArea, uint) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared&_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SurveyArea{}, &model.TransectPlan{}, &model.SonarRun{}, &model.CoverageGap{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	area := model.SurveyArea{
		AreaCode: "T-DETOUR", Name: "绕行测试测区",
		BoundaryGeoJSON:   []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1000,0],[1000,600],[0,600],[0,0]]]}}`),
		TargetResolutionM: 20,
		CoordinateSystem:  "EPSG:32650", DefaultSwathM: 180, OwnerTeam: "测试组",
		Status: constants.AreaActive, Version: 1,
	}
	if err := db.Create(&area).Error; err != nil {
		t.Fatalf("create area: %v", err)
	}
	// 三条东西向直线，间距 200 m。
	lines := []byte(`{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[40,100],[960,100]],[[40,300],[960,300]],[[40,500],[960,500]]]}}`)
	plan := model.TransectPlan{SurveyAreaID: area.ID, Name: "待绕行草稿", LineGeoJSON: lines, PlannedHeading: 90, PlannedSwathM: 180, LineSpacingM: 200, PlanState: constants.PlanDraft, Version: 1, CreatedBy: 1}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	service := NewTransectPlanService(
		repository.NewTransectPlanRepository(db),
		repository.NewSurveyAreaRepository(db),
		NewAuditService(repository.NewSupportRepository(db)),
	)
	return service, area, plan.ID
}

func detourActor() Actor {
	return Actor{RequestID: "req-detour-test", UserID: 1, Username: "planner", Role: constants.RoleSurveyPlanner}
}

func detourPoint(x, y float64) *dto.DetourPoint {
	return &dto.DetourPoint{X: &x, Y: &y}
}

func applyMiddleDetourRequest(version uint) dto.ApplyDetourRequest {
	return dto.ApplyDetourRequest{
		LineIndex:       0,
		FirstVertex:     detourPoint(400, 200),
		SecondVertex:    detourPoint(600, 200),
		ExpectedVersion: version,
	}
}

func TestApplyDetourReplacesOnlyTargetLineAndBumpsVersion(t *testing.T) {
	service, _, planID := newDetourTestStore(t)
	updated, err := service.ApplyDetour(planID, applyMiddleDetourRequest(1), detourActor())
	if err != nil {
		t.Fatalf("apply detour: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}
	if updated.LineSpacingM != 200 || updated.PlannedSwathM != 180 {
		t.Fatal("line spacing and swath must remain unchanged")
	}
	lines, err := geometry.ParseLines(updated.LineGeoJSON)
	if err != nil {
		t.Fatalf("parse updated lines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3", len(lines))
	}
	want := []orb.Point{{40, 100}, {400, 200}, {600, 200}, {960, 100}}
	if len(lines[0]) != len(want) {
		t.Fatalf("detoured line vertices = %d, want %d: %v", len(lines[0]), len(want), lines[0])
	}
	for index, point := range want {
		if lines[0][index] != point {
			t.Fatalf("vertex %d = %v, want %v", index, lines[0][index], point)
		}
	}
	if got := lines[1]; len(got) != 2 || got[0][1] != 300 {
		t.Fatalf("adjacent line must stay unchanged: %v", got)
	}
	feature, err := geojson.UnmarshalFeature(updated.LineGeoJSON)
	if err != nil {
		t.Fatalf("unmarshal feature: %v", err)
	}
	if feature.Properties["detours"] == nil {
		t.Fatal("detour vertices must be recorded in feature properties")
	}
}

func TestApplyDetourRejectsVertexOutsideAreaAndKeepsGeometry(t *testing.T) {
	service, _, planID := newDetourTestStore(t)
	before, _ := service.Get(planID)
	_, err := service.ApplyDetour(planID, dto.ApplyDetourRequest{
		LineIndex: 0, FirstVertex: detourPoint(400, 200),
		SecondVertex: detourPoint(1200, 200), ExpectedVersion: 1,
	}, detourActor())
	assertAppError(t, err, 422, "DETOUR_OUTSIDE_AREA")
	after, _ := service.Get(planID)
	if string(after.LineGeoJSON) != string(before.LineGeoJSON) || after.Version != 1 {
		t.Fatal("original geometry and version must be preserved on rejection")
	}
}

func TestApplyDetourRejectsCollisionWithAdjacentLine(t *testing.T) {
	service, _, planID := newDetourTestStore(t)
	before, _ := service.Get(planID)
	// 折点越过 y=300 的相邻测线，绕行段必然与其相交。
	_, err := service.ApplyDetour(planID, dto.ApplyDetourRequest{
		LineIndex: 0, FirstVertex: detourPoint(400, 350),
		SecondVertex: detourPoint(600, 350), ExpectedVersion: 1,
	}, detourActor())
	appErr := assertAppError(t, err, 422, "DETOUR_CONFLICT")
	details, ok := appErr.Details.(map[string]any)
	if !ok {
		t.Fatalf("conflict details missing, got %T", appErr.Details)
	}
	if details["conflict_line_index"] != 1 {
		t.Fatalf("conflict line index = %v, want 1", details["conflict_line_index"])
	}
	if _, ok := details["intersection"]; !ok {
		t.Fatal("conflict must report intersection location")
	}
	if !strings.Contains(appErr.Message, "相邻测线") {
		t.Fatalf("conflict message must name adjacent line: %s", appErr.Message)
	}
	after, _ := service.Get(planID)
	if string(after.LineGeoJSON) != string(before.LineGeoJSON) {
		t.Fatal("original geometry must be preserved on conflict")
	}
}

func TestApplyDetourRejectsLockedPlan(t *testing.T) {
	service, _, planID := newDetourTestStore(t)
	if _, err := service.Lock(planID, dto.PlanTransitionRequest{TargetState: constants.PlanLocked, ExpectedVersion: 1}, detourActor()); err != nil {
		t.Fatalf("lock plan: %v", err)
	}
	_, err := service.ApplyDetour(planID, applyMiddleDetourRequest(2), detourActor())
	assertAppError(t, err, 409, "PLAN_LOCKED")
}

func TestCopyAfterDetourKeepsVerticesButLockedSourceCannotChange(t *testing.T) {
	service, _, planID := newDetourTestStore(t)
	detoured, err := service.ApplyDetour(planID, applyMiddleDetourRequest(1), detourActor())
	if err != nil {
		t.Fatalf("apply detour: %v", err)
	}
	if _, err := service.Lock(planID, dto.PlanTransitionRequest{TargetState: constants.PlanLocked, ExpectedVersion: detoured.Version}, detourActor()); err != nil {
		t.Fatalf("lock detoured plan: %v", err)
	}
	copied, err := service.Copy(planID, detourActor())
	if err != nil {
		t.Fatalf("copy locked plan: %v", err)
	}
	if copied.PlanState != constants.PlanDraft {
		t.Fatalf("copied plan state = %s, want draft", copied.PlanState)
	}
	originalLines, err := geometry.ParseLines(detoured.LineGeoJSON)
	if err != nil {
		t.Fatalf("parse original lines: %v", err)
	}
	copiedLines, err := geometry.ParseLines(copied.LineGeoJSON)
	if err != nil {
		t.Fatalf("parse copied lines: %v", err)
	}
	if len(copiedLines) != len(originalLines) {
		t.Fatalf("copied line count = %d, want %d", len(copiedLines), len(originalLines))
	}
	if len(copiedLines[0]) != 4 {
		t.Fatalf("copied detoured line must keep 4 vertices, got %d", len(copiedLines[0]))
	}
	feature, err := geojson.UnmarshalFeature(copied.LineGeoJSON)
	if err != nil {
		t.Fatalf("unmarshal copy feature: %v", err)
	}
	if feature.Properties["detours"] == nil {
		t.Fatal("copied draft must preserve detour vertex metadata")
	}
	// 原锁定版本仍不可修改。
	_, err = service.ApplyDetour(planID, applyMiddleDetourRequest(detoured.Version+1), detourActor())
	assertAppError(t, err, 409, "PLAN_LOCKED")
}

func TestApplyDetourRejectsStaleVersion(t *testing.T) {
	service, _, planID := newDetourTestStore(t)
	_, err := service.ApplyDetour(planID, applyMiddleDetourRequest(7), detourActor())
	assertAppError(t, err, 409, "VERSION_CONFLICT")
}

func assertAppError(t *testing.T, err error, status int, code string) *api.AppError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %s, got nil", code)
	}
	appErr := &api.AppError{}
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not an AppError", err)
	}
	if appErr.Status != status {
		t.Fatalf("status = %d, want %d (%v)", appErr.Status, status, err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
	return appErr
}
