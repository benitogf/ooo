function ConfirmModal({ visible, title, message, confirmText, cancelText, danger, onConfirm, onCancel, loading, error }) {
  if (!visible) return null;
  
  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget && !loading) onCancel();
  };
  
  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-title">{title}</div>
        <div className="modal-message">{message}</div>
        {error && <div className="modal-error">{error}</div>}
        <div className="modal-actions">
          <button className="btn-cancel" onClick={onCancel} disabled={loading}>
            {cancelText || 'Cancel'}
          </button>
          <button 
            className={danger ? 'btn-danger' : 'btn-confirm'} 
            onClick={onConfirm}
            disabled={loading}
          >
            {loading ? 'Processing...' : (confirmText || 'Confirm')}
          </button>
        </div>
      </div>
    </div>
  );
}

window.ConfirmModal = ConfirmModal;
