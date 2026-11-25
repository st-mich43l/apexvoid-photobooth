import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { AppProvider } from "./state";
import "./styles.css";
import "../features/booth/guest/guest.css";
import "../features/studio/operator/operator.css";
import "../features/studio/studio-shell.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode><AppProvider><App /></AppProvider></StrictMode>,
);
