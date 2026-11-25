import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { AppProvider } from "./state";
import "./styles.css";
import "./guest/guest.css";
import "./operator/operator.css";
import "./studio-shell.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode><AppProvider><App /></AppProvider></StrictMode>,
);
