import { writable } from 'svelte/store';

export const currentPage = writable('dashboard');
export const activeProfileId = writable(null);
export const toastMessage = writable(null);

let toastTimer;

export function showToast(msg, type = 'success') {
  toastMessage.set({ text: msg, type });
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => toastMessage.set(null), 3000);
}

export function navigate(page) {
  const base = page.split('?')[0];
  currentPage.set(base);
  window.location.hash = '#' + page;
}

// Restore hash on load
if (typeof window !== 'undefined') {
  const hash = window.location.hash.replace('#', '');
  if (hash) {
    const base = hash.split('?')[0];
    currentPage.set(base);
  }
  window.addEventListener('hashchange', () => {
    const h = window.location.hash.replace('#', '');
    const base = h.split('?')[0];
    if (h) currentPage.set(base);
  });
}
