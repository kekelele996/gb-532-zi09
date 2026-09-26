import type { GeoJSONFeature, Position } from '../types/api'

export function geometryLines(feature:GeoJSONFeature):Position[][]{
  const geometry=feature.geometry
  if(geometry.type==='LineString')return[geometry.coordinates]
  if(geometry.type==='MultiLineString')return geometry.coordinates
  if(geometry.type==='Polygon')return geometry.coordinates
  return geometry.coordinates.flat()
}

export function geometryBounds(features:GeoJSONFeature[]){
  const points=features.flatMap(feature=>geometryLines(feature).flat())
  if(points.length===0)return null
  const xs=points.map(point=>point[0]),ys=points.map(point=>point[1])
  return{minX:Math.min(...xs),minY:Math.min(...ys),maxX:Math.max(...xs),maxY:Math.max(...ys),pointCount:points.length}
}

// Point-in-polygon with an explicit edge tolerance: a point on the polygon
// boundary is outside, matching the backend requirement that detour vertices
// fall strictly inside the survey area.
export function pointInPolygon(point:Position,feature:GeoJSONFeature):boolean{
  const rings=polygonRings(feature)
  if(rings.length===0)return false
  if(!ringContains(point,rings[0]!)||pointOnRing(point,rings[0]!))return false
  return rings.slice(1).every(ring=>!ringContains(point,ring)&&!pointOnRing(point,ring))
}

function polygonRings(feature:GeoJSONFeature):Position[][]{
  const geometry=feature.geometry
  if(geometry.type==='Polygon')return geometry.coordinates
  if(geometry.type==='MultiPolygon')return geometry.coordinates.flat()
  return []
}

function ringContains(point:Position,ring:Position[]):boolean{
  let inside=false
  for(let current=0,previous=ring.length-1;current<ring.length;previous=current,current++){
    const a=ring[current]!,b=ring[previous]!
    const intersects=(a[1]>point[1])!==(b[1]>point[1])&&point[0]<((b[0]-a[0])*(point[1]-a[1]))/(b[1]-a[1])+a[0]
    if(intersects)inside=!inside
  }
  return inside
}

function pointOnRing(point:Position,ring:Position[]):boolean{
  for(let index=1;index<ring.length;index++){
    if(pointSegmentDistance(point,ring[index-1]!,ring[index]!)<=1e-9)return true
  }
  return false
}

function pointSegmentDistance(point:Position,start:Position,end:Position):number{
  const dx=end[0]-start[0],dy=end[1]-start[1]
  if(dx===0&&dy===0)return Math.hypot(point[0]-start[0],point[1]-start[1])
  let t=((point[0]-start[0])*dx+(point[1]-start[1])*dy)/(dx*dx+dy*dy)
  t=Math.max(0,Math.min(1,t))
  return Math.hypot(point[0]-(start[0]+t*dx),point[1]-(start[1]+t*dy))
}

// Orders the two detour vertices by projection on the base line, mirroring
// geometry.OrderedDetourVertices on the backend.
export function orderDetourVertices(base:Position[],vertices:[Position,Position]):[Position,Position]{
  if(base.length<2)return vertices
  const start=base[0]!,end=base[base.length-1]!
  const dx=end[0]-start[0],dy=end[1]-start[1]
  const lengthSquared=dx*dx+dy*dy
  if(lengthSquared===0)return vertices
  const projection=(vertex:Position)=>((vertex[0]-start[0])*dx+(vertex[1]-start[1])*dy)/lengthSquared
  return projection(vertices[1]!)<projection(vertices[0]!)?[vertices[1]!,vertices[0]!]:vertices
}

// Builds the replacement feature for one selected line after inserting the
// two detour vertices; neighbouring lines are passed through unchanged.
export function buildDetouredFeature(planFeature:GeoJSONFeature,lineIndex:number,vertices:[Position,Position]):GeoJSONFeature{
  const lines=geometryLines(planFeature)
  const base=lines[lineIndex]
  if(!base||base.length<2)return planFeature
  const ordered=orderDetourVertices(base,vertices)
  const detoured:Position[]=[base[0]!,ordered[0],ordered[1],base[base.length-1]!]
  const next=lines.map((line,index)=>index===lineIndex?detoured:line)
  const geometry=planFeature.geometry
  if(geometry.type==='LineString'){
    return {...planFeature,geometry:{type:'LineString',coordinates:detoured}}
  }
  return {...planFeature,geometry:{type:'MultiLineString',coordinates:next}}
}
