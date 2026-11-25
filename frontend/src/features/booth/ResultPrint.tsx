import { useLocale } from "../../shared/locale";
import { translate } from "../../shared/i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "../../shared/api";
import { newRequestId } from "../../shared/printInfo";
import { Icon } from "../../shared/icons";
import { GuestMessage } from "./guest/parts";
import { useApp } from "../../app/state";
import { Modal } from "../../shared/ui";
import { PrintPanel } from "../printer/PrintPanel";

/** Print the finished image. Booth: one tap with the printer the operator chose (no printer settings for guests).
 *  Studio: the full print dialog with the real placement preview. The honest wording matters: a job in the queue is not paper in the tray. */
export function ResultPrint({ mediaId, booth }: { mediaId: string; booth: boolean }) {
  const { settings } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const [open, setOpen] = useState(false);
  const [state, setState] = useState<{ kind: "idle" | "sending" | "sent" | "error"; msg: string }>({ kind: "idle", msg: "" });
  const reqId = useRef(newRequestId()); // one id per tap: a double tap prints once
  const locked = useRef(false);
  useEffect(() => { reqId.current = newRequestId(); locked.current = false; setState({ kind: "idle", msg: "" }); }, [mediaId]);

  if (!booth) {
    return (
      <>
        <button onClick={() => setOpen(true)}>{t("printEllipsis")}</button>
        {open && <Modal title={t("print")} onClose={() => setOpen(false)} wide><div className="modal-body"><PrintPanel mediaId={mediaId} onPrinted={() => setOpen(false)} /></div></Modal>}
      </>
    );
  }
  if (!settings.printerName) return null; // printing is not set up: guests are not shown a button that cannot work
  const print = async () => {
    if (locked.current) return;
    locked.current = true;
    setState({ kind: "sending", msg: "" });
    try {
      const j = await api.printResult(mediaId, reqId.current);
      setState(j.status === "failed" ? { kind: "error", msg: t("printerAttention") } : { kind: "sent", msg: t("printerSentHint") });
    } catch {
      // A failed request may have reached the spooler; never invite a blind retry.
      setState({ kind: "error", msg: t("printerUnknown") });
    }
  };
  return (
    <div className="g-print">
      <button type="button" className="g-btn g-secondary" disabled={state.kind !== "idle"} onClick={print}>
        <Icon name="printer" size={24} /><span>{state.kind === "sending" ? t("sending") : state.kind === "sent" ? t("sentToPrinter") : state.kind === "error" ? t("askOperator") : t("print")}</span>
      </button>
      {state.kind === "sent" && <p className="g-print-note" role="status">{state.msg} {t("extraCopy")}</p>}
      {state.kind === "error" && <GuestMessage kind="err">{state.msg}</GuestMessage>}
    </div>
  );
}
