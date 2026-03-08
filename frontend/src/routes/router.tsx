import { createBrowserRouter, Navigate } from "react-router-dom";
import { RootLayout } from "@/layouts/RootLayout";
import { AdminLayout } from "@/layouts/AdminLayout";
import { ProtectedRoute } from "@/routes/ProtectedRoute";
import { AdminRoute } from "@/routes/AdminRoute";
import { HomePage } from "@/pages/HomePage";
import { BrowsePage } from "@/pages/BrowsePage";
import { UploadPage } from "@/pages/UploadPage";
import { TorrentDetailPage } from "@/pages/TorrentDetailPage";
import { TorrentEditPage } from "@/pages/TorrentEditPage";
import { LoginPage } from "@/pages/LoginPage";
import { SignupPage } from "@/pages/SignupPage";
import { ForgotPasswordPage } from "@/pages/ForgotPasswordPage";
import { ResetPasswordPage } from "@/pages/ResetPasswordPage";
import { UserProfilePage } from "@/pages/UserProfilePage";
import { UserSettingsPage } from "@/pages/UserSettingsPage";
import { AdminUsersPage } from "@/pages/admin/AdminUsersPage";
import { AdminReportsPage } from "@/pages/admin/AdminReportsPage";
import { AdminGroupsPage } from "@/pages/admin/AdminGroupsPage";
import { RSSBuilderPage } from "@/pages/RSSBuilderPage";
import { ActivityLogPage } from "@/pages/ActivityLogPage";
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
      {
        path: "rss",
        element: (
          <ProtectedRoute>
            <RSSBuilderPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "torrent/:id/edit",
        element: (
          <ProtectedRoute>
            <TorrentEditPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "user/:id",
        element: (
          <ProtectedRoute>
            <UserProfilePage />
          </ProtectedRoute>
        ),
      },
      {
        path: "settings",
        element: (
          <ProtectedRoute>
            <UserSettingsPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "admin",
        element: (
          <AdminRoute>
            <AdminLayout />
          </AdminRoute>
        ),
        children: [
          { index: true, element: <Navigate to="users" replace /> },
          { path: "users", element: <AdminUsersPage /> },
          { path: "reports", element: <AdminReportsPage /> },
          { path: "groups", element: <AdminGroupsPage /> },
        ],
      },
      {
        path: "log",
        element: (
          <ProtectedRoute>
            <ActivityLogPage />
          </ProtectedRoute>
        ),
      },
      { path: "login", element: <LoginPage /> },
      { path: "signup", element: <SignupPage /> },
      { path: "forgot-password", element: <ForgotPasswordPage /> },
      { path: "reset-password", element: <ResetPasswordPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
]);
