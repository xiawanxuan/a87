<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useAnnotationStore } from '@/stores/annotation';
import type { ToolType } from '@/types';
import {
  Pointer, Rank, Operation, Crop, Delete, ZoomIn, ZoomOut,
  RefreshLeft, RefreshRight, DocumentChecked, Connection, Setting, Download,
} from '@element-plus/icons-vue';
import { globalWs } from '@/services/ws';
import { ElMessage, ElMessageBox } from 'element-plus';
import { SnapshotApi } from '@/services/api';

const store = useAnnotationStore();
const tools: { key: ToolType; icon: unknown; label: string }[] = [
  { key: 'select', icon: Pointer, label: '选择 (V)' },
  { key: 'pan', icon: Rank, label: '平移 (H)' },
  { key: 'polygon', icon: Operation, label: '多边形标注 (P)' },
  { key: 'rect', icon: Crop, label: '矩形标注 (R)' },
  { key: 'zoom', icon: ZoomIn, label: '缩放 (Z)' },
];

const scale = computed(() => Math.round(store.transform.scale * 100));

function setTool(t: ToolType): void {
  store.currentTool = t;
}

function zoomBy(factor: number): void {
  if (!store.currentImage) return;
  const { width: w, height: h } = store.currentImage;
  const prev = store.transform;
  const newScale = Math.max(0.05, Math.min(10, prev.scale * factor));
  const cx = w / 2, cy = h / 2;
  const sx = cx * prev.scale + prev.offsetX;
  const sy = cy * prev.scale + prev.offsetY;
  store.transform = {
    scale: newScale,
    offsetX: sx - cx * newScale,
    offsetY: sy - cy * newScale,
  };
}

function resetView(): void {
  if (!store.currentImage) return;
  store.resetViewForImage(store.currentImage.width, store.currentImage.height);
}

async function onDeleteSelected(): Promise<void> {
  if (store.selectedAnnotationIds.size === 0) return;
  try {
    await ElMessageBox.confirm(`删除 ${store.selectedAnnotationIds.size} 个标注?`, '删除', { type: 'warning' });
    await store.deleteSelected();
    ElMessage.success('已删除');
  } catch {}
}

async function createSnapshot(): Promise<void> {
  if (!store.currentImage) return;
  try {
    const v = await SnapshotApi.create(store.currentImage.id, 'manual snapshot');
    await store.refreshSnapshots(store.currentImage.id);
    ElMessage.success(`快照已创建 v${v}`);
  } catch (e) {
    ElMessage.error('创建快照失败');
  }
}

const wsStatus = computed(() => globalWs.connected.value);

function onUndo(): void { ElMessage.info('撤销: 请使用右侧快照版本回滚'); }
function onRedo(): void { ElMessage.info('重做: 请使用右侧快照版本'); }

function saveNow(): void {
  store.saveLocalDraft().then(() => ElMessage.success('草稿已保存到本地'));
}

function onKeyDown(e: KeyboardEvent): void {
  if ((e.target as HTMLElement)?.tagName === 'INPUT' || (e.target as HTMLElement)?.tagName === 'TEXTAREA') return;
  switch (e.key.toLowerCase()) {
    case 'v': setTool('select'); break;
    case 'h': setTool('pan'); break;
    case 'p': setTool('polygon'); break;
    case 'r': setTool('rect'); break;
    case 'z':
      if (e.ctrlKey || e.metaKey) { e.preventDefault(); onUndo(); }
      else setTool('zoom');
      break;
    case '+': case '=': zoomBy(1.25); break;
    case '-': case '_': zoomBy(0.8); break;
    case '0': resetView(); break;
    case 'delete': case 'backspace': onDeleteSelected(); break;
    case 's':
      if (e.ctrlKey || e.metaKey) { e.preventDefault(); saveNow(); }
      break;
  }
}

const mountedFlag = ref(false);
watch(
  mountedFlag,
  (v) => {
    if (v) window.addEventListener('keydown', onKeyDown);
  },
  { immediate: true },
);
</script>

<template>
  <header class="top-toolbar">
    <div class="brand">
      <el-icon :size="20" color="#4dabf7"><Setting /></el-icon>
      <h1 class="brand-title">古建筑木构件超声波探伤标注系统</h1>
    </div>

    <div class="tool-group">
      <el-tooltip v-for="t in tools" :key="t.key" :content="t.label" placement="bottom">
        <button
          class="tool-btn"
          :class="{ active: store.currentTool === t.key }"
          @click="setTool(t.key)"
        >
          <el-icon><component :is="t.icon" /></el-icon>
        </button>
      </el-tooltip>
    </div>

    <div class="divider" />

    <div class="tool-group">
      <el-tooltip content="缩小 ( - )">
        <button class="tool-btn" @click="zoomBy(0.8)"><el-icon><ZoomOut /></el-icon></button>
      </el-tooltip>
      <span class="scale-display">{{ scale }}%</span>
      <el-tooltip content="放大 ( + )">
        <button class="tool-btn" @click="zoomBy(1.25)"><el-icon><ZoomIn /></el-icon></button>
      </el-tooltip>
      <el-tooltip content="重置视图 ( 0 )">
        <button class="tool-btn" @click="resetView"><el-icon><RefreshLeft /></el-icon></button>
      </el-tooltip>
    </div>

    <div class="divider" />

    <div class="tool-group">
      <el-tooltip content="撤销">
        <button class="tool-btn" @click="onUndo"><el-icon><RefreshRight /></el-icon></button>
      </el-tooltip>
      <el-tooltip content="重做">
        <button class="tool-btn" @click="onRedo"><el-icon><RefreshLeft /></el-icon></button>
      </el-tooltip>
      <el-tooltip content="删除选中 (Del)">
        <button class="tool-btn danger" @click="onDeleteSelected" :disabled="store.selectedAnnotationIds.size === 0">
          <el-icon><Delete /></el-icon>
        </button>
      </el-tooltip>
      <el-tooltip content="创建快照版本">
        <button class="tool-btn success" @click="createSnapshot" :disabled="!store.currentImage">
          <el-icon><DocumentChecked /></el-icon>
        </button>
      </el-tooltip>
      <el-tooltip content="立即保存本地草稿">
        <button class="tool-btn" @click="saveNow"><el-icon><Download /></el-icon></button>
      </el-tooltip>
    </div>

    <div class="spacer" />

    <div class="status-group">
      <span class="ws-status" :class="{ connected: wsStatus }">
        <span class="dot" />
        {{ wsStatus ? '协同在线' : '协同离线' }}
      </span>
      <span class="img-info" v-if="store.currentImage">
        {{ store.currentImage.width }}×{{ store.currentImage.height }} · 标注 {{ store.annotations.length }}
      </span>
      <span class="img-info" v-if="store.isSaving">
        <el-icon class="spin"><Connection /></el-icon>保存中...
      </span>
    </div>
  </header>
</template>

<style lang="scss" scoped>
.top-toolbar {
  height: var(--toolbar-height);
  background: var(--color-panel);
  border-bottom: 1px solid var(--color-border);
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 14px;
  flex-shrink: 0;
}
.brand { display: flex; align-items: center; gap: 10px; margin-right: 10px; }
.brand-title {
  font-size: 15px;
  font-weight: 600;
  margin: 0;
  color: var(--color-text);
  letter-spacing: 0.5px;
}
.tool-group { display: flex; align-items: center; gap: 2px; padding: 0 4px; }
.divider {
  width: 1px; height: 26px; background: var(--color-border);
}
.tool-btn {
  width: 36px; height: 36px;
  border-radius: 6px;
  background: transparent;
  border: 1px solid transparent;
  color: var(--color-text-dim);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.15s;
  &:hover:not(:disabled) {
    background: rgba(77, 171, 247, 0.12);
    color: var(--color-accent);
    border-color: rgba(77, 171, 247, 0.3);
  }
  &.active {
    background: rgba(77, 171, 247, 0.2);
    color: var(--color-accent);
    border-color: rgba(77, 171, 247, 0.5);
  }
  &.danger:hover:not(:disabled) { color: var(--color-danger); border-color: rgba(255,107,107,0.3); }
  &.success:hover:not(:disabled) { color: var(--color-success); border-color: rgba(81,207,102,0.3); }
  &:disabled { opacity: 0.4; cursor: not-allowed; }
}
.scale-display {
  min-width: 54px;
  text-align: center;
  font-variant-numeric: tabular-nums;
  font-size: 12px;
  color: var(--color-text-dim);
}
.spacer { flex: 1; }
.status-group {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 12px;
}
.ws-status {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-dim);
  .dot {
    width: 8px; height: 8px; border-radius: 50%;
    background: var(--color-danger);
    box-shadow: 0 0 8px var(--color-danger);
  }
  &.connected {
    color: var(--color-success);
    .dot { background: var(--color-success); box-shadow: 0 0 8px var(--color-success); }
  }
}
.img-info { color: var(--color-text-dim); font-variant-numeric: tabular-nums; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }
</style>
