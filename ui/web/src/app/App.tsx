import { BrowserRouter, Route, Routes } from "react-router-dom";

import { LoginPage } from "../auth/LoginPage";
import { SessionBoundary } from "../auth/SessionBoundary";
import { BrowseShell } from "../browse/BrowseShell";
import { GalleryDetailPage } from "../browse/GalleryDetailPage";
import { GalleryIndexPage } from "../browse/GalleryIndexPage";
import { EntityIndexPage } from "../browse/EntityIndexPage";
import { CharacterDetailPage, CoserDetailPage, CoserTimelinePage, TagDetailPage, WorkDetailPage } from "../browse/EntityDetailPages";
import { RandomPage } from "../browse/RandomPage";
import { SearchPage } from "../browse/SearchPage";
import { TimelinePage } from "../browse/TimelinePage";
import { MediaDetailPage } from "../browse/MediaDetailPage";
import { FavoritesPage, HistoryPage } from "../browse/PersonalPages";
import { ManageGalleryEditorPage } from "../manage/ManageGalleryEditorPage";
import { ManageGalleryIndexPage } from "../manage/ManageGalleryIndexPage";
import { ManageShell } from "../manage/ManageShell";
import { ManageLibrariesPage } from "../manage/ManageLibrariesPage";
import { ManageSettingsPage } from "../manage/ManageSettingsPage";
import { ManageTasksPage } from "../manage/ManageTasksPage";
import { ManageCoreEntitiesPage } from "../manage/ManageCoreEntitiesPage";
import { ManageOperationsPage } from "../manage/ManageOperationsPage";
import { MaintenancePage } from "../manage/MaintenancePage";
import { Providers } from "./Providers";
import { SetupPage } from "../setup/SetupPage";

export function AppRoutes() {
  return (
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
