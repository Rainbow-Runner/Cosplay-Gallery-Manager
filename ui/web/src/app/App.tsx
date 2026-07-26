import { lazy, Suspense } from "react";
import { BrowserRouter, Route, Routes } from "react-router-dom";

import { SessionBoundary } from "../auth/SessionBoundary";
import { Providers } from "./Providers";

const LoginPage = lazy(() => import("../auth/LoginPage").then((module) => ({ default: module.LoginPage })));
const BrowseShell = lazy(() => import("../browse/BrowseShell").then((module) => ({ default: module.BrowseShell })));
const GalleryDetailPage = lazy(() => import("../browse/GalleryDetailPage").then((module) => ({ default: module.GalleryDetailPage })));
const GalleryIndexPage = lazy(() => import("../browse/GalleryIndexPage").then((module) => ({ default: module.GalleryIndexPage })));
const EntityIndexPage = lazy(() => import("../browse/EntityIndexPage").then((module) => ({ default: module.EntityIndexPage })));
const CharacterDetailPage = lazy(() => import("../browse/EntityDetailPages").then((module) => ({ default: module.CharacterDetailPage })));
const CoserDetailPage = lazy(() => import("../browse/EntityDetailPages").then((module) => ({ default: module.CoserDetailPage })));
const CoserTimelinePage = lazy(() => import("../browse/EntityDetailPages").then((module) => ({ default: module.CoserTimelinePage })));
const TagDetailPage = lazy(() => import("../browse/EntityDetailPages").then((module) => ({ default: module.TagDetailPage })));
const WorkDetailPage = lazy(() => import("../browse/EntityDetailPages").then((module) => ({ default: module.WorkDetailPage })));
const RandomPage = lazy(() => import("../browse/RandomPage").then((module) => ({ default: module.RandomPage })));
const SearchPage = lazy(() => import("../browse/SearchPage").then((module) => ({ default: module.SearchPage })));
const TimelinePage = lazy(() => import("../browse/TimelinePage").then((module) => ({ default: module.TimelinePage })));
const MediaDetailPage = lazy(() => import("../browse/MediaDetailPage").then((module) => ({ default: module.MediaDetailPage })));
const FavoritesPage = lazy(() => import("../browse/PersonalPages").then((module) => ({ default: module.FavoritesPage })));
const HistoryPage = lazy(() => import("../browse/PersonalPages").then((module) => ({ default: module.HistoryPage })));
const ManageGalleryEditorPage = lazy(() => import("../manage/ManageGalleryEditorPage").then((module) => ({ default: module.ManageGalleryEditorPage })));
const ManageGalleryIndexPage = lazy(() => import("../manage/ManageGalleryIndexPage").then((module) => ({ default: module.ManageGalleryIndexPage })));
const ManageShell = lazy(() => import("../manage/ManageShell").then((module) => ({ default: module.ManageShell })));
const ManageLibrariesPage = lazy(() => import("../manage/ManageLibrariesPage").then((module) => ({ default: module.ManageLibrariesPage })));
const ManageSettingsPage = lazy(() => import("../manage/ManageSettingsPage").then((module) => ({ default: module.ManageSettingsPage })));
const ManageTasksPage = lazy(() => import("../manage/ManageTasksPage").then((module) => ({ default: module.ManageTasksPage })));
const ManageCoreEntitiesPage = lazy(() => import("../manage/ManageCoreEntitiesPage").then((module) => ({ default: module.ManageCoreEntitiesPage })));
const ManageOperationsPage = lazy(() => import("../manage/ManageOperationsPage").then((module) => ({ default: module.ManageOperationsPage })));
const MaintenancePage = lazy(() => import("../manage/MaintenancePage").then((module) => ({ default: module.MaintenancePage })));
const SetupPage = lazy(() => import("../setup/SetupPage").then((module) => ({ default: module.SetupPage })));

export function AppRoutes() {
  return (
    <Suspense fallback={<div role="status">Loading…</div>}>
      <Routes>
        <Route path="/manage" element={<ManageShell />}>
          <Route index element={<ManageGalleryIndexPage />} />
          <Route path="gallery/:setID" element={<ManageGalleryEditorPage />} />
          <Route path="cosers" element={<ManageCoreEntitiesPage coserOnly />} />
          <Route path="entities" element={<ManageCoreEntitiesPage />} />
          <Route path="libraries" element={<ManageLibrariesPage />} />
          <Route path="tasks" element={<ManageTasksPage />} />
          <Route path="operations" element={<ManageOperationsPage />} />
          <Route path="settings" element={<ManageSettingsPage />} />
        </Route>
        <Route path="/setup/*" element={<SetupPage />} />
        <Route path="/login" element={<LoginPage />} />
        <Route path="/maintenance" element={<MaintenancePage />} />
        <Route element={<BrowseShell />}>
          <Route index element={<GalleryIndexPage home />} />
          <Route path="list" element={<GalleryIndexPage scope="LIST" />} />
          <Route path="magic" element={<GalleryIndexPage scope="MAGIC" />} />
          <Route path="gallery/:slug" element={<GalleryDetailPage />} />
          <Route path="cosers" element={<EntityIndexPage kind="COSER" titleID="page.cosers" />} />
          <Route path="coser/:slug" element={<CoserDetailPage />} />
          <Route path="coser/:slug/timeline" element={<CoserTimelinePage />} />
          <Route path="works" element={<EntityIndexPage kind="WORK" titleID="page.works" />} />
          <Route path="work/:slug" element={<WorkDetailPage />} />
          <Route path="characters" element={<EntityIndexPage kind="CHARACTER" titleID="page.characters" />} />
          <Route path="character/:slug" element={<CharacterDetailPage />} />
          <Route path="tags" element={<EntityIndexPage kind="TAG" titleID="page.tags" />} />
          <Route path="tag/:slug" element={<TagDetailPage />} />
          <Route path="timeline" element={<TimelinePage />} />
          <Route path="random" element={<RandomPage />} />
          <Route path="favorites" element={<FavoritesPage />} />
          <Route path="history" element={<HistoryPage />} />
          <Route path="media/:uuid" element={<MediaDetailPage />} />
          <Route path="search" element={<SearchPage />} />
        </Route>
      </Routes>
    </Suspense>
  );
}

export function App() {
  return (
    <Providers>
      <BrowserRouter>
        <SessionBoundary><AppRoutes /></SessionBoundary>
      </BrowserRouter>
    </Providers>
  );
}
