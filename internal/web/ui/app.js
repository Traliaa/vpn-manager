// ============================================================
// VPN Manager — SPA Client
// ============================================================

// ---- Router ----
function navigate(hash) {
	if (!hash || hash === '#') hash = '#dashboard';

	document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
	document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));

	const page = hash.split('/')[0].replace('#', '');
	const el = document.getElementById('page-' + page);
	const nav = document.querySelector(`.nav-item[data-page="${page}"]`);
	if (el) el.classList.add('active');
	if (nav) nav.classList.add('active');

	document.getElementById('page-rules')?.classList.remove('active'); // sub-page

	switch (page) {
		case 'dashboard': renderDashboard(); break;
		case 'providers': renderProviders(); break;
		case 'profiles': renderProfiles(); break;
		case 'routing': renderRouting(); break;
		case 'rules': renderRules(getRuleProfileId()); break;
	}
}

window.addEventListener('hashchange', () => navigate(window.location.hash));
window.addEventListener('DOMContentLoaded', () => navigate(window.location.hash || '#dashboard'));

// ---- API ----
const API = '/api/v1';

async function apiGet(path) {
	const r = await fetch(API + path);
	if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText); }
	return r.json().catch(() => ({}));
}

async function apiPost(path, body) {
	const r = await fetch(API + path, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body),
	});
	if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText); }
	return r.json().catch(() => ({}));
}

async function apiPut(path, body) {
	const r = await fetch(API + path, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body),
	});
	if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText); }
	return r.json().catch(() => ({}));
}

async function apiDelete(path) {
	const r = await fetch(API + path, { method: 'DELETE' });
	if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText); }
	return r.json().catch(() => ({}));
}

// ---- Toast ----
let toastTimer;

function toast(msg, type = 'success') {
	const el = document.getElementById('toast');
	el.textContent = msg;
	el.className = 'toast ' + type;
	el.classList.remove('hidden');
	clearTimeout(toastTimer);
	toastTimer = setTimeout(() => el.classList.add('hidden'), 3000);
}

// ---- Modal ----
function openModal(title, bodyHtml) {
	document.getElementById('modal-title').textContent = title;
	document.getElementById('modal-body').innerHTML = bodyHtml;
	document.getElementById('modal').classList.remove('hidden');
}

function closeModal() {
	document.getElementById('modal').classList.add('hidden');
}

// Close modal on Escape and click outside
document.addEventListener('keydown', e => { if (e.key === 'Escape') closeModal(); });
document.getElementById('modal').addEventListener('click', e => {
	if (e.target === document.getElementById('modal')) closeModal();
});

// ============================================================
// Dashboard
// ============================================================
async function renderDashboard() {
	const el = document.getElementById('page-dashboard');
	el.innerHTML = '<h1>📊 Дашборд</h1><div class="card-row" id="dash-cards"></div><div id="dash-quick"></div><div id="dash-providers"></div><div id="dash-routing"></div>';

	try {
		const [vpnData, routingData, healthData, provData] = await Promise.all([
			apiGet('/vpn/interfaces'),
			apiGet('/routing/status'),
			apiGet('/health'),
			apiGet('/providers'),
		]);

		const interfaces = vpnData.interfaces || [];
		const providers = provData.providers || [];

		// Status cards
		const up = interfaces.filter(i => i.state === 'up').length;
		const down = interfaces.filter(i => i.state === 'down' || i.state === 'error').length;
		const total = interfaces.length;

		document.getElementById('dash-cards').innerHTML = `
			<div class="card"><div class="card-title">Провайдеры</div><div class="card-value">${total}</div></div>
			<div class="card"><div class="card-title">Активны</div><div class="card-value status-up">${up}</div></div>
			<div class="card"><div class="card-title">Неактивны</div><div class="card-value ${down > 0 ? 'status-down' : ''}">${down}</div></div>
			<div class="card"><div class="card-title">База данных</div><div class="card-value ${healthData.database === 'connected' ? 'status-up' : 'status-error'}">${healthData.database || 'unknown'}</div></div>
		`;

		// Quick route section
		const quickHtml = providers.length ? `
			<h2>⚡ Быстрая маршрутизация</h2>
			<div class="card">
				<div class="quick-route">
					<input id="qr-domain" type="text" placeholder="Введите сайт, например youtube.com" style="flex:1;padding:8px;border:1px solid #444;border-radius:6px;background:#2a2a2a;color:#fff;font-size:14px">
					<select id="qr-provider" style="padding:8px;border:1px solid #444;border-radius:6px;background:#2a2a2a;color:#fff;font-size:14px">
						<option value="">Выберите VPN</option>
						${providers.filter(p => p.enabled).map(p => `<option value="${p.id}">${esc(p.name)} (${p.provider_type})</option>`).join('')}
					</select>
					<button class="btn btn-primary" onclick="addQuickRoute()">➕ Добавить</button>
				</div>
				<div id="qr-last" style="margin-top:8px;font-size:13px;color:#999"></div>
			</div>
		` : '';

		document.getElementById('dash-quick').innerHTML = quickHtml;

		// Provider status table
		if (interfaces.length) {
			document.getElementById('dash-providers').innerHTML = `
				<h2>Состояние провайдеров</h2>
				<div class="card"><table>
					<thead><tr><th>Имя</th><th>Тип</th><th>Статус</th></tr></thead>
					<tbody>${interfaces.map(i => `
						<tr><td>${esc(i.name)}</td><td>${esc(i.type)}</td><td><span class="badge badge-${i.state}">${i.state}</span></td></tr>
					`).join('')}</tbody>
				</table></div>
			`;
		} else {
			document.getElementById('dash-providers').innerHTML = '<div class="card"><div class="empty">Нет зарегистрированных провайдеров</div></div>';
		}

		// Routing status with quick controls
		const active = routingData.active;
		document.getElementById('dash-routing').innerHTML = `
			<h2>Маршрутизация</h2>
			<div class="card">
				<p>Статус: <strong>${active ? '✅ Активен' : '⏸ Неактивен'}</strong></p>
				${active && routingData.profile ? `<p>Профиль: <strong>${esc(routingData.profile.name)}</strong></p>` : ''}
				<div style="margin-top:8px">
					${active ? `<button class="btn btn-sm btn-danger" onclick="quickDeactivate()">⏹ Выключить</button>` : ''}
				</div>
			</div>
		`;
	} catch (e) {
		el.innerHTML += `<div class="card"><p class="status-error">Ошибка загрузки: ${esc(e.message)}</p></div>`;
	}
}

// Quick add route: creates profile if needed, adds rule, activates
async function addQuickRoute() {
	const domain = document.getElementById('qr-domain').value.trim();
	const providerId = document.getElementById('qr-provider').value;

	if (!domain) { toast('Введите сайт', 'error'); return; }
	if (!providerId) { toast('Выберите VPN', 'error'); return; }

	try {
		// 1. Find or create "Default" profile
		let profiles = (await apiGet('/profiles')).profiles || [];
		let profile = profiles.find(p => p.name === 'Default');

		if (!profile) {
			profile = await apiPost('/profiles', {
				name: 'Default',
				description: 'Авто-профиль для быстрой маршрутизации',
				is_default: true,
			});
			toast('📋 Создан профиль "Default"');
		}

		// 2. Add rule
		await apiPost('/rules', {
			profile_id: profile.id,
			provider_id: providerId,
			rule_type: 'domain',
			value: domain,
			enabled: true,
		});

		// 3. Activate profile
		await apiPost('/profiles/' + profile.id + '/activate');

		document.getElementById('qr-last').textContent = `✅ ${domain} → VPN (маршрутизация активна)`;
		document.getElementById('qr-domain').value = '';
		toast(`✅ ${domain} добавлен в маршрутизацию`);
		renderDashboard();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function quickDeactivate() {
	try {
		await apiPost('/routing/deactivate');
		toast('⏹ Маршрутизация выключена');
		renderDashboard();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}
}

// ============================================================
// Providers
// ============================================================
async function renderProviders() {
	const el = document.getElementById('page-providers');
	el.innerHTML = '<h1>🔌 Провайдеры</h1><div class="btn-group"><button class="btn btn-primary" onclick="showCreateProvider()">+ Добавить</button><button class="btn btn-ghost" onclick="showImportConfig()">📂 Загрузить .conf</button><button class="btn btn-ghost" onclick="syncProviders()">🔄 Синхронизировать</button></div><div id="providers-list"><div class="spinner"></div></div>';

	try {
		const data = await apiGet('/providers');
		const providers = data.providers || [];

		if (!providers.length) {
			document.getElementById('providers-list').innerHTML = '<div class="card"><div class="empty">Нет провайдеров. Добавьте первый!</div></div>';
			return;
		}

		document.getElementById('providers-list').innerHTML = `
			<div class="card"><table>
				<thead><tr><th>Имя</th><th>Тип</th><th></th><th>Приоритет</th><th></th></tr></thead>
				<tbody>${providers.map(p => `
					<tr>
						<td>${esc(p.name)}</td>
						<td>${esc(p.provider_type)}</td>
						<td><span class="badge badge-${p.enabled ? 'up' : 'down'}" style="cursor:pointer" onclick="toggleProvider('${p.id}', ${!p.enabled})">${p.enabled ? '🟢 Вкл' : '🔴 Выкл'}</span></td>
						<td>${p.priority}</td>
						<td>
							<button class="btn btn-sm btn-ghost" onclick="toggleProvider('${p.id}', ${!p.enabled})" title="${p.enabled ? 'Выключить' : 'Включить'}">${p.enabled ? '⏹' : '▶️'}</button>
							<button class="btn btn-sm btn-ghost" onclick="showEditProvider('${p.id}')">✏️</button>
							<button class="btn btn-sm btn-danger" onclick="deleteProvider('${p.id}','${esc(p.name)}')">🗑</button>
						</td>
					</tr>
				`).join('')}</tbody>
			</table></div>
		`;
	} catch (e) {
		document.getElementById('providers-list').innerHTML = `<div class="card"><p class="status-error">Ошибка: ${esc(e.message)}</p></div>`;
	}
}

async function syncProviders() {
	try {
		await apiPost('/vpn/sync');
		toast('Провайдеры синхронизированы');
		renderProviders();
	} catch (e) {
		toast('Ошибка синхронизации: ' + e.message, 'error');
	}
}

function showImportConfig() {
	openModal('📂 Импорт .conf', `
		<p>Загрузите WireGuard или AmneziaWG конфиг-файл. Тип провайдера определится автоматически.</p>
		<div class="form-group">
			<label>Файл конфигурации (.conf)</label>
			<input id="import-file" type="file" accept=".conf,.txt" style="display:block;margin-top:4px">
		</div>
		<div class="form-group">
			<label>Или вставьте текст конфига</label>
			<textarea id="import-text" placeholder="[Interface]
PrivateKey = ...
Address = 10.0.0.2/32

[Peer]
PublicKey = ...
Endpoint = example.com:51820
AllowedIPs = 0.0.0.0/0" rows="6"></textarea>
		</div>
		<div class="form-group">
			<label>Название провайдера (оставьте пустым для автогенерации)</label>
			<input id="import-name" placeholder="my-vpn">
		</div>
		<button class="btn btn-primary" onclick="importConfig()">📂 Импортировать</button>
	`);
}

async function importConfig() {
	const fileInput = document.getElementById('import-file');
	const textArea = document.getElementById('import-text');
	const name = document.getElementById('import-name').value.trim();
	const file = fileInput.files?.[0];

	let formData;

	if (file) {
		formData = new FormData();
		formData.append('config_file', file);
		if (name) formData.append('name', name);
	} else if (textArea.value.trim()) {
		// JSON mode — apiPost handles JSON.stringify
		formData = null;
	} else {
		toast('Выберите файл или введите текст конфига', 'error');
		return;
	}

	try {
		let result;
		if (file) {
			const r = await fetch('/api/v1/providers/import', {
				method: 'POST',
				body: formData,
			});
			if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText); }
			result = await r.json();
		} else {
			result = await apiPost('/providers/import', {
				config_text: textArea.value.trim(),
				name: name || undefined,
			});
		}

		const detected = result.detected || '';
		toast('✅ Провайдер импортирован (определён как ' + detected + ')');
		closeModal();
		renderProviders();
	} catch (e) {
		toast('Ошибка импорта: ' + e.message, 'error');
	}
}

async function showCreateProvider() {
	const types = ['amneziawg', 'wireguard', 'vless', 'hysteria2', 'tuic', 'shadowsocks'];
	const configExamples = {
		amneziawg: '{\n  "private_key": "...",\n  "server_public_key": "...",\n  "endpoint": "example.com:51820",\n  "dns": "1.1.1.1",\n  "junk_packet_count": 10\n}',
		wireguard: '{\n  "private_key": "...",\n  "server_public_key": "...",\n  "endpoint": "example.com:51820",\n  "dns": "1.1.1.1"\n}',
		vless: '{\n  "server": "example.com",\n  "server_port": 443,\n  "uuid": "..."\n}',
		hysteria2: '{\n  "server": "example.com",\n  "server_port": 443,\n  "password": "...",\n  "sni": "example.com"\n}',
		tuic: '{\n  "server": "example.com",\n  "server_port": 443,\n  "password": "...",\n  "uuid": "..."\n}',
		shadowsocks: '{\n  "server": "example.com",\n  "server_port": 443,\n  "method": "aes-256-gcm",\n  "password": "..."\n}',
	};

	openModal('Новый провайдер', `
		<div class="form-group">
			<label>Имя</label>
			<input id="prov-name" placeholder="my-vpn">
		</div>
		<div class="form-group">
			<label>Тип</label>
			<select id="prov-type" onchange="updateConfigExample('${esc(JSON.stringify(configExamples))}')">
				${types.map(t => `<option value="${t}">${t}</option>`).join('')}
			</select>
		</div>
		<div class="form-group">
			<label>Конфигурация (JSON)</label>
			<textarea id="prov-config">${configExamples.vless}</textarea>
		</div>
		<div class="form-group">
			<label><input type="checkbox" id="prov-enabled" checked> Включён</label>
		</div>
		<div class="form-group">
			<label>Приоритет</label>
			<input id="prov-priority" type="number" value="100">
		</div>
		<div class="form-group">
			<label>Health check host</label>
			<input id="prov-health" placeholder="google.com">
		</div>
		<button class="btn btn-primary" onclick="createProvider()">Создать</button>
	`);
}

function updateConfigExample(configExamplesJson) {
	const type = document.getElementById('prov-type').value;
	const examples = JSON.parse(configExamplesJson);
	document.getElementById('prov-config').value = examples[type] || '{}';
}

async function createProvider() {
	const name = document.getElementById('prov-name').value.trim();
	const type = document.getElementById('prov-type').value;
	const config = document.getElementById('prov-config').value.trim();
	const enabled = document.getElementById('prov-enabled').checked;
	const priority = parseInt(document.getElementById('prov-priority').value) || 100;
	const healthHost = document.getElementById('prov-health').value.trim();

	if (!name) { toast('Имя обязательно', 'error'); return; }

	let configObj;
	try { configObj = JSON.parse(config); } catch (e) { toast('Неверный JSON конфига', 'error'); return; }

	try {
		await apiPost('/providers', { name, provider_type: type, config: configObj, enabled, priority, health_host: healthHost });
		toast('Провайдер создан');
		closeModal();
		renderProviders();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function showEditProvider(id) {
	try {
		const p = await apiGet('/providers/' + id);
		openModal('Редактировать провайдера', `
			<div class="form-group">
				<label>Имя</label>
				<input id="prov-name" value="${esc(p.name)}">
			</div>
			<div class="form-group">
				<label>Тип</label>
				<select id="prov-type">
					<option value="amneziawg" ${p.provider_type === 'amneziawg' ? 'selected' : ''}>amneziawg</option>
					<option value="wireguard" ${p.provider_type === 'wireguard' ? 'selected' : ''}>wireguard</option>
					<option value="vless" ${p.provider_type === 'vless' ? 'selected' : ''}>vless</option>
					<option value="hysteria2" ${p.provider_type === 'hysteria2' ? 'selected' : ''}>hysteria2</option>
					<option value="tuic" ${p.provider_type === 'tuic' ? 'selected' : ''}>tuic</option>
					<option value="shadowsocks" ${p.provider_type === 'shadowsocks' ? 'selected' : ''}>shadowsocks</option>
				</select>
			</div>
			<div class="form-group">
				<label>Конфигурация (JSON)</label>
				<textarea id="prov-config">${esc(jsonPretty(p.config))}</textarea>
			</div>
			<div class="form-group">
				<label><input type="checkbox" id="prov-enabled" ${p.enabled ? 'checked' : ''}> Включён</label>
			</div>
			<div class="form-group">
				<label>Приоритет</label>
				<input id="prov-priority" type="number" value="${p.priority}">
			</div>
			<div class="form-group">
				<label>Health check host</label>
				<input id="prov-health" value="${p.health_host || ''}">
			</div>
			<button class="btn btn-primary" onclick="updateProvider('${id}')">Сохранить</button>
		`);
	} catch (e) {
		toast('Ошибка загрузки: ' + e.message, 'error');
	}
}

async function updateProvider(id) {
	const name = document.getElementById('prov-name').value.trim();
	const type = document.getElementById('prov-type').value;
	const config = document.getElementById('prov-config').value.trim();
	const enabled = document.getElementById('prov-enabled').checked;
	const priority = parseInt(document.getElementById('prov-priority').value) || 100;
	const healthHost = document.getElementById('prov-health').value.trim();

	if (!name) { toast('Имя обязательно', 'error'); return; }

	let configObj;
	try { configObj = JSON.parse(config); } catch (e) { toast('Неверный JSON конфига', 'error'); return; }

	try {
		await apiPut('/providers/' + id, { name, provider_type: type, config: configObj, enabled, priority, health_host: healthHost });
		toast('Провайдер обновлён');
		closeModal();
		renderProviders();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function deleteProvider(id, name) {
	if (!confirm(`Удалить провайдера "${name}"?`)) return;
	try {
		await apiDelete('/providers/' + id);
		toast('Провайдер удалён');
		renderProviders();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function toggleProvider(id, enabled) {
	try {
		await apiPut('/providers/' + id, { enabled });
		toast(enabled ? '✅ Провайдер включён' : '⏹ Провайдер выключен');
		// Синхронизируем чтобы применить изменения
		try { await apiPost('/vpn/sync'); } catch (_) {}
		renderProviders();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

// ============================================================
// Profiles
// ============================================================
async function renderProfiles() {
	const el = document.getElementById('page-profiles');
	el.innerHTML = '<h1>📋 Профили маршрутизации</h1><div class="btn-group"><button class="btn btn-primary" onclick="showCreateProfile()">+ Создать</button></div><div id="profiles-list"><div class="spinner"></div></div>';

	try {
		const data = await apiGet('/profiles');
		const profiles = data.profiles || [];

		if (!profiles.length) {
			document.getElementById('profiles-list').innerHTML = '<div class="card"><div class="empty">Нет профилей. Создайте первый!</div></div>';
			return;
		}

		// Fetch routing status to know which profile is active
		let activeProfileId = null;
		try {
			const routingData = await apiGet('/routing/status');
			if (routingData.active && routingData.profile) {
				activeProfileId = routingData.profile.id;
			}
		} catch (_) {}

		document.getElementById('profiles-list').innerHTML = `
			<div class="card"><table>
				<thead><tr><th>Имя</th><th>Описание</th><th>По умолчанию</th><th>Активен</th><th></th></tr></thead>
				<tbody>${profiles.map(p => `
					<tr>
						<td>${esc(p.name)}</td>
						<td>${p.description || '—'}</td>
						<td>${p.is_default ? '⭐' : '—'}</td>
						<td>${activeProfileId === p.id ? '✅' : '—'}</td>
						<td>
							<button class="btn btn-sm btn-primary" onclick="activateProfile('${p.id}')">▶ Активировать</button>
							<button class="btn btn-sm btn-ghost" onclick="showEditProfile('${p.id}')">✏️</button>
							<button class="btn btn-sm btn-ghost" onclick="showRulesPage('${p.id}')">📜 Правила</button>
							<button class="btn btn-sm btn-danger" onclick="deleteProfile('${p.id}','${esc(p.name)}')">🗑</button>
						</td>
					</tr>
				`).join('')}</tbody>
			</table></div>
		`;
	} catch (e) {
		document.getElementById('profiles-list').innerHTML = `<div class="card"><p class="status-error">Ошибка: ${esc(e.message)}</p></div>`;
	}
}

function showCreateProfile() {
	openModal('Новый профиль', `
		<div class="form-group">
			<label>Имя</label>
			<input id="prof-name" placeholder="Мой профиль">
		</div>
		<div class="form-group">
			<label>Описание</label>
			<input id="prof-desc" placeholder="Описание профиля">
		</div>
		<div class="form-group">
			<label><input type="checkbox" id="prof-default"> Профиль по умолчанию</label>
		</div>
		<button class="btn btn-primary" onclick="createProfile()">Создать</button>
	`);
}

async function createProfile() {
	const name = document.getElementById('prof-name').value.trim();
	const description = document.getElementById('prof-desc').value.trim();
	const isDefault = document.getElementById('prof-default').checked;

	if (!name) { toast('Имя обязательно', 'error'); return; }

	try {
		await apiPost('/profiles', { name, description, is_default: isDefault });
		toast('Профиль создан');
		closeModal();
		renderProfiles();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function showEditProfile(id) {
	try {
		const p = await apiGet('/profiles/' + id);
		openModal('Редактировать профиль', `
			<div class="form-group">
				<label>Имя</label>
				<input id="prof-name" value="${esc(p.name)}">
			</div>
			<div class="form-group">
				<label>Описание</label>
				<input id="prof-desc" value="${p.description || ''}">
			</div>
			<div class="form-group">
				<label><input type="checkbox" id="prof-default" ${p.is_default ? 'checked' : ''}> Профиль по умолчанию</label>
			</div>
			<button class="btn btn-primary" onclick="updateProfile('${id}')">Сохранить</button>
		`);
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function updateProfile(id) {
	const name = document.getElementById('prof-name').value.trim();
	const description = document.getElementById('prof-desc').value.trim();
	const isDefault = document.getElementById('prof-default').checked;

	if (!name) { toast('Имя обязательно', 'error'); return; }

	try {
		await apiPut('/profiles/' + id, { name, description, is_default: isDefault });
		toast('Профиль обновлён');
		closeModal();
		renderProfiles();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function deleteProfile(id, name) {
	if (!confirm(`Удалить профиль "${name}"?`)) return;
	try {
		await apiDelete('/profiles/' + id);
		toast('Профиль удалён');
		renderProfiles();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function activateProfile(id) {
	try {
		await apiPost('/profiles/' + id + '/activate');
		toast('Профиль активирован');
		renderProfiles();
		renderDashboard();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

function showRulesPage(profileId) {
	setRuleProfileId(profileId);
	window.location.hash = '#rules';
}

// ============================================================
// Rules (sub-page for a profile)
// ============================================================
let _ruleProfileId = null;

function setRuleProfileId(id) { _ruleProfileId = id; }
function getRuleProfileId() { return _ruleProfileId; }

async function renderRules(profileId) {
	const el = document.getElementById('page-rules');
	el.classList.add('active');

	if (!profileId) {
		el.innerHTML = '<div class="card"><div class="empty">Выберите профиль из списка</div></div>';
		return;
	}

	el.innerHTML = '<h1>📜 Правила маршрутизации</h1><div class="btn-group"><button class="btn btn-ghost" onclick="window.location.hash=\'#profiles\'">← Назад к профилям</button><button class="btn btn-primary" onclick="showCreateRule()">+ Добавить правило</button></div><div id="rules-list"><div class="spinner"></div></div>';

	try {
		const data = await apiGet('/profiles/' + profileId + '/rules');
		const rules = data.rules || [];

		if (!rules.length) {
			document.getElementById('rules-list').innerHTML = '<div class="card"><div class="empty">Нет правил. Добавьте первое!</div></div>';
			return;
		}

		// Get providers for name lookup
		const provData = await apiGet('/providers');
		const providers = (provData.providers || []).reduce((acc, p) => { acc[p.id] = p.name; return acc; }, {});

		document.getElementById('rules-list').innerHTML = `
			<div class="card"><table>
				<thead><tr><th>Тип</th><th>Значение</th><th>Провайдер</th><th>Приоритет</th><th>Включено</th><th></th></tr></thead>
				<tbody>${rules.map(r => `
					<tr>
						<td>${esc(r.rule_type)}</td>
						<td class="text-mono truncate">${esc(r.value)}</td>
						<td>${esc(providers[r.provider_id] || '—')}</td>
						<td>${r.priority}</td>
						<td>${r.enabled ? '✅' : '❌'}</td>
						<td>
							<button class="btn btn-sm btn-ghost" onclick="showEditRule('${r.id}', '${profileId}')">✏️</button>
							<button class="btn btn-sm btn-danger" onclick="deleteRule('${r.id}')">🗑</button>
						</td>
					</tr>
				`).join('')}</tbody>
			</table></div>
		`;
	} catch (e) {
		document.getElementById('rules-list').innerHTML = `<div class="card"><p class="status-error">Ошибка: ${esc(e.message)}</p></div>`;
	}
}

async function showCreateRule() {
	try {
		const provData = await apiGet('/providers');
		const providers = provData.providers || [];

		const ruleTypes = ['domain', 'domain_suffix', 'domain_keyword', 'ip', 'cidr', 'asn', 'geoip', 'geosite'];
		const typeExamples = {
			domain: 'example.com',
			domain_suffix: '.example.com',
			domain_keyword: 'example',
			ip: '1.2.3.4',
			cidr: '1.2.3.0/24',
			asn: '12345',
			geoip: 'ru',
			geosite: 'google',
		};

		openModal('Новое правило', `
			<div class="form-group">
				<label>Тип</label>
				<select id="rule-type" onchange="document.getElementById('rule-val').placeholder = '${esc(JSON.stringify(typeExamples))}'[this.value] || ''">
					${ruleTypes.map(t => `<option value="${t}">${t}</option>`).join('')}
				</select>
			</div>
			<div class="form-group">
				<label>Значение</label>
				<input id="rule-val" placeholder="example.com">
			</div>
			<div class="form-group">
				<label>Провайдер</label>
				<select id="rule-provider">
					<option value="">— не выбран —</option>
					${providers.map(p => `<option value="${p.id}">${esc(p.name)} (${p.provider_type})</option>`).join('')}
				</select>
			</div>
			<div class="form-group">
				<label>Приоритет</label>
				<input id="rule-priority" type="number" value="500">
			</div>
			<div class="form-group">
				<label>Описание</label>
				<input id="rule-desc" placeholder="Описание правила">
			</div>
			<div class="form-group">
				<label><input type="checkbox" id="rule-enabled" checked> Включено</label>
			</div>
			<button class="btn btn-primary" onclick="createRule('${_ruleProfileId}')">Создать</button>
		`);
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function createRule(profileId) {
	const ruleType = document.getElementById('rule-type').value;
	const value = document.getElementById('rule-val').value.trim();
	const providerId = document.getElementById('rule-provider').value;
	const priority = parseInt(document.getElementById('rule-priority').value) || 500;
	const description = document.getElementById('rule-desc').value.trim();
	const enabled = document.getElementById('rule-enabled').checked;

	if (!value) { toast('Значение обязательно', 'error'); return; }

	try {
		await apiPost('/rules', {
			profile_id: profileId,
			rule_type: ruleType,
			value,
			provider_id: providerId || undefined,
			priority,
			description,
			enabled,
		});
		toast('Правило создано');
		closeModal();
		renderRules(profileId);
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function showEditRule(ruleId, profileId) {
	try {
		const [r, provData] = await Promise.all([
			apiGet('/rules/' + ruleId),
			apiGet('/providers'),
		]);
		const providers = provData.providers || [];

		const ruleTypes = ['domain', 'domain_suffix', 'domain_keyword', 'ip', 'cidr', 'asn', 'geoip', 'geosite'];

		openModal('Редактировать правило', `
			<div class="form-group">
				<label>Тип</label>
				<select id="rule-type">
					${ruleTypes.map(t => `<option value="${t}" ${r.rule_type === t ? 'selected' : ''}>${t}</option>`).join('')}
				</select>
			</div>
			<div class="form-group">
				<label>Значение</label>
				<input id="rule-val" value="${esc(r.value)}">
			</div>
			<div class="form-group">
				<label>Провайдер</label>
				<select id="rule-provider">
					<option value="">— не выбран —</option>
					${providers.map(p => `<option value="${p.id}" ${r.provider_id === p.id ? 'selected' : ''}>${esc(p.name)} (${p.provider_type})</option>`).join('')}
				</select>
			</div>
			<div class="form-group">
				<label>Приоритет</label>
				<input id="rule-priority" type="number" value="${r.priority}">
			</div>
			<div class="form-group">
				<label>Описание</label>
				<input id="rule-desc" value="${r.description || ''}">
			</div>
			<div class="form-group">
				<label><input type="checkbox" id="rule-enabled" ${r.enabled ? 'checked' : ''}> Включено</label>
			</div>
			<button class="btn btn-primary" onclick="updateRule('${ruleId}')">Сохранить</button>
		`);
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function updateRule(ruleId) {
	const ruleType = document.getElementById('rule-type').value;
	const value = document.getElementById('rule-val').value.trim();
	const providerId = document.getElementById('rule-provider').value;
	const priority = parseInt(document.getElementById('rule-priority').value) || 500;
	const description = document.getElementById('rule-desc').value.trim();
	const enabled = document.getElementById('rule-enabled').checked;

	if (!value) { toast('Значение обязательно', 'error'); return; }

	try {
		const data = { rule_type: ruleType, value, priority, description, enabled };
		if (providerId) data.provider_id = providerId;
		await apiPut('/rules/' + ruleId, data);
		toast('Правило обновлено');
		closeModal();
		renderRules(_ruleProfileId);
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function deleteRule(ruleId) {
	if (!confirm('Удалить правило?')) return;
	try {
		await apiDelete('/rules/' + ruleId);
		toast('Правило удалено');
		renderRules(_ruleProfileId);
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

// ============================================================
// Routing
// ============================================================
async function renderRouting() {
	const el = document.getElementById('page-routing');
	el.innerHTML = '<h1>🌐 Маршрутизация</h1><div id="routing-status"><div class="spinner"></div></div>';

	try {
		const data = await apiGet('/routing/status');
		const active = data.active;
		const profile = data.profile;

		el.innerHTML = `
			<h1>🌐 Маршрутизация</h1>
			<div class="card-row">
				<div class="card">
					<div class="card-title">Статус</div>
					<div class="card-value ${active ? 'status-up' : ''}">${active ? 'Активна' : 'Неактивна'}</div>
				</div>
				<div class="card">
					<div class="card-title">Активный профиль</div>
					<div class="card-value">${profile ? esc(profile.name) : '—'}</div>
				</div>
			</div>
			<div class="btn-group">
				${active ? `<button class="btn btn-danger" onclick="deactivateRouting()">⏹ Деактивировать</button>` : ''}
				${active ? `<button class="btn btn-primary" onclick="reapplyRouting()">🔄 Переприменить</button>` : ''}
			</div>
		`;
	} catch (e) {
		document.getElementById('routing-status').innerHTML = `<div class="card"><p class="status-error">Ошибка: ${esc(e.message)}</p></div>`;
	}
}

async function deactivateRouting() {
	try {
		await apiPost('/routing/deactivate');
		toast('Маршрутизация деактивирована');
		renderRouting();
		renderDashboard();
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

async function reapplyRouting() {
	try {
		await apiPost('/routing/reapply');
		toast('Маршрутизация переприменена');
	} catch (e) {
		toast('Ошибка: ' + e.message, 'error');
	}
}

// ============================================================
// Helpers
// ============================================================
function esc(s) {
	if (s == null) return '';
	return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function jsonPretty(obj) {
	if (!obj) return '{}';
	if (typeof obj === 'string') {
		try { return JSON.stringify(JSON.parse(obj), null, 2); } catch (_) { return obj; }
	}
	return JSON.stringify(obj, null, 2);
}
