import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import { router } from "./router";
import { initTheme } from "./lib/theme";
import "./i18n";

initTheme();

const root = document.getElementById("root");
if (!root) throw new Error("Root element not found");

createRoot(root).render(<RouterProvider router={router} />);
