export interface Point {
  x: number;
  y: number;
}

export interface BoundingBox {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

export interface DiseaseType {
  id: number;
  code: 'wormhole' | 'crack' | 'decay' | 'hollowing' | string;
  name: string;
  colorHex: string;
  description?: string;
  severityOrder: number;
  enabled: boolean;
  createdAt: string;
}

export interface ComponentCategory {
  id: number;
  name: string;
  description?: string;
}

export interface WoodComponent {
  id: number;
  parentId?: number;
  categoryId?: number;
  name: string;
  code?: string;
  description?: string;
  buildingName?: string;
  location?: string;
  material?: string;
  category?: ComponentCategory;
  children?: WoodComponent[];
}

export interface ScanImage {
  id: number;
  componentId: number;
  fileName: string;
  filePath: string;
  fileSize: number;
  mimeType?: string;
  width: number;
  height: number;
  bitsDepth: number;
  scanDate?: string;
  operator?: string;
  equipment?: string;
  remark?: string;
  uploadedBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AnnotationLayer {
  id: number;
  scanImageId: number;
  layerName: string;
  diseaseTypeId?: number;
  opacity: number;
  visible: boolean;
  locked: boolean;
  sortOrder: number;
  createdBy?: string;
  diseaseType?: DiseaseType;
}

export interface PolygonAnnotation {
  id: number;
  scanImageId: number;
  layerId?: number;
  diseaseTypeId: number;
  label?: string;
  points: Point[];
  boundingBox?: BoundingBox;
  areaPx?: number;
  areaCm2?: number;
  severity: 1 | 2 | 3 | 4 | 5;
  note?: string;
  operator?: string;
  diseaseType?: DiseaseType;
  createdAt: string;
  updatedAt: string;
}

export interface AnnotationSnapshot {
  id: number;
  scanImageId: number;
  versionNumber: number;
  snapshotData: PolygonAnnotation[];
  diffSummary?: string;
  operator?: string;
  createdAt: string;
}

export interface CollabUser {
  userId: string;
  userName: string;
  cursorPos?: Point;
  activeTool?: string;
  joinedAt: string;
  lastBeat: string;
  color?: string;
}

export type ToolType = 'select' | 'pan' | 'polygon' | 'rect' | 'brush' | 'eraser' | 'zoom' | 'measure';

export type OperationType =
  | 'add' | 'update' | 'delete' | 'bulk_replace'
  | 'cursor' | 'selection' | 'rollback' | 'lock' | 'unlock'
  | 'presence' | 'heartbeat' | 'ping' | 'pong' | 'broadcast' | 'error';

export interface OperationMessage<T = unknown> {
  type: OperationType | string;
  userId: string;
  userName?: string;
  ts?: number;
  timestamp?: number;
  seq: number;
  payload?: T;
}

export interface CursorPayload {
  x: number;
  y: number;
  imageId: number;
}

export interface CanvasTransform {
  scale: number;
  offsetX: number;
  offsetY: number;
}

export interface AnnotationStats {
  totalCount: number;
  totalAreaPx: number;
  byDiseaseCount: Record<string, number>;
}

export interface ApiResponse<T = unknown> {
  data?: T;
  error?: string;
  ok?: boolean;
  version?: number;
  locked?: boolean;
}

export interface DraftSnapshot {
  imageId: number;
  userId: string;
  annotations: PolygonAnnotation[];
  tool: ToolType;
  transform: CanvasTransform;
  createdAt: number;
}
