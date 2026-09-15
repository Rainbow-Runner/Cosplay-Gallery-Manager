import { NavLink, Outlet } from "react-router-dom";
import { useIntl } from "react-intl";

import { AppLogo } from "../ui/AppLogo";
import { Icon, type IconName } from "../ui/Icon";

const links: Array<[string, string, IconName, boolean?]> = [
  ["/manage", "manage.galleries", "images", true],
  ["/manage/cosers", "manage.cosers", "user"],
  ["/manage/entities", "manage.entities", "tags"],
  ["/manage/libraries", "manage.libraries", "search"],
  ["/manage/tasks", "manage.tasks", "history"],
  ["/manage/operations", "Operations", "settings"],
  ["/manage/settings", "manage.settings", "settings"],
  ["/manage/help", "manage.help", "book"],
  ["/legal", "nav.legal", "info"],
];

export function ManageShell() {
  const intl = useIntl();
  return (
    <div className="manage-shell" data-testid="manage-shell">
      <aside className="manage-nav">
        <NavLink className="manage-brand" to="/"><AppLogo subtitle={intl.formatMessage({ id: "shell.manage.title" })} /></NavLink>
        <span className="cgm-badge">Manage</span>
        <nav aria-label={intl.formatMessage({ id: "shell.manage.title" })}>
          {links.map(([to, id, icon, end]) => (
            <NavLink key={to} end={end} to={to}><Icon name={icon} /><span>{id.includes(".") ? intl.formatMessage({ id }) : id}</span></NavLink>
          ))}
        </nav>
      </aside>
      <div className="manage-content"><Outlet /></div>
    </div>
  );
}
