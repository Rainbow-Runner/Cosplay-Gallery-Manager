import { type FormEvent, useState } from "react";
import { useIntl } from "react-intl";

export function LoginPage() {
  const intl = useIntl();
  const [password, setPassword] = useState(""); const [error, setError] = useState(""); const [busy, setBusy] = useState(false);
  const [recovering, setRecovering] = useState(false); const [token, setToken] = useState("");
  const [newPassword, setNewPassword] = useState(""); const [confirm, setConfirm] = useState(""); const [message, setMessage] = useState("");
  async function login(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    const response = await fetch("/session/login", { method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password }) });
    setBusy(false);
    if (!response.ok) { setError(intl.formatMessage({ id: "login.invalid" })); return; }
    window.location.assign("/");
  }
  async function recover(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(""); setMessage("");
    try {
      const response = await fetch("/session/recover", { method: "POST", credentials: "same-origin",
        headers: { "Content-Type": "application/json" }, body: JSON.stringify({ token: token.trim(), newPassword }) });
      if (!response.ok) throw new Error();
      setToken(""); setNewPassword(""); setConfirm(""); setRecovering(false);
      setMessage(intl.formatMessage({ id: "login.recoverySuccess" }));
    } catch { setError(intl.formatMessage({ id: "login.recoveryFailed" })); }
    finally { setBusy(false); }
  }
  if (recovering) return <main className="setup-shell"><form className="setup-card login-card" onSubmit={recover}><header><p>Cosplay Gallery Manager</p><h1>{intl.formatMessage({ id: "login.recoveryTitle" })}</h1></header>
    <p className="setup-note">{intl.formatMessage({ id: "login.recoveryHelp" })}</p>
    <label className="field"><span>{intl.formatMessage({ id: "login.recoveryToken" })}</span><input type="password" autoComplete="one-time-code" value={token} onChange={(event) => setToken(event.target.value)} /></label>
    <label className="field"><span>{intl.formatMessage({ id: "login.newPassword" })}</span><input type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /></label>
    <label className="field"><span>{intl.formatMessage({ id: "setup.confirmPassword" })}</span><input type="password" autoComplete="new-password" value={confirm} onChange={(event) => setConfirm(event.target.value)} /></label>
    {error ? <p className="form-error" role="alert">{error}</p> : null}<footer><button type="button" onClick={() => { setRecovering(false); setError(""); }}>{intl.formatMessage({ id: "setup.back" })}</button><button disabled={busy || token.trim().length < 32 || newPassword.length < 8 || newPassword !== confirm} type="submit">{intl.formatMessage({ id: "login.resetPassword" })}</button></footer></form></main>;
  return <main className="setup-shell"><form className="setup-card login-card" onSubmit={login}><header><p>Cosplay Gallery Manager</p><h1>{intl.formatMessage({ id: "login.title" })}</h1></header>
    <label className="field"><span>{intl.formatMessage({ id: "setup.password" })}</span><input autoFocus type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" /></label>
    {error ? <p className="form-error" role="alert">{error}</p> : null}{message ? <p role="status">{message}</p> : null}<footer><a href="/legal">{intl.formatMessage({ id: "nav.legal" })}</a><button type="button" onClick={() => { setRecovering(true); setError(""); }}>{intl.formatMessage({ id: "login.forgot" })}</button><button disabled={busy || !password} type="submit">{intl.formatMessage({ id: "login.submit" })}</button></footer></form></main>;
}
