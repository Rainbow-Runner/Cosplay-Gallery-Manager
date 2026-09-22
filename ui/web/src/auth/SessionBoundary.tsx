import { type PropsWithChildren, useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";

interface SessionStatus { setupComplete: boolean; authenticated: boolean; trustedMode: boolean; maintenanceState?: string }

export function SessionBoundary({ children }: PropsWithChildren) {
  const location = useLocation(); const navigate = useNavigate();
  const [status, setStatus] = useState<SessionStatus | null>(null);
  useEffect(() => {
    let active = true;
    fetch("/session/status", { credentials: "same-origin", cache: "no-store" })
      .then((response) => response.ok ? response.json() : Promise.reject())
      .then(async (value: SessionStatus) => {
        if (!value.authenticated) return value;
        const response = await fetch("/maintenance/status", { credentials: "same-origin", cache: "no-store" });
        if (!response.ok) return value;
        const maintenance: { state: string } = await response.json();
        return { ...value, maintenanceState: maintenance.state };
      })
      .then((value: SessionStatus) => { if (active) setStatus(value); })
      .catch(() => { if (active) setStatus({ setupComplete: true, authenticated: false, trustedMode: false }); });
    return () => { active = false; };
  }, [location.key]);
  useEffect(() => {
    if (!status) return;
    if (location.pathname === "/legal") return;
    if (!status.setupComplete && !location.pathname.startsWith("/setup")) navigate("/setup", { replace: true });
    else if (status.setupComplete && !status.authenticated && location.pathname !== "/login") navigate("/login", { replace: true });
    else if (status.setupComplete && status.authenticated && status.maintenanceState && status.maintenanceState !== "NORMAL" && location.pathname !== "/maintenance") navigate("/maintenance", { replace: true });
    else if (status.setupComplete && status.authenticated && status.maintenanceState === "NORMAL" && location.pathname === "/maintenance") navigate("/manage/operations", { replace: true });
    else if (status.setupComplete && status.authenticated && (location.pathname === "/login" || location.pathname.startsWith("/setup"))) navigate("/", { replace: true });
  }, [location.pathname, navigate, status]);
  if (!status && location.pathname !== "/login" && location.pathname !== "/legal" && !location.pathname.startsWith("/setup")) return <div className="session-loading" aria-busy="true" />;
  return <>{children}</>;
}
