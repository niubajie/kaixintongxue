import { create } from "zustand";

type ToastKind = "success" | "error" | "info";

interface ToastItem {
  id: number;
  kind: ToastKind;
  text: string;
}

interface ToastState {
  items: ToastItem[];
  push: (kind: ToastKind, text: string) => void;
  remove: (id: number) => void;
}

let seq = 0;

export const useToastStore = create<ToastState>((set) => ({
  items: [],
  push: (kind, text) => {
    const id = ++seq;
    set((s) => ({ items: [...s.items, { id, kind, text }] }));
    window.setTimeout(() => {
      set((s) => ({ items: s.items.filter((t) => t.id !== id) }));
    }, 3200);
  },
  remove: (id) => set((s) => ({ items: s.items.filter((t) => t.id !== id) }))
}));

export const toast = {
  success: (text: string) => useToastStore.getState().push("success", text),
  error: (text: string) => useToastStore.getState().push("error", text),
  info: (text: string) => useToastStore.getState().push("info", text)
};

export function ToastHost() {
  const items = useToastStore((s) => s.items);
  const remove = useToastStore((s) => s.remove);
  return (
    <div className="toast-host">
      {items.map((t) => (
        <div key={t.id} className={`toast toast-${t.kind}`} onClick={() => remove(t.id)}>
          {t.text}
        </div>
      ))}
    </div>
  );
}
