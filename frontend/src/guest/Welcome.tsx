import { translate } from "../i18n";
import type { BoothReadiness } from "../types";
import { BrandMark, GuestMessage, PrimaryAction } from "./parts";

/** Decorative photo strips. Purely visual (aria-hidden, no pointer events), animated with transforms only. */
function WelcomeArt() {
  return (
    <div className="g-art" aria-hidden="true">
      <span className="g-orb a" /><span className="g-orb b" />
      <span className="g-strip s1"><i /><i /><i /><i /></span>
      <span className="g-strip s2"><i /><i /><i /><i /></span>
      <span className="g-strip s3"><i /><i /><i /><i /></span>
      <span className="g-spark k1" /><span className="g-spark k2" /><span className="g-spark k3" />
    </div>
  );
}

export interface WelcomeProps {
  language?: string; blocked: boolean; resetting: boolean; guestText: string; readiness: BoothReadiness | null;
  resetFailed: boolean; resetError: string; modeError?: string;
  onStart: () => void; onRetryReset: () => void; onLanguageChange?: (language: "en" | "vi") => void;
}

/** The first thing a guest sees. One big action; any reason it cannot start is stated plainly, never hidden by the artwork. */
export function Welcome(p: WelcomeProps) {
  const t = (key: Parameters<typeof translate>[1]) => translate(p.language, key);
  const sub = p.blocked ? t("welcomeBlocked") : t("welcomeSub");
  return (
    <div className="g-welcome">
      <WelcomeArt />
      <div className="g-welcome-card">
        <div className="g-rise r1"><BrandMark /></div>
        {p.onLanguageChange && <label className="guest-language">{t("guestLanguageLabel")}
          <select aria-label={t("guestLanguageLabel")} value={p.language === "vi" ? "vi" : "en"} disabled={p.resetting} onChange={e => p.onLanguageChange?.(e.target.value as "en" | "vi")}>
            <option value="en">English — EN</option><option value="vi">Tiếng Việt — VI</option>
          </select>
        </label>}
        <h1 className="g-rise r2">{t("welcomeTitle")}</h1>
        <p className="g-sub g-rise r3">{sub}</p>
        {p.blocked && (
          <GuestMessage kind="warn" hint={p.readiness ? t(p.readiness.status === "reconnecting" ? "hostReconnect" : p.readiness.status === "attention" ? "hostAttention" : "hostSetup") : undefined}>
            {p.guestText}
          </GuestMessage>
        )}
        {p.resetFailed && !p.resetting && (
          <GuestMessage kind="warn" role="alert" hint={t("hostRecovery")} action={<button type="button" className="g-btn g-secondary" onClick={p.onRetryReset}>{t("tryAgain")}</button>}>
            {t("tidy")}
          </GuestMessage>
        )}
        {p.modeError && <GuestMessage kind="err">{p.modeError}</GuestMessage>}
        <div className="g-rise r4">
          <PrimaryAction icon="camera" className="g-start" disabled={p.blocked || p.resetting} onClick={p.onStart}>
            {p.resetting ? t("wait") : t("takePhotos")}
          </PrimaryAction>
        </div>
        <p className="g-hint g-rise r5">{p.resetting ? t("gettingReady") : t("chooseFrame")}</p>
      </div>
    </div>
  );
}
