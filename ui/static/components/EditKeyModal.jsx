function EditKeyModal({ visible, keyPath, filterPath, onClose, onDelete }) {
  const { useState, useRef, useEffect } = React;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  const { IconTrash, IconX, IconSend } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [initialContent, setInitialContent] = useState(null);
  const [ready, setReady] = useState(false);
  const [deleteModalVisible, setDeleteModalVisible] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState('');
  const editorRef = useRef(null);

  useEffect(() => {
    if (visible && keyPath) {
      setReady(false);
      setLoading(true);
      setError('');
      setSaving(false);
      setInitialContent(null);
      
      // Small delay to ensure loading state renders first
      setTimeout(() => setReady(true), 50);
      
      const minDelay = new Promise(r => setTimeout(r, 400));
      
      Promise.all([
        fetch('/' + keyPath).then(res => {
          if (!res.ok) throw new Error('Failed to load key');
          return res.json();
        }),
        minDelay
      ])
        .then(([data]) => {
          const content = data.data !== undefined ? data.data : data;
          setInitialContent(content);
          setLoading(false);
        })
        .catch(err => {
          setError('Failed to load: ' + err.message);
          setLoading(false);
        });
    } else if (!visible) {
      setReady(false);
    }
  }, [visible, keyPath]);

  if (!visible) return null;

  const handleOverlayClick = (e) => {
    if (e.target === e.currentTarget && !saving && !deleteLoading) onClose();
  };

  const save = async () => {
    setSaving(true);
    setError('');
    
    const minDelay = new Promise(r => setTimeout(r, 500));
    
    try {
      const content = editorRef.current?.getContent() || { json: {} };
      const dataToSave = content.json !== undefined 
        ? JSON.stringify(content.json) 
        : content.text !== undefined ? content.text : '{}';
      
      const [res] = await Promise.all([
        fetch('/' + keyPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: dataToSave
        }),
        minDelay
      ]);
      
      if (!res.ok) throw new Error(await res.text());
      
      setSaving(false);
      onClose();
    } catch (err) {
      setSaving(false);
      setError('Failed to save: ' + err.message);
    }
  };

  const openDeleteConfirm = () => {
    setDeleteError('');
    setDeleteModalVisible(true);
  };

  const closeDeleteConfirm = () => {
    if (deleteLoading) return;
    setDeleteModalVisible(false);
    setDeleteError('');
  };

  const executeDelete = async () => {
    setDeleteLoading(true);
    setDeleteError('');
    
    try {
      const res = await fetch('/' + keyPath, { method: 'DELETE' });
      if (!res.ok) throw new Error(await res.text());
      
      setDeleteLoading(false);
      setDeleteModalVisible(false);
      onClose();
      if (onDelete) onDelete();
    } catch (err) {
      setDeleteLoading(false);
      setDeleteError('Failed to delete: ' + err.message);
    }
  };

  // Extract display name from path
  const displayName = keyPath.split('/').pop() || keyPath;

  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal edit-key-modal" onClick={(e) => e.stopPropagation()}>
        <div className="edit-key-modal-header">
          <div className="modal-title">Edit: {displayName}</div>
          <button 
            className="modal-close-btn" 
            onClick={onClose}
            disabled={saving}
            title="Close"
          >
            <IconX />
          </button>
        </div>
        <div className="edit-key-modal-path">{keyPath}</div>
        
        {!ready || loading ? (
          <div className="edit-key-modal-loading">
            <div className="spinner"></div>
            <div>Loading...</div>
          </div>
        ) : (
          <div className="edit-key-modal-editor">
            <JsonEditorWrapper 
              content={initialContent} 
              editorRef={editorRef} 
            />
          </div>
        )}
        
        {error && <div className="modal-error">{error}</div>}
        
        <div className="modal-actions">
          <button 
            className="btn danger" 
            onClick={openDeleteConfirm}
            disabled={saving || loading}
          >
            <IconTrash color="#fff" /> Delete
          </button>
          <div className="modal-actions-right">
            <button className="btn-cancel" onClick={onClose} disabled={saving}>
              Cancel
            </button>
            <button 
              className="btn-confirm" 
              onClick={save}
              disabled={saving || loading}
            >
              <IconSend /> {saving ? 'Saving...' : 'Save Changes'}
            </button>
          </div>
        </div>
      </div>

      <ConfirmModal
        visible={deleteModalVisible}
        title="Delete Key"
        message={`Are you sure you want to delete "${displayName}"? This action cannot be undone.`}
        confirmText="Delete"
        danger={true}
        onConfirm={executeDelete}
        onCancel={closeDeleteConfirm}
        loading={deleteLoading}
        error={deleteError}
      />
    </div>
  );
}

window.EditKeyModal = EditKeyModal;
