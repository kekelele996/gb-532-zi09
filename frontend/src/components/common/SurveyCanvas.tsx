import { useEffect, useRef } from 'react'
import { Box } from '@mui/material'
import type { GeoJSONFeature, Position } from '../../types/api'
import { geometryBounds, geometryLines } from '../../utils/geometry'

export interface MapLayer{feature:GeoJSONFeature;label:string;color:string;fill?:string;width?:number;dash?:number[]}
export interface CanvasHit{layerIndex:number;lineIndex:number;point:Position}
export interface CanvasVertex{point:Position;color:string}
export type CanvasClickHandler=(world:Position,hit:CanvasHit|null)=>void

export function SurveyCanvas({layers,ariaLabel='测绘几何画布',pickableLayerIndex,onCanvasClick,temporaryVertices}:{layers:MapLayer[];ariaLabel?:string;pickableLayerIndex?:number;onCanvasClick?:CanvasClickHandler;temporaryVertices?:CanvasVertex[]}){
  const canvasRef=useRef<HTMLCanvasElement>(null);const hostRef=useRef<HTMLDivElement>(null)
  const pickRef=useRef<{layerIndex:number;onClick:CanvasClickHandler}|null>(null)
  pickRef.current=pickableLayerIndex===undefined||!onCanvasClick?null:{layerIndex:pickableLayerIndex,onClick:onCanvasClick}
  const verticesRef=useRef<CanvasVertex[]>(temporaryVertices??[])
  verticesRef.current=temporaryVertices??[]
  const drawRef=useRef<()=>void>(()=>{})
  useEffect(()=>{
    const canvas=canvasRef.current,host=hostRef.current;if(!canvas||!host)return
    const draw=()=>{
      const rect=host.getBoundingClientRect();const ratio=Math.min(window.devicePixelRatio||1,2);canvas.width=Math.max(1,Math.round(rect.width*ratio));canvas.height=Math.max(1,Math.round(rect.height*ratio));canvas.style.width=`${rect.width}px`;canvas.style.height=`${rect.height}px`
      const context=canvas.getContext('2d');if(!context)return;context.setTransform(ratio,0,0,ratio,0,0);const width=rect.width,height=rect.height
      context.fillStyle='#edf4f3';context.fillRect(0,0,width,height);context.strokeStyle='#d2dfdd';context.lineWidth=1
      for(let x=24;x<width;x+=48){context.beginPath();context.moveTo(x,0);context.lineTo(x,height);context.stroke()}for(let y=24;y<height;y+=48){context.beginPath();context.moveTo(0,y);context.lineTo(width,y);context.stroke()}
      const bounds=geometryBounds(layers.map(layer=>layer.feature));if(!bounds){context.fillStyle='#617475';context.font='14px sans-serif';context.fillText('等待几何证据',24,36);return}
      let{minX,maxX,minY,maxY}=bounds;if(maxX===minX){maxX+=1;minX-=1}if(maxY===minY){maxY+=1;minY-=1}
      const padding=32,scale=Math.min((width-padding*2)/(maxX-minX),(height-padding*2)/(maxY-minY));const project=(point:Position):Position=>[padding+(point[0]-minX)*scale,height-padding-(point[1]-minY)*scale]
      layers.forEach(layer=>{context.strokeStyle=layer.color;context.fillStyle=layer.fill??'transparent';context.lineWidth=layer.width??2;context.setLineDash(layer.dash??[]);geometryLines(layer.feature).forEach(line=>{if(!line.length)return;context.beginPath();line.forEach((point,index)=>{const [x,y]=project(point);if(index===0)context.moveTo(x,y);else context.lineTo(x,y)});if(layer.feature.geometry.type.includes('Polygon'))context.closePath();if(layer.fill)context.fill();context.stroke()});context.setLineDash([])});
      verticesRef.current.forEach(vertex=>{const [x,y]=project(vertex.point);context.beginPath();context.fillStyle=vertex.color;context.strokeStyle='#ffffff';context.lineWidth=1.5;context.arc(x,y,5,0,Math.PI*2);context.fill();context.stroke()})
      context.fillStyle='#415b5c';context.font='11px ui-monospace, monospace';context.fillText(`${minX.toFixed(0)} m`,padding,height-10);context.textAlign='right';context.fillText(`${maxX.toFixed(0)} m`,width-padding,height-10);context.textAlign='left'
    }
    drawRef.current=draw
    const onClick=(event:MouseEvent)=>{
      const pick=pickRef.current;if(!pick)return
      const rect=canvas.getBoundingClientRect()
      const bounds=geometryBounds(layers.map(layer=>layer.feature));if(!bounds)return
      let{minX,maxX,minY,maxY}=bounds;if(maxX===minX){maxX+=1;minX-=1}if(maxY===minY){maxY+=1;minY-=1}
      const padding=32,scale=Math.min((rect.width-padding*2)/(maxX-minX),(rect.height-padding*2)/(maxY-minY))
      const sx=event.clientX-rect.left,sy=event.clientY-rect.top
      const world:Position=[Math.round((minX+(sx-padding)/scale)*100)/100,Math.round((maxY-(sy-padding)/scale)*100)/100]
      const layer=layers[pick.layerIndex];if(!layer){pick.onClick(world,null);return}
      let bestLineIndex=-1;let bestDistancePx=Infinity;let bestPoint:Position|null=null
      geometryLines(layer.feature).forEach((line,lineIndex)=>{for(let index=1;index<line.length;index++){const start=line[index-1],end=line[index];if(!start||!end)continue;const projection=projectOnSegment(world,start,end);const distancePx=projection.distance*scale;if(distancePx<bestDistancePx){bestDistancePx=distancePx;bestLineIndex=lineIndex;bestPoint=projection.point}}})
      if(bestPoint&&bestDistancePx<=12){
        pick.onClick(world,{layerIndex:pick.layerIndex,lineIndex:bestLineIndex,point:[Math.round(bestPoint[0]*100)/100,Math.round(bestPoint[1]*100)/100]})
      }else{
        pick.onClick(world,null)
      }
    }
    draw();const observer=new ResizeObserver(draw);observer.observe(host);canvas.addEventListener('click',onClick);return()=>{observer.disconnect();canvas.removeEventListener('click',onClick)}
  },[layers])
  // 折点等临时标记变化时只需重绘，避免重新绑定点击监听。
  useEffect(()=>{drawRef.current()},[temporaryVertices])
  return <Box ref={hostRef} className="survey-canvas" role="img" aria-label={ariaLabel} style={{cursor:pickableLayerIndex!==undefined&&onCanvasClick?'crosshair':undefined}}><canvas ref={canvasRef}/></Box>
}

// projectOnSegment 返回点击位置（米制世界坐标）到线段的最近投影点及米制距离。
function projectOnSegment(click:Position,start:Position,end:Position){
  const dx=end[0]-start[0],dy=end[1]-start[1];const lengthSquared=dx*dx+dy*dy
  let t=0;if(lengthSquared>0)t=Math.min(1,Math.max(0,((click[0]-start[0])*dx+(click[1]-start[1])*dy)/lengthSquared))
  const point:Position=[start[0]+t*dx,start[1]+t*dy]
  return{point,distance:Math.hypot(click[0]-point[0],click[1]-point[1])}
}
