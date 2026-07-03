<script>
  import { onMount } from 'svelte';
  import { get, post } from '../lib/api.js';
  import { showToast } from '../lib/stores.js';
  import { fade, slide } from 'svelte/transition';

  let loading = true;
  let active = false;
  let profile = null;

  onMount(load);

  async function load() {
    try {
      const data = await get('/routing/status');
      active = data.active;
      profile = data.profile || null;
      loading = false;
    } catch (e) {
      loading = false;
    }
  }

  async function deactivate() {
    try {
      await post('/routing/deactivate');
      showToast('⏹ Маршрутизация выключена');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function reapply() {
    try {
      await post('/routing/reapply');
      showToast('🔄 Маршрутизация переприменена');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }
</script>

<div class="page">
  <h1>🌐 Маршрутизация</h1>

  {#if loading}
    <div class="loading-pulse"><div class="spinner"></div></div>
  {:else}
    <div class="cards-grid">
      <div class="status-card" class:active>
        <div class="status-icon">{active ? '🛡️' : '🔓'}</div>
        <div class="status-info">
          <div class="status-label">Статус</div>
          <div class="status-value">{active ? 'Активна' : 'Неактивна'}</div>
        </div>
        <div class="status-indicator" class:on={active}></div>
      </div>

      {#if profile}
        <div class="status-card">
          <div class="status-icon">📋</div>
          <div class="status-info">
            <div class="status-label">Активный профиль</div>
            <div class="status-value">{profile.name}</div>
            {#if profile.description}
              <div class="status-desc">{profile.description}</div>
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <div class="actions-box">
      {#if active}
        <p class="active-note">Трафик маршрутизируется согласно правилам активного профиля.</p>
      {:else}
        <p class="inactive-note">Весь трафик идёт напрямую. Активируйте профиль на странице <a href="#profiles" class="link">Профили</a> или используйте быструю маршрутизацию на дашборде.</p>
      {/if}

      <div class="action-buttons">
        {#if active}
          <button class="btn btn-danger" onclick={deactivate}>⏹ Деактивировать</button>
          <button class="btn btn-primary" onclick={reapply}>🔄 Переприменить</button>
        {:else}
          <a href="#profiles" class="btn btn-primary">📋 Перейти к профилям</a>
          <a href="#dashboard" class="btn btn-ghost">📊 На дашборд</a>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .page { max-width: 800px; }
  h1 { font-size: 28px; font-weight: 700; margin-bottom: 24px; }

  .cards-grid { display: flex; gap: 12px; margin-bottom: 24px; }

  .status-card {
    flex: 1;
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
    padding: 20px;
    display: flex;
    align-items: center;
    gap: 16px;
    position: relative;
  }
  .status-card.active { border-color: #4ade8040; }
  .status-icon { font-size: 32px; }
  .status-info { flex: 1; }
  .status-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; color: #888; font-weight: 600; margin-bottom: 4px; }
  .status-value { font-size: 20px; font-weight: 700; }
  .status-desc { font-size: 13px; color: #888; margin-top: 4px; }
  .status-indicator {
    width: 12px; height: 12px; border-radius: 50%;
    background: #f87171;
    flex-shrink: 0;
  }
  .status-indicator.on { background: #4ade80; box-shadow: 0 0 8px rgba(74,222,128,0.5); animation: pulse 2s infinite; }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }

  .actions-box {
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
    padding: 24px;
  }
  .active-note { color: #4ade80; font-size: 14px; margin-bottom: 16px; }
  .inactive-note { color: #888; font-size: 14px; margin-bottom: 16px; line-height: 1.6; }
  .link { color: #9b7fff; text-decoration: none; }
  .link:hover { text-decoration: underline; }

  .action-buttons { display: flex; gap: 8px; }

  .btn {
    display: inline-flex; align-items: center; gap: 5px;
    padding: 10px 18px; border: none; border-radius: 10px;
    font-size: 14px; font-weight: 600; cursor: pointer;
    transition: all 0.2s; font-family: inherit; text-decoration: none;
  }
  .btn-primary { background: #7c5cfc; color: #fff; }
  .btn-primary:hover { background: #6b4de6; transform: translateY(-1px); }
  .btn-ghost { background: transparent; color: #888; border: 1px solid #2a2a4a; }
  .btn-ghost:hover { background: #1e1e38; color: #ccc; }
  .btn-danger { background: transparent; color: #f87171; border: 1px solid #f8717140; }
  .btn-danger:hover { background: rgba(248,113,113,0.1); }

  .loading-pulse { display: flex; align-items: center; gap: 12px; color: #666; padding: 40px; }
  .spinner { width: 20px; height: 20px; border: 2px solid #2a2a4a; border-top-color: #7c5cfc; border-radius: 50%; animation: spin 0.6s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
