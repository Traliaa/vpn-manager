<script>
  import { onMount } from 'svelte';
  import { get, post, put, del } from '../lib/api.js';
  import { showToast, currentPage } from '../lib/stores.js';
  import { fade, slide } from 'svelte/transition';

  let profileId = '';
  let profileName = '';
  let rules = [];
  let providers = [];
  let loading = true;

  // Modal state
  let showModal = false;
  let editId = null;
  let form = { rule_type: 'domain', value: '', provider_id: '', priority: 500, description: '', enabled: true };

  onMount(() => {
    const params = new URLSearchParams(window.location.hash.split('?')[1] || '');
    profileId = params.get('profileId') || '';
    profileName = params.get('name') || '';
    if (profileId) load();
  });

  async function load() {
    try {
      const [rData, pData] = await Promise.all([
        get('/profiles/' + profileId + '/rules'),
        get('/providers'),
      ]);
      rules = rData.rules || [];
      providers = pData.providers || [];
      loading = false;
    } catch (e) {
      loading = false;
    }
  }

  function openCreate() {
    editId = null;
    form = { rule_type: 'domain', value: '', provider_id: '', priority: 500, description: '', enabled: true };
    showModal = true;
  }

  async function openEdit(id) {
    try {
      const r = await get('/rules/' + id);
      editId = id;
      form = {
        rule_type: r.rule_type,
        value: r.value,
        provider_id: r.provider_id || '',
        priority: r.priority,
        description: r.description || '',
        enabled: r.enabled,
      };
      showModal = true;
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  function closeModal() { showModal = false; }

  async function save() {
    if (!form.value.trim()) { showToast('Значение обязательно', 'error'); return; }
    const body = {
      profile_id: profileId,
      rule_type: form.rule_type,
      value: form.value.trim(),
      priority: form.priority,
      description: form.description || undefined,
      enabled: form.enabled,
      provider_id: form.provider_id || undefined,
    };
    try {
      if (editId) {
        await put('/rules/' + editId, body);
        showToast('✅ Правило обновлено');
      } else {
        await post('/rules', body);
        showToast('✅ Правило добавлено');
      }
      closeModal();
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function deleteRule(id) {
    if (!confirm('Удалить правило?')) return;
    try {
      await del('/rules/' + id);
      showToast('🗑 Правило удалено');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  function goBack() {
    currentPage.set('profiles');
    window.location.hash = '#profiles';
  }

  $: provMap = Object.fromEntries(providers.map(p => [p.id, p.name]));
  const typeLabels = { domain: 'Домен', domain_suffix: 'Субдомен', domain_keyword: 'Ключ. слово', ip: 'IP', cidr: 'Сеть', asn: 'ASN', geoip: 'GeoIP', geosite: 'GeoSite' };
</script>

<svelte:window onkeydown={e => e.key === 'Escape' && closeModal()} />

<div class="page">
  <div class="page-header">
    <div class="header-left">
      <button class="btn btn-ghost btn-sm" onclick={goBack}>← Назад</button>
      <h1>📜 Правила: {profileName}</h1>
    </div>
    <div class="header-actions">
      <button class="btn btn-primary" onclick={openCreate}>+ Добавить правило</button>
    </div>
  </div>

  {#if loading}
    <div class="loading-pulse"><div class="spinner"></div></div>
  {:else if rules.length === 0}
    <div class="empty-state">
      <p>Нет правил для этого профиля</p>
      <button class="btn btn-primary" onclick={openCreate}>+ Добавить правило</button>
    </div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr><th>Тип</th><th>Значение</th><th>Провайдер</th><th>Приоритет</th><th>Статус</th><th></th></tr>
        </thead>
        <tbody>
          {#each rules as r (r.id)}
            <tr transition:fade>
              <td><span class="badge badge-type">{typeLabels[r.rule_type] || r.rule_type}</span></td>
              <td class="text-value">{r.value}</td>
              <td class="text-dim">{provMap[r.provider_id] || '—'}</td>
              <td>{r.priority}</td>
              <td><span class="badge" class:badge-on={r.enabled} class:badge-off={!r.enabled}>{r.enabled ? '🟢 Вкл' : '🔴 Выкл'}</span></td>
              <td class="actions">
                <button class="btn btn-sm btn-ghost" onclick={() => openEdit(r.id)}>✏️</button>
                <button class="btn btn-sm btn-danger" onclick={() => deleteRule(r.id)}>🗑</button>
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
      <div class="modal-header">
        <h3>{editId ? 'Редактировать правило' : 'Новое правило'}</h3>
        <button class="modal-close" onclick={closeModal}>&times;</button>
      </div>
      <div class="modal-body">
        <div class="form-row">
          <label>Тип</label>
          <select bind:value={form.rule_type}>
            {#each Object.entries(typeLabels) as [k, v]}
              <option value={k}>{v}</option>
            {/each}
          </select>
        </div>
        <div class="form-row">
          <label>Значение</label>
          <input type="text" bind:value={form.value} placeholder="youtube.com" />
        </div>
        <div class="form-row">
          <label>Провайдер</label>
          <select bind:value={form.provider_id}>
            <option value="">— не выбран —</option>
            {#each providers as p}
              <option value={p.id}>{p.name} ({p.provider_type})</option>
            {/each}
          </select>
        </div>
        <div class="form-row-inline">
          <label>Приоритет</label>
          <input type="number" bind:value={form.priority} class="input-sm" />
        </div>
        <div class="form-row">
          <label>Описание</label>
          <input type="text" bind:value={form.description} placeholder="Описание правила" />
        </div>
        <div class="form-row-check">
          <label><input type="checkbox" bind:checked={form.enabled} /> Включено</label>
        </div>
        <button class="btn btn-primary full-width" onclick={save}>
          {editId ? 'Сохранить' : 'Добавить'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { max-width: 1000px; }
  h1 { font-size: 28px; font-weight: 700; }
  .page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
  .header-left { display: flex; align-items: center; gap: 12px; }
  .header-actions { display: flex; gap: 8px; }

  .btn {
    display: inline-flex; align-items: center; gap: 5px;
    padding: 9px 16px; border: none; border-radius: 10px;
    font-size: 13px; font-weight: 600; cursor: pointer;
    transition: all 0.2s; font-family: inherit;
  }
  .btn-primary { background: #7c5cfc; color: #fff; }
  .btn-primary:hover { background: #6b4de6; transform: translateY(-1px); }
  .btn-ghost { background: transparent; color: #888; border: 1px solid #2a2a4a; }
  .btn-ghost:hover { background: #1e1e38; color: #ccc; }
  .btn-danger { background: transparent; color: #f87171; border: 1px solid #2a2a4a; }
  .btn-danger:hover { background: rgba(248,113,113,0.1); }
  .btn-sm { padding: 5px 10px; font-size: 12px; }
  .full-width { width: 100%; justify-content: center; margin-top: 8px; }

  .table-wrap { background: #16162b; border: 1px solid #2a2a4a; border-radius: 12px; overflow: hidden; }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 12px 16px; font-size: 13px; border-bottom: 1px solid #2a2a4a; }
  th { color: #666; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px; }
  tr:last-child td { border-bottom: none; }
  tr:hover td { background: rgba(124, 92, 252, 0.04); }
  .text-value { font-family: 'SF Mono', 'Fira Code', monospace; font-size: 13px; max-width: 250px; overflow: hidden; text-overflow: ellipsis; }
  .text-dim { color: #888; }
  .actions { display: flex; gap: 4px; justify-content: flex-end; }

  .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
  .badge-type { background: rgba(124, 92, 252, 0.1); color: #9b7fff; }
  .badge-on { background: rgba(74,222,128,0.12); color: #4ade80; }
  .badge-off { background: rgba(248,113,113,0.12); color: #f87171; }

  /* Modal */
  .modal-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 100; backdrop-filter: blur(4px); }
  .modal-content { background: #1a1a30; border: 1px solid #2a2a4a; border-radius: 16px; width: 90%; max-width: 460px; box-shadow: 0 8px 40px rgba(0,0,0,0.5); }
  .modal-header { display: flex; justify-content: space-between; align-items: center; padding: 20px 24px 0; }
  .modal-header h3 { font-size: 18px; }
  .modal-close { background: none; border: none; color: #666; font-size: 28px; cursor: pointer; }
  .modal-close:hover { color: #ccc; }
  .modal-body { padding: 16px 24px 24px; }

  .form-row { margin-bottom: 14px; }
  .form-row label { display: block; font-size: 12px; color: #888; margin-bottom: 4px; font-weight: 500; text-transform: uppercase; letter-spacing: 0.3px; }
  .form-row input, .form-row select { width: 100%; padding: 10px 12px; background: #121224; border: 1px solid #2a2a4a; border-radius: 8px; color: #e0e0e0; font-size: 13px; font-family: inherit; outline: none; }
  .form-row input:focus, .form-row select:focus { border-color: #7c5cfc; }
  .form-row-inline { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
  .form-row-inline label { font-size: 12px; color: #888; }
  .input-sm { width: 80px; padding: 6px 8px; background: #121224; border: 1px solid #2a2a4a; border-radius: 6px; color: #e0e0e0; }
  .form-row-check label { display: flex; align-items: center; gap: 6px; font-size: 13px; color: #ccc; cursor: pointer; }

  .empty-state { text-align: center; padding: 60px 40px; color: #666; }
  .empty-state p { margin-bottom: 16px; font-size: 15px; }

  .loading-pulse { display: flex; align-items: center; gap: 12px; color: #666; padding: 40px; }
  .spinner { width: 20px; height: 20px; border: 2px solid #2a2a4a; border-top-color: #7c5cfc; border-radius: 50%; animation: spin 0.6s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
