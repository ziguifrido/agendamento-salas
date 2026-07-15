const dialog = document.querySelector('#booking-dialog');

document.querySelector('#new-booking')?.addEventListener('click', () => dialog.showModal());
if (dialog?.dataset.open !== undefined) dialog.showModal();
document.querySelector('#booking-form')?.addEventListener('submit', () => dialog.close());
document.querySelectorAll('.cancel-form').forEach(form => form.addEventListener('submit', event => {
  if (!confirm('Cancelar este agendamento?')) event.preventDefault();
}));
document.querySelectorAll('.toast-close').forEach(button => button.addEventListener('click', () => button.parentElement.remove()));
document.querySelectorAll('.toast').forEach(toast => setTimeout(() => toast.remove(), 3000));
