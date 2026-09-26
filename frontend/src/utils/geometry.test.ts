import { describe, expect, it } from 'vitest'
import { buildDetouredFeature, geometryBounds, geometryLines, orderDetourVertices, pointInPolygon } from './geometry'
import type { GeoJSONFeature } from '../types/api'

describe('survey geometry utilities',()=>{
  const polygon:GeoJSONFeature={type:'Feature',properties:{},geometry:{type:'Polygon',coordinates:[[[0,0],[120,0],[120,80],[0,80],[0,0]]]}}
  const tracks:GeoJSONFeature={type:'Feature',properties:{},geometry:{type:'MultiLineString',coordinates:[[[10,20],[110,20]],[[10,60],[110,60]]]}}
  it('keeps polygon rings and multi-lines as separate drawable lines',()=>{expect(geometryLines(polygon)).toHaveLength(1);expect(geometryLines(tracks)).toHaveLength(2)})
  it('calculates stable projected bounds across layers',()=>{expect(geometryBounds([polygon,tracks])).toEqual({minX:0,minY:0,maxX:120,maxY:80,pointCount:9})})
})

describe('detour geometry utilities',()=>{
  const area:GeoJSONFeature={type:'Feature',properties:{},geometry:{type:'Polygon',coordinates:[[[0,0],[1000,0],[1000,1000],[0,1000],[0,0]]]}}
  const plan:GeoJSONFeature={type:'Feature',properties:{},geometry:{type:'MultiLineString',coordinates:[[[0,200],[1000,200]],[[0,800],[1000,800]]]}}
  it('accepts interior vertices but rejects boundary and exterior points',()=>{
    expect(pointInPolygon([500,500],area)).toBe(true)
    expect(pointInPolygon([500,0],area)).toBe(false)
    expect(pointInPolygon([1001,500],area)).toBe(false)
  })
  it('orders reverse-submitted vertices by projection on the base line',()=>{
    expect(orderDetourVertices([[0,200],[1000,200]],[[800,350],[200,350]])).toEqual([[200,350],[800,350]])
  })
  it('replaces only the selected line and keeps neighbours',()=>{
    const detoured=buildDetouredFeature(plan,0,[[800,350],[200,350]])
    if(detoured.geometry.type!=='MultiLineString')throw new Error('expected MultiLineString')
    expect(detoured.geometry.coordinates[0]).toEqual([[0,200],[200,350],[800,350],[1000,200]])
    expect(detoured.geometry.coordinates[1]).toEqual([[0,800],[1000,800]])
  })
})
