import { useEffect, useRef, useState } from "react";
import { api, toAppError } from "./api";
import { newRequestId } from "./printInfo";
import { Icon } from "./icons";
import { GuestMessage } from "./guest/parts";
import { useApp } from "./state";
import { Modal } from "./ui";
import { PrintPanel } from "./PrintPanel";

/** Print the finished image. Booth: one tap with the printer the operator chose (no printer settings for guests).
 *  Studio: the full print dialog with the real placement preview. The honest wording matters: a job in the queue is not paper in the tray. */
export function ResultPrint({ mediaId, booth }: { mediaId: string; booth: boolean }) {
  const { settings } = useApp();
  const [open, setOpen] = useState(false);
  const [state, setState] = useState<{ kind: "idle" | "sending" | "sent" | "error"; msg: string }>({ kind: "idle", msg: "" });
  const reqId = useRef(newRequestId()); // one id per tap: a double tap prints once
  const locked = useRef(false);
  useEffect(() => { reqId.current = newRequestId(); locked.current = false; setState({ kind: "idle", msg: "" }); }, [mediaId]);

  if (!booth) {
    return (
      <>
        <button onClick={() => setOpen(true)}>Print…</button>
        {open && <Modal title="Print" onClose={() => setOpen(false)} wide><div className="modal-body"><PrintPanel mediaId={mediaId} onPrinted={() => setOpen(false)} /></div></Modal>}
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
      setState({ kind: "sent", msg: j.status === "failed" ? j.detail : "Sent to the printer. If nothing comes out, please ask the host." });
    } catch (e) {
      const err = toAppError(e);
      setState({ kind: "error", msg: /unknown/i.test(err.message) ? "Submission status unknown. Please ask the operator to check the printer." : "The printer needs attention. Please ask the operator for help." });
    }
  };
  return (
    <div className="g-print">
      <button type="button" className="g-btn g-secondary" disabled={state.kind !== "idle"} onClick={print}>
        <Icon name="printer" size={24} /><span>{state.kind === "sending" ? "Sending…" : state.kind === "sent" ? "Sent to printer" : state.kind === "error" ? "Ask the operator" : "Print"}</span>
      </button>
      {state.kind === "sent" && <p className="g-print-note" role="status">{state.msg} For another copy, please ask the operator.</p>}
      {state.kind === "error" && <GuestMessage kind="err">{state.msg}</GuestMessage>}
    </div>
  );
}
