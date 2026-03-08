import {
  createContext,
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAuth } from "@/features/auth";
import { getAccessToken } from "@/features/auth/token";
import { getConfig } from "@/config";
import { chatSocket, type ChatMessage, type ChatListener } from "./ChatSocket";

export interface ChatContextValue {
  messages: ChatMessage[];
  connected: boolean;
  isStaff: boolean;
  /** Whether the full-size shoutbox (home page) is currently mounted */
  mainChatVisible: boolean;
  setMainChatVisible: (visible: boolean) => void;
  sendMessage: (text: string) => void;
  deleteMessage: (id: number) => Promise<void>;
  loadMore: () => Promise<void>;
}

export const ChatContext = createContext<ChatContextValue | null>(null);

export function ChatProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [connected, setConnected] = useState(false);
  const [mainChatVisible, setMainChatVisible] = useState(false);
  const loadingMoreRef = useRef(false);

  useEffect(() => {
    if (!isAuthenticated) {
      chatSocket.disconnect();
      return;
    }

    // Register listener BEFORE connect — if the WS connects instantly
    // (cached/fast network), onopen can fire before the next line runs.
    const onEvent: ChatListener = (event) => {
      switch (event.type) {
        case "connected":
          setConnected(true);
          break;
        case "disconnected":
          setConnected(false);
          break;
        case "backfill":
          setMessages(event.messages);
          break;
        case "message":
          setMessages((prev) => [...prev, event.message]);
          break;
        case "delete":
          setMessages((prev) => prev.filter((m) => m.id !== event.id));
          break;
      }
    };

    chatSocket.addListener(onEvent);
    chatSocket.connect();
    return () => {
      chatSocket.removeListener(onEvent);
    };
  }, [isAuthenticated]);

  const sendMessage = useCallback((text: string) => {
    const trimmed = text.trim();
    if (trimmed) chatSocket.send(trimmed);
  }, []);

  const deleteMessage = useCallback(async (id: number) => {
    const token = getAccessToken();
    if (!token) return;
    try {
      await fetch(`${getConfig().API_URL}/api/v1/chat/${id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
    } catch {
      // ignore
    }
  }, []);

  const loadMore = useCallback(async () => {
    if (loadingMoreRef.current) return;
    if (messages.length === 0) return;

    const oldestId = messages[0].id;
    const token = getAccessToken();
    if (!token) return;

    loadingMoreRef.current = true;
    try {
      const resp = await fetch(
        `${getConfig().API_URL}/api/v1/chat/history?before_id=${oldestId}&limit=50`,
        { headers: { Authorization: `Bearer ${token}` } },
      );
      if (!resp.ok) return;
      const data = await resp.json();
      if (data?.messages?.length > 0) {
        setMessages((prev) => [...data.messages, ...prev]);
      }
    } catch {
      // ignore
    } finally {
      loadingMoreRef.current = false;
    }
  }, [messages]);

  return (
    <ChatContext.Provider
      value={{
        messages,
        connected,
        isStaff: user?.isStaff ?? false,
        mainChatVisible,
        setMainChatVisible,
        sendMessage,
        deleteMessage,
        loadMore,
      }}
    >
      {children}
    </ChatContext.Provider>
  );
}
