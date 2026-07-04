<script>
  import { onMount } from 'svelte';
  import { get, post, put, del } from '../lib/api.js';
  import { showToast, navigate } from '../lib/stores.js';
  import { fade, slide } from 'svelte/transition';

  let profiles = [];
  let activeProfileId = null;
  let loading = true;

  onMount(load);

  // Modal state
  let showModal = false;
  let editId = null;
  let form = { name: '', description: '', is_default: false };

  async function load() {
    try {
      const [pData, rData] = await Promise.all([
        get('/profiles'),
        get('/routing/status'),
      ]);
      profiles = pData.profiles || [];
      if (rData.active && rData.profile) activeProfileId = rData.profile.id;
      loading = false;
    } catch (e) {
      loading = false;
    }
  }

  function openCreate() {
    editId = null;
    form = { name: '', description: '', is_default: false };
    showModal = true;
  }

  async function openEdit(id) {
    try {
      const p = await get('/profiles/' + id);
      editId = id;
      form = { name: p.name, description: p.description || '', is_default: p.is_default };
      showModal = true;
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  function closeModal() { showModal = false; }

  async function save() {
    if (!form.name.trim()) { showToast('Имя обязательно', 'error'); return; }
    try {
      if (editId) {
        await put('/profiles/' + editId, form);
        showToast('✅ Профиль обновлён');
      } else {
        await post('/profiles', form);
        showToast('✅ Профиль создан');
      }
      closeModal();
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function activateProfile(id) {
    try {
      await post('/profiles/' + id + '/activate');
      showToast('✅ Профиль активирован');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  async function deleteProfile(id, name) {
    if (!confirm(`Удалить профиль "${name}"?`)) return;
    try {
      await del('/profiles/' + id);
      showToast('🗑 Профиль удалён');
      load();
    } catch (e) {
      showToast('Ошибка: ' + e.message, 'error');
    }
  }

  function openRules(profileId, profileName) {
    navigate('rules?profileId=' + profileId + '&name=' + encodeURIComponent(profileName));
  }
</script>

<svelte:window onkeydown={e => e.key === 'Escape' && closeModal()} />

<div class="page">
  <div class="page-header">
    <h1>📋 Профили маршрутизации</h1>
    <button class="btn btn-primary" onclick={openCreate}>+ Создать</button>
  </div>

  {#if loading}
    <div class="loading-pulse"><div class="spinner"></div></div>
  {:else if profiles.length === 0}
    <div class="empty-state">
      <p>Нет профилей. Создайте первый профиль маршрутизации</p>
      <button class="btn btn-primary" onclick={openCreate}>+ Создать профиль</button>
    </div>
  {:else}
    <div class="table-wrap">
      <table>
        <thead>
          <tr><th>Имя</th><th>Описание</th><th>По умолч.</th><th></th></tr>
        </thead>
        <tbody>
          {#each profiles as p (p.id)}
            <tr transition:fade>
              <td class="text-bold">
                {p.name}
                {#if p.id === activeProfileId}
                  <span class="badge badge-active">Активен</span>
                {/if}
              </td>
              <td class="text-dim">{p.description || '—'}</td>
              <td>{p.is_default ? '⭐' : '—'}</td>
              <td class="actions">
                <button class="btn btn-sm btn-primary" onclick={() => activateProfile(p.id)}>▶ Активировать</button>
                <button class="btn btn-sm btn-ghost" onclick={() => openEdit(p.id)}>✏️</button>
                <button class="btn btn-sm btn-ghost" onclick={() => openRules(p.id, p.name)}>📜 Правила</button>
                <button class="btn btn-sm btn-danger" onclick={() => deleteProfile(p.id, p.name)}>🗑</button>
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
        <h3>{editId ? 'Редактировать профиль' : 'Новый профиль'}</h3>
        <button class="modal-close" onclick={closeModal}>&times;</button>
      </div>
      <div class="modal-body">
        <div class="form-row">
          <label>Имя</label>
          <input type="text" bind:value={form.name} placeholder="Мой профиль" />
        </div>
        <div class="form-row">
          <label>Описание</label>
          <input type="text" bind:value={form.description} placeholder="Описание" />
        </div>
        <div class="form-row-check">
          <label><input type="checkbox" bind:checked={form.is_default} /> Профиль по умолчанию</label>
        </div>
        <button class="btn btn-primary full-width" onclick={save}>
          {editId ? 'Сохранить' : 'Создать'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { max-width: 1000px; }
  h1 { font-size: 28px; font-weight: 700; }
  .page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }

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
  .text-bold { font-weight: 600; }
  .text-dim { color: #888; }
  .actions { display: flex; gap: 4px; justify-content: flex-end; }

  .badge-active { background: rgba(74,222,128,0.12); color: #4ade80; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; margin-left: 6px; }

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
  .form-row input { width: 100%; padding: 10px 12px; background: #121224; border: 1px solid #2a2a4a; border-radius: 8px; color: #e0e0e0; font-size: 13px; font-family: inherit; outline: none; }
  .form-row input:focus { border-color: #7c5cfc; }
  .form-row-check label { display: flex; align-items: center; gap: 6px; font-size: 13px; color: #ccc; cursor: pointer; }

  .empty-state { text-align: center; padding: 60px 40px; color: #666; }
  .empty-state p { margin-bottom: 16px; font-size: 15px; }

  .loading-pulse { display: flex; align-items: center; gap: 12px; color: #666; padding: 40px; }
  .spinner { width: 20px; height: 20px; border: 2px solid #2a2a4a; border-top-color: #7c5cfc; border-radius: 50%; animation: spin 0.6s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
