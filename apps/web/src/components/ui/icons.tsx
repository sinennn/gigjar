import type { ReactNode, SVGProps } from "react";

export type IconName =
  | "home"
  | "jar"
  | "link"
  | "activity"
  | "send"
  | "wallet"
  | "settings"
  | "more"
  | "plus"
  | "copy"
  | "check"
  | "external"
  | "back";

const PATHS: Record<IconName, ReactNode> = {
  home: <path d="M4 11 12 4l8 7v8a1 1 0 0 1-1 1h-4v-6H9v6H5a1 1 0 0 1-1-1z" />,
  jar: (
    <>
      <path d="M9 3h6v3H9z" />
      <rect x="5" y="6" width="14" height="15" rx="4" />
      <path d="M9 14h6" />
    </>
  ),
  link: (
    <>
      <path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1" />
      <path d="M14 10a4 4 0 0 0-5.7 0l-3 3A4 4 0 0 0 11 18.7l1-1" />
    </>
  ),
  activity: <path d="M4 12h4l3-7 4 14 3-7h2" />,
  send: <path d="M7 17 17 7m0 0H9.5M17 7v7.5" />,
  wallet: (
    <>
      <path d="M4 8a2 2 0 0 1 2-2h12v4" />
      <rect x="4" y="8" width="16" height="11" rx="3" />
      <circle cx="16" cy="13.5" r="1" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 3v2m0 14v2M3 12h2m14 0h2M5.6 5.6 7 7m10 10 1.4 1.4M18.4 5.6 17 7M7 17l-1.4 1.4" />
    </>
  ),
  more: (
    <>
      <circle cx="6" cy="12" r="1.2" />
      <circle cx="12" cy="12" r="1.2" />
      <circle cx="18" cy="12" r="1.2" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  copy: (
    <>
      <rect x="9" y="9" width="11" height="11" rx="3" />
      <path d="M5 15V6a2 2 0 0 1 2-2h8" />
    </>
  ),
  check: <path d="m5 12.5 4.5 4.5L19 7.5" />,
  external: <path d="M14 5h5v5m0-5-8 8M18 14v4a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h4" />,
  back: <path d="M19 12H5m0 0 6-6m-6 6 6 6" />,
};

export function Icon({ name, className = "size-5", ...rest }: { name: IconName } & SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className={className}
      {...rest}
    >
      {PATHS[name]}
    </svg>
  );
}
