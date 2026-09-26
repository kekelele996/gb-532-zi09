import AddRoadRounded from '@mui/icons-material/AddRoadRounded'
import ContentCopyRounded from '@mui/icons-material/ContentCopyRounded'
import LockRounded from '@mui/icons-material/LockRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import TimelineRounded from '@mui/icons-material/TimelineRounded'
import { Alert, Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControl, IconButton, InputLabel, LinearProgress, MenuItem, Select, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { useEffect, useMemo, useState } from 'react'
import { GeometryDetailDrawer } from '../components/common/GeometryDetailDrawer'
import { MapLegend } from '../components/common/MapLegend'
import { PageHeader } from '../components/common/PageHeader'
import { SurveyCanvas, type CanvasHit, type CanvasVertex } from '../components/common/SurveyCanvas'
import { useAuth } from '../hooks/useAuth'
import { useCoverageLayers } from '../hooks/useCoverageLayers'
import { ApiError } from '../api/client'
import type { Position } from '../types/api'
import { useSurveyAreaStore } from '../stores/survey-area-store'
import { useTransectPlanStore } from '../stores/transect-plan-store'
import type { GenerateTransectPlan } from '../types/transect-plan'
import { planDetourIndices } from '../types/transect-plan'
import { geometryLines } from '../utils/geometry'

type DetourStage='idle'|'pick-line'|'pick-vertices'
interface DetourState{stage:DetourStage;lineIndex:number|null;vertices:Position[];error:string}
const initialDetour:DetourState={stage:'idle',lineIndex:null,vertices:[],error:''}

export function PlansPage(){const{hasRole}=useAuth();const areas=useSurveyAreaStore(state=>state.items);const fetchAreas=useSurveyAreaStore(state=>state.fetch);const{items,selected,loading,fetch,select,generate,lock,copy,applyDetour}=useTransectPlanStore();const[open,setOpen]=useState(false);const[drawer,setDrawer]=useState(false);const[detour,setDetour]=useState<DetourState>(initialDetour);const[submitting,setSubmitting]=useState(false);const[form,setForm]=useState<GenerateTransectPlan>({survey_area_id:1,name:'自动平行测线',heading:90,line_spacing_m:160,planned_swath_m:180});useEffect(()=>{void Promise.all([fetch(),fetchAreas()])},[fetch,fetchAreas]);useEffect(()=>{if(areas[0]&&form.survey_area_id===1&&!areas.some(area=>area.id===1))setForm(value=>({...value,survey_area_id:areas[0]!.id}))},[areas,form.survey_area_id]);useEffect(()=>{setDetour(initialDetour)},[selected?.id]);const area=selected?.survey_area??areas.find(value=>value.id===selected?.survey_area_id);const layers=useCoverageLayers(area,selected);const legends=useMemo(()=>layers.map(layer=>({label:layer.label,color:layer.color,pattern:layer.dash?'dash' as const:layer.fill?'fill' as const:'solid' as const})),[layers]);const planLayerIndex=useMemo(()=>layers.findIndex(layer=>layer.label.startsWith('计划测线')),[layers]);const submit=async()=>{await generate(form);setOpen(false)};const canEdit=hasRole('admin','survey_planner');const detourable=canEdit&&selected?.plan_state==='draft';const lineCount=selected?geometryLines(selected.line_geojson).length:0
const temporaryVertices=useMemo<CanvasVertex[]>(()=>detour.stage==='pick-vertices'?detour.vertices.map((point,index)=>({point,color:index===0?'#255d86':'#b8443c'})):[],[detour.stage,detour.vertices])
const existingDetours=useMemo(()=>selected?planDetourIndices(selected.line_geojson):new Set<number>(),[selected])
const handleCanvasPick=(world:Position,hit:CanvasHit|null)=>{
  if(detour.stage==='pick-line'){
    if(hit)setDetour({stage:'pick-vertices',lineIndex:hit.lineIndex,vertices:[],error:''})
    return
  }
  if(detour.stage==='pick-vertices'){
    setDetour(state=>state.vertices.length>=2?state:{...state,vertices:[...state.vertices,world]})
  }
}
const submitDetour=async()=>{
  if(detour.lineIndex===null||detour.vertices.length!==2||!selected)return
  setSubmitting(true);setDetour(state=>({...state,error:''}))
  try{
    await applyDetour(selected,{line_index:detour.lineIndex,first_vertex:{x:detour.vertices[0]![0],y:detour.vertices[0]![1]},second_vertex:{x:detour.vertices[1]![0],y:detour.vertices[1]![1]}})
    setDetour(initialDetour)
  }catch(error){
    const message=error instanceof ApiError?describeDetourError(error):'绕行保存失败，请稍后重试'
    setDetour(state=>({...state,error:message}))
  }finally{setSubmitting(false)}
}
return <>
  <PageHeader eyebrow="TRANSECT DESIGN DESK" title="测线规划器" description="从测区边界生成平行测线，锁定用于航迹导入；礁区可在锁定前为单条直线安排两处折点绕行。" actions={<><Tooltip title="刷新"><IconButton aria-label="刷新规划" onClick={()=>void fetch()}><RefreshRounded/></IconButton></Tooltip>{canEdit&&<Button variant="contained" startIcon={<AddRoadRounded/>} onClick={()=>setOpen(true)}>生成测线</Button>}</>}/>{loading&&<LinearProgress/>}
  <Box className="planner-workspace"><Box className="plan-list" role="list" aria-label="测线规划列表"><Box className="section-heading"><Typography variant="h6">规划版本</Typography><Typography variant="caption">{items.length} 套</Typography></Box>{items.map(item=><button type="button" role="listitem" key={item.id} className={selected?.id===item.id?'plan-row active':'plan-row'} onClick={()=>select(item)}><span><strong>{item.name}</strong><small>{item.survey_area?.area_code??`AREA #${item.survey_area_id}`} · V{item.version}</small></span><span><Chip size="small" label={item.plan_state==='locked'?'已锁定':'草稿'} color={item.plan_state==='locked'?'success':'default'} variant="outlined"/><small>{item.planned_heading}° / {item.line_spacing_m} m</small></span></button>)}</Box>
    <Box className="map-workbench"><Box className="section-heading"><Box><Typography variant="overline">LOCAL CANVAS / METRE GRID</Typography><Typography variant="h6">{selected?.name??'选择规划'}</Typography></Box>{selected&&<Stack direction="row" gap={1} flexWrap="wrap">{detourable&&detour.stage==='idle'&&<Button size="small" startIcon={<TimelineRounded/>} onClick={()=>setDetour({stage:'pick-line',lineIndex:null,vertices:[],error:''})}>礁区绕行</Button>}{canEdit&&selected.plan_state==='draft'&&<Button size="small" startIcon={<LockRounded/>} onClick={()=>void lock(selected)}>锁定</Button>}{canEdit&&selected.plan_state==='locked'&&<Button size="small" startIcon={<ContentCopyRounded/>} onClick={()=>void copy(selected)}>复制版本</Button>}<Button size="small" onClick={()=>setDrawer(true)}>几何详情</Button></Stack>}</Box>
      {selected&&detourable&&detour.stage!=='idle'&&<DetourPanel detour={detour} lineCount={lineCount} existingDetours={existingDetours} submitting={submitting} onCancel={()=>setDetour(initialDetour)} onResetVertices={()=>setDetour(state=>({...state,vertices:[]}))} onSubmit={()=>void submitDetour()}/>}
      <SurveyCanvas layers={layers} pickableLayerIndex={detour.stage!=='idle'?planLayerIndex:undefined} onCanvasClick={handleCanvasPick} temporaryVertices={temporaryVertices}/>
      <MapLegend items={legends}/>{selected&&<Box className="metric-strip"><div><span>计划航向</span><strong>{selected.planned_heading}°</strong></div><div><span>线间距</span><strong>{selected.line_spacing_m} m</strong></div><div><span>计划扫幅</span><strong>{selected.planned_swath_m} m</strong></div><div><span>边界投影</span><strong>{area?.coordinate_system??'--'}</strong></div></Box>}</Box></Box>
  <Dialog open={open} onClose={()=>setOpen(false)} fullWidth maxWidth="sm"><DialogTitle>生成平行测线</DialogTitle><DialogContent><Stack gap={2} sx={{pt:1}}><FormControl><InputLabel>测区</InputLabel><Select label="测区" value={form.survey_area_id} onChange={event=>setForm({...form,survey_area_id:Number(event.target.value)})}>{areas.filter(area=>area.status!=='archived').map(area=><MenuItem key={area.id} value={area.id}>{area.area_code} · {area.name}</MenuItem>)}</Select></FormControl><TextField label="规划名称" value={form.name} onChange={event=>setForm({...form,name:event.target.value})}/><Stack direction={{xs:'column',sm:'row'}} gap={2}><TextField fullWidth type="number" label="航向 (°)" value={form.heading} onChange={event=>setForm({...form,heading:Number(event.target.value)})}/><TextField fullWidth type="number" label="线间距 (m)" value={form.line_spacing_m} onChange={event=>setForm({...form,line_spacing_m:Number(event.target.value)})}/><TextField fullWidth type="number" label="计划扫幅 (m)" value={form.planned_swath_m} onChange={event=>setForm({...form,planned_swath_m:Number(event.target.value)})}/></Stack></Stack></DialogContent><DialogActions><Button onClick={()=>setOpen(false)}>取消</Button><Button variant="contained" onClick={()=>void submit()}>生成草稿</Button></DialogActions></Dialog>
  <GeometryDetailDrawer open={drawer} onClose={()=>setDrawer(false)} title={selected?.name??'测线几何'} geometry={selected?.line_geojson} metadata={selected?{'规划状态':selected.plan_state,'计划航向':`${selected.planned_heading}°`,'线间距':`${selected.line_spacing_m} m`,'版本':selected.version,'绕行折点':existingDetours.size?`${[...existingDetours].map(index=>`#${index+1}`).join('、')} 测线`:'无'}:undefined}/>
  </>}

function DetourPanel({detour,lineCount,existingDetours,submitting,onCancel,onResetVertices,onSubmit}:{detour:DetourState;lineCount:number;existingDetours:Set<number>;submitting:boolean;onCancel:()=>void;onResetVertices:()=>void;onSubmit:()=>void}){
  const alreadyDetoured=detour.lineIndex!==null&&existingDetours.has(detour.lineIndex)
  return <Alert severity={detour.error?'error':'info'} variant="outlined" sx={{alignItems:'center',textAlign:'left'}} action={<Stack direction="row" gap={1}>
    {detour.stage==='pick-vertices'&&<Button size="small" onClick={onResetVertices} disabled={detour.vertices.length===0||submitting}>重选折点</Button>}
    <Button size="small" onClick={onCancel} disabled={submitting}>取消</Button>
    {detour.stage==='pick-vertices'&&<Button size="small" variant="contained" onClick={onSubmit} disabled={detour.vertices.length!==2||submitting}>{submitting?'保存中…':'保存绕行'}</Button>}
  </Stack>}>
    <Typography variant="body2">
      {detour.stage==='pick-line'&&<>第 1 步：在画布上点击需要绕开礁区的测线（共 {lineCount} 条，按列表顺序编号）。</>}
      {detour.stage==='pick-vertices'&&<>第 2 步：在选中的第 {detour.lineIndex!+1} 条测线两侧点击两处折点（米制投影坐标），按测线方向自动排序；折点必须落在测区内，绕行段不得穿越相邻测线。{alreadyDetoured&&<> 该测线已有绕行，保存后将以新折点替换。</>}<br/>已选折点：{detour.vertices.length===0?'无':detour.vertices.map((point,index)=>` ${index===0?'P1':'P2'}(${point[0]}, ${point[1]})`).join('；')}{detour.vertices.length<2&&`（还需 ${2-detour.vertices.length} 处）`}</>}
    </Typography>
    {detour.error&&<Typography variant="body2" sx={{mt:0.5,fontWeight:600}}>{detour.error}</Typography>}
  </Alert>
}

function describeDetourError(error:ApiError):string{
  if(error.code==='DETOUR_CONFLICT'){
    const details=error.details as {conflict_line_index?:number;intersection?:[number,number]}|undefined
    const line=details?.conflict_line_index!==undefined?`第 ${details.conflict_line_index+1} 条`:''
    const point=Array.isArray(details?.intersection)?`（交点约 ${details!.intersection![0].toFixed(1)} m, ${details!.intersection![1].toFixed(1)} m）`:''
    return `拒绝保存：绕行段碰到${line}相邻测线${point}，原几何已保留，请调整折点后重试。`
  }
  if(error.code==='DETOUR_OUTSIDE_AREA'||error.code==='DETOUR_VERTEX_DUPLICATE'||error.code==='DETOUR_LINE_NOT_FOUND')return `拒绝保存：${error.message}，原几何已保留。`
  return error.message
}
