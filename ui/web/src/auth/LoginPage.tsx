import { type FormEvent, useState } from "react";
import { useIntl } from "react-intl";

export function LoginPage() {
  const intl = useIntl();
  const [password, setPassword] = useState(""); const [error, setError] = useState(""); const [busy, setBusy] = useState(false);
  async function login(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    const response = await fetch("/session/login", { method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json" }, body: JSON.stringify({ password }) });
    setBusy(false);
    if (!response.ok) { setError(intl.formatMessage({ id: "login.invalid" })); return; }
    window.location.assign("/");
  }
  return <main className="setup-shell"><form className="setup-card login-card" onSubmit={login}><header><p>Cosplay Gallery Manager</p><h1>{intl.formatMessage({ id: "login.title" })}</h1></header>
    <label className="field"><span>{intl.formatMessage({ id: "setup.password" })}</span><input autoFocus type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" /></label>
    {error ? <p className="form-error" role="alert">{error}</p> : null}<footer><span /><button disabled={busy || !password} type="submit">{intl.formatMessage({ id: "login.submit" })}</button></footer></form></main>;
}
