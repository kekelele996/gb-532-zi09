package dto

import "encoding/json"

type CreateTransectPlanRequest struct {
	SurveyAreaID   uint            `json:"survey_area_id" binding:"required,gt=0"`
	Name           string          `json:"name" binding:"required,min=3,max=160"`
	LineGeoJSON    json.RawMessage `json:"line_geojson" binding:"required"`
	PlannedHeading float64         `json:"planned_heading" binding:"gte=0,lt=360"`
	PlannedSwathM  float64         `json:"planned_swath_m" binding:"required,gt=0,lte=2000"`
	LineSpacingM   float64         `json:"line_spacing_m" binding:"required,gt=0,lte=2000"`
}

type UpdateTransectPlanRequest struct {
	Name            string          `json:"name" binding:"required,min=3,max=160"`
	LineGeoJSON     json.RawMessage `json:"line_geojson" binding:"required"`
	PlannedHeading  float64         `json:"planned_heading" binding:"gte=0,lt=360"`
	PlannedSwathM   float64         `json:"planned_swath_m" binding:"required,gt=0,lte=2000"`
	LineSpacingM    float64         `json:"line_spacing_m" binding:"required,gt=0,lte=2000"`
	ExpectedVersion uint            `json:"expected_version" binding:"required,gt=0"`
}

type GenerateLinesRequest struct {
	SurveyAreaID  uint    `json:"survey_area_id" binding:"required,gt=0"`
	Name          string  `json:"name" binding:"required,min=3,max=160"`
	Heading       float64 `json:"heading" binding:"gte=0,lt=360"`
	LineSpacingM  float64 `json:"line_spacing_m" binding:"required,gt=0,lte=2000"`
	PlannedSwathM float64 `json:"planned_swath_m" binding:"required,gt=0,lte=2000"`
}

type PlanTransitionRequest struct {
	TargetState     string `json:"target_state" binding:"required,oneof=locked"`
	ExpectedVersion uint   `json:"expected_version" binding:"required,gt=0"`
}

// DetourPoint 是绕行折点的米制投影坐标（不接受经纬度）。
type DetourPoint struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

// ApplyDetourRequest 在指定测线上安排两处绕行折点。
type ApplyDetourRequest struct {
	LineIndex       int          `json:"line_index"`
	FirstVertex     *DetourPoint `json:"first_vertex"`
	SecondVertex    *DetourPoint `json:"second_vertex"`
	ExpectedVersion uint         `json:"expected_version" binding:"required,gt=0"`
}

type TransectPlanQuery struct {
	SurveyAreaID uint
	State        string
	Page         int
	PageSize     int
}
