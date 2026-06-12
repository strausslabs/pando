export function Tree({ size = 200 }: { size?: number }) {
  return (
    <svg
      className="aspen"
      width={size}
      height={size}
      viewBox="0 0 200 220"
      fill="none"
      role="img"
      aria-label="Pando — branch away"
    >
      <defs>
        <linearGradient id="stem" x1="0" y1="200" x2="0" y2="20">
          <stop offset="0" stopColor="#14b8a6" />
          <stop offset="1" stopColor="#5eead4" />
        </linearGradient>
      </defs>

      <path className="grow root r1" d="M100 150 L60 196" stroke="#2c443c" strokeWidth="2.5" strokeLinecap="round" />
      <path className="grow root r2" d="M100 150 L140 196" stroke="#2c443c" strokeWidth="2.5" strokeLinecap="round" />
      <path className="grow root r3" d="M100 150 L100 200" stroke="#2c443c" strokeWidth="2.5" strokeLinecap="round" />

      <path className="grow trunk" d="M100 150 L100 70" stroke="url(#stem)" strokeWidth="5" strokeLinecap="round" />

      <path className="grow b b1" d="M100 110 L66 74" stroke="url(#stem)" strokeWidth="4" strokeLinecap="round" />
      <path className="grow b b2" d="M100 100 L134 66" stroke="url(#stem)" strokeWidth="4" strokeLinecap="round" />
      <path className="grow b b3" d="M100 86 L78 52" stroke="url(#stem)" strokeWidth="3.5" strokeLinecap="round" />
      <path className="grow b b4" d="M100 80 L122 48" stroke="url(#stem)" strokeWidth="3.5" strokeLinecap="round" />

      <circle className="root-node" cx="100" cy="150" r="6" fill="#e6efea" />

      {TIPS.map((t, i) => (
        <circle
          key={i}
          className="leaf-dot"
          cx={t.x}
          cy={t.y}
          r={t.r}
          fill={t.c}
          style={{ animationDelay: `${1 + i * 0.12}s, ${2 + i * 0.2}s` }}
        />
      ))}
    </svg>
  );
}

const TIPS = [
  { x: 66, y: 74, r: 6, c: "#4ade80" },
  { x: 134, y: 66, r: 6, c: "#38bdf8" },
  { x: 78, y: 52, r: 5, c: "#2dd4bf" },
  { x: 122, y: 48, r: 5, c: "#4ade80" },
  { x: 100, y: 70, r: 7, c: "#5eead4" },
];

export function Mark() {
  return (
    <svg className="mark" viewBox="0 0 64 64" fill="none" role="img" aria-label="Pando">
      <defs>
        <linearGradient id="markStem" x1="0" y1="52" x2="0" y2="13">
          <stop offset="0" stopColor="#14b8a6" />
          <stop offset="1" stopColor="#5eead4" />
        </linearGradient>
      </defs>

      <g stroke="#2c443c" strokeWidth="3.6" strokeLinecap="round" fill="none">
        <path d="M32 51 Q22 58 16 62" />
        <path d="M32 51 Q42 58 48 62" />
        <path d="M32 51 L32 63" />
      </g>

      <g className="mark-canopy" stroke="url(#markStem)" strokeLinecap="round" fill="none">
        <path d="M32 52 L32 30" strokeWidth="6.5" />
        <path d="M32 42 L16 27" strokeWidth="5" />
        <path d="M32 42 L48 27" strokeWidth="5" />
        <path d="M32 36 L22 18" strokeWidth="4.6" />
        <path d="M32 36 L42 18" strokeWidth="4.6" />
        <path d="M32 32 L32 13" strokeWidth="4.6" />
      </g>

      <g className="mark-tips">
        <circle cx="16" cy="27" r="5.4" fill="#4ade80" />
        <circle cx="48" cy="27" r="5.4" fill="#38bdf8" />
        <circle cx="22" cy="18" r="4.6" fill="#2dd4bf" />
        <circle cx="42" cy="18" r="4.6" fill="#5eead4" />
        <circle cx="32" cy="13" r="5.4" fill="#4ade80" />
      </g>

      <circle cx="32" cy="51" r="4.6" fill="#e6efea" />
    </svg>
  );
}
