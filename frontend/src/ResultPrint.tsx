import { useRef, useState } from "react";
import { api, toAppError } from "./api";
import { newRequestId } from "./printInfo";
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
  const last = useRef(mediaId);
  if (last.current !== mediaId) { last.current = mediaId; reqId.current = newRequestId(); }

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
    setState({ kind: "sending", msg: "" });
    try {
      const j = await api.printResult(mediaId, reqId.current);
      reqId.current = newRequestId();
      setState({ kind: "sent", msg: j.status === "failed" ? j.detail : "Sent to the printer. Please wait a moment." });
    } catch (e) {
      const err = toAppError(e);
      setState({ kind: "error", msg: err.action ? `${err.message} ${err.action}` : err.message });
    }
  };
  return (
    <div className="result-print">
      <button className="big" disabled={state.kind === "sending"} onClick={print}>{state.kind === "sending" ? "Sending…" : state.kind === "sent" ? "Print another copy" : "🖨 Print"}</button>
      {state.kind === "sent" && <p className="muted" role="status">{state.msg}</p>}
      {state.kind === "error" && <div className="note err" role="alert"><div>{state.msg}</div></div>}
    </div>
  );
}
