import { createContext, useContext, useEffect, type ReactNode } from "react";

export type Language = "en" | "vi";
const Locale = createContext<Language | undefined>(undefined);
export function useLocale(fallback?: string): Language {
  const selected = useContext(Locale);
  return selected ?? (fallback === "vi" ? "vi" : "en");
}
export function OperatorLocale({ language, children }: { language?: string; children: ReactNode }) {
  return <Locale.Provider value={language === "vi" ? "vi" : "en"}>{children}</Locale.Provider>;
}
/** Guest locale is scoped to Booth and never writes the operator preference. */
export function GuestLocale({ language, operatorLanguage, children }: { language: Language; operatorLanguage?: string; children: ReactNode }) {
  useEffect(() => {
    document.documentElement.lang = language;
    return () => { document.documentElement.lang = operatorLanguage === "vi" ? "vi" : "en"; };
  }, [language, operatorLanguage]);
  return <Locale.Provider value={language}>{children}</Locale.Provider>;
}
