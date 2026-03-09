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
import { useToast } from "@/components/toast";
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
  deleteUserMessages: (userId: number) => Promise<void>;
  muteUser: (
    userId: number,
    durationMinutes: number,
    reason: string,
  ) => Promise<void>;
  unmuteUser: (userId: number) => Promise<void>;
  loadMore: () => Promise<void>;
}

export const ChatContext = createContext<ChatContextValue | null>(null);

export function ChatProvider({ children }: { children: ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const toast = useToast();
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [connected, setConnected] = useState(false);
  const [mainChatVisible, setMainChatVisible] = useState(false);
  const loadingMoreRef = useRef(false);
  const messagesRef = useRef(messages);
  messagesRef.current = messages;

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
        case "delete_user":
          setMessages((prev) =>
            prev.filter((m) => m.user_id !== event.user_id),
          );
          break;
        case "error":
          toast.error(event.message);
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

  const deleteMessage = useCallback(
    async (id: number) => {
      const token = getAccessToken();
      if (!token) return;
      try {
        const resp = await fetch(
          `${getConfig().API_URL}/api/v1/admin/chat/messages/${id}`,
          {
            method: "DELETE",
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!resp.ok) {
          toast.error("Failed to delete message");
        }
      } catch (err) {
        console.error("deleteMessage failed:", err);
        toast.error("Failed to delete message");
      }
    },
    [toast],
  );

  const deleteUserMessages = useCallback(
    async (userId: number) => {
      const token = getAccessToken();
      if (!token) return;
      try {
        const resp = await fetch(
          `${getConfig().API_URL}/api/v1/admin/chat/users/${userId}/messages`,
          {
            method: "DELETE",
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!resp.ok) {
          toast.error("Failed to delete user messages");
        }
      } catch (err) {
        console.error("deleteUserMessages failed:", err);
        toast.error("Failed to delete user messages");
      }
    },
    [toast],
  );

  const muteUser = useCallback(
    async (userId: number, durationMinutes: number, reason: string) => {
      const token = getAccessToken();
      if (!token) return;
      try {
        const resp = await fetch(
          `${getConfig().API_URL}/api/v1/admin/chat/users/${userId}/mute`,
          {
            method: "POST",
            headers: {
              Authorization: `Bearer ${token}`,
              "Content-Type": "application/json",
            },
            body: JSON.stringify({ duration_minutes: durationMinutes, reason }),
          },
        );
        if (!resp.ok) {
          toast.error("Failed to mute user");
        }
      } catch (err) {
        console.error("muteUser failed:", err);
        toast.error("Failed to mute user");
      }
    },
    [toast],
  );

  const unmuteUser = useCallback(
    async (userId: number) => {
      const token = getAccessToken();
      if (!token) return;
      try {
        const resp = await fetch(
          `${getConfig().API_URL}/api/v1/admin/chat/users/${userId}/mute`,
          {
            method: "DELETE",
            headers: { Authorization: `Bearer ${token}` },
          },
        );
        if (!resp.ok) {
          toast.error("Failed to unmute user");
        }
      } catch (err) {
        console.error("unmuteUser failed:", err);
        toast.error("Failed to unmute user");
      }
    },
    [toast],
  );

  const loadMore = useCallback(async () => {
    if (loadingMoreRef.current) return;
    if (messagesRef.current.length === 0) return;

    const oldestId = messagesRef.current[0].id;
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
  }, []);

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
        deleteUserMessages,
        muteUser,
        unmuteUser,
        loadMore,
      }}
    >
      {children}
    </ChatContext.Provider>
  );
}
