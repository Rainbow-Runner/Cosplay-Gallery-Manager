import { NavLink, Outlet } from "react-router-dom";
import { useIntl } from "react-intl";

export function ManageShell() {
  const intl = useIntl();
  return <div className="manage-shell" data-testid="manage-shell"><aside className="manage-nav"><NavLink className="manage-brand" to="/">CGM</NavLink><h1>{intl.formatMessage({ id: "shell.manage.title" })}</h1>
    <nav><NavLink end to="/manage">{intl.formatMessage({ id: "manage.galleries" })}</NavLink><NavLink to="/manage/cosers">Coser</NavLink>
      <NavLink to="/manage/entities">Work / Character / Tag</NavLink><NavLink to="/manage/libraries">{intl.formatMessage({ id: "manage.libraries" })}</NavLink>
      <NavLink to="/manage/tasks">{intl.formatMessage({ id: "manage.tasks" })}</NavLink><NavLink to="/manage/operations">Operations</NavLink><NavLink to="/manage/settings">{intl.formatMessage({ id: "manage.settings" })}</NavLink></nav></aside>
    <div className="manage-content"><Outlet /></div></div>;
}
