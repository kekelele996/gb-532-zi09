package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/geometry"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

var detourDBCounter int64

func newDetourTestService(t *testing.T) (*TransectPlanService, *gorm.DB, model.TransectPlan) {
	t.Helper()
	dsn := fmt.Sprintf("file:detour-%d?mode=memory&cache=shared", atomic.AddInt64(&detourDBCounter, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SurveyArea{}, &model.TransectPlan{}, &model.SonarRun{}, &model.CoverageGap{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	boundary := []byte(`{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1000,0],[1000,1000],[0,1000],[0,0]]]}}`)
	area := model.SurveyArea{AreaCode: "AREA-DETOUR", Name: "礁区测区", BoundaryGeoJSON: datatypes.JSON(boundary), TargetResolutionM: 5, CoordinateSystem: "EPSG:32650", DefaultSwathM: 120, OwnerTeam: "planners", Status: constants.AreaActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&area).Error; err != nil {
		t.Fatalf("create area: %v", err)
	}
	lines := []byte(`{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[0,200],[1000,200]],[[0,500],[1000,500]],[[0,800],[1000,800]]]}}`)
	plan := model.TransectPlan{SurveyAreaID: area.ID, Name: "三条平行测线", LineGeoJSON: datatypes.JSON(lines), PlannedHeading: 90, PlannedSwathM: 180, LineSpacingM: 300, PlanState: constants.PlanDraft, Version: 1, CreatedBy: 7, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	planRepository := repository.NewTransectPlanRepository(db)
	areaRepository := repository.NewSurveyAreaRepository(db)
	auditService := NewAuditService(repository.NewSupportRepository(db))
	svc := NewTransectPlanService(planRepository, areaRepository, auditService)
	return svc, db, plan
}

func detourActor() Actor {
	return Actor{RequestID: "req-detour-test", UserID: 7, Username: "planner", Role: constants.RoleSurveyPlanner}
}

func TestDetourSuccessUpdatesOnlyGeometryAndVersion(t *testing.T) {
	svc, _, plan := newDetourTestService(t)
	updated, err := svc.Detour(plan.ID, dto.ApplyDetourRequest{LineIndex: 0, Vertices: []dto.DetourVertexRequest{{X: 250, Y: 350}, {X: 750, Y: 350}}, ExpectedVersion: 1}, detourActor())
	if err != nil {
		t.Fatalf("detour: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}
	if updated.PlanState != constants.PlanDraft {
		t.Fatal("plan must stay a draft after detour")
	}
	if updated.LineSpacingM != 300 || updated.PlannedSwathM != 180 {
		t.Fatal("spacing and planned swath must be preserved")
	}
	lines, err := geometry.ParseLines(updated.LineGeoJSON)
	if err != nil {
		t.Fatalf("parse updated lines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want 3", len(lines))
	}
	if len(lines[0]) != 4 {
		t.Fatalf("detoured line vertex count = %d, want 4", len(lines[0]))
	}
	if lines[1][0] != (orbPoint(0, 500)) || lines[2][0] != (orbPoint(0, 800)) {
		t.Fatal("neighbouring lines must be untouched")
	}
}

func TestDetourRejectsVertexOutsideAreaAndKeepsGeometry(t *testing.T) {
	svc, db, plan := newDetourTestService(t)
	_, err := svc.Detour(plan.ID, dto.ApplyDetourRequest{LineIndex: 0, Vertices: []dto.DetourVertexRequest{{X: 250, Y: 350}, {X: 1200, Y: 350}}, ExpectedVersion: 1}, detourActor())
	if err == nil {
		t.Fatal("vertex outside the survey area must reject the save")
	}
	var appErr *api.AppError
	if !asAppError(err, &appErr) || appErr.Code != "DETOUR_VERTEX_OUTSIDE_AREA" || appErr.Status != 422 {
		t.Fatalf("error = %v, want DETOUR_VERTEX_OUTSIDE_AREA/422", err)
	}
	var stored model.TransectPlan
	if findErr := db.First(&stored, plan.ID).Error; findErr != nil {
		t.Fatal(findErr)
	}
	if stored.Version != 1 || !strings.Contains(string(stored.LineGeoJSON), `"coordinates":[[[0,200],[1000,200]]`) {
		t.Fatalf("original geometry and version must be preserved: version=%d", stored.Version)
	}
}

func TestDetourRejectsNeighbourConflictWithLocation(t *testing.T) {
	svc, db, plan := newDetourTestService(t)
	// Second vertex at y=500 makes the detour leg run into line index 1.
	_, err := svc.Detour(plan.ID, dto.ApplyDetourRequest{LineIndex: 0, Vertices: []dto.DetourVertexRequest{{X: 250, Y: 350}, {X: 500, Y: 500}}, ExpectedVersion: 1}, detourActor())
	if err == nil {
		t.Fatal("detour crossing a neighbouring line must reject the save")
	}
	var appErr *api.AppError
	if !asAppError(err, &appErr) || appErr.Code != "DETOUR_LINE_CONFLICT" || appErr.Status != 409 {
		t.Fatalf("error = %v, want DETOUR_LINE_CONFLICT/409", err)
	}
	details, _ := json.Marshal(appErr.Details)
	if !strings.Contains(string(details), `"neighbour_index":1`) {
		t.Fatalf("conflict details must name the neighbour line: %s", details)
	}
	var stored model.TransectPlan
	if findErr := db.First(&stored, plan.ID).Error; findErr != nil {
		t.Fatal(findErr)
	}
	if stored.Version != 1 {
		t.Fatalf("geometry/version preserved, got version %d", stored.Version)
	}
}

func TestDetourLockedPlanAndCopyKeepsVertices(t *testing.T) {
	svc, _, plan := newDetourTestService(t)
	detoured, err := svc.Detour(plan.ID, dto.ApplyDetourRequest{LineIndex: 2, Vertices: []dto.DetourVertexRequest{{X: 300, Y: 650}, {X: 700, Y: 650}}, ExpectedVersion: 1}, detourActor())
	if err != nil {
		t.Fatalf("detour: %v", err)
	}
	locked, err := svc.Lock(detoured.ID, dto.PlanTransitionRequest{TargetState: constants.PlanLocked, ExpectedVersion: 2}, detourActor())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if _, err := svc.Detour(locked.ID, dto.ApplyDetourRequest{LineIndex: 0, Vertices: []dto.DetourVertexRequest{{X: 250, Y: 350}, {X: 750, Y: 350}}, ExpectedVersion: 3}, detourActor()); err == nil {
		t.Fatal("locked plan must not accept a new detour")
	}
	copied, err := svc.Copy(locked.ID, detourActor())
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if copied.PlanState != constants.PlanDraft {
		t.Fatal("copied plan must be a new draft")
	}
	copiedLines, err := geometry.ParseLines(copied.LineGeoJSON)
	if err != nil {
		t.Fatalf("parse copied lines: %v", err)
	}
	if len(copiedLines[2]) != 4 {
		t.Fatalf("copied draft must keep detour vertices, got %d positions", len(copiedLines[2]))
	}
	lockedFresh, err := svc.Get(locked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if lockedFresh.PlanState != constants.PlanLocked {
		t.Fatal("source locked version must remain locked/immutable")
	}
}

func asAppError(err error, target **api.AppError) bool {
	for err != nil {
		if appErr, ok := err.(*api.AppError); ok {
			*target = appErr
			return true
		}
		type wrapper interface{ Unwrap() error }
		unwrapped, ok := err.(wrapper)
		if !ok {
			return false
		}
		err = unwrapped.Unwrap()
	}
	return false
}

func orbPoint(x, y float64) [2]float64 { return [2]float64{x, y} }
