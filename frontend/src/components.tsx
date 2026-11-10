import type { ButtonHTMLAttributes } from "react";
import {
  ArrowUpRight,
  ChevronRight,
  FileCode2,
  Folder,
  FolderGit2,
  Folders,
  GitBranch,
  Layers2,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  X,
} from "lucide-react";

const icons = {
  close: X,
  chevron: ChevronRight,
  search: Search,
  inbox: Folders,
  plus: Plus,
  folder: Folder,
  layers: Layers2,
  download: RefreshCw,
  settings: Settings2,
  project: FolderGit2,
  branch: GitBranch,
  arrow: ArrowUpRight,
  file: FileCode2,
  refresh: RefreshCw,
};

export function Icon({
  name,
  size = 18,
}: {
  name: keyof typeof icons;
  size?: number;
}) {
  const Symbol = icons[name];
  return (
    <Symbol
      size={size}
      strokeWidth={1.75}
      className="shrink-0"
      aria-hidden="true"
    />
  );
}

export function Brand({ size = 28 }: { size?: number }) {
  return (
    <span
      className="inline-flex shrink-0 items-center justify-center"
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <img src="/brand.svg" width={22.5} height={12.5} alt="" />
    </span>
  );
}

export function Button({
  primary,
  className = "",
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { primary?: boolean }) {
  return (
    <button
      {...props}
      className={`${primary ? "bg-accent text-white hover:bg-accent-hover border-accent" : "bg-surface border-transparent hover:bg-nav-hover"} inline-flex min-h-9 shrink-0 items-center justify-center gap-2 rounded-md border px-3 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-45 ${className}`}
    />
  );
}

export function Empty({
  title,
  children,
  icon = "folder",
}: {
  title: string;
  children: React.ReactNode;
  icon?: keyof typeof icons;
}) {
  return (
    <div className="empty-state">
      <span className="mb-3 text-muted">
        <Icon name={icon} size={28} />
      </span>
      <h2 className="text-lg font-semibold leading-6">{title}</h2>
      <div className="mt-1.5 text-sm leading-5 text-muted">{children}</div>
    </div>
  );
}
