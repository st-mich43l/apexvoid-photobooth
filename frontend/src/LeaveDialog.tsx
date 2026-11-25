import { formatTranslation, photoCount, translate } from "./i18n";
import { useLocale } from "./locale";
import { Modal } from "./ui";

/** Asked before a guest abandons an unfinished photo session (Home, Start over, Cancel). It says what
 *  will happen to the photos, and the safe answer ("Keep going") is the default. */
export function LeaveDialog({ state, taken, busy, error, confirmLabel, language: preferredLanguage, onKeep, onLeave }:
  { state: string; taken: number; busy: boolean; error?: string; language?: string; confirmLabel?: string; onKeep: () => void; onLeave: () => void }) {
  const language = useLocale(preferredLanguage);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const shooting = state === "CAPTURING" || state === "SAVING";
  return (
    <Modal title={t("leaveTitle")} onClose={busy ? () => {} : onKeep}>
      <div className="modal-body">
        {shooting ? <p>{t(state === "SAVING" ? "leaveSaving" : "leaveCapturing")}</p>
          : state === "REVIEW" ? <p>{formatTranslation(language, "leaveReview", { count: photoCount(language, taken) })}</p>
          : <p>{t("leaveBefore")}</p>}
        {error && <div className="note err" role="alert"><div>{error}</div></div>}
      </div>
      <footer>
        <button className="primary" autoFocus onClick={onKeep} disabled={busy}>{t("keepGoing")}</button>
        <button className="danger" onClick={onLeave} disabled={busy || shooting}>{busy ? t("wait") : confirmLabel ?? t("leave")}</button>
      </footer>
    </Modal>
  );
}
