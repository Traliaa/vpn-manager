<script>
  import { toastMessage } from '../lib/stores.js';
  import { fade, fly } from 'svelte/transition';
</script>

{#if $toastMessage}
  <div class="toast-wrapper" transition:fly={{ y: 20, duration: 300 }}>
    <div class="toast" class:success={$toastMessage.type === 'success'} class:error={$toastMessage.type === 'error'}>
      <span class="toast-icon">{ $toastMessage.type === 'success' ? '✅' : '❌' }</span>
      <span>{ $toastMessage.text }</span>
    </div>
  </div>
{/if}

<style>
  .toast-wrapper {
    position: fixed;
    bottom: 24px;
    right: 24px;
    z-index: 1000;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 20px;
    border-radius: 12px;
    background: #1e1e38;
    border: 1px solid #2a2a4a;
    box-shadow: 0 4px 20px rgba(0,0,0,0.4);
    font-size: 14px;
    font-weight: 500;
    min-width: 200px;
    backdrop-filter: blur(12px);
  }
  .toast.success { border-color: #4ade80; }
  .toast.error { border-color: #f87171; }
  .toast-icon { font-size: 16px; }
</style>
