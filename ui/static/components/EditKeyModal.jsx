function EditKeyModal({ visible, keyPath, filterPath, onClose, onDelete, readOnly }) {
  const { useState, useRef, useEffect } = React;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  const { IconTrash, IconX, IconSend, IconCopy, IconCheck, IconChevronDown, IconChevronRight } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [initialContent, setInitialContent] = useState(null);
  const [ready, setReady] = useState(false);
  const [deleteModalVisible, setDeleteModalVisible] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState('');
  const [filterInfo, setFilterInfo] = useState(null);
  const [schemaExpanded, setSchemaExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const editorRef = useRef(null);

  useEffect(() => {
    if (visible && keyPath) {
      setReady(false);
      setLoading(true);
      setError('');
      setSaving(false);
      setInitialContent(null);
      setSchemaExpanded(false);
      setCopied(false);
      
      // Small delay to ensure loading state renders first
      setTimeout(() => setReady(true), 50);
      
      const minDelay = new Promise(r => setTimeout(r, 400));
      
      // Fetch filter info for schema
      const checkPath = filterPath || keyPath;
      fetch('/?api=filters')
        .then(res => res.json())
        .then(data => {
          const info = (data.filters || []).find(f => 
            f.path === checkPath || checkPath.match(new RegExp('^' + f.path.replace(/\*/g, '[^/]+') + '$'))
          );
          setFilterInfo(info);
        })
        .catch(() => setFilterInfo(null));
      
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
  }, [visible, keyPath, filterPath]);

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

  const hasSchema = filterInfo?.schema && Object.keys(filterInfo.schema).length > 0;

  const copySchemaTemplate = async () => {
    if (!hasSchema) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(filterInfo.schema, null, 2));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <div className="modal-overlay" onClick={handleOverlayClick}>
      <div className="modal edit-key-modal" onClick={(e) => e.stopPropagation()}>
        <div className="edit-key-modal-header">
          <div className="modal-title">{readOnly ? 'View' : 'Edit'}: {displayName}</div>
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
        
        {hasSchema && !loading && (
          <div className="schema-section">
            <div 
              className="schema-section-header" 
              onClick={() => setSchemaExpanded(!schemaExpanded)}
            >
              {schemaExpanded ? <IconChevronDown /> : <IconChevronRight />}
              <span>Expected Schema</span>
              <div className="schema-section-actions" onClick={(e) => e.stopPropagation()}>
                <button 
                  className="btn ghost sm" 
                  onClick={copySchemaTemplate}
                  title="Copy schema template"
                >
                  {copied ? <IconCheck /> : <IconCopy />}
                  {copied ? 'Copied!' : 'Copy'}
                </button>
              </div>
            </div>
            {schemaExpanded && (
              <pre className="schema-preview">{JSON.stringify(filterInfo.schema, null, 2)}</pre>
            )}
          </div>
        )}
        
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
              readOnly={readOnly}
            />
          </div>
        )}
        
        {error && <div className="modal-error">{error}</div>}
        
        {!loading && !(!ready) && (readOnly ? (
          <div className="modal-actions">
            <div></div>
            <button className="btn-cancel" onClick={onClose}>
              Close
            </button>
          </div>
        ) : (
          <div className="modal-actions">
            <button 
              className="btn danger" 
              onClick={openDeleteConfirm}
              disabled={saving}
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
                disabled={saving}
              >
                <IconSend /> {saving ? 'Saving...' : 'Save Changes'}
              </button>
            </div>
          </div>
        ))}
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
