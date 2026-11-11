import { useEffect, useId, useReducer, useRef, type ReactNode } from "react";
import { Button, Icon } from "./components";

export default function ActionDialog({
  title,
  trigger = title,
  children,
  className,
  triggerClassName,
  open,
  onOpenChange,
  onOpen,
  busy = false,
  error,
  notice,
}: {
  title: ReactNode;
  trigger?: ReactNode;
  children: ReactNode;
  className?: string;
  triggerClassName?: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  onOpen?: () => void;
  busy?: boolean;
  error?: string;
  notice?: string;
}) {
  const [localOpen, setLocalOpen] = useReducer(
    (_current: boolean, next: boolean) => next,
    false,
  );
  const visible = open ?? localOpen;
  const dialog = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const dialogId = useId();
  const changeOpen = (next: boolean) => {
    setLocalOpen(next);
    onOpenChange?.(next);
  };

  useEffect(() => {
    const element = dialog.current;
    if (visible) element?.showModal();
    else element?.close();
  }, [visible]);

  const show = () => {
    changeOpen(true);
    onOpen?.();
  };

  return (
    <div className={className}>
      {trigger === null ? null : triggerClassName ? (
        <button
          type="button"
          className={triggerClassName}
          aria-haspopup="dialog"
          aria-controls={dialogId}
          onClick={show}
        >
          {trigger}
          <Icon name="chevron" size={16} />
        </button>
      ) : (
        <Button
          type="button"
          aria-haspopup="dialog"
          aria-controls={dialogId}
          onClick={show}
        >
          {trigger}
        </Button>
      )}
      <dialog
        ref={dialog}
        id={dialogId}
        aria-labelledby={titleId}
        onCancel={(event) => {
          event.preventDefault();
          if (!busy) changeOpen(false);
        }}
        onClose={() => {
          if (!dialog.current?.open) changeOpen(false);
        }}
        className="m-auto max-h-[calc(100dvh-32px)] w-[680px] max-w-[calc(100vw-32px)] overflow-auto rounded-xl bg-white p-4 text-ink shadow-xl backdrop:bg-black/20"
      >
        <div className="mb-4 flex items-start justify-between gap-3">
          <h2
            id={titleId}
            className="min-w-0 break-words text-lg font-semibold"
          >
            {title}
          </h2>
          <button
            type="button"
            className="icon-button shrink-0"
            aria-label="关闭"
            disabled={busy}
            onClick={() => changeOpen(false)}
          >
            <Icon name="close" />
          </button>
        </div>
        {error && (
          <p role="alert" className="mb-3 break-words text-sm text-removed">
            {error}
          </p>
        )}
        {notice && (
          <p role="status" className="mb-3 break-words text-sm text-muted">
            {notice}
          </p>
        )}
        <div className="min-w-0 space-y-3">{children}</div>
      </dialog>
    </div>
  );
}
