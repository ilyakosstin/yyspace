import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import { AuthGate } from "./auth";
import { applyTheme } from "./settings";
import "./styles.css";

applyTheme();
const router = createBrowserRouter([{ path: "/*", element: <AuthGate /> }]);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <RouterProvider router={router} />
  </StrictMode>,
);
