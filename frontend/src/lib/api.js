const BASE = '/api/v1';

async function request(path, opts = {}) {
  const res = await fetch(BASE + path, {
    headers: { 'Content-Type': 'application/json', ...opts.headers },
    ...opts,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || err.message || res.statusText);
  }
  return res.json().catch(() => ({}));
}

export async function get(path) {
  return request(path);
}

export async function post(path, body) {
  return request(path, { method: 'POST', body: JSON.stringify(body) });
}

export async function put(path, body) {
  return request(path, { method: 'PUT', body: JSON.stringify(body) });
}

export async function del(path) {
  return request(path, { method: 'DELETE' });
}

export async function uploadFile(path, file, extraFields = {}) {
  const fd = new FormData();
  fd.append('config_file', file);
  for (const [k, v] of Object.entries(extraFields)) {
    if (v) fd.append(k, v);
  }
  const res = await fetch(BASE + path, { method: 'POST', body: fd });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error || res.statusText);
  }
  return res.json().catch(() => ({}));
}
