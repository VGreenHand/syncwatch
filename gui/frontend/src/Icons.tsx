// 统一线性图标集：只用 stroke，不混用 emoji 与文本符号。
// 尺寸由 CSS 控制（currentColor + 1em），保证与文字基线对齐。
type P = { size?: number; className?: string }

const base = (size = 20) => ({
  width: size,
  height: size,
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round' as const,
  strokeLinejoin: 'round' as const,
})

export const IconPlay = ({ size, className }: P) => (
  <svg {...base(size)} className={className}><path d="M8 5.5v13l11-6.5-11-6.5z" fill="currentColor" stroke="none" /></svg>
)

export const IconPause = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <rect x="7" y="5" width="3.5" height="14" rx="1" fill="currentColor" stroke="none" />
    <rect x="13.5" y="5" width="3.5" height="14" rx="1" fill="currentColor" stroke="none" />
  </svg>
)

export const IconVolume = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <path d="M4 9.5h3.5L12 6v12l-4.5-3.5H4z" /><path d="M16 9a4 4 0 0 1 0 6" />
  </svg>
)

export const IconMute = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <path d="M4 9.5h3.5L12 6v12l-4.5-3.5H4z" /><path d="M16 9.5l4 5M20 9.5l-4 5" />
  </svg>
)

export const IconFullscreen = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <path d="M4 9V4h5M20 15v5h-5M20 9V4h-5M4 15v5h5" />
  </svg>
)

export const IconExitFullscreen = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <path d="M9 4v5H4M15 20v-5h5M15 4v5h5M9 20v-5H4" />
  </svg>
)

export const IconUsers = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <circle cx="9" cy="8" r="3.2" />
    <path d="M3.5 19c0-3 2.5-5 5.5-5s5.5 2 5.5 5" />
    <path d="M16 5.5a3 3 0 0 1 0 5.6M17 14c2 .6 3.5 2.3 3.5 5" />
  </svg>
)

export const IconCopy = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <rect x="9" y="9" width="11" height="11" rx="2" />
    <path d="M15 5.5A2.5 2.5 0 0 0 12.5 3H6a3 3 0 0 0-3 3v6.5A2.5 2.5 0 0 0 5.5 15" />
  </svg>
)

export const IconBack = ({ size, className }: P) => (
  <svg {...base(size)} className={className}><path d="M15 5l-7 7 7 7" /></svg>
)

export const IconSettings = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2 2 2 0 1 1-4 0 1.7 1.7 0 0 0-2.9-1.2l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.7 1.7 0 0 0 4.6 15a2 2 0 1 1 0-4 1.7 1.7 0 0 0 1.2-2.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.7 1.7 0 0 0 11 4.6a2 2 0 1 1 4 0 1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1A1.7 1.7 0 0 0 19.4 11a2 2 0 1 1 0 4z" />
  </svg>
)

export const IconLog = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <path d="M5 4h9l5 5v11a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z" />
    <path d="M14 4v5h5M8 13h6M8 17h4" />
  </svg>
)

export const IconClose = ({ size, className }: P) => (
  <svg {...base(size)} className={className}><path d="M6 6l12 12M18 6L6 18" /></svg>
)

export const IconPlus = ({ size, className }: P) => (
  <svg {...base(size)} className={className}><path d="M12 5v14M5 12h14" /></svg>
)

export const IconArrowRight = ({ size, className }: P) => (
  <svg {...base(size)} className={className}><path d="M5 12h13M13 6l6 6-6 6" /></svg>
)

export const IconFilm = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <rect x="3" y="4" width="18" height="16" rx="2" />
    <path d="M7 4v16M17 4v16M3 9h4M3 15h4M17 9h4M17 15h4" />
  </svg>
)

export const IconLink = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <path d="M10 13a4 4 0 0 0 5.7 0l2.3-2.3a4 4 0 1 0-5.7-5.7L11 6.3" />
    <path d="M14 11a4 4 0 0 0-5.7 0L6 13.3a4 4 0 1 0 5.7 5.7L13 17.7" />
  </svg>
)

export const IconAlert = ({ size, className }: P) => (
  <svg {...base(size)} className={className}>
    <circle cx="12" cy="12" r="9" /><path d="M12 8v5M12 16.2v.2" />
  </svg>
)

export const IconCheck = ({ size, className }: P) => (
  <svg {...base(size)} className={className}><path d="M5 12.5l4.5 4.5L19 7" /></svg>
)
