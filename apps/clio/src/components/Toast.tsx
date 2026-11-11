import { useEffect } from "react";
import "./Toast.css";

interface Props {
  message: string;
  onDone: () => void;
  durationMs?: number;
}

/** A one-line notice that dismisses itself. */
export function Toast({ message, onDone, durationMs = 4000 }: Props) {
  useEffect(() => {
    const id = setTimeout(onDone, durationMs);
    return () => clearTimeout(id);
  }, [message, onDone, durationMs]);
  return (
    <div className="toast" role="status">
      {message}
    </div>
  );
}
