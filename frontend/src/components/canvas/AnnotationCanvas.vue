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

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
