import { ref, shallowRef, type ShallowRef } from 'vue';
import mitt from 'mitt';
import type { OperationMessage } from '@/types';
import { getUserId, getUserName } from './api';

export type WsEventMap = {
  open: Event;
  close: CloseEvent;
  error: Event;
  message: OperationMessage;
  sync: SyncPayload;
  gap: SeqGapPayload;
  stats: WsStats;
  [k: string]: unknown;
};

export interface WsEnvelope {
  seq: number;
  type: string;
  timestamp: number;
  userId?: string;
  userName?: string;
  payload: unknown;
}

export interface SyncPayload {
  startSeq: number;
  endSeq: number;
  messageCount: number;
  clientJoinedAt: number;
}

export interface SeqGapPayload {
  expected: number;
  received: number;
  missing: number[];
}

export interface WsStats {
  connected: boolean;
  lastSeq: number;
  recvCount: number;
  dropCount: number;
  duplicateCount: number;
  gapCount: number;
  ackSent: number;
  retryCount: number;
  latencyMs: number;
}

export class AnnotationWebSocket {
  private ws: WebSocket | null = null;
  private imageId: number = 0;
  private manualClose = false;
  private reconnectTimer: number | null = null;
  private seq = 0;
  private reconnectAttempts = 0;
  private maxReconnect = 20;

  private lastProcessedSeq = 0;
  private highestSeenSeq = 0;
  private seenSeqs = new Set<number>();
  private pendingQueue = new Map<number, WsEnvelope>();
  private recvCount = 0;
  private dropCount = 0;
  private duplicateCount = 0;
  private gapCount = 0;
  private ackSent = 0;

  private lastPongAt = 0;
  private lastPingAt = 0;
  private latencyMs = 0;

  private readonly WINDOW_SIZE = 1000;

  readonly connected = ref(false);
  readonly events = mitt<WsEventMap>();
  readonly lastMessage: ShallowRef<OperationMessage | null> = shallowRef(null);
  readonly stats = ref<WsStats>({
    connected: false,
    lastSeq: 0,
    recvCount: 0,
    dropCount: 0,
    duplicateCount: 0,
    gapCount: 0,
    ackSent: 0,
    retryCount: 0,
    latencyMs: 0,
  });

  connect(imageId: number): void {
    this.imageId = imageId;
    this.disconnect();
    this.manualClose = false;
    this.reconnectAttempts = 0;
    this.openConnection();
  }

  private openConnection(): void {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/ws/scan-images/${this.imageId}?userId=${encodeURIComponent(getUserId())}&userName=${encodeURIComponent(getUserName())}&lastSeq=${this.lastProcessedSeq}`;
    this.ws = new WebSocket(url);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onopen = (e) => {
      this.connected.value = true;
      this.reconnectAttempts = 0;
      this.events.emit('open', e);
      this.startHeartbeat();
      this.emitStats();
    };
    this.ws.onclose = (e) => {
      this.connected.value = false;
      this.emitStats();
      this.events.emit('close', e);
      if (!this.manualClose && this.reconnectAttempts < this.maxReconnect) {
        this.scheduleReconnect();
      }
      this.stopHeartbeat();
    };
    this.ws.onerror = (e) => {
      this.events.emit('error', e);
    };
    this.ws.onmessage = (e) => {
      try {
        const env: WsEnvelope = typeof e.data === 'string'
          ? JSON.parse(e.data)
          : JSON.parse(new TextDecoder().decode(e.data as ArrayBuffer));
        this.handleEnvelope(env);
      } catch (err) {
        console.warn('[ws] parse fail', err);
      }
    };
  }

  private handleEnvelope(env: WsEnvelope): void {
    if (env.type === 'pong') {
      this.lastPongAt = Date.now();
      if (this.lastPingAt > 0) {
        this.latencyMs = this.lastPongAt - this.lastPingAt;
      }
      return;
    }

    if (env.type === 'sync') {
      const payload = env.payload as SyncPayload;
      this.events.emit('sync', payload);
      this.lastProcessedSeq = payload.endSeq;
      console.info(`[ws] sync: server seq range ${payload.startSeq}->${payload.endSeq}, replaying ${payload.messageCount} messages`);
      return;
    }

    const seq = env.seq;
    if (seq > 0) {
      if (this.seenSeqs.has(seq)) {
        this.duplicateCount++;
        this.sendAck(seq);
        this.emitStats();
        return;
      }

      if (seq > this.highestSeenSeq) {
        this.highestSeenSeq = seq;
      }

      this.seenSeqs.add(seq);
      if (this.seenSeqs.size > this.WINDOW_SIZE * 2) {
        const threshold = this.highestSeenSeq - this.WINDOW_SIZE;
        for (const s of Array.from(this.seenSeqs)) {
          if (s < threshold) this.seenSeqs.delete(s);
        }
        for (const s of Array.from(this.pendingQueue.keys())) {
          if (s < threshold) this.pendingQueue.delete(s);
        }
      }

      const expected = this.lastProcessedSeq + 1;
      if (seq > expected) {
        const missing: number[] = [];
        for (let s = expected; s < seq; s++) {
          if (!this.seenSeqs.has(s)) {
            missing.push(s);
          }
        }
        if (missing.length > 0) {
          this.gapCount++;
          this.dropCount += missing.length;
          this.pendingQueue.set(seq, env);
          this.events.emit('gap', { expected, received: seq, missing });
          console.warn(`[ws] seq gap: expected ${expected}, got ${seq}, missing ${missing.length} msgs, requesting resync`);
          this.sendRaw({ type: 'resync', lastProcessedSeq: this.lastProcessedSeq });
          this.emitStats();
          return;
        }
      }

      if (seq <= this.lastProcessedSeq) {
        this.duplicateCount++;
        this.sendAck(seq);
        this.emitStats();
        return;
      }

      this.lastProcessedSeq = seq;
      this.sendAck(seq);

      this.processPending();
    }

    this.recvCount++;
    this.emitStats();

    const msg = env as unknown as OperationMessage;
    this.lastMessage.value = msg;
    this.events.emit('message', msg);
    if (env.type && env.type !== 'message') {
      this.events.emit(env.type, env.payload as never);
    }
  }

  private processPending(): void {
    let processed = 0;
    while (true) {
      const nextSeq = this.lastProcessedSeq + 1;
      const next = this.pendingQueue.get(nextSeq);
      if (!next) break;

      this.pendingQueue.delete(nextSeq);
      this.lastProcessedSeq = nextSeq;
      this.sendAck(nextSeq);

      const msg = next as unknown as OperationMessage;
      this.lastMessage.value = msg;
      this.events.emit('message', msg);
      if (next.type && next.type !== 'message') {
        this.events.emit(next.type, next.payload as never);
      }

      this.recvCount++;
      processed++;
      if (processed > 100) {
        setTimeout(() => this.processPending(), 0);
        break;
      }
    }
    if (processed > 0) {
      this.emitStats();
    }
  }

  private sendAck(seq: number): void {
    this.ackSent++;
    this.sendRaw({ type: 'ack', seq });
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    this.reconnectAttempts++;
    const delay = Math.min(1000 * Math.pow(2, Math.min(this.reconnectAttempts, 6)), 30000);
    console.info(`[ws] reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      this.openConnection();
    }, delay);
  }

  private heartbeatTimer: number | null = null;
  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = window.setInterval(() => {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.lastPingAt = Date.now();
        this.sendRaw({ type: 'ping', lastProcessedSeq: this.lastProcessedSeq });
      }
    }, 25000);
  }
  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  send(type: string, payload: unknown): number {
    const seq = ++this.seq;
    return this.sendRaw({ type, payload, seq, lastProcessedSeq: this.lastProcessedSeq }) ? seq : -1;
  }

  broadcast(type: string, payload: unknown): number {
    return this.send('broadcast', { type, payload, userId: getUserId(), userName: getUserName() });
  }

  private sendRaw(obj: unknown): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    try {
      this.ws.send(JSON.stringify(obj));
      return true;
    } catch (e) {
      console.warn('[ws] send fail', e);
      return false;
    }
  }

  sendCursor(x: number, y: number): void {
    this.send('cursor', { x, y, imageId: this.imageId });
  }

  requestResync(): void {
    this.sendRaw({ type: 'resync', lastProcessedSeq: this.lastProcessedSeq });
  }

  disconnect(): void {
    this.manualClose = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopHeartbeat();
    if (this.ws) {
      try {
        this.ws.close(1000, 'client disconnect');
      } catch {}
      this.ws = null;
    }
    this.connected.value = false;
    this.emitStats();
  }

  isConnected(): boolean {
    return this.connected.value;
  }

  getLastProcessedSeq(): number {
    return this.lastProcessedSeq;
  }

  private emitStats(): void {
    this.stats.value = {
      connected: this.connected.value,
      lastSeq: this.lastProcessedSeq,
      recvCount: this.recvCount,
      dropCount: this.dropCount,
      duplicateCount: this.duplicateCount,
      gapCount: this.gapCount,
      ackSent: this.ackSent,
      retryCount: 0,
      latencyMs: this.latencyMs,
    };
    this.events.emit('stats', this.stats.value);
  }

  resetCounters(): void {
    this.recvCount = 0;
    this.dropCount = 0;
    this.duplicateCount = 0;
    this.gapCount = 0;
    this.ackSent = 0;
    this.lastProcessedSeq = 0;
    this.highestSeenSeq = 0;
    this.seenSeqs.clear();
    this.pendingQueue.clear();
    this.emitStats();
  }
}

export const globalWs = new AnnotationWebSocket();
