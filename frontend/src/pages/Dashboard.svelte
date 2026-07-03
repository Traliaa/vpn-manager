<script>
  import { onMount, onDestroy } from 'svelte';
  import { get, post, put } from '../lib/api.js';
  import { showToast, navigate } from '../lib/stores.js';
  import StatusCard from '../components/StatusCard.svelte';
  import TrafficChart from '../components/TrafficChart.svelte';

  let providers = [];
  let interfaces = [];
  let routingActive = false;
  let routingProfile = null;
  let health = {};
  let loading = true;

  let txHistory = [];
  let rxHistory = [];
  let autoRefresh = null;

  const MAX_POINTS = 30;

  async function load() {
    try {
      const [provData, vpnData, routeData, healthData] = await Promise.all([
        get('/providers'),
        get('/vpn/interfaces'),
        get('/routing/status'),
        get('/health'),
      ]);
      providers = provData.providers || [];
      interfaces = vpnData.interfaces || [];
      routingActive = routeData.active;
      routingProfile = routeData.profile || null;
      health = healthData;
      loading = false;

      // Traffic tracking
      const totalTx = interfaces.reduce((s, i) => s + (i.tx_bytes || 0), 0);
      const totalRx = interfaces.reduce((s, i) => s + (i.rx_bytes || 0), 0);
      txHistory = [...txHistory.slice(-(MAX_POINTS - 1)), totalTx / 1024 / 1024];
      rxHistory = [...rxHistory.slice(-(MAX_POINTS - 1)), totalRx / 1024 / 1024];
    } catch (e) {
      loading = false;
    }
  }

  onMount(() => {
    load();
    autoRefresh = setInterval(load, 5000);
  });

  onDestroy(() => {
    if (autoRefresh) clearInterval(autoRefresh);
  });

  // Quick route
  let qrDomain = '';
  let qrProvider = '';

  async function addQuickRoute() {
    if (!qrDomain.trim()) { showToast('Введите сайт', 'error'); return; }
    if (!qrProvider) { showToast('Выберите VPN', 'error'); return; }

    try {
      let profiles = (await get('/profiles')).profiles || [];
      let profile = profiles.find(p => p.name === 'Default');
      if (!profile) {
        profile = await post('/profiles', { name: 'Default', description: 'Авто-профиль', is_default: true });
      }

      await post('/rules', {
        profile_id: profile.id,
        provider_id: qrProvider,
        rule_type: 'domain',
        value: qrDomain.trim(),
        enabled: true,
      });

      await post(`/profiles/${profile.id}/activate`);
      showToast(`✅ ${qrDomain} → VPN активен`);
      qrDomain = '';
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function toggleRouting() {
    try {
      if (routingActive) {
        await post('/routing/deactivate');
        showToast('⏹ Маршрутизация выключена');
      }
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  $: upCount = interfaces.filter(i => i.state === 'up').length;
  $: downCount = interfaces.filter(i => i.state === 'down' || i.state === 'error').length;
  $: enabledProviders = providers.filter(p => p.enabled);
</script>

<div class="page">
  <h1>Дашборд</h1>

  {#if loading}
    <div class="loading-pulse"><div class="spinner"></div> Загрузка...</div>
  {:else}
    <!-- Status Cards -->
    <div class="cards-grid">
      <StatusCard title="Провайдеры" value={interfaces.length} sub="Всего зарегистрировано" color="#7c5cfc" icon="🔌" />
      <StatusCard title="Активны" value={upCount} sub="VPN интерфейсов в работе" color="#4ade80" icon="✅" />
      <StatusCard title="Маршрутизация" value={routingActive ? 'Активна' : 'Неактивна'} sub={routingProfile ? `Профиль: ${routingProfile.name}` : ''} color={routingActive ? '#4ade80' : '#f87171'} icon={routingActive ? '🛡️' : '🔓'} />
      <StatusCard title="База данных" value={health.database === 'connected' ? 'Подключена' : 'Ошибка'} sub="PostgreSQL" color={health.database === 'connected' ? '#4ade80' : '#f87171'} icon="🗄️" />
    </div>

    <!-- Traffic Charts -->
    <div class="charts-row">
      <TrafficChart data={txHistory} color="#7c5cfc" label="TX (MB)" />
      <TrafficChart data={rxHistory} color="#4ade80" label="RX (MB)" />
    </div>

    <!-- Quick Route -->
    <div class="section">
      <h2>⚡ Быстрая маршрутизация</h2>
      <div class="quick-route-box">
        <input
          type="text"
          class="input"
          placeholder="Введите сайт, например youtube.com"
          bind:value={qrDomain}
          onkeydown={e => e.key === 'Enter' && addQuickRoute()}
        />
        <select class="input select" bind:value={qrProvider}>
          <option value="">Выберите VPN</option>
          {#each enabledProviders as p}
            <option value={p.id}>{p.name} ({p.provider_type})</option>
          {/each}
        </select>
        <button class="btn btn-primary" onclick={addQuickRoute}>➕ Добавить</button>
        {#if routingActive}
          <button class="btn btn-ghost" onclick={toggleRouting}>⏹ Выключить</button>
        {/if}
      </div>
    </div>

    <!-- Interface Status -->
    <div class="section">
      <h2>Состояние провайдеров</h2>
      {#if interfaces.length}
        <div class="table-wrap">
          <table>
            <thead>
              <tr><th>Имя</th><th>Тип</th><th>Статус</th><th>TX</th><th>RX</th></tr>
            </thead>
            <tbody>
              {#each interfaces as i}
                <tr>
                  <td class="text-bold">{i.name}</td>
                  <td><span class="badge badge-type">{i.type}</span></td>
                  <td>
                    <span class="badge" class:badge-up={i.state === 'up'} class:badge-down={i.state === 'down' || i.state === 'error'}>
                      {i.state === 'up' ? '🟢 В работе' : i.state === 'down' ? '🔴 Остановлен' : '⚠️ Ошибка'}
                    </span>
                  </td>
                  <td class="text-mono">{fmtBytes(i.tx_bytes)}</td>
                  <td class="text-mono">{fmtBytes(i.rx_bytes)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <div class="empty-state">
          <p>Нет зарегистрированных провайдеров</p>
          <button class="btn btn-primary" onclick={() => navigate('providers')}>🔌 Добавить провайдера</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<script context="module">
  function fmtBytes(b) {
    if (!b || b === 0) return '—';
    if (b >= 1<<30) return (b/(1<<30)).toFixed(1) + ' GiB';
    if (b >= 1<<20) return (b/(1<<20)).toFixed(1) + ' MiB';
    if (b >= 1<<10) return (b/(1<<10)).toFixed(1) + ' KiB';
    return b + ' B';
  }
</script>

<style>
  .page { max-width: 1100px; }
  h1 { font-size: 28px; font-weight: 700; margin-bottom: 24px; }
  h2 { font-size: 18px; font-weight: 600; margin-bottom: 16px; }

  .cards-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 12px;
    margin-bottom: 20px;
  }

  .charts-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 24px;
  }

  .section { margin-bottom: 24px; }

  .quick-route-box {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .input {
    padding: 10px 14px;
    border: 1px solid #2a2a4a;
    border-radius: 10px;
    background: #16162b;
    color: #e0e0e0;
    font-size: 14px;
    font-family: inherit;
    outline: none;
    transition: border-color 0.2s;
  }
  .input:focus { border-color: #7c5cfc; }
  .input::placeholder { color: #555; }
  .select { min-width: 200px; cursor: pointer; }

  .btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 10px 18px;
    border: none;
    border-radius: 10px;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    font-family: inherit;
    white-space: nowrap;
  }
  .btn-primary { background: #7c5cfc; color: #fff; }
  .btn-primary:hover { background: #6b4de6; transform: translateY(-1px); }
  .btn-ghost { background: transparent; color: #888; border: 1px solid #2a2a4a; }
  .btn-ghost:hover { background: #1e1e38; color: #ccc; }

  .table-wrap {
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
    overflow: hidden;
  }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 12px 16px; font-size: 14px; border-bottom: 1px solid #2a2a4a; }
  th { color: #666; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
  tr:last-child td { border-bottom: none; }
  tr:hover td { background: rgba(124, 92, 252, 0.05); }

  .text-bold { font-weight: 600; }
  .text-mono { font-family: 'SF Mono', monospace; font-size: 13px; color: #aaa; }

  .badge {
    display: inline-block;
    padding: 3px 10px;
    border-radius: 6px;
    font-size: 12px;
    font-weight: 600;
  }
  .badge-up { background: rgba(74, 222, 128, 0.12); color: #4ade80; }
  .badge-down { background: rgba(248, 113, 113, 0.12); color: #f87171; }
  .badge-type { background: rgba(124, 92, 252, 0.1); color: #9b7fff; }

  .empty-state {
    text-align: center;
    padding: 40px;
    color: #666;
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
  }
  .empty-state p { margin-bottom: 16px; }

  .loading-pulse {
    display: flex;
    align-items: center;
    gap: 12px;
    color: #666;
    padding: 40px;
  }
  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid #2a2a4a;
    border-top-color: #7c5cfc;
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
