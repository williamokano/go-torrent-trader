import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";

export interface ChatMessage {
  id: number;
  user_id: number;
  username: string;
  message: string;
  created_at: string;
}

type WSMessage =
  | { type: "backfill"; messages: ChatMessage[] }
  | ({ type: "message" } & ChatMessage)
  | { type: "delete"; id: number }
  | { type: "delete_user"; user_id: number }
  | { type: "error"; message: string };

export type ChatListener = (
  event:
    | { type: "connected" }
    | { type: "disconnected" }
    | { type: "backfill"; messages: ChatMessage[] }
    | { type: "message"; message: ChatMessage }
    | { type: "delete"; id: number }
    | { type: "delete_user"; user_id: number }
    | { type: "error"; message: string },
) => void;

function getWebSocketURL(): string {
  return getConfig().API_URL.replace(/^http/, "ws") + "/ws/chat";
}

/**
 * Singleton WebSocket manager for the chat shoutbox.
 * Lives outside React — not affected by Strict Mode double-mounting.
 * Components subscribe via addListener/removeListener.
 */
class ChatSocket {
  private ws: WebSocket | null = null;
  private listeners = new Set<ChatListener>();
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private reconnectDelay = 1000;
  private shouldReconnect = false;

  get isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  addListener(listener: ChatListener): void {
    this.listeners.add(listener);
  }

  removeListener(listener: ChatListener): void {
    this.listeners.delete(listener);
  }

  private emit(event: Parameters<ChatListener>[0]): void {
    for (const listener of this.listeners) {
      listener(event);
    }
  }

  connect(): void {
    // Already connected or connecting
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    const token = getAccessToken();
    if (!token) return;

    this.shouldReconnect = true;
    const url = `${getWebSocketURL()}?token=${encodeURIComponent(token)}`;
    const ws = new WebSocket(url);
    this.ws = ws;

    ws.onopen = () => {
      if (this.ws !== ws) return;
      this.reconnectDelay = 1000;
      this.emit({ type: "connected" });
    };

    ws.onmessage = (event) => {
      if (this.ws !== ws) return;
      try {
        const data = JSON.parse(event.data as string) as WSMessage;
        switch (data.type) {
          case "backfill":
            this.emit({ type: "backfill", messages: data.messages });
            break;
          case "message":
            this.emit({
              type: "message",
              message: {
                id: data.id,
                user_id: data.user_id,
                username: data.username,
                message: data.message,
                created_at: data.created_at,
              },
            });
            break;
          case "delete":
            this.emit({ type: "delete", id: data.id });
            break;
          case "delete_user":
            this.emit({ type: "delete_user", user_id: data.user_id });
            break;
          case "error":
            this.emit({ type: "error", message: data.message });
            break;
        }
      } catch {
        // Ignore malformed messages.
      }
    };

    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      this.emit({ type: "disconnected" });

      if (this.shouldReconnect) {
        const delay = this.reconnectDelay;
        this.reconnectTimeout = setTimeout(() => {
          this.reconnectDelay = Math.min(delay * 2, 30000);
          this.connect();
        }, delay);
      }
    };

    ws.onerror = () => {
      // onclose will fire after onerror.
    };
  }

  disconnect(): void {
    this.shouldReconnect = false;
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  send(text: string): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return;
    this.ws.send(JSON.stringify({ type: "message", text }));
  }
}

// Module-level singleton — one connection per browser tab
export const chatSocket = new ChatSocket();
