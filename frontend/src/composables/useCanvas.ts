import { ref, computed, onBeforeUnmount } from 'vue';
import type { CanvasTransform, Point, PolygonAnnotation, ToolType } from '@/types';
import {
  screenToImage, imageToScreen, polygonBBox, polygonCentroid,
  pointInPolygon, distanceToPolygonEdge, clamp, simplifyPolygon,
} from '@/utils/geometry';
import { useAnnotationStore } from '@/stores/annotation';

export interface DragState {
  active: boolean;
  mode: 'pan' | 'move-ann' | 'move-point' | 'draw' | 'select-rect' | null;
  startScreen: Point;
  startImage: Point;
  currentScreen: Point;
  currentImage: Point;
  targetId: number | null;
  targetPointIdx: number;
  offsetX: number;
  offsetY: number;
}

export function useCanvas(containerRef: { value: HTMLElement | null }) {
  const store = useAnnotationStore();

  const canvas = ref<HTMLCanvasElement | null>(null);
  const overlay = ref<HTMLCanvasElement | null>(null);
  const ctx2d = ref<CanvasRenderingContext2D | null>(null);
  const overlayCtx = ref<CanvasRenderingContext2D | null>(null);
  const imageEl = ref<HTMLImageElement | null>(null);
  const imageLoaded = ref(false);
  const imageLoading = ref(false);

  const dpr = Math.max(1, window.devicePixelRatio || 1);

  const hoverPoint = ref<Point | null>(null);
  const hoveredAnnotation = ref<PolygonAnnotation | null>(null);
  const hoverScreenPos = ref<{ x: number; y: number }>({ x: 0, y: 0 });
  const drag = ref<DragState>({
    active: false, mode: null,
    startScreen: { x: 0, y: 0 }, startImage: { x: 0, y: 0 },
    currentScreen: { x: 0, y: 0 }, currentImage: { x: 0, y: 0 },
    targetId: null, targetPointIdx: -1, offsetX: 0, offsetY: 0,
  });

  const drawingPoints = ref<Point[]>([]);
  const lastCursorStyle = ref('default');

  const offscreenCanvas = ref<HTMLCanvasElement | null>(null);
  const offscreenCtx = ref<CanvasRenderingContext2D | null>(null);
  const offscreenDirty = ref(true);
  const lastRenderedTransform = ref<CanvasTransform | null>(null);
  const lastRenderedImageId = ref<number | null>(null);

  const gestureState = ref({
    active: false,
    initialDistance: 0,
    initialScale: 1,
    initialOffsetX: 0,
    initialOffsetY: 0,
    midPoint: { x: 0, y: 0 },
    lastTapTime: 0,
  });

  const viewport = computed(() => {
    const el = containerRef.value;
    return { w: el?.clientWidth || 0, h: el?.clientHeight || 0 };
  });

  function resizeCanvas(): void {
    const el = containerRef.value;
    if (!el || !canvas.value || !overlay.value) return;
    const w = el.clientWidth, h = el.clientHeight;
    for (const c of [canvas.value, overlay.value]) {
      c.width = w * dpr;
      c.height = h * dpr;
      c.style.width = w + 'px';
      c.style.height = h + 'px';
      const cc = c === canvas.value ? ctx2d.value : overlayCtx.value;
      cc?.setTransform(dpr, 0, 0, dpr, 0, 0);
    }
    requestRender();
  }

  let raf = 0;
  let dirty = false;
  function requestRender(): void {
    if (dirty) return;
    dirty = true;
    raf = requestAnimationFrame(() => {
      dirty = false;
      renderAll();
    });
  }

  function ensureOffscreen(w: number, h: number): void {
    if (!offscreenCanvas.value) {
      offscreenCanvas.value = document.createElement('canvas');
      offscreenCtx.value = offscreenCanvas.value.getContext('2d');
    }
    if (offscreenCanvas.value.width !== w || offscreenCanvas.value.height !== h) {
      offscreenCanvas.value.width = w;
      offscreenCanvas.value.height = h;
    }
  }

  function isTransformChanged(): boolean {
    const t = store.transform;
    const last = lastRenderedTransform.value;
    const imgId = store.currentImage?.id || null;
    if (imgId !== lastRenderedImageId.value) {
      lastRenderedImageId.value = imgId;
      return true;
    }
    if (!last) return true;
    const changed = Math.abs(t.scale - last.scale) > 0.001 ||
      Math.abs(t.offsetX - last.offsetX) > 0.5 ||
      Math.abs(t.offsetY - last.offsetY) > 0.5;
    return changed;
  }

  function renderAll(): void {
    const c = ctx2d.value;
    if (!c || !canvas.value) return;
    const { w, h } = viewport.value;

    if (isTransformChanged() || offscreenDirty.value) {
      ensureOffscreen(w, h);
      renderToOffscreen(w, h);
      offscreenDirty.value = false;
      lastRenderedTransform.value = { ...store.transform };
    }

    if (offscreenCanvas.value && offscreenCtx.value) {
      c.clearRect(0, 0, w, h);
      c.drawImage(offscreenCanvas.value, 0, 0);
    } else {
      c.save();
      c.clearRect(0, 0, w, h);
      c.fillStyle = '#0e0e1a';
      c.fillRect(0, 0, w, h);
      c.restore();
    }

    renderOverlay();
  }

  function renderToOffscreen(w: number, h: number): void {
    const oc = offscreenCtx.value;
    if (!oc) return;
    oc.save();
    oc.clearRect(0, 0, w, h);
    oc.fillStyle = '#0e0e1a';
    oc.fillRect(0, 0, w, h);

    const t = store.transform;
    if (imageLoaded.value && imageEl.value) {
      oc.setTransform(t.scale, 0, 0, t.scale, t.offsetX, t.offsetY);
      oc.imageSmoothingEnabled = t.scale < 2;
      oc.imageSmoothingQuality = t.scale < 1 ? 'high' : 'medium';
      try {
        oc.drawImage(imageEl.value, 0, 0);
      } catch {}
    }
    oc.restore();

    renderAnnotations(oc);
  }

  function renderAnnotations(c: CanvasRenderingContext2D): void {
    const t = store.transform;
    const { w, h } = viewport.value;
    const halfLine = Math.max(1, 1.2 / Math.max(0.1, t.scale));
    for (const a of store.annotations) {
      if (!a.points?.length) continue;
      const color = store.diseaseColor(a.diseaseTypeId);
      const bbox = a.boundingBox || polygonBBox(a.points);
      const sx1 = bbox.minX * t.scale + t.offsetX;
      const sy1 = bbox.minY * t.scale + t.offsetY;
      const sx2 = bbox.maxX * t.scale + t.offsetX;
      const sy2 = bbox.maxY * t.scale + t.offsetY;
      if (sx2 < -50 || sy2 < -50 || sx1 > w + 50 || sy1 > h + 50) continue;

      const selected = store.selectedAnnotationIds.has(a.id);
      c.save();
      c.beginPath();
      for (let i = 0; i < a.points.length; i++) {
        const p = a.points[i];
        const sx = p.x * t.scale + t.offsetX;
        const sy = p.y * t.scale + t.offsetY;
        if (i === 0) c.moveTo(sx, sy);
        else c.lineTo(sx, sy);
      }
      c.closePath();
      c.fillStyle = hexWithAlpha(color, selected ? 0.5 : 0.32);
      c.fill('evenodd');
      c.lineWidth = selected ? halfLine * 2.2 : halfLine;
      c.strokeStyle = color;
      c.stroke();
      c.restore();

      if (a.label || selected) {
        const cent = polygonCentroid(a.points);
        const sxc = cent.x * t.scale + t.offsetX;
        const syc = cent.y * t.scale + t.offsetY;
        drawBadge(c, sxc, syc, a.label || `#${a.id}`, color, selected);
      }
    }
  }

  function renderOverlay(): void {
    const oc = overlayCtx.value;
    if (!oc || !overlay.value) return;
    const { w, h } = viewport.value;
    oc.save();
    oc.clearRect(0, 0, w, h);
    const t = store.transform;

    if (drawingPoints.value.length > 0) {
      const dType = store.diseaseTypeMap.get(store.activeDiseaseTypeId);
      const color = dType?.colorHex || '#FF6B6B';
      oc.save();
      oc.beginPath();
      for (let i = 0; i < drawingPoints.value.length; i++) {
        const p = drawingPoints.value[i];
        const sx = p.x * t.scale + t.offsetX;
        const sy = p.y * t.scale + t.offsetY;
        if (i === 0) oc.moveTo(sx, sy);
        else oc.lineTo(sx, sy);
      }
      if (drag.value.mode === 'draw' && hoverPoint.value) {
        const lp = drawingPoints.value[drawingPoints.value.length - 1];
        const sp = imageToScreen(hoverPoint.value.x, hoverPoint.value.y, t.scale, t.offsetX, t.offsetY);
        oc.moveTo(lp.x * t.scale + t.offsetX, lp.y * t.scale + t.offsetY);
        oc.lineTo(sp.x, sp.y);
        if (drawingPoints.value.length >= 2) {
          const fp = drawingPoints.value[0];
          oc.lineTo(fp.x * t.scale + t.offsetX, fp.y * t.scale + t.offsetY);
        }
      }
      oc.lineWidth = Math.max(1, 1.5 / Math.max(0.1, t.scale));
      oc.setLineDash([6, 4]);
      oc.strokeStyle = color;
      oc.stroke();
      oc.setLineDash([]);
      oc.fillStyle = hexWithAlpha(color, 0.15);
      oc.fill();
      oc.restore();

      const handleR = Math.max(3, 6 / Math.max(0.2, t.scale));
      for (const p of drawingPoints.value) {
        const sx = p.x * t.scale + t.offsetX;
        const sy = p.y * t.scale + t.offsetY;
        oc.beginPath();
        oc.arc(sx, sy, handleR, 0, Math.PI * 2);
        oc.fillStyle = '#fff';
        oc.fill();
        oc.lineWidth = 1.5;
        oc.strokeStyle = color;
        oc.stroke();
      }
    }

    for (const id of Array.from(store.selectedAnnotationIds)) {
      const a = store.annotations.find((x) => x.id === id);
      if (!a || !a.points?.length) continue;
      const handleR = Math.max(3, 6 / Math.max(0.2, t.scale));
      for (let i = 0; i < a.points.length; i++) {
        const p = a.points[i];
        const sx = p.x * t.scale + t.offsetX;
        const sy = p.y * t.scale + t.offsetY;
        oc.beginPath();
        oc.arc(sx, sy, handleR, 0, Math.PI * 2);
        oc.fillStyle = 'rgba(77,171,247,0.9)';
        oc.fill();
        oc.lineWidth = 1.2;
        oc.strokeStyle = '#fff';
        oc.stroke();
      }
    }

    if (drag.value.active && drag.value.mode === 'select-rect') {
      const s = drag.value.startScreen, cur = drag.value.currentScreen;
      oc.save();
      oc.strokeStyle = '#4dabf7';
      oc.setLineDash([4, 3]);
      oc.lineWidth = 1.2;
      oc.strokeRect(
        Math.min(s.x, cur.x), Math.min(s.y, cur.y),
        Math.abs(cur.x - s.x), Math.abs(cur.y - s.y),
      );
      oc.fillStyle = 'rgba(77,171,247,0.1)';
      oc.fillRect(
        Math.min(s.x, cur.x), Math.min(s.y, cur.y),
        Math.abs(cur.x - s.x), Math.abs(cur.y - s.y),
      );
      oc.restore();
    }
    oc.restore();
  }

  function drawBadge(
    c: CanvasRenderingContext2D, x: number, y: number,
    text: string, color: string, selected: boolean,
  ): void {
    const padX = 6, padY = 3;
    c.save();
    c.font = `${selected ? 600 : 500} 11px -apple-system, Segoe UI, PingFang SC`;
    const tw = c.measureText(text).width;
    const bx = x - tw / 2 - padX, by = y - 9 - padY;
    c.fillStyle = selected ? color : '#1a1a2e';
    c.strokeStyle = color;
    c.lineWidth = 1;
    roundRect(c, bx, by, tw + padX * 2, 18, 4);
    c.fill();
    c.stroke();
    c.fillStyle = selected ? '#fff' : color;
    c.textAlign = 'center';
    c.textBaseline = 'middle';
    c.fillText(text, x, y - 9);
    c.restore();
  }

  function roundRect(c: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number): void {
    c.beginPath();
    c.moveTo(x + r, y);
    c.arcTo(x + w, y, x + w, y + h, r);
    c.arcTo(x + w, y + h, x, y + h, r);
    c.arcTo(x, y + h, x, y, r);
    c.arcTo(x, y, x + w, y, r);
    c.closePath();
  }

  function hexWithAlpha(hex: string, a: number): string {
    let h = hex.replace('#', '');
    if (h.length === 3) h = h.split('').map((x) => x + x).join('');
    const n = parseInt(h, 16);
    const r = (n >> 16) & 255, g = (n >> 8) & 255, b = n & 255;
    return `rgba(${r},${g},${b},${a})`;
  }

  function loadImage(url: string): Promise<void> {
    imageLoading.value = true;
    imageLoaded.value = false;
    offscreenDirty.value = true;
    return new Promise((resolve, reject) => {
      const img = new Image();
      img.decoding = 'async';
      img.crossOrigin = 'anonymous';
      img.onload = () => {
        imageEl.value = img;
        imageLoaded.value = true;
        imageLoading.value = false;
        offscreenDirty.value = true;
        requestRender();
        resolve();
      };
      img.onerror = (e) => {
        imageLoading.value = false;
        reject(e);
      };
      img.src = url;
    });
  }

  function markDirty(): void {
    offscreenDirty.value = true;
    requestRender();
  }

  function onPointerDown(e: PointerEvent): void {
    const p = eventPos(e);
    if (!p) return;
    (e.target as Element).setPointerCapture?.(e.pointerId);
    drag.value.startScreen = p;
    drag.value.currentScreen = p;
    drag.value.startImage = screenToImage(p.x, p.y, store.transform.scale, store.transform.offsetX, store.transform.offsetY);
    drag.value.currentImage = { ...drag.value.startImage };

    const tool = effectiveTool(e);
    if (tool === 'pan' || e.button === 1 || (e.button === 0 && e.altKey)) {
      drag.value.mode = 'pan';
      drag.value.active = true;
      lastCursorStyle.value = 'grabbing';
      return;
    }
    if (tool === 'zoom') {
      zoomAt(p, e.button === 2 ? 0.8 : 1.25);
      return;
    }
    if (tool === 'polygon') {
      drag.value.active = true;
      drag.value.mode = 'draw';
      handlePolygonStart(drag.value.startImage, e);
      return;
    }
    if (tool === 'rect') {
      drag.value.active = true;
      drag.value.mode = 'draw';
      drawingPoints.value = [
        drag.value.startImage,
        { x: drag.value.startImage.x, y: drag.value.startImage.y },
        { x: drag.value.startImage.x, y: drag.value.startImage.y },
        { x: drag.value.startImage.x, y: drag.value.startImage.y },
      ];
      return;
    }
    // select / move
    const hit = hitTestAnnotation(drag.value.startImage, e.shiftKey);
    if (hit.annotationId != null) {
      if (hit.pointIdx >= 0) {
        drag.value.mode = 'move-point';
        drag.value.targetId = hit.annotationId;
        drag.value.targetPointIdx = hit.pointIdx;
      } else {
        drag.value.mode = 'move-ann';
        drag.value.targetId = hit.annotationId;
        const ann = store.annotations.find((a) => a.id === hit.annotationId);
        if (ann && ann.points?.[0]) {
          drag.value.offsetX = drag.value.startImage.x - ann.points[0].x;
          drag.value.offsetY = drag.value.startImage.y - ann.points[0].y;
        }
      }
      drag.value.active = true;
      lastCursorStyle.value = 'move';
      return;
    }
    // drag-select rect
    drag.value.mode = 'select-rect';
    drag.value.active = true;
    if (!e.shiftKey) store.clearSelection();
  }

  function onPointerMove(e: PointerEvent): void {
    const p = eventPos(e);
    if (!p) return;
    const imgPt = screenToImage(p.x, p.y, store.transform.scale, store.transform.offsetX, store.transform.offsetY);
    hoverPoint.value = imgPt;
    hoverScreenPos.value = p;

    if (!drag.value.active) {
      updateHoveredAnnotation(imgPt);
      updateCursorStyle(imgPt, effectiveTool(e));
      return;
    }
    drag.value.currentScreen = p;
    drag.value.currentImage = imgPt;

    if (drag.value.mode === 'pan') {
      store.transform = {
        ...store.transform,
        offsetX: store.transform.offsetX + (p.x - drag.value.startScreen.x),
        offsetY: store.transform.offsetY + (p.y - drag.value.startScreen.y),
      };
      drag.value.startScreen = p;
      requestRender();
      return;
    }
    if (drag.value.mode === 'move-ann' && drag.value.targetId != null) {
      const ann = store.annotations.find((a) => a.id === drag.value.targetId);
      if (ann) {
        const dx = imgPt.x - drag.value.startImage.x;
        const dy = imgPt.y - drag.value.startImage.y;
        ann.points = ann.points.map((pt) => ({ x: pt.x + dx, y: pt.y + dy }));
        ann.boundingBox = polygonBBox(ann.points);
        drag.value.startImage = imgPt;
        markDirty();
      }
      return;
    }
    if (drag.value.mode === 'move-point' && drag.value.targetId != null) {
      const ann = store.annotations.find((a) => a.id === drag.value.targetId);
      if (ann && ann.points[drag.value.targetPointIdx]) {
        ann.points[drag.value.targetPointIdx] = clampToImage(imgPt);
        ann.boundingBox = polygonBBox(ann.points);
        markDirty();
      }
      return;
    }
    if (drag.value.mode === 'draw' && store.currentTool === 'rect' && drawingPoints.value.length === 4) {
      const s = drag.value.startImage;
      drawingPoints.value = [
        { x: s.x, y: s.y },
        { x: imgPt.x, y: s.y },
        { x: imgPt.x, y: imgPt.y },
        { x: s.x, y: imgPt.y },
      ];
      requestRender();
      return;
    }
    if (drag.value.mode === 'select-rect') {
      requestRender();
      return;
    }
    requestRender();
  }

  async function onPointerUp(e: PointerEvent): Promise<void> {
    const p = eventPos(e);
    if (!p) { returnFinish(); return; }
    const imgPt = screenToImage(p.x, p.y, store.transform.scale, store.transform.offsetX, store.transform.offsetY);

    try {
      if (drag.value.mode === 'draw') {
        if (store.currentTool === 'rect') {
          if (drawingPoints.value.length >= 3 && polygonAreaImage(drawingPoints.value) > 9) {
            const pts = drawingPoints.value;
            drawingPoints.value = [];
            await store.createAnnotation(pts);
          } else {
            drawingPoints.value = [];
          }
          return;
        }
        // polygon mode: single click add point, double click / near start -> finish
        return;
      }
      if (drag.value.mode === 'select-rect') {
        const s = drag.value.startScreen, cur = p;
        const x1 = Math.min(s.x, cur.x), x2 = Math.max(s.x, cur.x);
        const y1 = Math.min(s.y, cur.y), y2 = Math.max(s.y, cur.y);
        if (Math.abs(cur.x - s.x) < 4 && Math.abs(cur.y - s.y) < 4) {
          return;
        }
        const t = store.transform;
        for (const a of store.annotations) {
          const bb = a.boundingBox || polygonBBox(a.points);
          const bx1 = bb.minX * t.scale + t.offsetX;
          const by1 = bb.minY * t.scale + t.offsetY;
          const bx2 = bb.maxX * t.scale + t.offsetX;
          const by2 = bb.maxY * t.scale + t.offsetY;
          if (bx2 >= x1 && bx1 <= x2 && by2 >= y1 && by1 <= y2) {
            store.selectAnnotation(a.id, true);
          }
        }
        return;
      }
      if ((drag.value.mode === 'move-ann' || drag.value.mode === 'move-point') && drag.value.targetId != null) {
        const ann = store.annotations.find((a) => a.id === drag.value.targetId);
        if (ann) {
          await store.updateAnnotation(ann.id, { points: ann.points });
        }
        return;
      }
    } finally {
      returnFinish();
    }
  }

  function returnFinish(): void {
    drag.value.active = false;
    drag.value.mode = null;
    drag.value.targetId = null;
    drag.value.targetPointIdx = -1;
    requestRender();
  }

  function onDblClick(e: MouseEvent): void {
    if (store.currentTool === 'polygon' && drawingPoints.value.length >= 3) {
      finishPolygon();
    }
  }

  function onKeyDown(e: KeyboardEvent): void {
    if (store.currentTool === 'polygon') {
      if (e.key === 'Enter' && drawingPoints.value.length >= 3) {
        finishPolygon();
      } else if (e.key === 'Escape') {
        drawingPoints.value = [];
        requestRender();
      } else if (e.key === 'Backspace' || e.key === 'z') {
        if (e.ctrlKey || e.metaKey || e.key === 'Backspace') {
          if (drawingPoints.value.length > 0) {
            drawingPoints.value.pop();
            requestRender();
            e.preventDefault();
          }
        }
      }
    }
  }

  async function handlePolygonStart(p: Point, _e: PointerEvent): Promise<void> {
    if (drawingPoints.value.length === 0) {
      drawingPoints.value = [clampToImage(p)];
      requestRender();
      return;
    }
    const first = drawingPoints.value[0];
    const dStart = Math.hypot(first.x - p.x, first.y - p.y);
    const t = store.transform;
    const thresholdClose = 8 / Math.max(0.1, t.scale);
    if (drawingPoints.value.length >= 3 && dStart < thresholdClose) {
      await finishPolygon();
      return;
    }
    const last = drawingPoints.value[drawingPoints.value.length - 1];
    if (Math.hypot(last.x - p.x, last.y - p.y) >= 1.5 / Math.max(0.1, t.scale)) {
      drawingPoints.value.push(clampToImage(p));
      requestRender();
    }
  }

  async function finishPolygon(): Promise<void> {
    if (drawingPoints.value.length < 3) {
      drawingPoints.value = [];
      requestRender();
      return;
    }
    const pts = simplifyPolygon(drawingPoints.value, 1.2 / Math.max(0.1, store.transform.scale));
    drawingPoints.value = [];
    if (pts.length < 3) { requestRender(); return; }
    await store.createAnnotation(pts);
    requestRender();
  }

  function hitTestAnnotation(pt: Point, multi: boolean): { annotationId: number | null; pointIdx: number } {
    const t = store.transform;
    const handleHitR = Math.max(5, 9 / Math.max(0.2, t.scale));
    let bestId: number | null = null;
    let bestIdx = -1;
    let bestDist = Infinity;

    for (const id of Array.from(store.selectedAnnotationIds)) {
      const a = store.annotations.find((x) => x.id === id);
      if (!a || !a.points?.length) continue;
      for (let i = 0; i < a.points.length; i++) {
        const p = a.points[i];
        const sp = imageToScreen(p.x, p.y, t.scale, t.offsetX, t.offsetY);
        const spt = imageToScreen(pt.x, pt.y, t.scale, t.offsetX, t.offsetY);
        const d = Math.hypot(sp.x - spt.x, sp.y - spt.y);
        if (d <= handleHitR && d < bestDist) {
          bestDist = d;
          bestId = a.id;
          bestIdx = i;
        }
      }
    }
    if (bestId != null) return { annotationId: bestId, pointIdx: bestIdx };

    // select smallest annotation containing the point
    const candidates: { id: number; area: number }[] = [];
    for (const a of store.annotations) {
      if (!a.points?.length) continue;
      if (pointInPolygon(pt, a.points)) {
        candidates.push({ id: a.id, area: polygonAreaImage(a.points) });
      } else {
        const d = distanceToPolygonEdge(pt, a.points);
        if (d < 3 / Math.max(0.1, t.scale)) {
          candidates.push({ id: a.id, area: polygonAreaImage(a.points) + 1e9 });
        }
      }
    }
    if (candidates.length > 0) {
      candidates.sort((a, b) => a.area - b.area);
      const id = candidates[0].id;
      store.selectAnnotation(id, multi);
      return { annotationId: id, pointIdx: -1 };
    }
    if (!multi) store.clearSelection();
    return { annotationId: null, pointIdx: -1 };
  }

  function polygonAreaImage(points: Point[]): number {
    if (points.length < 3) return 0;
    let s = 0;
    for (let i = 0, n = points.length; i < n; i++) {
      const p = points[i], nx = points[(i + 1) % n];
      s += p.x * nx.y - nx.x * p.y;
    }
    return Math.abs(s) / 2;
  }

  function updateHoveredAnnotation(pt: Point): void {
    let found: PolygonAnnotation | null = null;
    let smallestArea = Infinity;
    for (const a of store.annotations) {
      if (!a.points?.length) continue;
      if (pointInPolygon(pt, a.points) || distanceToPolygonEdge(pt, a.points) < 3 / Math.max(0.1, store.transform.scale)) {
        const area = polygonAreaImage(a.points);
        if (area < smallestArea) {
          smallestArea = area;
          found = a;
        }
      }
    }
    hoveredAnnotation.value = found;
  }

  function effectiveTool(e: PointerEvent | MouseEvent | KeyboardEvent): ToolType {
    if (e.altKey) return 'pan';
    if (e.ctrlKey || e.metaKey) return 'select';
    return store.currentTool;
  }

  function updateCursorStyle(pt: Point, tool: ToolType): void {
    const el = canvas.value || overlay.value;
    if (!el) return;
    const t = store.transform;
    let cursor = 'default';
    if (tool === 'pan') cursor = 'grab';
    else if (tool === 'zoom') cursor = 'zoom-in';
    else if (tool === 'polygon' || tool === 'rect') cursor = 'crosshair';
    else {
      // 检测是否命中锚点或标注
      const handleHitR = Math.max(5, 9 / Math.max(0.2, t.scale));
      let hitHandle = false;
      for (const id of Array.from(store.selectedAnnotationIds)) {
        const a = store.annotations.find((x) => x.id === id);
        if (!a?.points) continue;
        for (const p of a.points) {
          const sp = imageToScreen(p.x, p.y, t.scale, t.offsetX, t.offsetY);
          const spt = imageToScreen(pt.x, pt.y, t.scale, t.offsetX, t.offsetY);
          if (Math.hypot(sp.x - spt.x, sp.y - spt.y) <= handleHitR) { hitHandle = true; break; }
        }
        if (hitHandle) break;
      }
      if (hitHandle) cursor = 'move';
      else {
        for (const a of store.annotations) {
          if (!a.points) continue;
          if (pointInPolygon(pt, a.points) || distanceToPolygonEdge(pt, a.points) < 2 / Math.max(0.1, t.scale)) {
            cursor = 'move'; break;
          }
        }
      }
    }
    if (lastCursorStyle.value !== cursor) {
      el.style.cursor = cursor;
      if (overlay.value) overlay.value.style.cursor = cursor;
      lastCursorStyle.value = cursor;
    }
  }

  function zoomAt(screenPt: Point, factor: number): void {
    const prev = store.transform;
    const newScale = clamp(prev.scale * factor, 0.05, 10);
    const actualFactor = newScale / prev.scale;
    const dx = screenPt.x - prev.offsetX;
    const dy = screenPt.y - prev.offsetY;
    store.transform = {
      scale: newScale,
      offsetX: screenPt.x - dx * actualFactor,
      offsetY: screenPt.y - dy * actualFactor,
    };
    requestRender();
  }

  function onWheel(e: WheelEvent): void {
    if (!containerRef.value) return;
    const rect = containerRef.value.getBoundingClientRect();
    const sp = { x: e.clientX - rect.left, y: e.clientY - rect.top };
    const factor = e.deltaY < 0 ? 1.1 : 1 / 1.1;
    zoomAt(sp, factor);
    e.preventDefault();
  }

  function clampToImage(p: Point): Point {
    if (!store.currentImage) return p;
    return {
      x: clamp(p.x, 0, store.currentImage.width),
      y: clamp(p.y, 0, store.currentImage.height),
    };
  }

  function eventPos(e: PointerEvent | MouseEvent): Point | null {
    if (!containerRef.value) return null;
    const r = containerRef.value.getBoundingClientRect();
    return { x: e.clientX - r.left, y: e.clientY - r.top };
  }

  function prevent(e: Event): void { e.preventDefault(); }

  function getTouchDistance(t1: Touch, t2: Touch): number {
    const dx = t1.clientX - t2.clientX;
    const dy = t1.clientY - t2.clientY;
    return Math.hypot(dx, dy);
  }

  function getTouchMidpoint(t1: Touch, t2: Touch): Point {
    if (!containerRef.value) return { x: 0, y: 0 };
    const rect = containerRef.value.getBoundingClientRect();
    return {
      x: (t1.clientX + t2.clientX) / 2 - rect.left,
      y: (t1.clientY + t2.clientY) / 2 - rect.top,
    };
  }

  function onTouchStart(e: TouchEvent): void {
    if (e.touches.length === 2) {
      e.preventDefault();
      drag.value.active = false;
      drawingPoints.value = [];
      const t1 = e.touches[0], t2 = e.touches[1];
      gestureState.value = {
        active: true,
        initialDistance: getTouchDistance(t1, t2),
        initialScale: store.transform.scale,
        initialOffsetX: store.transform.offsetX,
        initialOffsetY: store.transform.offsetY,
        midPoint: getTouchMidpoint(t1, t2),
        lastTapTime: gestureState.value.lastTapTime,
      };
    } else if (e.touches.length === 1 && !gestureState.value.active) {
      const now = Date.now();
      if (now - gestureState.value.lastTapTime < 300) {
        if (store.currentTool === 'polygon' && drawingPoints.value.length >= 3) {
          finishPolygon();
        }
      }
      gestureState.value.lastTapTime = now;
    }
  }

  function onTouchMove(e: TouchEvent): void {
    if (e.touches.length === 2 && gestureState.value.active) {
      e.preventDefault();
      const t1 = e.touches[0], t2 = e.touches[1];
      const currentDist = getTouchDistance(t1, t2);
      const mid = getTouchMidpoint(t1, t2);
      const scaleFactor = currentDist / gestureState.value.initialDistance;
      const newScale = clamp(
        gestureState.value.initialScale * scaleFactor,
        0.05, 10,
      );
      const actualFactor = newScale / gestureState.value.initialScale;
      const dx = mid.x - gestureState.value.midPoint.x;
      const dy = mid.y - gestureState.value.midPoint.y;
      const cx = gestureState.value.midPoint.x - gestureState.value.initialOffsetX;
      const cy = gestureState.value.midPoint.y - gestureState.value.initialOffsetY;
      store.transform = {
        scale: newScale,
        offsetX: mid.x - cx * actualFactor + dx,
        offsetY: mid.y - cy * actualFactor + dy,
      };
      requestRender();
    }
  }

  function onTouchEnd(e: TouchEvent): void {
    if (e.touches.length < 2 && gestureState.value.active) {
      gestureState.value.active = false;
    }
  }

  let ro: ResizeObserver | null = null;
  function setupResize(): void {
    if (!containerRef.value) return;
    ro = new ResizeObserver(() => {
      resizeCanvas();
    });
    ro.observe(containerRef.value);
    containerRef.value.addEventListener('touchstart', onTouchStart, { passive: false });
    containerRef.value.addEventListener('touchmove', onTouchMove, { passive: false });
    containerRef.value.addEventListener('touchend', onTouchEnd, { passive: false });
    containerRef.value.addEventListener('touchcancel', onTouchEnd, { passive: false });
  }

  onBeforeUnmount(() => {
    if (raf) cancelAnimationFrame(raf);
    ro?.disconnect();
    drawingPoints.value = [];
    if (containerRef.value) {
      containerRef.value.removeEventListener('touchstart', onTouchStart);
      containerRef.value.removeEventListener('touchmove', onTouchMove);
      containerRef.value.removeEventListener('touchend', onTouchEnd);
      containerRef.value.removeEventListener('touchcancel', onTouchEnd);
    }
  });

  return {
    canvas, overlay, ctx2d, overlayCtx, imageEl, imageLoaded, imageLoading,
    dpr, viewport, hoverPoint, hoveredAnnotation, hoverScreenPos,
    drag, drawingPoints, gestureState,
    resizeCanvas, requestRender, renderAll, markDirty,
    loadImage, setupResize,
    onPointerDown, onPointerMove, onPointerUp, onDblClick, onWheel, onKeyDown,
    onTouchStart, onTouchMove, onTouchEnd,
    prevent, zoomAt,
  };
}
