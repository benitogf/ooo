function ConfirmModal({ visible, title, message, confirmText, cancelText, danger, onConfirm, onCancel }) {
  if (!visible) return null;
  
  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget) onCancel();
  };
  
  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-title">{title}</div>
        <div className="modal-message">{message}</div>
        <div className="modal-actions">
          <button className="btn-cancel" onClick={onCancel}>{cancelText || 'Cancel'}</button>
          <button className={danger ? 'btn-danger' : 'btn-confirm'} onClick={onConfirm}>
            {confirmText || 'Confirm'}
          </button>
        </div>
      </div>
    </div>
  );
}

window.ConfirmModal = ConfirmModal;
