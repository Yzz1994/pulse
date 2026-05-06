import { toast as sonnerToast, Toaster as SonnerToaster } from "sonner";

export type ToastType = "success" | "error" | "info";

export function toast(message: string, type: ToastType = "info") {
  if (type === "success") sonnerToast.success(message);
  else if (type === "error") sonnerToast.error(message);
  else sonnerToast.info(message);
}

toast.success = (message: string) => sonnerToast.success(message);
toast.error = (message: string) => sonnerToast.error(message);
toast.info = (message: string) => sonnerToast.info(message);

export function Toaster() {
  return <SonnerToaster position="bottom-right" richColors />;
}
