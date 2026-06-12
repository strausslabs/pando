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

export function Mark({ size = 26 }: { size?: number }) {
  return (
    <svg className="mark" width={size} height={size} viewBox="0 0 64 64" fill="none" role="img" aria-label="Pando">
      <defs>
        <linearGradient id="markStem" x1="0" y1="54" x2="0" y2="14">
          <stop offset="0" stopColor="#14b8a6" />
          <stop offset="1" stopColor="#5eead4" />
        </linearGradient>
      </defs>

      <g stroke="#2c443c" strokeWidth="2.4" strokeLinecap="round" fill="none">
        <path d="M32 50 Q26 56 21 59" />
        <path d="M32 50 Q38 56 43 59" />
        <path d="M32 50 Q32 56 32 61" />
      </g>

      <g className="mark-canopy" stroke="url(#markStem)" strokeLinecap="round" fill="none">
        <path d="M32 50 Q33 38 30 28" strokeWidth="4" />
        <path d="M31 40 Q24 36 18 30" strokeWidth="3" />
        <path d="M31 34 Q39 30 45 25" strokeWidth="3" />
        <path d="M30 30 Q25 24 21 19" strokeWidth="2.6" />
        <path d="M30 30 Q35 24 39 18" strokeWidth="2.6" />
      </g>

      <g className="mark-tips">
        <circle cx="30" cy="28" r="3.6" fill="#5eead4" />
        <circle cx="18" cy="30" r="3" fill="#4ade80" />
        <circle cx="45" cy="25" r="3" fill="#38bdf8" />
        <circle cx="21" cy="19" r="2.6" fill="#2dd4bf" />
        <circle cx="39" cy="18" r="2.6" fill="#4ade80" />
      </g>

      <circle cx="32" cy="50" r="3.6" fill="#e6efea" />
    </svg>
  );
}
