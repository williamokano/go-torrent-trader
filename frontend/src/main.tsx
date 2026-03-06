import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@/index.css";
import { ThemeProvider } from "@/themes";
import { AuthProvider } from "@/features/auth";
import App from "@/App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <AuthProvider>
        <App />
      </AuthProvider>
    </ThemeProvider>
  </StrictMode>,
);
