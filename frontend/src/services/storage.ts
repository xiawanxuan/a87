import { openDB, type IDBPDatabase, type DBSchema } from 'idb';
import type { PolygonAnnotation, DraftSnapshot, CanvasTransform, ToolType } from '@/types';
import { getUserId } from './api';

interface AnnotationRecord {
  id: string;
  imageId: number;
  userId: string;
  annotations: PolygonAnnotation[];
  updatedAt: number;
}

interface AppDB extends DBSchema {
  annotations: {
    key: string;
    value: AnnotationRecord;
    indexes: { 'by-image-user': [number, string] };
  };
  drafts: {
    key: string;
    value: DraftSnapshot & { id: string };
    indexes: { 'by-image-user': [number, string] };
  };
  operations: {
    key: string;
    value: {
      id: string;
      imageId: number;
      userId: string;
      type: string;
      payload: unknown;
      createdAt: number;
    };
  };
}

const DB_NAME = 'ultrasound-annotation-db';
const DB_VERSION = 1;

let dbPromise: Promise<IDBPDatabase<AppDB>> | null = null;

function getDB(): Promise<IDBPDatabase<AppDB>> {
  if (!dbPromise) {
    dbPromise = openDB<AppDB>(DB_NAME, DB_VERSION, {
      upgrade(db) {
        if (!db.objectStoreNames.contains('annotations')) {
          const s1 = db.createObjectStore('annotations', { keyPath: 'id' });
          s1.createIndex('by-image-user', ['imageId', 'userId'], { unique: true });
        }
        if (!db.objectStoreNames.contains('drafts')) {
          const s2 = db.createObjectStore('drafts', { keyPath: 'id' });
          s2.createIndex('by-image-user', ['imageId', 'userId'], { unique: true });
        }
        if (!db.objectStoreNames.contains('operations')) {
          db.createObjectStore('operations', { keyPath: 'id' });
        }
      },
    });
  }
  return dbPromise;
}

function annKey(imageId: number, userId: string): string {
  return `${imageId}-${userId}`;
}

export const LocalDraft = {
  async saveAnnotations(imageId: number, annotations: PolygonAnnotation[], userId = getUserId()): Promise<void> {
    const db = await getDB();
    await db.put('annotations', {
      id: annKey(imageId, userId),
      imageId, userId,
      annotations,
      updatedAt: Date.now(),
    });
  },

  async loadAnnotations(imageId: number, userId = getUserId()): Promise<PolygonAnnotation[] | null> {
    const db = await getDB();
    const r = await db.get('annotations', annKey(imageId, userId));
    return r ? r.annotations : null;
  },

  async clearAnnotations(imageId: number, userId = getUserId()): Promise<void> {
    const db = await getDB();
    await db.delete('annotations', annKey(imageId, userId));
  },

  async saveDraft(
    imageId: number,
    annotations: PolygonAnnotation[],
    tool: ToolType,
    transform: CanvasTransform,
    userId = getUserId(),
  ): Promise<void> {
    const db = await getDB();
    await db.put('drafts', {
      id: annKey(imageId, userId),
      imageId, userId, annotations, tool, transform,
      createdAt: Date.now(),
    });
  },

  async loadDraft(imageId: number, userId = getUserId()): Promise<DraftSnapshot | null> {
    const db = await getDB();
    const r = await db.get('drafts', annKey(imageId, userId));
    if (!r) return null;
    const { id: _id, ...rest } = r;
    return rest;
  },
};

export function useIndexedDB() {
  return { LocalDraft, getDB };
}
