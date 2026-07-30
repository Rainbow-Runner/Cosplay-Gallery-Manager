import { useRef, useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { useIntl } from "react-intl";

import { AppLogo } from "../ui/AppLogo";
import { Icon, type IconName } from "../ui/Icon";
import { Drawer } from "../ui/Patterns";
import { IconButton } from "../ui/Primitives";

interface NavigationItem {
  to: string;
  messageID: string;
  icon: IconName;
  end?: boolean;
}

interface NavigationGroup {
  labelID?: string;
  items: NavigationItem[];
}

const primaryGroups: NavigationGroup[] = [
  { items: [{ to: "/", messageID: "nav.home", icon: "home", end: true }] },
  {
    labelID: "nav.group.galleries",
    items: [
      { to: "/list", messageID: "nav.list", icon: "images" },
      { to: "/magic", messageID: "nav.magic", icon: "sparkles" },
    ],
  },
  {
    labelID: "nav.group.library",
    items: [
      { to: "/cosers", messageID: "nav.cosers", icon: "user" },
      { to: "/works", messageID: "nav.works", icon: "book" },
      { to: "/characters", messageID: "nav.characters", icon: "character" },
      { to: "/tags", messageID: "nav.tags", icon: "tags" },
    ],
  },
  {
    labelID: "nav.group.explore",
    items: [
      { to: "/timeline", messageID: "nav.timeline", icon: "calendar" },
      { to: "/random", messageID: "nav.random", icon: "shuffle" },
      { to: "/favorites", messageID: "nav.favorites", icon: "heart" },
      { to: "/history", messageID: "nav.history", icon: "history" },
    ],
  },
];

const utilityItems: NavigationItem[] = [
  { to: "/search", messageID: "nav.search", icon: "search" },
  { to: "/manage", messageID: "nav.manage", icon: "settings" },
  { to: "/legal", messageID: "nav.legal", icon: "info" },
];

function NavigationLink({ item, onNavigate }: { item: NavigationItem; onNavigate?: () => void }) {
  const intl = useIntl();
  return (
    <NavLink to={item.to} end={item.end} onClick={onNavigate}>
      <Icon name={item.icon} />
      <span>{intl.formatMessage({ id: item.messageID })}</span>
    </NavLink>
  );
}

function BrowseNavigation({ onNavigate }: { onNavigate?: () => void }) {
  const intl = useIntl();
  return (
    <nav className="browse-navigation" aria-label={intl.formatMessage({ id: "nav.primary" })}>
      <div className="browse-navigation__primary">
        {primaryGroups.map((group, groupIndex) => (
          <section key={group.labelID ?? `primary-${groupIndex}`}>
            {group.labelID ? <h2>{intl.formatMessage({ id: group.labelID })}</h2> : null}
            {group.items.map((item) => <NavigationLink key={item.to} item={item} onNavigate={onNavigate} />)}
          </section>
        ))}
      </div>
      <div className="browse-navigation__utility">
        {utilityItems.map((item) => <NavigationLink key={item.to} item={item} onNavigate={onNavigate} />)}
      </div>
    </nav>
  );
}

export function BrowseShell() {
  const intl = useIntl();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);

  function closeDrawer() {
    setDrawerOpen(false);
    menuButtonRef.current?.focus();
  }

  return (
    <div className="browse-shell" data-testid="browse-shell">
      <aside className="browse-sidebar">
        <div className="browse-sidebar__spacer" aria-hidden="true" />
        <BrowseNavigation />
      </aside>

      <div className="browse-workspace">
        <header className="browse-topbar">
          <IconButton
            ref={menuButtonRef}
            className="browse-topbar__menu"
            variant="ghost"
            label={intl.formatMessage({ id: "nav.openMenu" })}
            aria-expanded={drawerOpen}
            aria-controls="browse-mobile-navigation"
            onClick={() => setDrawerOpen(true)}
          >
            <Icon name="menu" />
          </IconButton>
          <NavLink className="browse-topbar__brand" to="/" aria-label="Cosplay Gallery Manager home">
            <AppLogo />
          </NavLink>
          <div className="browse-topbar__actions">
            <NavLink to="/search" aria-label={intl.formatMessage({ id: "nav.search" })}><Icon name="search" /></NavLink>
            <NavLink className="browse-topbar__manage" to="/manage" aria-label={intl.formatMessage({ id: "nav.manage" })}><Icon name="settings" /></NavLink>
          </div>
        </header>
        <div className="browse-content"><Outlet /></div>
      </div>

      <Drawer
        open={drawerOpen}
        title={intl.formatMessage({ id: "nav.primary" })}
        closeLabel={intl.formatMessage({ id: "nav.closeMenu" })}
        onClose={closeDrawer}
      >
        <div id="browse-mobile-navigation" className="browse-drawer-content">
          <NavLink className="browse-drawer-brand" to="/" onClick={closeDrawer}><AppLogo subtitle="Collection" /></NavLink>
          <BrowseNavigation onNavigate={closeDrawer} />
        </div>
      </Drawer>
    </div>
  );
}
