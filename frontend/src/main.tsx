import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@/index.css";
import { ThemeProvider } from "@/themes";
import { AuthProvider } from "@/features/auth";
import { ToastProvider } from "@/components/toast";
import { ChatProvider } from "@/lib/ChatProvider";
import App from "@/App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <ToastProvider>
        <AuthProvider>
          <ChatProvider>
            <App />
          </ChatProvider>
        </AuthProvider>
      </ToastProvider>
    </ThemeProvider>
  </StrictMode>,
);
