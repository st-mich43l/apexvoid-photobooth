import { useRef, useState } from "react";
import { api } from "./api";
import { translate } from "./i18n";
import type { Language } from "./locale";
import { useApp } from "./state";

/** Changes only the persisted operator language, preserving all other settings. */
export function LanguageSwitcher() {
  const { settings, setSettings } = useApp();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const changing = useRef(false);
  const change = async (language: Language) => {
    if (changing.current || language === (settings.language ?? "en")) return;
    const previous = settings.language ?? "en";
    changing.current = true; setBusy(true); setFailed(false);
    setSettings(current => ({ ...current, language }));
    try {
      await api.patchSettings({ language });
    } catch {
      setSettings(current => current.language === language ? { ...current, language: previous } : current);
      setFailed(true);
    }
    finally { changing.current = false; setBusy(false); }
  };
  return <span className="language-switcher">
    <select aria-label={translate(settings.language, "interfaceLanguage")} value={settings.language ?? "en"} disabled={busy} onChange={e => { void change(e.target.value as Language); }}>
      <option value="en">English — EN</option><option value="vi">Tiếng Việt — VI</option>
    </select>
    {failed && <span role="alert">{translate(settings.language, "languageChangeFailed")}</span>}
  </span>;
}
