<script>
  import { onMount } from 'svelte';
  import { get, post, put, del, uploadFile } from '../lib/api.js';
  import { showToast } from '../lib/stores.js';
  import { fade, slide } from 'svelte/transition';

  let providers = [];
  let loading = true;

  onMount(load);

  // Modal state
  let showModal = false;
  let modalMode = 'create'; // 'create' | 'edit' | 'import'
  let editId = null;
  let form = { name: '', provider_type: 'vless', config: '{}', enabled: true, priority: 100, health_host: '' };
  let configError = '';

  // Import state
  let importFile = null;
  let importText = '';
  let importName = '';

  async function load() {
    try {
      const data = await get('/providers');
      providers = data.providers || [];
      loading = false;
    } catch (e) {
      loading = false;
    }
  }

  async function loadEdit(id) {
    try {
      const p = await get('/providers/' + id);
      editId = id;
      form = {
        name: p.name,
        provider_type: p.provider_type,
        config: JSON.stringify(p.config || {}, null, 2),
        enabled: p.enabled,
        priority: p.priority,
        health_host: p.health_host || '',
      };
      modalMode = 'edit';
      showModal = true;
    } catch (e) {
      showToast('Ошибка загрузки: ' + e.message, 'error');
    }
  }

  function openCreate() {
    editId = null;
    form = { name: '', provider_type: 'wireguard', config: '{}', enabled: true, priority: 100, health_host: '' };
    modalMode = 'create';
    showModal = true;
  }

  function openImport() {
    importFile = null;
    importText = '';
    importName = '';
    modalMode = 'import';
    showModal = true;
  }

  function closeModal() {
    showModal = false;
    configError = '';
  }

  async function save() {
    try {
      JSON.parse(form.config);
    } catch (_) {
      configError = 'Неверный JSON';
      return;
    }
    configError = '';

    const body = {
      name: form.name,
      provider_type: form.provider_type,
      config: JSON.parse(form.config),
      enabled: form.enabled,
      priority: form.priority,
      health_host: form.health_host || undefined,
    };

    try {
      if (modalMode === 'create') {
        await post('/providers', body);
        showToast('✅ Провайдер создан');
      } else {
        await put('/providers/' + editId, body);
        showToast('✅ Провайдер обновлён');
      }
      closeModal();
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function doImport() {
    try {
      if (importFile) {
        const result = await uploadFile('/providers/import', importFile, { name: importName || undefined });
        showToast(`✅ Импортирован (${result.detected || '?'})`);
      } else if (importText.trim()) {
        const result = await post('/providers/import', { config_text: importText.trim(), name: importName || undefined });
        showToast(`✅ Импортирован (${result.detected || '?'})`);
      } else {
        showToast('Выберите файл или введите текст', 'error');
        return;
      }
      closeModal();
      load();
    } catch (e) {
      showToast('Ошибка импорта: ' + e.message, 'error');
    }
  }

  async function toggleProvider(id, enabled) {
    try {
      await put('/providers/' + id, { enabled });
      showToast(enabled ? '✅ Включён' : '⏹ Выключен');
      try { await post('/vpn/sync'); } catch (_) {}
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function deleteProvider(id, name) {
    if (!confirm(`Удалить провайдера "${name}"?`)) return;
    try {
      await del('/providers/' + id);
      showToast('🗑 Провайдер удалён');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function syncAll() {
    try {
      await post('/vpn/sync');
      showToast('🔄 Провайдеры синхронизированы');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  const typeExamples = {
    wireguard: '{\n  "private_key": "...",\n  "peer": {\n    "public_key": "...",\n    "endpoint": "example.com:51820",\n    "allowed_ips": ["0.0.0.0/0"]\n  }\n}',
    amneziawg: '{\n  "private_key": "...",\n  "address": "10.0.0.2/32",\n  "junk_packet_count": 3,\n  "peer": {\n    "public_key": "...",\n    "endpoint": "example.com:51820",\n    "allowed_ips": ["0.0.0.0/0"]\n  }\n}',
    vless: '{\n  "server": "example.com",\n  "server_port": 443,\n  "uuid": "...",\n  "tls": { "enabled": true, "server_name": "example.com" }\n}',
    hysteria2: '{\n  "server": "example.com",\n  "server_port": 443,\n  "password": "...",\n  "tls": { "enabled": true }\n}',
    tuic: '{\n  "server": "example.com",\n  "server_port": 443,\n  "token": "...",\n  "tls": { "enabled": true }\n}',
    shadowsocks: '{\n  "server": "example.com",\n  "server_port": 443,\n  "method": "aes-256-gcm",\n  "password": "..."\n}',
  };

  function updateExample() {
    form.config = typeExamples[form.provider_type] || '{}';
  }
</script>

<svelte:window onkeydown={e => e.key === 'Escape' && closeModal()} />

<div class="page">
  <div class="page-header">
    <h1>🔌 Провайдеры</h1>
    <div class="header-actions">
      <button class="btn btn-ghost" onclick={syncAll}>🔄 Синхронизировать</button>
      <button class="btn btn-ghost" onclick={openImport}>📂 Загрузить .conf</button>
      <button class="btn btn-primary" onclick={openCreate}>+ Добавить</button>
    </div>
  </div>

  {#if loading}
    <div class="loading-pulse"><div class="spinner"></div> Загрузка...</div>
  {:else if providers.length === 0}
    <div class="empty-state">
      <p>Нет провайдеров</p>
      <div class="empty-actions">
        <button class="btn btn-primary" onclick={openCreate}>+ Добавить</button>
        <button class="btn btn-ghost" onclick={openImport}>📂 Загрузить .conf</button>
      </div>
    </div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr><th>Имя</th><th>Тип</th><th>Статус</th><th>Приоритет</th><th></th></tr>
        </thead>
        <tbody>
          {#each providers as p (p.id)}
            <tr transition:fade>
              <td class="text-bold">{p.name}</td>
              <td><span class="badge badge-type">{p.provider_type}</span></td>
              <td>
                <button class="toggle-btn" class:on={p.enabled} onclick={() => toggleProvider(p.id, !p.enabled)}>
                  {p.enabled ? '🟢 Вкл' : '🔴 Выкл'}
                </button>
              </td>
              <td>{p.priority}</td>
              <td class="actions">
                <button class="btn btn-sm btn-ghost" onclick={() => toggleProvider(p.id, !p.enabled)} title={p.enabled ? 'Выключить' : 'Включить'}>
                  {p.enabled ? '⏹' : '▶️'}
                </button>
                <button class="btn btn-sm btn-ghost" onclick={() => loadEdit(p.id)}>✏️</button>
                <button class="btn btn-sm btn-danger" onclick={() => deleteProvider(p.id, p.name)}>🗑</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Modal -->
{#if showModal}
  <div class="modal-overlay" transition:fade onclick={closeModal} role="dialog">
    <div class="modal-content" onclick={e => e.stopPropagation()} transition:slide>
      <!-- Create / Edit -->
      {#if modalMode !== 'import'}
        <div class="modal-header">
          <h3>{modalMode === 'create' ? 'Новый провайдер' : 'Редактировать'}</h3>
          <button class="modal-close" onclick={closeModal}>&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-row">
            <label>Имя</label>
            <input type="text" bind:value={form.name} placeholder="my-vpn" />
          </div>
          <div class="form-row">
            <label>Тип</label>
            <select bind:value={form.provider_type} onchange={updateExample}>
              <option value="amneziawg">AmneziaWG</option>
              <option value="wireguard">WireGuard</option>
              <option value="vless">VLESS</option>
              <option value="hysteria2">Hysteria2</option>
              <option value="tuic">TUIC</option>
              <option value="shadowsocks">Shadowsocks</option>
            </select>
          </div>
          <div class="form-row">
            <label>Конфигурация (JSON)</label>
            <textarea bind:value={form.config} rows="8" spellcheck="false"></textarea>
            {#if configError}
              <span class="field-error">{configError}</span>
            {/if}
          </div>
          <div class="form-row-inline">
            <label>
              <input type="checkbox" bind:checked={form.enabled} /> Включён
            </label>
          </div>
          <div class="form-row-inline">
            <label>Приоритет</label>
            <input type="number" bind:value={form.priority} class="input-sm" />
          </div>
          <button class="btn btn-primary full-width" onclick={save}>
            {modalMode === 'create' ? 'Создать' : 'Сохранить'}
          </button>
        </div>

      {:else}
        <!-- Import -->
        <div class="modal-header">
          <h3>📂 Импорт .conf</h3>
          <button class="modal-close" onclick={closeModal}>&times;</button>
        </div>
        <div class="modal-body">
          <p class="modal-desc">Загрузите WireGuard или AmneziaWG конфиг. Тип определится автоматически.</p>
          <div class="form-row">
            <label>Файл конфигурации</label>
            <input type="file" accept=".conf,.txt" onchange={e => importFile = e.target.files[0]} />
          </div>
          <div class="form-divider"><span>или</span></div>
          <div class="form-row">
            <label>Вставьте текст конфига</label>
            <textarea bind:value={importText} rows="6" placeholder="[Interface]&#10;PrivateKey = ...&#10;Address = 10.0.0.2/32&#10;&#10;[Peer]&#10;PublicKey = ...&#10;Endpoint = ..." spellcheck="false"></textarea>
          </div>
          <div class="form-row">
            <label>Название (оставьте пустым для автогенерации)</label>
            <input type="text" bind:value={importName} placeholder="my-vpn" />
          </div>
          <button class="btn btn-primary full-width" onclick={doImport}>📂 Импортировать</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .page { max-width: 1000px; }
  h1 { font-size: 28px; font-weight: 700; }
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 24px;
  }
  .header-actions { display: flex; gap: 8px; }

  .btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 9px 16px;
    border: none;
    border-radius: 10px;
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    font-family: inherit;
  }
  .btn-primary { background: #7c5cfc; color: #fff; }
  .btn-primary:hover { background: #6b4de6; transform: translateY(-1px); }
  .btn-ghost { background: transparent; color: #888; border: 1px solid #2a2a4a; }
  .btn-ghost:hover { background: #1e1e38; color: #ccc; }
  .btn-danger { background: transparent; color: #f87171; border: 1px solid #2a2a4a; }
  .btn-danger:hover { background: rgba(248,113,113,0.1); }
  .btn-sm { padding: 5px 10px; font-size: 12px; }
  .full-width { width: 100%; justify-content: center; margin-top: 8px; }

  .table-wrap {
    background: #16162b;
    border: 1px solid #2a2a4a;
    border-radius: 12px;
    overflow: hidden;
  }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 12px 16px; font-size: 13px; border-bottom: 1px solid #2a2a4a; }
  th { color: #666; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
  tr:last-child td { border-bottom: none; }
  tr:hover td { background: rgba(124, 92, 252, 0.04); }
  .text-bold { font-weight: 600; }
  .actions { display: flex; gap: 4px; justify-content: flex-end; }

  .toggle-btn {
    padding: 3px 10px;
    border: 1px solid #2a2a4a;
    border-radius: 6px;
    background: transparent;
    color: #f87171;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
    font-family: inherit;
  }
  .toggle-btn.on { color: #4ade80; border-color: #4ade8040; background: rgba(74,222,128,0.08); }
  .toggle-btn:hover { filter: brightness(1.2); }

  .badge-type { background: rgba(124, 92, 252, 0.1); color: #9b7fff; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: 500; }

  /* Modal */
  .modal-overlay {
    position: fixed; inset: 0;
    background: rgba(0,0,0,0.6);
    display: flex; align-items: center; justify-content: center;
    z-index: 100;
    backdrop-filter: blur(4px);
  }
  .modal-content {
    background: #1a1a30;
    border: 1px solid #2a2a4a;
    border-radius: 16px;
    width: 90%;
    max-width: 540px;
    max-height: 85vh;
    overflow-y: auto;
    box-shadow: 0 8px 40px rgba(0,0,0,0.5);
  }
  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 20px 24px 0;
  }
  .modal-header h3 { font-size: 18px; }
  .modal-close { background: none; border: none; color: #666; font-size: 28px; cursor: pointer; padding: 0 4px; }
  .modal-close:hover { color: #ccc; }
  .modal-body { padding: 16px 24px 24px; }
  .modal-desc { font-size: 13px; color: #888; margin-bottom: 16px; line-height: 1.5; }
  .form-divider { text-align: center; color: #444; margin: 8px 0; font-size: 12px; }
  .form-divider span { background: #1a1a30; padding: 0 8px; }

  .form-row { margin-bottom: 14px; }
  .form-row label { display: block; font-size: 12px; color: #888; margin-bottom: 4px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.3px; }
  .form-row input, .form-row select, .form-row textarea {
    width: 100%; padding: 10px 12px;
    background: #121224; border: 1px solid #2a2a4a;
    border-radius: 8px; color: #e0e0e0;
    font-size: 13px; font-family: inherit;
    outline: none;
  }
  .form-row input:focus, .form-row select:focus, .form-row textarea:focus { border-color: #7c5cfc; }
  .form-row textarea { font-family: 'SF Mono', 'Fira Code', monospace; resize: vertical; min-height: 100px; }
  .form-row input[type="file"] { padding: 8px; }
  .form-row-inline { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
  .form-row-inline label { font-size: 13px; color: #ccc; cursor: pointer; display: flex; align-items: center; gap: 6px; }
  .input-sm { width: 80px; padding: 6px 8px; background: #121224; border: 1px solid #2a2a4a; border-radius: 6px; color: #e0e0e0; }
  .field-error { color: #f87171; font-size: 12px; margin-top: 4px; display: block; }

  .empty-state { text-align: center; padding: 60px 40px; color: #666; }
  .empty-state p { margin-bottom: 16px; font-size: 15px; }
  .empty-actions { display: flex; gap: 8px; justify-content: center; }

  .loading-pulse { display: flex; align-items: center; gap: 12px; color: #666; padding: 40px; }
  .spinner { width: 20px; height: 20px; border: 2px solid #2a2a4a; border-top-color: #7c5cfc; border-radius: 50%; animation: spin 0.6s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
