import { NavLink, Outlet } from "react-router-dom";
import { useIntl } from "react-intl";

const links = [
  ["/", "nav.home"], ["/list", "nav.list"], ["/magic", "nav.magic"],
  ["/cosers", "nav.cosers"], ["/works", "nav.works"], ["/characters", "nav.characters"],
  ["/tags", "nav.tags"], ["/timeline", "nav.timeline"], ["/random", "nav.random"],
] as const;

export function BrowseShell() {
  const intl = useIntl();
  return (
    <div className="browse-shell" data-testid="browse-shell">
      <header className="site-header">
        <NavLink className="site-header__brand" to="/">CGM <span>Collection</span></NavLink>
        <nav className="site-nav" aria-label={intl.formatMessage({ id: "nav.primary" })}>
          {links.map(([to, id]) => <NavLink key={to} to={to} end={to === "/"}>{intl.formatMessage({ id })}</NavLink>)}
        </nav>
        <div className="site-header__actions">
          <NavLink to="/search" aria-label={intl.formatMessage({ id: "nav.search" })}>⌕</NavLink>
          <NavLink to="/favorites" aria-label={intl.formatMessage({ id: "nav.favorites" })}>♡</NavLink>
          <NavLink to="/legal" aria-label={intl.formatMessage({ id: "nav.legal" })}>ⓘ</NavLink>
          <NavLink to="/manage" aria-label={intl.formatMessage({ id: "nav.manage" })}>⚙</NavLink>
        </div>
      </header>
      <Outlet />
    </div>
  );
}
