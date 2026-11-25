import { useState } from "react";
import { api } from "../../../shared/api";
import { formatTranslation, translate } from "../../../shared/i18n";
import { useLocale } from "../../../shared/locale";
import { useApp } from "../../../app/state";
import { Confirm, ErrorNote, useAction } from "../../../shared/ui";

/** A test print is always an explicit, confirmed operator action: it uses paper and ink. Never run at start-up or by a page opening. */
export function TestPrint({ printer, paper, disabled, onSent }: { printer: string; paper: string; disabled?: boolean; onSent?: () => void }) {
  const { toast, settings } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const [ask, setAsk] = useState(false);
  const [sent, setSent] = useState(false);
  const [inspected, setInspected] = useState(false);
  const act = useAction();
  return (
    <>
      <button disabled={disabled || act.busy || !printer} onClick={() => setAsk(true)}>{t("testPrintLabel")}</button>
      {sent && <label className="check"><input type="checkbox" checked={inspected} onChange={(e) => setInspected(e.target.checked)} /> {t("testPrintInspected")}</label>}
      <ErrorNote error={act.error} onClose={act.clear} />
      {ask && (
        <Confirm title={t("testPrintTitle")} confirmLabel={t("testPrintConfirm")} onNo={() => setAsk(false)}
          body={<p>{formatTranslation(language, "testPrintBody", { printer, paper: paper || t("defaultPaperLabel") })}</p>}
          onYes={() => { setAsk(false); void act.run(async () => { const j = await api.testPrint(printer, paper, "portrait"); toast(formatTranslation(language, "testPrintSent", { id: j.id })); setSent(true); setInspected(false); onSent?.(); }); }} />
      )}
    </>
  );
}
