import { ref, shallowRef, type ShallowRef } from 'vue';
import mitt from 'mitt';
import type { OperationMessage } from '@/types';
import { getUserId, getUserName } from './api';

export type WsEventMap = {
  open: Event;
  close: CloseEvent;
  error: Event;
  message: OperationMessage;
  [k: string]: unknown;
};

export class AnnotationWebSocket {
  private ws: WebSocket | null = null;
  private imageId: number = 0;
  private manualClose = false;
  private reconnectTimer: number | null = null;
  private seq = 0;
  private reconnectAttempts = 0;
  private maxReconnect = 20;

  readonly connected = ref(false);
  readonly events = mitt<WsEventMap>();
  readonly lastMessage: ShallowRef<OperationMessage | null> = shallowRef(null);

  connect(imageId: number): void {
    this.imageId = imageId;
    this.disconnect();
    this.manualClose = false;
    this.reconnectAttempts = 0;
    this.openConnection();
  }

  private openConnection(): void {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/ws/scan-images/${this.imageId}?userId=${encodeURIComponent(getUserId())}&userName=${encodeURIComponent(getUserName())}`;
    this.ws = new WebSocket(url);
    this.ws.binaryType = 'arraybuffer';

    this.ws.onopen = (e) => {
      this.connected.value = true;
      this.reconnectAttempts = 0;
      this.events.emit('open', e);
      this.startHeartbeat();
    };
    this.ws.onclose = (e) => {
      this.connected.value = false;
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
        const msg: OperationMessage = typeof e.data === 'string'
          ? JSON.parse(e.data)
          : JSON.parse(new TextDecoder().decode(e.data as ArrayBuffer));
        this.lastMessage.value = msg;
        this.events.emit('message', msg);
        if (msg.type && msg.type !== 'message') {
          this.events.emit(msg.type as string, msg.payload as never);
        }
      } catch (err) {
        console.warn('[ws] parse fail', err);
      }
    };
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    this.reconnectAttempts++;
    const delay = Math.min(1000 * Math.pow(2, Math.min(this.reconnectAttempts, 6)), 30000);
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
        this.sendRaw({ type: 'ping' });
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
    return this.sendRaw({ type, payload, seq }) ? seq : -1;
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
  }

  isConnected(): boolean {
    return this.connected.value;
  }
}

export const globalWs = new AnnotationWebSocket();
