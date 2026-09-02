import { createBrowserRouter } from "react-router-dom";
import { RootLayout } from "@/layouts/RootLayout";
import { AdminLayout } from "@/layouts/AdminLayout";
import { ProtectedRoute } from "@/routes/ProtectedRoute";
import { AdminRoute, AdminIndexRedirect } from "@/routes/AdminRoute";
import { HomePage } from "@/pages/HomePage";
import { BrowsePage } from "@/pages/BrowsePage";
import { UploadPage } from "@/pages/UploadPage";
import { MetadataIssuesPage } from "@/pages/MetadataIssuesPage";
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
import { AdminModerationPage } from "@/pages/admin/AdminModerationPage";
import { AdminGroupsPage } from "@/pages/admin/AdminGroupsPage";
import { AdminPromotionPage } from "@/pages/admin/AdminPromotionPage";
import { AdminHitAndRunPage } from "@/pages/admin/AdminHitAndRunPage";
import { AdminInviteDistributionPage } from "@/pages/admin/AdminInviteDistributionPage";
import { AdminSettingsPage } from "@/pages/admin/AdminSettingsPage";
import { AdminBansPage } from "@/pages/admin/AdminBansPage";
import { AdminWarningsPage } from "@/pages/admin/AdminWarningsPage";
import { AdminCategoriesPage } from "@/pages/admin/AdminCategoriesPage";
import { AdminCategoryEditPage } from "@/pages/admin/AdminCategoryEditPage";
import { RSSBuilderPage } from "@/pages/RSSBuilderPage";
import { MembersPage } from "@/pages/MembersPage";
import { StaffPage } from "@/pages/StaffPage";
import { InvitesPage } from "@/pages/InvitesPage";
import { BonusStorePage } from "@/pages/BonusStorePage";
import { MessagesPage } from "@/pages/MessagesPage";
import { ActivityLogPage } from "@/pages/ActivityLogPage";
import { TodaysTorrentsPage } from "@/pages/TodaysTorrentsPage";
import { NeedSeedPage } from "@/pages/NeedSeedPage";
import { CompletedPage } from "@/pages/CompletedPage";
import { TorrentPeersPage } from "@/pages/TorrentPeersPage";
import { ConfirmEmailPage } from "@/pages/ConfirmEmailPage";
import { ResendConfirmationPage } from "@/pages/ResendConfirmationPage";
import { CheckEmailPage } from "@/pages/CheckEmailPage";
import { NotFoundPage } from "@/pages/NotFoundPage";
import { FAQPage } from "@/pages/FAQPage";
import { RulesPage } from "@/pages/RulesPage";
import { FormattingPage } from "@/pages/FormattingPage";
import { AdminDashboardPage } from "@/pages/admin/AdminDashboardPage";
import { AdminChatMutesPage } from "@/pages/admin/AdminChatMutesPage";
import { AdminNewsPage } from "@/pages/admin/AdminNewsPage";
import { AdminUserDetailPage } from "@/pages/admin/AdminUserDetailPage";
import { AdminTorrentsPage } from "@/pages/admin/AdminTorrentsPage";
import { NewsListPage } from "@/pages/NewsListPage";
import { NewsDetailPage } from "@/pages/NewsDetailPage";
import { ForumIndexPage } from "@/pages/ForumIndexPage";
import { ForumTopicListPage } from "@/pages/ForumTopicListPage";
import { ForumTopicViewPage } from "@/pages/ForumTopicViewPage";
import { ForumNewTopicPage } from "@/pages/ForumNewTopicPage";
import { ForumSearchPage } from "@/pages/ForumSearchPage";
import { AdminForumsPage } from "@/pages/admin/AdminForumsPage";
import { AdminCheatFlagsPage } from "@/pages/admin/AdminCheatFlagsPage";
import { AdminBackupsPage } from "@/pages/admin/AdminBackupsPage";
import { AdminConnectorsPage } from "@/pages/admin/AdminConnectorsPage";
import { LiveReleasesPage } from "@/pages/LiveReleasesPage";
import { NotificationsPage } from "@/pages/NotificationsPage";

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
        path: "completed",
        element: (
          <ProtectedRoute>
            <CompletedPage />
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
        path: "metadata-issues",
        element: (
          <ProtectedRoute>
            <MetadataIssuesPage />
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
        path: "user/:username",
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
        path: "live",
        element: (
          <ProtectedRoute>
            <LiveReleasesPage />
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
          {
            index: true,
            element: (
              <>
                <AdminIndexRedirect />
                <AdminDashboardPage />
              </>
            ),
          },
          { path: "users", element: <AdminUsersPage /> },
          { path: "users/:id", element: <AdminUserDetailPage /> },
          { path: "reports", element: <AdminReportsPage /> },
          { path: "moderation", element: <AdminModerationPage /> },
          { path: "torrents", element: <AdminTorrentsPage /> },
          { path: "categories", element: <AdminCategoriesPage /> },
          { path: "categories/new", element: <AdminCategoryEditPage /> },
          { path: "categories/:id/edit", element: <AdminCategoryEditPage /> },
          { path: "groups", element: <AdminGroupsPage /> },
          { path: "promotion", element: <AdminPromotionPage /> },
          { path: "hit-and-run", element: <AdminHitAndRunPage /> },
          {
            path: "invite-distribution",
            element: <AdminInviteDistributionPage />,
          },
          { path: "warnings", element: <AdminWarningsPage /> },
          { path: "chat-mutes", element: <AdminChatMutesPage /> },
          { path: "news", element: <AdminNewsPage /> },
          { path: "settings", element: <AdminSettingsPage /> },
          { path: "bans", element: <AdminBansPage /> },
          { path: "cheat-flags", element: <AdminCheatFlagsPage /> },
          { path: "forums", element: <AdminForumsPage /> },
          { path: "backups", element: <AdminBackupsPage /> },
          { path: "connectors", element: <AdminConnectorsPage /> },
        ],
      },
      {
        path: "forums",
        element: (
          <ProtectedRoute>
            <ForumIndexPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "forums/search",
        element: (
          <ProtectedRoute>
            <ForumSearchPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "forums/:id",
        element: (
          <ProtectedRoute>
            <ForumTopicListPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "forums/:id/new",
        element: (
          <ProtectedRoute>
            <ForumNewTopicPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "forums/topics/:id",
        element: (
          <ProtectedRoute>
            <ForumTopicViewPage />
          </ProtectedRoute>
        ),
      },
      {
        path: "news",
        element: <NewsListPage />,
      },
      {
        path: "news/:id",
        element: <NewsDetailPage />,
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
        path: "notifications",
        element: (
          <ProtectedRoute>
            <NotificationsPage />
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
        path: "store",
        element: (
          <ProtectedRoute>
            <BonusStorePage />
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
