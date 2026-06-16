import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import type {
  WoodComponent, ScanImage, DiseaseType, PolygonAnnotation,
  AnnotationSnapshot, CollabUser, ToolType, CanvasTransform, OperationMessage
} from '@/types';
import {
  ComponentApi, ScanImageApi, DiseaseTypeApi, AnnotationApi,
  SnapshotApi, CollabApi
} from '@/services/api';
import { globalWs } from '@/services/ws';
import { LocalDraft } from '@/services/storage';

export const useAnnotationStore = defineStore('annotation', () => {
  const componentTree = ref<WoodComponent[]>([]);
  const diseaseTypes = ref<DiseaseType[]>([]);

  const selectedComponentId = ref<number | null>(null);
  const componentImages = ref<ScanImage[]>([]);
  const currentImage = ref<ScanImage | null>(null);

  const annotations = ref<PolygonAnnotation[]>([]);
  const selectedAnnotationIds = ref<Set<number>>(new Set());
  const activeDraftPoints = ref<{ x: number; y: number }[]>([]);

  const currentTool = ref<ToolType>('polygon');
  const activeDiseaseTypeId = ref<number>(0);
  const severity = ref<1 | 2 | 3 | 4 | 5>(3);

  const transform = ref<CanvasTransform>({ scale: 1, offsetX: 0, offsetY: 0 });

  const snapshots = ref<AnnotationSnapshot[]>([]);
  const latestVersion = ref(0);

  const collabUsers = ref<CollabUser[]>([]);
  const isSaving = ref(false);
  const error = ref<string | null>(null);

  const diseaseTypeMap = computed(() => {
    const m = new Map<number, DiseaseType>();
    diseaseTypes.value.forEach((d) => m.set(d.id, d));
    return m;
  });
  const diseaseCodeColor = computed(() => {
    const m = new Map<string, string>();
    diseaseTypes.value.forEach((d) => m.set(d.code, d.colorHex));
    return m;
  });

  function diseaseColor(typeId: number): string {
    return diseaseTypeMap.value.get(typeId)?.colorHex || '#FF6B6B';
  }

  async function init(): Promise<void> {
    try {
      const [tree, types] = await Promise.all([
        ComponentApi.listTree(),
        DiseaseTypeApi.list(),
      ]);
      componentTree.value = tree;
      diseaseTypes.value = types;
      if (types.length > 0) {
        activeDiseaseTypeId.value = types[0].id;
      }
    } catch (e: unknown) {
      error.value = (e as Error).message;
    }
  }

  async function selectComponent(id: number): Promise<void> {
    selectedComponentId.value = id;
    currentImage.value = null;
    annotations.value = [];
    try {
      componentImages.value = await ScanImageApi.listByComponent(id);
      if (componentImages.value.length > 0) {
        await selectImage(componentImages.value[0].id);
      }
    } catch (e: unknown) {
      error.value = (e as Error).message;
    }
  }

  async function selectImage(id: number): Promise<void> {
    if (currentImage.value?.id === id && annotations.value.length > 0) return;
    try {
      const [img, list] = await Promise.all([
        ScanImageApi.get(id),
        AnnotationApi.listByImage(id),
      ]);
      currentImage.value = img;
      annotations.value = list;
      resetViewForImage(img.width, img.height);
      activeDraftPoints.value = [];
      selectedAnnotationIds.value.clear();

      const localDraft = await LocalDraft.loadAnnotations(id);
      if (localDraft && localDraft.length > list.length) {
        console.info('[draft] using local draft with', localDraft.length, 'annotations');
      }

      connectWs(id);
      await refreshSnapshots(id);
      await refreshCollabUsers(id);
      latestVersion.value = await SnapshotApi.latestVersion(id).catch(() => 0);
    } catch (e: unknown) {
      error.value = (e as Error).message;
    }
  }

  function resetViewForImage(w: number, h: number): void {
    const containerW = Math.max(window.innerWidth - 560, 800);
    const containerH = Math.max(window.innerHeight - 120, 600);
    const scale = Math.min(containerW / w, containerH / h, 1);
    transform.value = {
      scale,
      offsetX: (containerW - w * scale) / 2,
      offsetY: (containerH - h * scale) / 2,
    };
  }

  function connectWs(imageId: number): void {
    globalWs.disconnect();
    globalWs.connect(imageId);
    globalWs.events.on('message', handleWsMessage);
    globalWs.events.on('open', () => refreshCollabUsers(imageId));
  }

  function handleWsMessage(msg: OperationMessage): void {
    if (!currentImage.value) return;
    switch (msg.type) {
      case 'add': {
        const a = msg.payload as PolygonAnnotation;
        if (a && a.id && !annotations.value.some((x) => x.id === a.id)) {
          annotations.value.push(a);
        }
        break;
      }
      case 'update': {
        const a = msg.payload as PolygonAnnotation;
        const i = annotations.value.findIndex((x) => x.id === a.id);
        if (i >= 0) annotations.value.splice(i, 1, a);
        break;
      }
      case 'delete': {
        const a = msg.payload as PolygonAnnotation;
        annotations.value = annotations.value.filter((x) => x.id !== a.id);
        break;
      }
      case 'bulk_replace': {
        const list = msg.payload as PolygonAnnotation[];
        if (Array.isArray(list)) annotations.value = list;
        break;
      }
      case 'rollback': {
        const p = msg.payload as { version: number; annotations: PolygonAnnotation[] };
        if (p?.annotations) {
          annotations.value = p.annotations;
          latestVersion.value = p.version;
          refreshSnapshots(currentImage.value.id);
        }
        break;
      }
      case 'presence': {
        const users = msg.payload as CollabUser[];
        if (Array.isArray(users)) {
          collabUsers.value = users;
        }
        break;
      }
    }
  }

  async function refreshCollabUsers(imageId: number): Promise<void> {
    try {
      collabUsers.value = await CollabApi.listUsers(imageId);
    } catch {}
  }

  async function refreshSnapshots(imageId: number): Promise<void> {
    try {
      snapshots.value = await SnapshotApi.list(imageId);
    } catch {}
  }

  function selectAnnotation(id: number, multi = false): void {
    if (multi) {
      if (selectedAnnotationIds.value.has(id)) selectedAnnotationIds.value.delete(id);
      else selectedAnnotationIds.value.add(id);
    } else {
      selectedAnnotationIds.value.clear();
      if (id != null) selectedAnnotationIds.value.add(id);
    }
  }

  function clearSelection(): void {
    selectedAnnotationIds.value.clear();
  }

  async function createAnnotation(
    points: { x: number; y: number }[],
    opts?: Partial<Pick<PolygonAnnotation, 'label' | 'note' | 'layerId'>>,
  ): Promise<PolygonAnnotation | null> {
    if (!currentImage.value || points.length < 3) return null;
    isSaving.value = true;
    try {
      const created = await AnnotationApi.create({
        scanImageId: currentImage.value.id,
        diseaseTypeId: activeDiseaseTypeId.value,
        severity: severity.value,
        points,
        ...(opts || {}),
      });
      if (created) {
        annotations.value.push(created);
        selectedAnnotationIds.value.clear();
        selectedAnnotationIds.value.add(created.id);
        latestVersion.value = await SnapshotApi.latestVersion(currentImage.value.id).catch(() => latestVersion.value);
        refreshSnapshots(currentImage.value.id);
      }
      return created || null;
    } catch (e: unknown) {
      error.value = (e as Error).message;
      return null;
    } finally {
      isSaving.value = false;
    }
  }

  async function updateAnnotation(
    id: number,
    patch: Partial<PolygonAnnotation>,
  ): Promise<PolygonAnnotation | null> {
    if (!currentImage.value) return null;
    isSaving.value = true;
    try {
      const updated = await AnnotationApi.update(id, patch);
      if (updated) {
        const i = annotations.value.findIndex((x) => x.id === id);
        if (i >= 0) annotations.value.splice(i, 1, updated);
        refreshSnapshots(currentImage.value.id);
      }
      return updated || null;
    } catch (e: unknown) {
      error.value = (e as Error).message;
      return null;
    } finally {
      isSaving.value = false;
    }
  }

  async function deleteAnnotation(id: number): Promise<void> {
    if (!currentImage.value) return;
    isSaving.value = true;
    try {
      await AnnotationApi.remove(id);
      annotations.value = annotations.value.filter((x) => x.id !== id);
      selectedAnnotationIds.value.delete(id);
      refreshSnapshots(currentImage.value.id);
    } catch (e: unknown) {
      error.value = (e as Error).message;
    } finally {
      isSaving.value = false;
    }
  }

  async function deleteSelected(): Promise<void> {
    for (const id of Array.from(selectedAnnotationIds.value)) {
      await deleteAnnotation(id);
    }
  }

  async function restoreVersion(version: number): Promise<void> {
    if (!currentImage.value) return;
    isSaving.value = true;
    try {
      const list = await SnapshotApi.restore(currentImage.value.id, version);
      if (list) annotations.value = list;
      latestVersion.value = version;
      refreshSnapshots(currentImage.value.id);
    } catch (e: unknown) {
      error.value = (e as Error).message;
    } finally {
      isSaving.value = false;
    }
  }

  async function saveLocalDraft(): Promise<void> {
    if (!currentImage.value) return;
    await LocalDraft.saveAnnotations(currentImage.value.id, annotations.value);
    await LocalDraft.saveDraft(
      currentImage.value.id,
      annotations.value,
      currentTool.value,
      transform.value,
    );
  }

  return {
    componentTree, diseaseTypes,
    selectedComponentId, componentImages, currentImage,
    annotations, selectedAnnotationIds, activeDraftPoints,
    currentTool, activeDiseaseTypeId, severity,
    transform, snapshots, latestVersion, collabUsers,
    isSaving, error,
    diseaseTypeMap, diseaseCodeColor, diseaseColor,
    init, selectComponent, selectImage,
    selectAnnotation, clearSelection,
    createAnnotation, updateAnnotation, deleteAnnotation, deleteSelected,
    restoreVersion, saveLocalDraft,
    resetViewForImage, refreshSnapshots, refreshCollabUsers,
  };
});
