import type { Point, BoundingBox } from '@/types';

export function screenToImage(
  sx: number, sy: number,
  scale: number, offsetX: number, offsetY: number,
): Point {
  return {
    x: (sx - offsetX) / scale,
    y: (sy - offsetY) / scale,
  };
}

export function imageToScreen(
  ix: number, iy: number,
  scale: number, offsetX: number, offsetY: number,
): Point {
  return {
    x: ix * scale + offsetX,
    y: iy * scale + offsetY,
  };
}

export function polygonArea(points: Point[]): number {
  if (points.length < 3) return 0;
  let sum = 0;
  const n = points.length;
  for (let i = 0; i < n; i++) {
    const p = points[i];
    const next = points[(i + 1) % n];
    sum += p.x * next.y - next.x * p.y;
  }
  return Math.abs(sum) / 2;
}

export function polygonCentroid(points: Point[]): Point {
  if (points.length === 0) return { x: 0, y: 0 };
  if (points.length < 3) {
    const xs = points.reduce((s, p) => s + p.x, 0);
    const ys = points.reduce((s, p) => s + p.y, 0);
    return { x: xs / points.length, y: ys / points.length };
  }
  let area = 0;
  let cx = 0, cy = 0;
  const n = points.length;
  for (let i = 0; i < n; i++) {
    const p = points[i];
    const next = points[(i + 1) % n];
    const cross = p.x * next.y - next.x * p.y;
    area += cross;
    cx += (p.x + next.x) * cross;
    cy += (p.y + next.y) * cross;
  }
  area = area * 3;
  return { x: cx / area, y: cy / area };
}

export function polygonBBox(points: Point[]): BoundingBox {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const p of points) {
    if (p.x < minX) minX = p.x;
    if (p.y < minY) minY = p.y;
    if (p.x > maxX) maxX = p.x;
    if (p.y > maxY) maxY = p.y;
  }
  return { minX, minY, maxX, maxY };
}

export function pointInPolygon(pt: Point, polygon: Point[]): boolean {
  if (polygon.length < 3) return false;
  let inside = false;
  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const xi = polygon[i].x, yi = polygon[i].y;
    const xj = polygon[j].x, yj = polygon[j].y;
    const intersect = ((yi > pt.y) !== (yj > pt.y))
      && (pt.x < ((xj - xi) * (pt.y - yi)) / ((yj - yi) || 1e-9) + xi);
    if (intersect) inside = !inside;
  }
  return inside;
}

export function distanceToPolygonEdge(pt: Point, polygon: Point[]): number {
  let min = Infinity;
  for (let i = 0; i < polygon.length; i++) {
    const a = polygon[i];
    const b = polygon[(i + 1) % polygon.length];
    const d = distancePointToSegment(pt, a, b);
    if (d < min) min = d;
  }
  return min;
}

export function distancePointToSegment(p: Point, a: Point, b: Point): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const len2 = dx * dx + dy * dy;
  if (len2 === 0) return Math.hypot(p.x - a.x, p.y - a.y);
  let t = ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2;
  t = Math.max(0, Math.min(1, t));
  const cx = a.x + t * dx;
  const cy = a.y + t * dy;
  return Math.hypot(p.x - cx, p.y - cy);
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v));
}

export function simplifyPolygon(points: Point[], tolerancePx = 1): Point[] {
  if (points.length <= 3) return points.slice();
  const result: Point[] = [points[0]];
  for (let i = 1; i < points.length - 1; i++) {
    const prev = result[result.length - 1];
    const cur = points[i];
    if (Math.hypot(cur.x - prev.x, cur.y - prev.y) >= tolerancePx) {
      result.push(cur);
    }
  }
  const last = points[points.length - 1];
  const tail = result[result.length - 1];
  if (Math.hypot(last.x - tail.x, last.y - tail.y) >= tolerancePx) {
    result.push(last);
  }
  return result;
}

export const COLORS = [
  '#FF6B6B', '#4ECDC4', '#95E1D3', '#FFD93D', '#6C5CE7',
  '#F8B500', '#55EFC4', '#FD79A8', '#0984E3', '#A29BFE',
];
export function colorForUser(userId: string): string {
  let hash = 0;
  for (let i = 0; i < userId.length; i++) hash = (hash * 31 + userId.charCodeAt(i)) >>> 0;
  return COLORS[hash % COLORS.length];
}
