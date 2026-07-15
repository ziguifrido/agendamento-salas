const dialog = document.querySelector('#booking-dialog');
const roomDialog = document.querySelector('#room-dialog');
const roomsDialog = document.querySelector('#rooms-dialog');
const editRoomDialog = document.querySelector('#edit-room-dialog');
const viewRoomDialog = document.querySelector('#view-room-dialog');

if ('serviceWorker' in navigator) navigator.serviceWorker.register('/static/sw.js');
const toastStack = document.createElement('div');
toastStack.className = 'toast-stack';
document.body.append(toastStack);
const toastKey = 'salas-toasts';
const now = Date.now();
let toasts = [];
try {
  toasts = JSON.parse(sessionStorage.getItem(toastKey) || '[]').filter(toast => toast.expires > now);
} catch {}
document.querySelectorAll('.toast').forEach(toast => {
  toasts.push({ id: `${now}-${Math.random()}`, kind: toast.classList.contains('error') ? 'error' : 'ok', text: toast.firstChild.textContent.trim(), expires: now + 5000 });
  toast.remove();
});
const saveToasts = () => {
  try { sessionStorage.setItem(toastKey, JSON.stringify(toasts)); } catch {}
};
const dismissToast = id => {
  toasts = toasts.filter(toast => toast.id !== id);
  saveToasts();
  document.querySelector(`[data-toast-id="${id}"]`)?.remove();
};
toasts.forEach(toast => {
  const element = document.createElement('div');
  element.className = `toast ${toast.kind}`;
  element.dataset.toastId = toast.id;
  element.setAttribute('role', toast.kind === 'error' ? 'alert' : 'status');
  element.append(toast.text);
  const close = document.createElement('button');
  close.className = 'toast-close';
  close.setAttribute('aria-label', 'Fechar notificação');
  close.textContent = '×';
  close.addEventListener('click', () => dismissToast(toast.id));
  element.append(close);
  toastStack.append(element);
  setTimeout(() => dismissToast(toast.id), toast.expires - now);
});
saveToasts();
document.querySelector('#new-booking')?.addEventListener('click', () => dialog.showModal());
document.querySelector('#new-room')?.addEventListener('click', button => {
  button.currentTarget.closest('details').open = false;
  roomDialog.showModal();
});
document.querySelector('#manage-rooms')?.addEventListener('click', button => {
  button.currentTarget.closest('details').open = false;
  roomsDialog.showModal();
});
if (dialog?.dataset.open !== undefined) dialog.showModal();
if (roomDialog?.dataset.open !== undefined) roomDialog.showModal();
if (roomsDialog?.dataset.open !== undefined) roomsDialog.showModal();
document.querySelector('#booking-form')?.addEventListener('submit', () => dialog.close());
document.querySelectorAll('#agenda-filter select').forEach(select => select.addEventListener('change', () => select.form.requestSubmit()));
const detailsDialog = document.querySelector('#booking-details-dialog');
document.querySelectorAll('.booking-details').forEach(button => button.addEventListener('click', () => {
  const details = button.dataset;
  detailsDialog.querySelector('[data-detail="title"]').textContent = details.title;
  detailsDialog.querySelector('[data-detail="room"]').textContent = details.room;
  detailsDialog.querySelector('[data-detail="day"]').textContent = details.day;
  detailsDialog.querySelector('[data-detail="time"]').textContent = `${details.starts}–${details.ends}`;
  detailsDialog.querySelector('[data-detail="owner"]').textContent = details.owner;
  detailsDialog.querySelector('[data-detail="description"]').textContent = details.description || 'Sem descrição.';
  detailsDialog.showModal();
}));
document.querySelectorAll('.edit-room').forEach(button => button.addEventListener('click', () => {
  const form = document.querySelector('#edit-room-form');
  form.action = `/rooms/${button.dataset.id}/edit`;
  ['name', 'capacity', 'location', 'resources', 'description'].forEach(field => form.elements[field].value = button.dataset[field]);
  editRoomDialog.showModal();
}));
document.querySelectorAll('.view-room').forEach(button => button.addEventListener('click', () => {
  const details = button.dataset;
  ['name', 'capacity', 'location', 'resources', 'description'].forEach(field => viewRoomDialog.querySelector(`[data-room-detail="${field}"]`).textContent = details[field] || 'Não informado');
  viewRoomDialog.querySelector('[data-room-detail="capacity"]').textContent = `${details.capacity} pessoas`;
  viewRoomDialog.showModal();
}));
document.querySelectorAll('.cancel-form').forEach(form => form.addEventListener('submit', event => {
  if (!confirm('Cancelar este agendamento?')) event.preventDefault();
}));
document.querySelectorAll('.delete-room').forEach(form => form.addEventListener('submit', event => {
  if (!confirm('Excluir esta sala?')) event.preventDefault();
}));
