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
import { AdminSettingsPage } from "@/pages/admin/AdminSettingsPage";
import { AdminBansPage } from "@/pages/admin/AdminBansPage";
import { AdminWarningsPage } from "@/pages/admin/AdminWarningsPage";
import { AdminCategoriesPage } from "@/pages/admin/AdminCategoriesPage";
import { RSSBuilderPage } from "@/pages/RSSBuilderPage";
import { MembersPage } from "@/pages/MembersPage";
import { StaffPage } from "@/pages/StaffPage";
import { InvitesPage } from "@/pages/InvitesPage";
import { MessagesPage } from "@/pages/MessagesPage";
import { ActivityLogPage } from "@/pages/ActivityLogPage";
import { TodaysTorrentsPage } from "@/pages/TodaysTorrentsPage";
import { NeedSeedPage } from "@/pages/NeedSeedPage";
import { TorrentPeersPage } from "@/pages/TorrentPeersPage";
import { ConfirmEmailPage } from "@/pages/ConfirmEmailPage";
import { ResendConfirmationPage } from "@/pages/ResendConfirmationPage";
import { CheckEmailPage } from "@/pages/CheckEmailPage";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { FAQPage } from "@/pages/FAQPage";
import { RulesPage } from "@/pages/RulesPage";
import { FormattingPage } from "@/pages/FormattingPage";

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
        path: "today",
        element: (
          <ProtectedRoute>
            <TodaysTorrentsPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "needseed",
        element: (
          <ProtectedRoute>
            <NeedSeedPage />
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
        path: "torrent/:id/peers",
        element: (
          <ProtectedRoute>
            <TorrentPeersPage />
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
          { path: "categories", element: <AdminCategoriesPage /> },
          { path: "groups", element: <AdminGroupsPage /> },
          { path: "warnings", element: <AdminWarningsPage /> },
          { path: "settings", element: <AdminSettingsPage /> },
          { path: "bans", element: <AdminBansPage /> },
        ],
      },
      {
        path: "members",
        element: (
          <ProtectedRoute>
            <MembersPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "staff",
        element: (
          <ProtectedRoute>
            <StaffPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "messages",
        element: (
          <ProtectedRoute>
            <MessagesPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "invites",
        element: (
          <ProtectedRoute>
            <InvitesPage />
          </ProtectedRoute>
        ),
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
      { path: "confirm-email", element: <ConfirmEmailPage /> },
      { path: "resend-confirmation", element: <ResendConfirmationPage /> },
      { path: "check-email", element: <CheckEmailPage /> },
      { path: "faq", element: <FAQPage /> },
      { path: "rules", element: <RulesPage /> },
      { path: "formatting", element: <FormattingPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
]);
