import axios, { type AxiosInstance, type AxiosRequestConfig, type AxiosResponse } from 'axios';
import type {
  WoodComponent, ScanImage, DiseaseType, PolygonAnnotation,
  AnnotationSnapshot, CollabUser, ApiResponse, AnnotationStats
} from '@/types';

const baseURL = '/api/v1';

export const http: AxiosInstance = axios.create({
  baseURL,
  timeout: 60000,
  headers: {
    'Content-Type': 'application/json',
  },
});

let currentUserId = localStorage.getItem('uid') || `u-${Math.random().toString(36).slice(2, 10)}`;
let currentUserName = localStorage.getItem('uname') || `用户${currentUserId.slice(-4)}`;
localStorage.setItem('uid', currentUserId);
localStorage.setItem('uname', currentUserName);

http.interceptors.request.use((config) => {
  config.headers['X-User-ID'] = currentUserId;
  config.headers['X-User-Name'] = encodeURIComponent(currentUserName);
  return config;
});

http.interceptors.response.use(
  (r) => r,
  (e) => {
    console.error('[API Error]', e.response?.status, e.config?.url, e.message);
    return Promise.reject(e);
  },
);

function dataOf<T>(r: AxiosResponse<ApiResponse<T>>): T {
  return (r.data as ApiResponse<T>).data as T;
}

export function getUserId(): string { return currentUserId; }
export function getUserName(): string { return currentUserName; }
export function setUser(id: string, name: string): void {
  currentUserId = id; currentUserName = name;
  localStorage.setItem('uid', id);
  localStorage.setItem('uname', name);
}

export const ComponentApi = {
  listTree: () => http.get<ApiResponse<WoodComponent[]>>('/components').then(dataOf),
  get: (id: number) => http.get<ApiResponse<WoodComponent>>(`/components/${id}`).then(dataOf),
  create: (body: Partial<WoodComponent>) => http.post<ApiResponse<WoodComponent>>('/components', body).then(dataOf),
  update: (id: number, body: Partial<WoodComponent>) => http.put<ApiResponse<WoodComponent>>(`/components/${id}`, body).then(dataOf),
  remove: (id: number) => http.delete<ApiResponse<unknown>>(`/components/${id}`).then(r => r.data),
};

export const ScanImageApi = {
  listByComponent: (componentId: number) =>
    http.get<ApiResponse<ScanImage[]>>('/scan-images', { params: { componentId } }).then(dataOf),
  get: (id: number) => http.get<ApiResponse<ScanImage>>(`/scan-images/${id}`).then(dataOf),
  create: (body: {
    componentId: number; fileName: string; filePath?: string;
    fileSize?: number; width?: number; height?: number; mimeType?: string;
    scanDate?: string; operator?: string; equipment?: string; remark?: string;
  }) => http.post<ApiResponse<ScanImage>>('/scan-images', body).then(dataOf),
  update: (id: number, body: Partial<ScanImage>) =>
    http.put<ApiResponse<ScanImage>>(`/scan-images/${id}`, body).then(dataOf),
  remove: (id: number) => http.delete<ApiResponse<unknown>>(`/scan-images/${id}`).then(r => r.data),
  upload: (componentId: number, file: File, w?: number, h?: number, onProgress?: (p: number) => void) => {
    const fd = new FormData();
    fd.append('file', file);
    return http.post<ApiResponse<ScanImage>>('/scan-images/upload', fd, {
      params: { componentId, w, h },
      headers: { 'Content-Type': 'multipart/form-data' },
      onUploadProgress: onProgress ? (e) => {
        if (e.total) onProgress(Math.round((e.loaded / e.total) * 100));
      } : undefined,
    }).then(dataOf);
  },
  downloadUrl: (id: number) => `${baseURL}/scan-images/${id}/download`,
  viewUrl: (image: ScanImage) => {
    if (!image) return '';
    if (image.filePath && image.filePath.startsWith('http')) return image.filePath;
    const path = image.filePath?.replace(/\\/g, '/').replace(/^\.\//, '');
    return `/static/uploads/${image.componentId}/` + (path?.split('/uploads/').pop() || image.fileName);
  },
};

export const DiseaseTypeApi = {
  list: () => http.get<ApiResponse<DiseaseType[]>>('/disease-types').then(dataOf),
  getByCode: (code: string) => http.get<ApiResponse<DiseaseType>>(`/disease-types/${code}`).then(dataOf),
};

export const AnnotationApi = {
  listByImage: (imageId: number) =>
    http.get<ApiResponse<PolygonAnnotation[]>>('/annotations', { params: { imageId } }).then(dataOf),
  get: (id: number) => http.get<ApiResponse<PolygonAnnotation>>(`/annotations/${id}`).then(dataOf),
  create: (body: Partial<PolygonAnnotation>) =>
    http.post<ApiResponse<PolygonAnnotation>>('/annotations', body).then(dataOf),
  update: (id: number, body: Partial<PolygonAnnotation>) =>
    http.put<ApiResponse<PolygonAnnotation>>(`/annotations/${id}`, body).then(dataOf),
  remove: (id: number) => http.delete<ApiResponse<unknown>>(`/annotations/${id}`).then(r => r.data),
  bulkReplace: (imageId: number, annotations: Partial<PolygonAnnotation>[]) =>
    http.post<ApiResponse<PolygonAnnotation[]>>('/annotations/bulk-replace', { imageId, annotations }).then(dataOf),
  stats: (imageId: number) =>
    http.get<ApiResponse<AnnotationStats>>('/annotations/stats', { params: { imageId } }).then(dataOf),
};

export const SnapshotApi = {
  list: (imageId: number) =>
    http.get<ApiResponse<AnnotationSnapshot[]>>('/snapshots', { params: { imageId } }).then(dataOf),
  latestVersion: (imageId: number) =>
    http.get<{ version: number }>('/snapshots/latest-version', { params: { imageId } }).then(r => r.data.version),
  create: (imageId: number, summary = '') =>
    http.post<{ version: number }>('/snapshots/create', { imageId, summary }).then(r => r.data.version),
  restore: (imageId: number, version: number) =>
    http.post<ApiResponse<PolygonAnnotation[]>>('/snapshots/restore', { imageId, version }).then(dataOf),
};

export const CollabApi = {
  listUsers: (imageId: number) =>
    http.get<ApiResponse<CollabUser[]>>('/collab/users', { params: { imageId } }).then(dataOf),
  heartbeat: (imageId: number, cursorPos?: { x: number; y: number }, activeTool?: string) =>
    http.post<ApiResponse<unknown>>('/collab/heartbeat', {
      imageId, userId: currentUserId, userName: currentUserName, cursorPos, activeTool,
    }).then(r => r.data),
  getDraft: (imageId: number) =>
    http.get<ApiResponse<unknown>>('/collab/draft', { params: { imageId, userId: currentUserId } }).then(dataOf),
  saveDraft: (imageId: number, data: unknown) =>
    http.put<ApiResponse<unknown>>('/collab/draft', { imageId, userId: currentUserId, data }).then(r => r.data),
  lockAnnotation: (imageId: number, annotationId: number, ttlSeconds = 30) =>
    http.post<ApiResponse<{ locked: boolean }>>('/collab/lock', {
      imageId, annotationId, userId: currentUserId, ttlSeconds,
    }).then(r => (r.data as { locked: boolean }).locked),
  unlockAnnotation: (imageId: number, annotationId: number) =>
    http.post<ApiResponse<unknown>>('/collab/unlock', { imageId, annotationId, userId: currentUserId }).then(r => r.data),
};

export function apiConfig(): unknown { return http.defaults; }
