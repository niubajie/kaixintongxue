import type { CSSProperties } from "react";

interface IconProps {
  size?: number;
  color?: string;
  style?: CSSProperties;
}

const base = (size: number, color: string, style?: CSSProperties) => ({
  width: size,
  height: size,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: color,
  strokeWidth: 1.7,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  style
});

export function IconMenu({ size = 24, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M4 7h16M4 12h16M4 17h10" />
    </svg>
  );
}

export function IconSearch({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <circle cx="11" cy="11" r="6.5" />
      <path d="M16 16l4.5 4.5" />
    </svg>
  );
}

export function IconHeadset({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M5 13a7 7 0 0 1 14 0" />
      <rect x="3.4" y="12.6" width="3.4" height="5.6" rx="1.7" />
      <rect x="17.2" y="12.6" width="3.4" height="5.6" rx="1.7" />
      <path d="M19 18.2c0 1.7-1.4 2.8-3.4 2.8" />
    </svg>
  );
}

export function IconSpeaker({ size = 24, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M4 9.5h3l4.5-3.6v12.2L7 14.5H4z" />
      <path d="M15.5 9a4 4 0 0 1 0 6" />
      <path d="M18 6.5a7.5 7.5 0 0 1 0 11" />
    </svg>
  );
}

export function IconCamera({ size = 24, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M3.5 8.5A2 2 0 0 1 5.5 6.5h1.8l1.2-1.8h6l1.2 1.8h1.8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z" />
      <circle cx="11.5" cy="12.5" r="3.2" />
    </svg>
  );
}

export function IconWave({ size = 24, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M9 9.5v5M12 8v8M15 10v4" />
    </svg>
  );
}

export function IconPlus({ size = 24, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M12 8.5v7M8.5 12h7" />
    </svg>
  );
}

export function IconPlusPlain({ size = 20, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M12 5v14M5 12h14" />
    </svg>
  );
}

export function IconClose({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M7 7l10 10M17 7L7 17" />
    </svg>
  );
}

export function IconBack({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M14.5 5.5L8 12l6.5 6.5" />
    </svg>
  );
}

export function IconChat({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M4 6.5h16v9.2H10.5L5.4 19v-3.3H4z" />
    </svg>
  );
}

export function IconFolder({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M3.5 8.2V17h17V8.2" />
      <path d="M3.5 8.2V7h5.6l1.6 1.9h9.8" />
      <path d="M6 10.5v5M9.5 10.5v5M13 10.5v5M16.5 10.5v5" />
    </svg>
  );
}

export function IconTool({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M15.2 3.8a4.4 4.4 0 0 0-5 5.9L4.6 15.3a2 2 0 0 0 2.8 2.8l5.6-5.6a4.4 4.4 0 0 0 5.9-5l-2.6 2.6-2.8-.6-.6-2.8z" />
    </svg>
  );
}

export function IconGear({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M4 7h8M16.5 7H20M4 12h3.5M12 12h8M4 17h8.5M17 17H20" />
      <circle cx="14.2" cy="7" r="2.1" />
      <circle cx="9.7" cy="12" r="2.1" />
      <circle cx="14.6" cy="17" r="2.1" />
    </svg>
  );
}

export function IconRefresh({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M19.5 12a7.5 7.5 0 1 1-2.4-5.5" />
      <path d="M19.5 4.5V8h-3.6" />
    </svg>
  );
}

export function IconClock({ size = 20, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 7.6V12l3 1.8" />
    </svg>
  );
}

export function IconCloud({ size = 20, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M7.5 18h9a3.5 3.5 0 0 0 .3-7A5 5 0 0 0 7.2 10 4 4 0 0 0 7.5 18z" />
    </svg>
  );
}

export function IconMonitor({ size = 20, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <rect x="3.5" y="4.5" width="17" height="11" rx="1.6" />
      <path d="M9 19h6M12 15.5V19" />
    </svg>
  );
}

export function IconBell({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M6.5 10a5.5 5.5 0 0 1 11 0c0 4 1.5 5.2 1.5 5.2H5S6.5 14 6.5 10z" />
      <path d="M10 18.2a2.2 2.2 0 0 0 4 0" />
    </svg>
  );
}

export function IconScan({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M4 8.5V6a2 2 0 0 1 2-2h2.5M15.5 4H18a2 2 0 0 1 2 2v2.5M20 15.5V18a2 2 0 0 1-2 2h-2.5M8.5 20H6a2 2 0 0 1-2-2v-2.5" />
      <path d="M4.5 12h15" />
    </svg>
  );
}

export function IconTrash({ size = 18, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M4.8 7h14.4M9 7V5.2h6V7M6.5 7l.8 12h9.4l.8-12" />
    </svg>
  );
}

export function IconCheck({ size = 16, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)} strokeWidth={2.4}>
      <path d="M5 12.5l4.5 4.5L19 7" />
    </svg>
  );
}

export function IconSpark({ size = 18, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M12 4l1.7 4.6L18.5 10l-4.8 1.4L12 16l-1.7-4.6L5.5 10l4.8-1.4z" />
    </svg>
  );
}

export function IconTarget({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <circle cx="12" cy="12" r="7.5" />
      <circle cx="12" cy="12" r="3.4" />
    </svg>
  );
}

export function IconBook({ size = 22, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M5 5.5h5.2A1.8 1.8 0 0 1 12 7.3v11a1.8 1.8 0 0 0-1.8-1.8H5z" />
      <path d="M19 5.5h-5.2A1.8 1.8 0 0 0 12 7.3v11a1.8 1.8 0 0 1 1.8-1.8H19z" />
    </svg>
  );
}

export function IconDoc({ size = 20, color = "currentColor", style }: IconProps) {
  return (
    <svg {...base(size, color, style)}>
      <path d="M6.5 3.5h7l4 4v13h-11z" />
      <path d="M13.5 3.5v4h4M9 12h6M9 15.5h6" />
    </svg>
  );
}
