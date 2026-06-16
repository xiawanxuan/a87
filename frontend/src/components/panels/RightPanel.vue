<script setup lang="ts">
import { ref, computed } from 'vue';
import { useAnnotationStore } from '@/stores/annotation';
import { ElMessage, ElMessageBox } from 'element-plus';
import {
  Grid, List, Clock, User, CircleCheck, Delete, Edit, View,
  RefreshRight, ArrowDown,
} from '@element-plus/icons-vue';
import type { PolygonAnnotation, DiseaseType, AnnotationSnapshot } from '@/types';
import { colorForUser } from '@/utils/geometry';

const store = useAnnotationStore();

const activeTab = ref<'disease' | 'annotations' | 'snapshots' | 'collab'>('disease');

const tabs = [
  { key: 'disease', label: '病害分类', icon: Grid },
  { key: 'annotations', label: '标注列表', icon: List },
  { key: 'snapshots', label: '版本快照', icon: Clock },
  { key: 'collab', label: '协同用户', icon: User },
] as const;

function setTab(t: typeof activeTab.value): void {
  activeTab.value = t;
}

const sortedDiseases = computed(() => {
  return [...store.diseaseTypes].sort((a, b) => a.severityOrder - b.severityOrder);
});

function selectDisease(id: number): void {
  store.activeDiseaseTypeId = id;
}

const sortedAnnotations = computed(() => {
  return [...store.annotations].sort((a, b) => b.id - a.id);
});

function diseaseName(id: number): string {
  return store.diseaseTypeMap.get(id)?.name || '未知';
}

function diseaseColor(id: number): string {
  return store.diseaseTypeMap.get(id)?.colorHex || '#FF6B6B';
}

function selectAnnotationItem(ann: PolygonAnnotation): void {
  store.selectAnnotation(ann.id, false);
}

function formatArea(px: number | undefined): string {
  if (!px) return '-';
  if (px < 10000) return px.toFixed(0) + ' px²';
  if (px < 1000000) return (px / 1000).toFixed(1) + ' Kpx²';
  return (px / 1000000).toFixed(2) + ' Mpx²';
}

async function deleteAnnotation(id: number): Promise<void> {
  try {
    await ElMessageBox.confirm('确认删除该标注?', '删除', { type: 'warning' });
    await store.deleteAnnotation(id);
    ElMessage.success('已删除');
  } catch {}
}

async function restoreSnapshot(snap: AnnotationSnapshot): Promise<void> {
  try {
    await ElMessageBox.confirm(
      `回滚到版本 v${snap.versionNumber}？\n${snap.diffSummary || ''}`,
      '版本回滚',
      { type: 'warning' },
    );
    await store.restoreVersion(snap.versionNumber);
    ElMessage.success(`已回滚到 v${snap.versionNumber}`);
  } catch {}
}

function formatTime(iso: string): string {
  if (!iso) return '-';
  const d = new Date(iso);
  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

const severityLabels = ['', '轻微', '较轻', '中等', '较重', '严重'];
const severityColors = ['', '#51cf66', '#94d82d', '#fcc419', '#ff922b', '#ff6b6b'];
</script>

<template>
  <div class="right-panel">
    <div class="panel-tabs">
      <button
        v-for="t in tabs"
        :key="t.key"
        class="tab-btn"
        :class="{ active: activeTab === t.key }"
        @click="setTab(t.key)"
      >
        <el-icon><component :is="t.icon" /></el-icon>
        <span class="tab-label">{{ t.label }}</span>
      </button>
    </div>

    <div class="panel-content">
      <div v-if="activeTab === 'disease'" class="disease-list">
        <div class="section-title">病害分类标签</div>
        <div class="disease-items">
          <div
            v-for="d in sortedDiseases"
            :key="d.id"
            class="disease-item"
            :class="{ active: store.activeDiseaseTypeId === d.id, disabled: !d.enabled }"
            @click="d.enabled && selectDisease(d.id)"
          >
            <span class="disease-color-dot" :style="{ background: d.colorHex }" />
            <div class="disease-info">
              <div class="disease-name">{{ d.name }}</div>
              <div class="disease-code">{{ d.code }}</div>
            </div>
            <el-icon v-if="store.activeDiseaseTypeId === d.id" class="check-icon">
              <CircleCheck />
            </el-icon>
          </div>
        </div>

        <div class="section-title" style="margin-top: 20px;">严重等级</div>
        <el-rate
          v-model="store.severity"
          :max="5"
          :colors="['#51cf66', '#94d82d', '#fcc419', '#ff922b', '#ff6b6b']"
          :texts="['轻微', '较轻', '中等', '较重', '严重']"
          show-text
          text-color="#9aa7bd"
        />

        <div class="section-title" style="margin-top: 20px;">标注统计</div>
        <div class="stats-grid">
          <div class="stat-item">
            <div class="stat-value">{{ store.annotations.length }}</div>
            <div class="stat-label">标注总数</div>
          </div>
          <div class="stat-item">
            <div class="stat-value">
              {{ formatArea(store.annotations.reduce((s, a) => s + (a.areaPx || 0), 0)) }}
            </div>
            <div class="stat-label">总面积</div>
          </div>
        </div>
      </div>

      <div v-if="activeTab === 'annotations'" class="annotations-panel">
        <div class="section-title">
          标注列表
          <span class="count-badge">{{ store.annotations.length }}</span>
        </div>

        <div v-if="store.annotations.length === 0" class="empty-state">
          <el-icon :size="32"><Edit /></el-icon>
          <p>暂无标注</p>
          <p class="hint">使用多边形工具在图谱上绘制病害区域</p>
        </div>

        <div v-else class="annotation-list">
          <div
            v-for="ann in sortedAnnotations"
            :key="ann.id"
            class="annotation-item"
            :class="{ selected: store.selectedAnnotationIds.has(ann.id) }"
            @click="selectAnnotationItem(ann)"
          >
            <span class="ann-color-bar" :style="{ background: diseaseColor(ann.diseaseTypeId) }" />
            <div class="ann-content">
              <div class="ann-title">
                <span class="ann-label">{{ ann.label || `#${ann.id}` }}</span>
                <span
                  class="ann-severity"
                  :style="{ color: severityColors[ann.severity] }"
                >
                  {{ severityLabels[ann.severity] }}
                </span>
              </div>
              <div class="ann-meta">
                <span class="ann-type">{{ diseaseName(ann.diseaseTypeId) }}</span>
                <span class="ann-area">{{ formatArea(ann.areaPx) }}</span>
                <span class="ann-time">{{ formatTime(ann.createdAt) }}</span>
              </div>
              <div v-if="ann.note" class="ann-note">{{ ann.note }}</div>
            </div>
            <div class="ann-actions" @click.stop>
              <el-button size="small" text type="danger" @click="deleteAnnotation(ann.id)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
          </div>
        </div>
      </div>

      <div v-if="activeTab === 'snapshots'" class="snapshots-panel">
        <div class="section-title">
          版本快照
          <span class="count-badge">{{ store.snapshots.length }}</span>
        </div>

        <div class="snapshot-tip">
          <el-icon><RefreshRight /></el-icon>
          <span>每次修改自动生成快照，保留最近 30 个版本</span>
        </div>

        <div v-if="store.snapshots.length === 0" class="empty-state">
          <el-icon :size="32"><Clock /></el-icon>
          <p>暂无快照</p>
          <p class="hint">保存标注后将自动生成版本快照</p>
        </div>

        <div v-else class="snapshot-list">
          <div
            v-for="(snap, idx) in store.snapshots"
            :key="snap.id"
            class="snapshot-item"
            :class="{ latest: idx === 0 }"
          >
            <div class="snap-header">
              <span class="snap-version">v{{ snap.versionNumber }}</span>
              <span v-if="idx === 0" class="snap-badge latest">当前</span>
            </div>
            <div class="snap-info">
              <el-icon class="snap-icon"><Clock /></el-icon>
              <div class="snap-detail">
                <div class="snap-summary">{{ snap.diffSummary || '版本快照' }}</div>
                <div class="snap-meta">
                  <span>{{ snap.operator || '系统' }}</span>
                  <span>{{ formatTime(snap.createdAt) }}</span>
                </div>
              </div>
            </div>
            <button class="snap-restore-btn" @click="restoreSnapshot(snap)">
              <el-icon><ArrowDown /></el-icon>
              回滚
            </button>
          </div>
        </div>
      </div>

      <div v-if="activeTab === 'collab'" class="collab-panel">
        <div class="section-title">
          在线协同
          <span class="count-badge online">{{ store.collabUsers.length }}</span>
        </div>

        <div class="collab-tip">
          <el-icon><User /></el-icon>
          <span>多人同时编辑，操作实时同步</span>
        </div>

        <div class="collab-user-list">
          <div
            v-for="u in store.collabUsers"
            :key="u.userId"
            class="collab-user-item"
          >
            <div
              class="user-avatar-lg"
              :style="{ background: colorForUser(u.userId) }"
            >
              {{ u.userName?.charAt(0) || 'U' }}
            </div>
            <div class="user-info">
              <div class="user-name">{{ u.userName }}</div>
              <div class="user-status">
                <span class="status-dot online" />
                在线
              </div>
            </div>
            <div class="user-tool" v-if="u.activeTool">
              <el-icon><View /></el-icon>
              <span>{{ u.activeTool }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.right-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  background: var(--color-panel);
}

.panel-tabs {
  display: flex;
  border-bottom: 1px solid var(--color-border);
  flex-shrink: 0;
}

.tab-btn {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  padding: 10px 4px;
  background: transparent;
  border: none;
  color: var(--color-text-dim);
  cursor: pointer;
  font-size: 11px;
  transition: all 0.15s;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;

  &:hover {
    color: var(--color-text);
    background: rgba(77, 171, 247, 0.06);
  }

  &.active {
    color: var(--color-accent);
    border-bottom-color: var(--color-accent);
    background: rgba(77, 171, 247, 0.08);
  }
}

.tab-label {
  font-size: 11px;
}

.panel-content {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-dim);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  padding: 12px 14px 8px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.count-badge {
  padding: 1px 8px;
  background: var(--color-panel-2);
  border-radius: 10px;
  font-size: 11px;
  color: var(--color-text-dim);
  font-weight: 500;

  &.online {
    background: rgba(81, 207, 102, 0.15);
    color: var(--color-success);
  }
}

.disease-list {
  padding-bottom: 16px;
}

.disease-items {
  padding: 0 10px;
}

.disease-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  margin-bottom: 4px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.15s;
  border: 1px solid transparent;

  &:hover {
    background: rgba(77, 171, 247, 0.08);
  }

  &.active {
    background: rgba(77, 171, 247, 0.15);
    border-color: rgba(77, 171, 247, 0.4);
  }

  &.disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
}

.disease-color-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  flex-shrink: 0;
  box-shadow: 0 0 0 2px rgba(255, 255, 255, 0.1);
}

.disease-info {
  flex: 1;
  min-width: 0;
}

.disease-name {
  font-size: 13px;
  color: var(--color-text);
  font-weight: 500;
}

.disease-code {
  font-size: 11px;
  color: var(--color-text-dim);
  font-family: monospace;
}

.check-icon {
  color: var(--color-accent);
  font-size: 16px;
}

.stats-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  padding: 0 14px;
}

.stat-item {
  padding: 12px;
  background: var(--color-panel-2);
  border-radius: 8px;
  text-align: center;
}

.stat-value {
  font-size: 16px;
  font-weight: 600;
  color: var(--color-accent);
  font-variant-numeric: tabular-nums;
}

.stat-label {
  font-size: 11px;
  color: var(--color-text-dim);
  margin-top: 4px;
}

.annotations-panel,
.snapshots-panel,
.collab-panel {
  padding-bottom: 16px;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  color: var(--color-text-dim);
  gap: 8px;

  p {
    margin: 0;
    font-size: 13px;
  }

  .hint {
    font-size: 12px;
    color: #4a5568;
  }
}

.annotation-list {
  padding: 0 10px;
}

.annotation-item {
  display: flex;
  align-items: stretch;
  gap: 0;
  margin-bottom: 6px;
  background: var(--color-panel-2);
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  transition: all 0.15s;
  border: 1px solid transparent;

  &:hover {
    border-color: rgba(77, 171, 247, 0.3);
  }

  &.selected {
    border-color: var(--color-accent);
    background: rgba(77, 171, 247, 0.1);
  }
}

.ann-color-bar {
  width: 4px;
  flex-shrink: 0;
}

.ann-content {
  flex: 1;
  min-width: 0;
  padding: 10px 12px;
}

.ann-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
}

.ann-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text);
}

.ann-severity {
  font-size: 11px;
  font-weight: 500;
}

.ann-meta {
  display: flex;
  gap: 10px;
  font-size: 11px;
  color: var(--color-text-dim);
  font-variant-numeric: tabular-nums;
}

.ann-note {
  margin-top: 6px;
  font-size: 12px;
  color: var(--color-text-dim);
  padding: 4px 8px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 4px;
}

.ann-actions {
  display: flex;
  align-items: center;
  padding: 0 8px;
  opacity: 0;
  transition: opacity 0.15s;
}

.annotation-item:hover .ann-actions {
  opacity: 1;
}

.snapshot-tip,
.collab-tip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  margin: 0 10px 10px;
  background: rgba(77, 171, 247, 0.08);
  border-radius: 6px;
  font-size: 12px;
  color: var(--color-text-dim);
}

.snapshot-list {
  padding: 0 10px;
}

.snapshot-item {
  padding: 12px;
  margin-bottom: 8px;
  background: var(--color-panel-2);
  border-radius: 8px;
  border: 1px solid transparent;
  transition: all 0.15s;

  &:hover {
    border-color: rgba(77, 171, 247, 0.3);
  }

  &.latest {
    border-color: rgba(81, 207, 102, 0.4);
    background: rgba(81, 207, 102, 0.05);
  }
}

.snap-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.snap-version {
  font-size: 14px;
  font-weight: 600;
  color: var(--color-text);
  font-family: monospace;
}

.snap-badge {
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 500;

  &.latest {
    background: rgba(81, 207, 102, 0.2);
    color: var(--color-success);
  }
}

.snap-info {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin-bottom: 10px;
}

.snap-icon {
  color: var(--color-text-dim);
  margin-top: 2px;
}

.snap-detail {
  flex: 1;
  min-width: 0;
}

.snap-summary {
  font-size: 12px;
  color: var(--color-text);
  margin-bottom: 4px;
}

.snap-meta {
  display: flex;
  gap: 10px;
  font-size: 11px;
  color: var(--color-text-dim);
}

.snap-restore-btn {
  width: 100%;
  padding: 6px 12px;
  background: rgba(77, 171, 247, 0.1);
  border: 1px solid rgba(77, 171, 247, 0.3);
  border-radius: 6px;
  color: var(--color-accent);
  font-size: 12px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  transition: all 0.15s;

  &:hover {
    background: rgba(77, 171, 247, 0.2);
  }
}

.collab-user-list {
  padding: 0 10px;
}

.collab-user-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  margin-bottom: 6px;
  background: var(--color-panel-2);
  border-radius: 8px;
}

.user-avatar-lg {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  flex-shrink: 0;
}

.user-info {
  flex: 1;
  min-width: 0;
}

.user-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--color-text);
}

.user-status {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;
  color: var(--color-text-dim);
  margin-top: 2px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-danger);

  &.online {
    background: var(--color-success);
    box-shadow: 0 0 6px var(--color-success);
  }
}

.user-tool {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--color-text-dim);
}
</style>
