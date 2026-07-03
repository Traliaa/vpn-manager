<script>
  let { title, value, sub, color = '#888', icon = '' } = $props();
</script>

<div class="card" style="--card-accent: {color}">
  <div class="card-header">
    <span class="card-icon">{icon}</span>
    <span class="card-title">{title}</span>
  </div>
  <div class="card-value">{value}</div>
  {#if sub}
    <div class="card-sub">{sub}</div>
  {/if}
</div>

<style>
  .card {
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
    padding: 20px;
    transition: all 0.2s;
  }
  .card:hover {
    border-color: var(--card-accent);
    transform: translateY(-1px);
    box-shadow: 0 4px 12px rgba(0,0,0,0.3);
  }
  .card-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
  }
  .card-icon { font-size: 20px; }
  .card-title {
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: #888;
    font-weight: 600;
  }
  .card-value {
    font-size: 28px;
    font-weight: 700;
    color: var(--card-accent);
  }
  .card-sub {
    font-size: 12px;
    color: #666;
    margin-top: 4px;
  }
</style>
