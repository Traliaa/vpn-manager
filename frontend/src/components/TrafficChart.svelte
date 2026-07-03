<script>
  let { data = [], color = '#7c5cfc', label = 'Трафик' } = $props();
  const W = 400, H = 100;
</script>

<div class="chart-box">
  <div class="chart-label">{label}</div>
  <svg width={W} height={H} viewBox="0 0 {W} {H}" class="chart-svg">
    {#if data.length > 1}
      {@const max = Math.max(...data, 1)}
      {@const stepX = W / (data.length - 1)}
      {@const points = data.map((v, i) => `${i * stepX},${H - (v / max) * H * 0.8 - 5}`).join(' ')}
      {@const gradientId = 'grad-' + Math.random().toString(36).slice(2)}
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={color} stop-opacity="0.3" />
          <stop offset="100%" stop-color={color} stop-opacity="0" />
        </linearGradient>
      </defs>
      <!-- Area -->
      <path d="M0,{H} {points.split(' ').map((p,i) => {
        const [x,y] = p.split(',');
        return (i === 0 ? `L${x},${y}` : `L${x},${y}`);
      }).join(' ')} L{W},{H} Z" fill="url(#{gradientId})" />
      <!-- Line -->
      <polyline points={points} fill="none" stroke={color} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      <!-- Dots -->
      {#each data as v, i}
        <circle cx={i * stepX} cy={H - (v / max) * H * 0.8 - 5} r="2.5" fill={color} stroke="#0f0f1a" stroke-width="1.5">
          <title>{v.toFixed(1)} MB</title>
        </circle>
      {/each}
    {:else}
      <text x={W/2} y={H/2} text-anchor="middle" fill="#444" font-size="12">Нет данных</text>
    {/if}
  </svg>
</div>

<style>
  .chart-box {
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
    padding: 16px;
  }
  .chart-label {
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: #888;
    font-weight: 600;
    margin-bottom: 8px;
  }
  .chart-svg { width: 100%; height: auto; }
</style>
