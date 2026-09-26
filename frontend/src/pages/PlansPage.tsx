import AddRoadRounded from '@mui/icons-material/AddRoadRounded'
import ContentCopyRounded from '@mui/icons-material/ContentCopyRounded'
import LockRounded from '@mui/icons-material/LockRounded'
import RefreshRounded from '@mui/icons-material/RefreshRounded'
import RouteRounded from '@mui/icons-material/RouteRounded'
import UndoRounded from '@mui/icons-material/UndoRounded'
import { Alert, Box, Button, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControl, IconButton, InputLabel, LinearProgress, MenuItem, Select, Stack, TextField, Tooltip, Typography } from '@mui/material'
import { useEffect, useMemo, useState } from 'react'
import { GeometryDetailDrawer } from '../components/common/GeometryDetailDrawer'
import { MapLegend } from '../components/common/MapLegend'
import { PageHeader } from '../components/common/PageHeader'
import { SurveyCanvas, type MapLayer } from '../components/common/SurveyCanvas'
import { useAuth } from '../hooks/useAuth'
import { useCoverageLayers } from '../hooks/useCoverageLayers'
import { useSurveyAreaStore } from '../stores/survey-area-store'
import { useTransectPlanStore } from '../stores/transect-plan-store'
import type { Position } from '../types/api'
import type { GenerateTransectPlan } from '../types/transect-plan'
import { buildDetouredFeature, geometryLines, orderDetourVertices, pointInPolygon } from '../utils/geometry'

function pointFeature(point:Position):MapLayer['feature']{
  return {type:'Feature',properties:{},geometry:{type:'LineString',coordinates:[[point[0]-12,point[1]-12],[point[0]+12,point[1]+12],[point[0]-12,point[1]+12],[point[0]+12,point[1]-12]]}}
}

export function PlansPage(){
  const{hasRole}=useAuth()
  const areas=useSurveyAreaStore(state=>state.items);const fetchAreas=useSurveyAreaStore(state=>state.fetch)
  const{items,selected,loading,fetch,select,generate,lock,detour,copy}=useTransectPlanStore()
  const[open,setOpen]=useState(false);const[drawer,setDrawer]=useState(false)
  const[form,setForm]=useState<GenerateTransectPlan>({survey_area_id:1,name:'自动平行测线',heading:90,line_spacing_m:160,planned_swath_m:180})
  const[detouring,setDetouring]=useState(false)
  const[detourLine,setDetourLine]=useState(0)
  const[picked,setPicked]=useState<Position[]>([])
  const[clientWarning,setClientWarning]=useState('')
  useEffect(()=>{void Promise.all([fetch(),fetchAreas()])},[fetch,fetchAreas])
  useEffect(()=>{if(areas[0]&&form.survey_area_id===1&&!areas.some(area=>area.id===1))setForm(value=>({...value,survey_area_id:areas[0]!.id}))},[areas,form.survey_area_id])
  useEffect(()=>{setDetouring(false);setPicked([]);setClientWarning('')},[selected?.id])
  const area=selected?.survey_area??areas.find(value=>value.id===selected?.survey_area_id)
  const baseLayers=useCoverageLayers(area,selected)
  const selectedLines=useMemo(()=>selected?geometryLines(selected.line_geojson):[], [selected])
  const planLayers=useMemo(()=>{
    if(!selected||!detouring||picked.length===0)return baseLayers
    const layers=[...baseLayers]
    if(picked.length===2){
      const ordered=orderDetourVertices(selectedLines[detourLine]??[], [picked[0]!,picked[1]!] as [Position,Position])
      layers.push({feature:buildDetouredFeature(selected.line_geojson,detourLine,ordered),label:'绕行预览',color:'#c38c20',width:3,dash:[9,4]})
    }
    picked.forEach((point,index)=>layers.push({feature:pointFeature(point),label:`折点 ${index+1}`,color:'#b8443c',width:2}))
    return layers
  },[baseLayers,selected,detouring,picked,detourLine,selectedLines])
  const legends=useMemo(()=>{
    const base=planLayers.map(layer=>({label:layer.label,color:layer.color,pattern:layer.dash?'dash' as const:layer.fill?'fill' as const:'solid' as const}))
    return detouring?[...base,{label:'折点（点击画布拾取）',color:'#b8443c',pattern:'solid' as const}]:base
  },[planLayers,detouring])
  const submit=async()=>{await generate(form);setOpen(false)}
  const canEdit=hasRole('admin','survey_planner')
  const startDetour=()=>{setDetouring(true);setPicked([]);setClientWarning('');setDetourLine(0)}
  const cancelDetour=()=>{setDetouring(false);setPicked([]);setClientWarning('')}
  const pickPoint=(point:Position)=>{
    if(!selected||!area)return
    const rounded:Position=[Math.round(point[0]*10)/10,Math.round(point[1]*10)/10]
    if(!pointInPolygon(rounded,area.boundary_geojson)){setClientWarning('折点必须落在测区内部（边界上不允许）。');return}
    setClientWarning('')
    setPicked(current=>current.length>=2?[rounded]:[...current,rounded])
  }
  const saveDetour=async()=>{
    if(!selected||picked.length!==2)return
    try{
      await detour(selected,{line_index:detourLine,vertices:[picked[0]!,picked[1]!]})
      setDetouring(false);setPicked([]);setClientWarning('')
    }catch{/* 统一 api:error Snackbar 已提示冲突坐标与原因；几何未修改，保留拾取状态供调整 */}
  }
  return <>
  <PageHeader eyebrow="TRANSECT DESIGN DESK" title="测线规划器" description="从测区边界生成平行测线，遇礁区可对单条直线安排两处绕行折点；锁定用于航迹导入，复制形成下一版本。" actions={<><Tooltip title="刷新"><IconButton aria-label="刷新规划" onClick={()=>void fetch()}><RefreshRounded/></IconButton></Tooltip>{canEdit&&<Button variant="contained" startIcon={<AddRoadRounded/>} onClick={()=>setOpen(true)}>生成测线</Button>}</>}/>{loading&&<LinearProgress/>}
  <Box className="planner-workspace"><Box className="plan-list" role="list" aria-label="测线规划列表"><Box className="section-heading"><Typography variant="h6">规划版本</Typography><Typography variant="caption">{items.length} 套</Typography></Box>{items.map(item=><button type="button" role="listitem" key={item.id} className={selected?.id===item.id?'plan-row active':'plan-row'} onClick={()=>select(item)}><span><strong>{item.name}</strong><small>{item.survey_area?.area_code??`AREA #${item.survey_area_id}`} · V{item.version}</small></span><span><Chip size="small" label={item.plan_state==='locked'?'已锁定':'草稿'} color={item.plan_state==='locked'?'success':'default'} variant="outlined"/><small>{item.planned_heading}° / {item.line_spacing_m} m</small></span></button>)}</Box>
    <Box className="map-workbench"><Box className="section-heading"><Box><Typography variant="overline">LOCAL CANVAS / METRE GRID</Typography><Typography variant="h6">{selected?.name??'选择规划'}</Typography></Box>{selected&&<Stack direction="row" gap={1} flexWrap="wrap">{canEdit&&selected.plan_state==='draft'&&!detouring&&<Button size="small" startIcon={<RouteRounded/>} onClick={startDetour}>绕行折点</Button>}{canEdit&&selected.plan_state==='draft'&&detouring&&<Button size="small" color="warning" startIcon={<UndoRounded/>} onClick={cancelDetour}>退出绕行</Button>}{canEdit&&selected.plan_state==='draft'&&!detouring&&<Button size="small" startIcon={<LockRounded/>} onClick={()=>void lock(selected)}>锁定</Button>}{canEdit&&selected.plan_state==='locked'&&<Button size="small" startIcon={<ContentCopyRounded/>} onClick={()=>void copy(selected)}>复制版本</Button>}<Button size="small" onClick={()=>setDrawer(true)}>几何详情</Button></Stack>}</Box>
      {selected&&detouring&&selected.plan_state==='draft'&&<Box className="detour-panel">
        <Stack direction={{xs:'column',sm:'row'}} gap={2} alignItems={{sm:'center'}}>
          <FormControl size="small" sx={{minWidth:220}}><InputLabel id="detour-line-label">绕行目标直线</InputLabel><Select labelId="detour-line-label" label="绕行目标直线" value={detourLine} onChange={event=>{setDetourLine(Number(event.target.value));setPicked([]);setClientWarning('')}}>{selectedLines.map((line,index)=><MenuItem key={index} value={index}>第 {index+1} 条 · {Math.round(line[0]![0])},{Math.round(line[0]![1])} → {Math.round(line[line.length-1]![0])},{Math.round(line[line.length-1]![1])}</MenuItem>)}</Select></FormControl>
          <Typography variant="body2" color="text.secondary">在画布上点击两处折点（{picked.length}/2）；其他测线、线间距和计划扫幅保持不变。</Typography>
          <Box sx={{ml:'auto'}}><Button size="small" disabled={picked.length===0} onClick={()=>setPicked(current=>current.slice(0,-1))}>撤销一点</Button><Button size="small" variant="contained" disabled={picked.length!==2||loading} onClick={()=>void saveDetour()}>保存绕行</Button></Box>
        </Stack>
        {clientWarning&&<Alert severity="warning" sx={{mt:1}}>{clientWarning}</Alert>}
      </Box>}
      <SurveyCanvas layers={planLayers} onPickPoint={detouring&&selected?.plan_state==='draft'?pickPoint:undefined} pickCursor={detouring&&selected?.plan_state==='draft'}/><MapLegend items={legends}/>{selected&&<Box className="metric-strip"><div><span>计划航向</span><strong>{selected.planned_heading}°</strong></div><div><span>线间距</span><strong>{selected.line_spacing_m} m</strong></div><div><span>计划扫幅</span><strong>{selected.planned_swath_m} m</strong></div><div><span>边界投影</span><strong>{area?.coordinate_system??'--'}</strong></div></Box>}</Box></Box>
  <Dialog open={open} onClose={()=>setOpen(false)} fullWidth maxWidth="sm"><DialogTitle>生成平行测线</DialogTitle><DialogContent><Stack gap={2} sx={{pt:1}}><FormControl><InputLabel>测区</InputLabel><Select label="测区" value={form.survey_area_id} onChange={event=>setForm({...form,survey_area_id:Number(event.target.value)})}>{areas.filter(area=>area.status!=='archived').map(area=><MenuItem key={area.id} value={area.id}>{area.area_code} · {area.name}</MenuItem>)}</Select></FormControl><TextField label="规划名称" value={form.name} onChange={event=>setForm({...form,name:event.target.value})}/><Stack direction={{xs:'column',sm:'row'}} gap={2}><TextField fullWidth type="number" label="航向 (°)" value={form.heading} onChange={event=>setForm({...form,heading:Number(event.target.value)})}/><TextField fullWidth type="number" label="线间距 (m)" value={form.line_spacing_m} onChange={event=>setForm({...form,line_spacing_m:Number(event.target.value)})}/><TextField fullWidth type="number" label="计划扫幅 (m)" value={form.planned_swath_m} onChange={event=>setForm({...form,planned_swath_m:Number(event.target.value)})}/></Stack></Stack></DialogContent><DialogActions><Button onClick={()=>setOpen(false)}>取消</Button><Button variant="contained" onClick={()=>void submit()}>生成草稿</Button></DialogActions></Dialog>
  <GeometryDetailDrawer open={drawer} onClose={()=>setDrawer(false)} title={selected?.name??'测线几何'} geometry={selected?.line_geojson} metadata={selected?{'规划状态':selected.plan_state,'计划航向':`${selected.planned_heading}°`,'线间距':`${selected.line_spacing_m} m`,'版本':selected.version}:undefined}/>
  </>}
