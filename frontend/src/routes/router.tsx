import { createBrowserRouter } from "react-router-dom";
import { RootLayout } from "@/layouts/RootLayout";
import { ProtectedRoute } from "@/routes/ProtectedRoute";
import { HomePage } from "@/pages/HomePage";
import { BrowsePage } from "@/pages/BrowsePage";
import { UploadPage } from "@/pages/UploadPage";
import { TorrentDetailPage } from "@/pages/TorrentDetailPage";
import { LoginPage } from "@/pages/LoginPage";
import { SignupPage } from "@/pages/SignupPage";
import { NotFoundPage } from "@/pages/NotFoundPage";

export const router = createBrowserRouter([
  {
    path: "/",
    element: <RootLayout />,
    errorElement: <NotFoundPage />,
    children: [
      {
        index: true,
        element: (
          <ProtectedRoute>
            <HomePage />
          </ProtectedRoute>
        ),
      },
      {
        path: "browse",
        element: (
          <ProtectedRoute>
            <BrowsePage />
          </ProtectedRoute>
        ),
      },
      {
        path: "upload",
        element: (
          <ProtectedRoute>
            <UploadPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "torrent/:id",
        element: (
          <ProtectedRoute>
            <TorrentDetailPage />
          </ProtectedRoute>
        ),
      },
      { path: "login", element: <LoginPage /> },
      { path: "signup", element: <SignupPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
]);
