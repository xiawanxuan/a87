<script setup lang="ts">
import { ref, computed, onMounted, defineComponent, h, type PropType } from 'vue';
import type { WoodComponent, ScanImage } from '@/types';
import { useAnnotationStore } from '@/stores/annotation';
import { ScanImageApi, ComponentApi } from '@/services/api';
import { ElMessage, ElMessageBox, ElTooltip } from 'element-plus';
import {
  Folder, FolderOpened, Picture, CaretRight, Plus, Upload, Delete,
} from '@element-plus/icons-vue';

const store = useAnnotationStore();
const expanded = ref<Set<number>>(new Set());
const uploadComponentId = ref<number | null>(null);

function toggleExpand(id: number): void {
  if (expanded.value.has(id)) expanded.value.delete(id);
  else expanded.value.add(id);
}

function isExpanded(id: number): boolean {
  return expanded.value.has(id);
}

function hasChildren(node: WoodComponent): boolean {
  return !!(node.children && node.children.length > 0);
}

function onSelectComponent(id: number): void {
  store.selectComponent(id);
}

function onSelectImage(img: ScanImage): void {
  store.selectImage(img.id);
}

const selectedComponentImages = computed(() => store.componentImages);

function expandAll(nodes: WoodComponent[]): void {
  for (const n of nodes) {
    if (n.id) expanded.value.add(n.id);
    if (n.children) expandAll(n.children);
  }
}

onMounted(() => {
  expandAll(store.componentTree);
});

function openUpload(cid: number): void {
  uploadComponentId.value = cid;
  setTimeout(() => document.getElementById('file-upload-input')?.click(), 30);
}

async function onFileChange(e: Event): Promise<void> {
  const input = e.target as HTMLInputElement;
  const files = input.files;
  if (!files || !files.length || !uploadComponentId.value) return;
  try {
    for (let i = 0; i < files.length; i++) {
      const f = files[i];
      await ScanImageApi.upload(uploadComponentId.value, f);
    }
    ElMessage.success('上传成功');
    await store.selectComponent(uploadComponentId.value);
  } catch (err) {
    ElMessage.error('上传失败');
  } finally {
    input.value = '';
  }
}

async function addChild(parentId: number | null): Promise<void> {
  try {
    const { value: name } = await ElMessageBox.prompt('输入构件名称', '新增构件', {
      inputPlaceholder: '如：太和殿主梁',
      confirmButtonText: '确认',
      cancelButtonText: '取消',
    });
    if (!name) return;
    await ComponentApi.create({ parentId: parentId || undefined, name });
    await store.init();
    ElMessage.success('已创建');
  } catch {}
}

async function deleteComponent(id: number): Promise<void> {
  try {
    await ElMessageBox.confirm('确认删除该构件及其子节点？', '删除确认', { type: 'warning' });
    await ComponentApi.remove(id);
    await store.init();
    ElMessage.success('已删除');
  } catch {}
}

interface TreeNodeProps {
  node: WoodComponent;
  depth: number;
  selectedComponentId: number | null;
  currentImageId: number | null;
  images: ScanImage[];
  isExpanded: boolean;
}

const TreeNode = defineComponent({
  name: 'TreeNode',
  props: {
    node: { type: Object as PropType<WoodComponent>, required: true },
    depth: { type: Number, required: true },
    selectedComponentId: { type: Number as PropType<number | null>, default: null },
    currentImageId: { type: Number as PropType<number | null>, default: null },
    images: { type: Array as PropType<ScanImage[]>, default: () => [] },
    isExpanded: { type: Boolean, default: false },
  },
  emits: ['toggle', 'select-component', 'select-image', 'upload', 'add-child', 'delete'],
  setup(props: TreeNodeProps, { emit }) {
    return (): unknown => {
      const { node, depth, selectedComponentId, currentImageId, images, isExpanded } = props;

      const iconFolder = hasChildren(node)
        ? (isExpanded ? h(FolderOpened) : h(Folder))
        : h(Picture);

      const expandBtn = hasChildren(node)
        ? h('span', {
            class: 'expand-btn',
            onClick: (e: Event) => { e.stopPropagation(); emit('toggle', node.id); },
          }, [
            h('el-icon', { class: isExpanded ? 'rotate-90' : '' }, () => h(CaretRight)),
          ])
        : h('span', { class: 'expand-placeholder' });

      const opsButtons = h('div', { class: 'node-ops', onClick: (e: Event) => e.stopPropagation() }, [
        h(ElTooltip, { content: '上传探伤图' }, () =>
          h('el-icon', { class: 'op-btn', onClick: () => emit('upload', node.id) }, () => h(Upload))
        ),
        h(ElTooltip, { content: '新增子构件' }, () =>
          h('el-icon', { class: 'op-btn', onClick: () => emit('add-child', node.id) }, () => h(Plus))
        ),
        h(ElTooltip, { content: '删除' }, () =>
          h('el-icon', { class: 'op-btn danger', onClick: () => emit('delete', node.id) }, () => h(Delete))
        ),
      ]);

      const nodeRow = h('div', {
        class: ['node-row'],
        style: { paddingLeft: (8 + depth * 20) + 'px' },
        onClick: () => emit('select-component', node.id),
      }, [
        expandBtn,
        h('el-icon', { class: 'node-icon' }, () => iconFolder),
        h('span', { class: 'node-name', title: node.name }, node.name),
        opsButtons,
      ]);

      const imageItems = (selectedComponentId === node.id && images.length > 0)
        ? images.map((img) => h('div', {
            class: ['image-item', { active: currentImageId === img.id }],
            style: { paddingLeft: (32 + depth * 20) + 'px' },
            onClick: (e: Event) => { e.stopPropagation(); emit('select-image', img); },
          }, [
            h('el-icon', () => h(Picture)),
            h('span', { class: 'img-name', title: img.fileName }, img.fileName),
            h('span', { class: 'img-size' }, `${img.width}×${img.height}`),
          ]))
        : null;

      const children: unknown = hasChildren(node) && isExpanded
        ? (node.children || []).map((child) =>
            h(TreeNode as any, {
              key: child.id,
              node: child,
              depth: depth + 1,
              selectedComponentId,
              currentImageId,
              images: selectedComponentId === child.id ? images : [],
              isExpanded: expanded.value.has(child.id || 0),
              onToggle: (id: number) => emit('toggle', id),
              onSelectComponent: (id: number) => emit('select-component', id),
              onSelectImage: (img: ScanImage) => emit('select-image', img),
              onUpload: (id: number) => emit('upload', id),
              onAddChild: (id: number) => emit('add-child', id),
              onDelete: (id: number) => emit('delete', id),
            })
          )
        : null;

      return h('div', {
        class: ['tree-node', { active: selectedComponentId === node.id }],
      }, [nodeRow, imageItems, children as any]);
    };
  },
});

defineOptions({ name: 'ComponentTree' });
</script>

<template>
  <div class="tree-wrap">
    <input
      id="file-upload-input"
      type="file"
      accept="image/*"
      multiple
      style="display:none"
      @change="onFileChange"
    />
    <div class="tree-actions">
      <el-button size="small" @click="addChild(null)">
        <el-icon><Plus /></el-icon>新建根节点
      </el-button>
    </div>
    <div class="tree-list">
      <TreeNode
        v-for="node in store.componentTree"
        :key="node.id"
        :node="node"
        :depth="0"
        :selected-component-id="store.selectedComponentId"
        :current-image-id="store.currentImage?.id || null"
        :images="store.selectedComponentId === node.id ? selectedComponentImages : []"
        :is-expanded="isExpanded(node.id!)"
        @toggle="toggleExpand"
        @select-component="onSelectComponent"
        @select-image="onSelectImage"
        @upload="openUpload"
        @add-child="addChild"
        @delete="deleteComponent"
      />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.tree-wrap { flex: 1; display: flex; flex-direction: column; min-height: 0; }
.tree-actions { padding: 8px; border-bottom: 1px solid var(--color-border); }
.tree-list { flex: 1; overflow-y: auto; padding: 4px 0; }
.tree-node { position: relative; }

:deep(.tree-node) {
  .node-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 8px;
    cursor: pointer;
    border-radius: 4px;
    margin: 2px 4px;
    transition: background 0.15s;
    &:hover { background: rgba(77, 171, 247, 0.12); }
  }
  &.active > .node-row {
    background: rgba(77, 171, 247, 0.2);
    color: var(--color-accent);
  }
}

:deep(.expand-btn) {
  width: 18px; height: 18px;
  display: flex; align-items: center; justify-content: center;
  color: var(--color-text-dim);
  transition: transform 0.15s;
  &:hover { color: var(--color-accent); }
  .rotate-90 { transform: rotate(90deg); }
}

:deep(.expand-placeholder) {
  width: 18px; display: inline-block;
}

:deep(.node-icon) {
  color: var(--color-text-dim);
}

:deep(.node-name) {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
}

:deep(.node-ops) {
  display: none;
  gap: 4px;
  opacity: 0;
  transition: opacity 0.15s;
}

:deep(.node-row:hover) {
  .node-ops { display: flex; opacity: 1; }
}

:deep(.op-btn) {
  padding: 3px;
  border-radius: 3px;
  color: var(--color-text-dim);
  &:hover { background: rgba(255,255,255,0.1); color: var(--color-accent); }
  &.danger:hover { color: var(--color-danger); }
}

:deep(.image-item) {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px 5px 42px;
  margin: 1px 4px;
  font-size: 12px;
  cursor: pointer;
  border-radius: 4px;
  color: var(--color-text-dim);
  &:hover { background: rgba(77, 171, 247, 0.08); color: var(--color-text); }
  &.active { background: rgba(77, 171, 247, 0.18); color: var(--color-accent); }
  .img-name { flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .img-size { color: var(--color-text-dim); font-size: 11px; flex-shrink: 0; }
}
</style>
