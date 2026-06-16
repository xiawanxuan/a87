<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue';
import { useAnnotationStore } from '@/stores/annotation';
import { useCanvas } from '@/composables/useCanvas';
import { ScanImageApi } from '@/services/api';
import { globalWs } from '@/services/ws';
import type { CollabUser } from '@/types';
import { colorForUser } from '@/utils/geometry';
import { Loading, Warning } from '@element-plus/icons-vue';

const store = useAnnotationStore();
const containerRef = ref<HTMLDivElement | null>(null);

const {
  canvas, overlay, imageLoaded, imageLoading, markDirty,
  hoveredAnnotation, hoverScreenPos,
  resizeCanvas, loadImage, setupResize,
  onPointerDown, onPointerMove, onPointerUp, onDblClick, onWheel, onKeyDown,
  prevent,
} = useCanvas(containerRef);

const imageUrl = computed(() => {
  if (!store.currentImage) return '';
  return ScanImageApi.viewUrl(store.currentImage);
});

watch(
  () => store.currentImage?.id,
  async (id) => {
    if (!id || !imageUrl.value) return;
    try {
      await loadImage(imageUrl.value);
    } catch (e) {
      console.error('[canvas] load image failed', e);
    }
  },
);

watch(
  () => store.annotations.length,
  () => {
    markDirty();
  },
);

watch(
  () => store.selectedAnnotationIds.size,
  () => {
    markDirty();
  },
);

function onContainerContextMenu(e: MouseEvent): void {
  e.preventDefault();
}

function sendCursor(e: PointerEvent): void {
  if (!globalWs.connected.value || !containerRef.value) return;
  const rect = containerRef.value.getBoundingClientRect();
  const x = e.clientX - rect.left;
  const y = e.clientY - rect.top;
  globalWs.sendCursor(x, y);
}

const collabCursors = computed(() => {
  return store.collabUsers.filter((u: CollabUser) => u.cursorPos && u.userId !== '');
});

function cursorColor(userId: string): string {
  return colorForUser(userId);
}

function formatAreaCm2(cm2: number | undefined): string {
  if (!cm2) return '-';
  if (cm2 < 1) return (cm2 * 100).toFixed(1) + ' mm²';
  if (cm2 < 100) return cm2.toFixed(2) + ' cm²';
  return (cm2 / 100).toFixed(2) + ' dm²';
}

function formatAreaPx(px: number | undefined): string {
  if (!px) return '-';
  if (px < 10000) return px.toFixed(0) + ' px²';
  if (px < 1000000) return (px / 1000).toFixed(1) + ' Kpx²';
  return (px / 1000000).toFixed(2) + ' Mpx²';
}

const tooltipStyle = computed(() => {
  if (!hoveredAnnotation.value) return { display: 'none' };
  const x = hoverScreenPos.value.x + 12;
  const y = hoverScreenPos.value.y + 12;
  return {
    left: x + 'px',
    top: y + 'px',
  };
});

function diseaseName(diseaseTypeId: number): string {
  return store.diseaseTypeMap.get(diseaseTypeId)?.name || '未知';
}

function diseaseColor(diseaseTypeId: number): string {
  return store.diseaseTypeMap.get(diseaseTypeId)?.colorHex || '#FF6B6B';
}

onMounted(() => {
  setupResize();
  window.addEventListener('keydown', onKeyDown);
  setTimeout(resizeCanvas, 50);
});

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown);
});
</script>

<template>
  <div
    ref="containerRef"
    class="canvas-container"
    @contextmenu.prevent="onContainerContextMenu"
    @pointerdown="onPointerDown"
    @pointermove="onPointerMove; sendCursor"
    @pointerup="onPointerUp"
    @pointercancel="onPointerUp"
    @pointerleave="onPointerUp"
    @dblclick="onDblClick"
    @wheel="onWheel"
    @touchstart.prevent="prevent"
    @touchmove.prevent="prevent"
  >
    <canvas ref="canvas" class="base-canvas" />
    <canvas ref="overlay" class="overlay-canvas" />

    <div
      v-for="u in collabCursors"
      :key="u.userId"
      class="collab-cursor"
      :style="{
        left: (u.cursorPos?.x || 0) + 'px',
        top: (u.cursorPos?.y || 0) + 'px',
        '--cursor-color': cursorColor(u.userId),
      }"
    >
      <div class="cursor-pointer" />
      <span class="cursor-label">{{ u.userName }}</span>
    </div>

    <div v-if="imageLoading" class="canvas-loading">
      <el-icon class="spin" :size="32"><Loading /></el-icon>
      <span>加载中...</span>
    </div>

    <div v-if="!store.currentImage" class="canvas-empty">
      <el-icon :size="48" color="#4a5568"><Warning /></el-icon>
      <p>请从左侧目录选择探伤图谱</p>
    </div>

    <div v-if="imageLoaded && store.currentImage" class="canvas-info">
      <span class="info-item">尺寸: {{ store.currentImage.width }}×{{ store.currentImage.height }}</span>
      <span class="info-item">标注: {{ store.annotations.length }}</span>
      <span class="info-item">版本: v{{ store.latestVersion }}</span>
    </div>

    <div v-if="store.collabUsers.length > 1" class="collab-users-bar">
      <div
        v-for="u in store.collabUsers.slice(0, 6)"
        :key="u.userId"
        class="user-avatar"
        :style="{ background: cursorColor(u.userId) }"
        :title="u.userName"
      >
        {{ u.userName?.charAt(0) || 'U' }}
      </div>
      <div v-if="store.collabUsers.length > 6" class="user-avatar more">
        +{{ store.collabUsers.length - 6 }}
      </div>
    </div>

    <div v-if="hoveredAnnotation" class="annotation-tooltip" :style="tooltipStyle">
      <div class="tooltip-header">
        <span class="tooltip-color-dot" :style="{ background: diseaseColor(hoveredAnnotation.diseaseTypeId) }" />
        <span class="tooltip-title">{{ hoveredAnnotation.label || `标注 #${hoveredAnnotation.id}` }}</span>
      </div>
      <div class="tooltip-body">
        <div class="tooltip-row">
          <span class="tooltip-label">病害类型</span>
          <span class="tooltip-value">{{ diseaseName(hoveredAnnotation.diseaseTypeId) }}</span>
        </div>
        <div class="tooltip-row">
          <span class="tooltip-label">严重等级</span>
          <span class="tooltip-value">
            <el-rate
              :model-value="hoveredAnnotation.severity"
              disabled
              :max="5"
              size="small"
              :colors="['#51cf66', '#94d82d', '#fcc419', '#ff922b', '#ff6b6b']"
            />
          </span>
        </div>
        <div class="tooltip-divider" />
        <div class="tooltip-row">
          <span class="tooltip-label">实际面积</span>
          <span class="tooltip-value highlight">{{ formatAreaCm2(hoveredAnnotation.areaCm2) }}</span>
        </div>
        <div class="tooltip-row">
          <span class="tooltip-label">像素面积</span>
          <span class="tooltip-value">{{ formatAreaPx(hoveredAnnotation.areaPx) }}</span>
        </div>
        <div class="tooltip-row">
          <span class="tooltip-label">顶点数</span>
          <span class="tooltip-value">{{ hoveredAnnotation.points?.length || 0 }}</span>
        </div>
        <div v-if="hoveredAnnotation.operator" class="tooltip-row">
          <span class="tooltip-label">操作人</span>
          <span class="tooltip-value">{{ hoveredAnnotation.operator }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.canvas-container {
  position: absolute;
  inset: 0;
  overflow: hidden;
  background: #0e0e1a;
  touch-action: none;
  user-select: none;
  -webkit-user-select: none;
}

.base-canvas,
.overlay-canvas {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
}

.overlay-canvas {
  pointer-events: none;
}

.canvas-loading {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  color: #9aa7bd;
  font-size: 14px;
  .spin { animation: spin 1s linear infinite; }
}

.canvas-empty {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 16px;
  color: #4a5568;
  font-size: 14px;
  p { margin: 0; }
}

.canvas-info {
  position: absolute;
  bottom: 12px;
  left: 12px;
  display: flex;
  gap: 16px;
  padding: 6px 12px;
  background: rgba(0, 0, 0, 0.6);
  border-radius: 4px;
  font-size: 12px;
  color: #9aa7bd;
  font-variant-numeric: tabular-nums;
  pointer-events: none;
}

.collab-users-bar {
  position: absolute;
  top: 12px;
  right: 12px;
  display: flex;
  gap: -6px;
  pointer-events: none;
}

.user-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  border: 2px solid #0e0e1a;
  margin-left: -8px;
  &:first-child { margin-left: 0; }
  &.more {
    background: #2a3f5f;
  }
}

.collab-cursor {
  position: absolute;
  pointer-events: none;
  z-index: 10;
  transition: opacity 0.2s;
  --cursor-color: #4dabf7;
}

.cursor-pointer {
  width: 0;
  height: 0;
  border-left: 10px solid var(--cursor-color);
  border-top: 6px solid transparent;
  border-bottom: 6px solid transparent;
  transform: rotate(20deg);
  filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.5));
}

.cursor-label {
  position: absolute;
  top: 10px;
  left: 8px;
  padding: 2px 6px;
  background: var(--cursor-color);
  color: #fff;
  font-size: 11px;
  border-radius: 3px;
  white-space: nowrap;
  font-weight: 500;
}

.annotation-tooltip {
  position: absolute;
  z-index: 20;
  pointer-events: none;
  background: rgba(26, 32, 44, 0.96);
  border: 1px solid rgba(74, 85, 104, 0.8);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
  min-width: 200px;
  max-width: 260px;
  backdrop-filter: blur(8px);
}

.tooltip-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border-bottom: 1px solid rgba(74, 85, 104, 0.5);
}

.tooltip-color-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.tooltip-title {
  font-size: 13px;
  font-weight: 600;
  color: #e2e8f0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tooltip-body {
  padding: 8px 12px;
}

.tooltip-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px 0;
  font-size: 12px;
}

.tooltip-label {
  color: #718096;
}

.tooltip-value {
  color: #cbd5e0;
  font-variant-numeric: tabular-nums;

  &.highlight {
    color: #4dabf7;
    font-weight: 600;
  }
}

.tooltip-divider {
  height: 1px;
  background: rgba(74, 85, 104, 0.4);
  margin: 6px 0;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
