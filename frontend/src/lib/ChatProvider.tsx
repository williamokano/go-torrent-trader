import {
  createContext,
  useCallback,
  useEffect,
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

  useEffect(() => {
    if (!isAuthenticated) {
      chatSocket.disconnect();
      return;
    }

    chatSocket.connect();

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
    setMessages((currentMessages) => {
      if (currentMessages.length === 0) return currentMessages;
      const oldestId = currentMessages[0].id;
      const token = getAccessToken();
      if (!token) return currentMessages;

      // Fire async load, update state when done
      fetch(
        `${getConfig().API_URL}/api/v1/chat/history?before_id=${oldestId}&limit=50`,
        { headers: { Authorization: `Bearer ${token}` } },
      )
        .then((r) => (r.ok ? r.json() : null))
        .then((data) => {
          if (data?.messages?.length > 0) {
            setMessages((prev) => [...data.messages, ...prev]);
          }
        })
        .catch(() => {});

      return currentMessages; // return unchanged for now
    });
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
        loadMore,
      }}
    >
      {children}
    </ChatContext.Provider>
  );
}
