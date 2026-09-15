<script lang="ts">
  let {
    size = "md",
    class: className = "",
  }: {
    size?: "sm" | "md" | "lg";
    class?: string;
  } = $props();

  const sizes = {
    sm: { box: "h-8 w-8", svg: 28 },
    md: { box: "h-10 w-10", svg: 34 },
    lg: { box: "h-14 w-14", svg: 48 },
  };

  let s = $derived(sizes[size]);
</script>

<div class="relative flex items-center justify-center {s.box} {className}">
  <svg viewBox="0 0 64 64" width={s.svg} height={s.svg}>
    <!-- Lantern body: paper with warm glow -->
    <rect
      x="22"
      y="11"
      width="20"
      height="28"
      rx="5"
      fill="#fef3c7"
      stroke="#f59e0b"
      stroke-width="0.8"
      opacity="0.95"
    />
    <!-- Inner glow -->
    <rect
      x="24.5"
      y="14"
      width="15"
      height="22"
      rx="3.5"
      fill="#f59e0b"
      opacity="0.2"
    />
    <!-- Vertical ribs -->
    <line
      x1="27"
      y1="11"
      x2="27"
      y2="39"
      stroke="#f59e0b"
      stroke-width="0.5"
      opacity="0.2"
    />
    <line
      x1="32"
      y1="11"
      x2="32"
      y2="39"
      stroke="#f59e0b"
      stroke-width="0.5"
      opacity="0.2"
    />
    <line
      x1="37"
      y1="11"
      x2="37"
      y2="39"
      stroke="#f59e0b"
      stroke-width="0.5"
      opacity="0.2"
    />
    <!-- Top cap -->
    <rect x="26" y="8" width="12" height="3" rx="1.5" fill="#334155" />
    <!-- Cord -->
    <line x1="32" y1="3" x2="32" y2="8" stroke="#334155" stroke-width="1" />
    <!-- Bottom cap -->
    <rect x="26" y="39" width="12" height="3" rx="1.5" fill="#334155" />
    <!-- Tassel -->
    <line
      x1="32"
      y1="42"
      x2="32"
      y2="49"
      stroke="#f59e0b"
      stroke-width="0.8"
      opacity="0.6"
    />
    <circle cx="32" cy="51" r="1.8" fill="#f59e0b" opacity="0.5" />
  </svg>
</div>
