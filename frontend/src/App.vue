<script setup lang="ts">
import { onMounted, onUnmounted, watch } from 'vue';
import { useAnnotationStore } from '@/stores/annotation';
import ComponentTree from '@/components/tree/ComponentTree.vue';
import AnnotationCanvas from '@/components/canvas/AnnotationCanvas.vue';
import TopToolbar from '@/components/toolbar/TopToolbar.vue';
import RightPanel from '@/components/panels/RightPanel.vue';

const store = useAnnotationStore();

onMounted(async () => {
  await store.init();
  if (store.componentTree.length > 0) {
    const first = store.componentTree[0];
    if (first.id) store.selectComponent(first.id);
  }
  window.addEventListener('beforeunload', onLeave);
});
onUnmounted(() => {
  window.removeEventListener('beforeunload', onLeave);
});

function onLeave(): void {
  store.saveLocalDraft().catch(() => {});
}

let saveTimer: number | null = null;
watch(
  () => [store.annotations.length, store.currentImage?.id],
  () => {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = window.setTimeout(() => store.saveLocalDraft().catch(() => {}), 1500);
  },
  { deep: true },
);
</script>

<template>
  <div class="app-root">
    <TopToolbar />
    <div class="app-body">
      <aside class="left-panel">
        <div class="panel-title">木构件目录</div>
        <ComponentTree />
      </aside>
      <main class="canvas-area">
        <AnnotationCanvas />
      </main>
      <aside class="right-panel">
        <RightPanel />
      </aside>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.app-root {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--color-bg);
}
.app-body {
  flex: 1;
  display: flex;
  min-height: 0;
}
.left-panel {
  width: var(--sidebar-width);
  background: var(--color-panel);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.canvas-area {
  flex: 1;
  position: relative;
  background: #0e0e1a;
  overflow: hidden;
  min-height: 0;
}
.right-panel {
  width: var(--right-panel-width);
  background: var(--color-panel);
  border-left: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  min-height: 0;
}
</style>
