import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

interface ToastOptions {
  durationMs?: number;
}

interface ToastContextValue {
  showToast: (msg: string, options?: ToastOptions) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

const DEFAULT_DURATION_MS = 1800;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<string | null>(null);
  const timerRef = useRef<number | null>(null);

  const showToast = useCallback((msg: string, options?: ToastOptions) => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    setToast(msg);
    const duration = options?.durationMs ?? DEFAULT_DURATION_MS;
    timerRef.current = window.setTimeout(() => {
      setToast(null);
      timerRef.current = null;
    }, duration);
  }, []);

  const value = useMemo<ToastContextValue>(() => ({ showToast }), [showToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      {toast && <div className="ws-toast">{toast}</div>}
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error("useToast must be used within a <ToastProvider>");
  }
  return ctx;
}
