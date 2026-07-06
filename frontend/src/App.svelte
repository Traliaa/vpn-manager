<script>
  import { currentPage, navigate, toastMessage } from './lib/stores.js';
  import Dashboard from './pages/Dashboard.svelte';
  import Providers from './pages/Providers.svelte';
  import Profiles from './pages/Profiles.svelte';
  import Rules from './pages/Rules.svelte';
  import Routing from './pages/Routing.svelte';
  import Toast from './components/Toast.svelte';
  import { fade, slide } from 'svelte/transition';
</script>

<div class="app-shell">
  <aside class="sidebar">
    <div class="logo">VPN Manager</div>
    <nav>
      <button class="nav-item" class:active={$currentPage === 'dashboard'} onclick={() => navigate('dashboard')}>
        <span class="nav-icon">📊</span> Дашборд
      </button>
      <button class="nav-item" class:active={$currentPage === 'providers'} onclick={() => navigate('providers')}>
        <span class="nav-icon">🔌</span> Провайдеры
      </button>
      <button class="nav-item" class:active={$currentPage === 'profiles'} onclick={() => navigate('profiles')}>
        <span class="nav-icon">📋</span> Профили
      </button>
      <button class="nav-item" class:active={$currentPage === 'routing'} onclick={() => navigate('routing')}>
        <span class="nav-icon">🌐</span> Маршрутизация
      </button>
      <a href="http://vpn-manager.etk3.xyz:3001" target="_blank" class="nav-item" rel="noreferrer">
        <span class="nav-icon">📊</span> Мониторинг ↗
      </a>
    </nav>
    <div class="sidebar-footer">
      <div class="status-dot" aria-label="Система работает"></div>
      <span class="status-text">Система работает</span>
    </div>
  </aside>

  <main class="content">
    {#if $currentPage === 'dashboard'}
      <div transition:slide><Dashboard /></div>
    {:else if $currentPage === 'providers'}
      <div transition:slide><Providers /></div>
    {:else if $currentPage === 'profiles'}
      <div transition:slide><Profiles /></div>
    {:else if $currentPage === 'rules'}
      <div transition:slide><Rules /></div>
    {:else if $currentPage === 'routing'}
      <div transition:slide><Routing /></div>
    {/if}
  </main>
</div>

<Toast />

<style>
  :global(*) { margin: 0; padding: 0; box-sizing: border-box; }
  :global(body) {
    font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
    background: #0f0f1a;
    color: #e0e0e0;
    line-height: 1.5;
    overflow: hidden;
  }

  .app-shell { display: flex; height: 100vh; }

  .sidebar {
    width: 240px;
    background: #16162b;
    border-right: 1px solid #2a2a4a;
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
  }

  .logo {
    font-size: 20px;
    font-weight: 700;
    color: #7c5cfc;
    padding: 24px 20px;
    border-bottom: 1px solid #2a2a4a;
    letter-spacing: -0.5px;
  }

  nav { flex: 1; padding: 12px 8px; display: flex; flex-direction: column; gap: 2px; }

  .nav-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    border: none;
    border-radius: 8px;
    background: transparent;
    color: #888;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s;
    text-align: left;
    width: 100%;
    font-family: inherit;
    text-decoration: none;
  }
  .nav-item:hover { background: rgba(124, 92, 252, 0.08); color: #ccc; }
  .nav-item.active {
    background: rgba(124, 92, 252, 0.15);
    color: #9b7fff;
  }
  .nav-icon { font-size: 18px; }

  .sidebar-footer {
    padding: 16px 20px;
    border-top: 1px solid #2a2a4a;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: #666;
  }
  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #4ade80;
    box-shadow: 0 0 6px rgba(74, 222, 128, 0.5);
    animation: pulse 2s infinite;
  }
  @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
  .status-text { color: #4ade80; }

  .content {
    flex: 1;
    overflow-y: auto;
    padding: 32px;
  }
</style>
