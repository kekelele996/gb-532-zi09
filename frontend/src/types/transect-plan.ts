import type { GeoJSONFeature, Position } from './api'
import type { SurveyArea } from './survey-area'

export interface TransectPlan {
  id:number; survey_area_id:number; name:string; line_geojson:GeoJSONFeature; planned_heading:number; planned_swath_m:number; line_spacing_m:number
  plan_state:'draft'|'locked'; version:number; created_by:number; created_at:string; updated_at:string; survey_area?:SurveyArea
}
export interface GenerateTransectPlan { survey_area_id:number; name:string; heading:number; line_spacing_m:number; planned_swath_m:number }
export interface DetourRequest { line_index:number; first_vertex:{x:number;y:number}; second_vertex:{x:number;y:number}; expected_version:number }
export function planDetourIndices(feature:GeoJSONFeature):Set<number>{
  const raw=feature.properties?.detours as Record<string,Position[]>|undefined
  return new Set(raw?Object.keys(raw).map(key=>Number(key)):[])
}
