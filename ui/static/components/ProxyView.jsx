function ProxyView({ keyPath, canRead, canWrite, canDelete, liveMode, fromFilter }) {
  const { useState, useEffect, useRef } = React;
  const Icons = window.Icons || {};
  const IconChevronLeft = Icons.IconChevronLeft || (() => null);
  const IconSave = Icons.IconSave || (() => null);
  const IconTrash = Icons.IconTrash || (() => null);
  const IconWifi = Icons.IconWifi || (() => null);
  const IconWifiOff = Icons.IconWifiOff || (() => null);
  const IconEdit = Icons.IconEdit || (() => null);
  const IconEye = Icons.IconEye || (() => null);
  const JsonEditorWrapper = window.JsonEditorWrapper;
  const ReactJson = window.reactJsonView ? window.reactJsonView.default : null;

  // Determine if we have any capabilities at all
  const hasAnyCaps = canRead || canWrite || canDelete;
  // Live mode only makes sense if we can read
  const effectiveLiveMode = canRead && liveMode;
  // Editor view requires read capability (to fetch data to edit)
  const showEditor = canRead;
  // Write-only mode: can write but can't read (need empty editor)
  const writeOnlyMode = canWrite && !canRead;

  const [editorContent, setEditorContent] = useState(null);
  const [metadata, setMetadata] = useState({ created: 0, updated: 0 });
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const editorRef = useRef(null);

  // Subscribe for real-time updates only in live mode AND if we can read
  // Always call the hook (React rules) but pass null path when not needed
  const subscriptionPath = effectiveLiveMode ? keyPath : null;
  const { data, connected, error: wsError } = Api.useSubscribe(subscriptionPath);

  // Load data via GET for edit mode (only if we can read or write)
  useEffect(() => {
    if (!effectiveLiveMode && keyPath && showEditor) {
      setLoading(true);
      fetch('/' + keyPath)
        .then(async res => {
          if (res.ok) {
            const responseData = await res.json();
            setMetadata({
              created: responseData.created || 0,
              updated: responseData.updated || 0
            });
            let content = {};
            if (responseData.data !== undefined) {
              if (typeof responseData.data === 'object' && responseData.data !== null) {
                content = JSON.parse(JSON.stringify(responseData.data));
              } else if (typeof responseData.data === 'string') {
                try { content = JSON.parse(atob(responseData.data)); }
                catch (e) {
                  try { content = JSON.parse(responseData.data); }
                  catch (e2) { content = { value: responseData.data }; }
                }
              } else {
                content = { value: responseData.data };
              }
            } else {
              content = JSON.parse(JSON.stringify(responseData));
            }
            setEditorContent(content);
          } else {
            setEditorContent({});
          }
        })
        .catch(() => setEditorContent({}))
        .finally(() => setLoading(false));
    }
  }, [keyPath, effectiveLiveMode, showEditor]);

  // Update content from subscription in live mode
  useEffect(() => {
    if (effectiveLiveMode && data && data.data) {
      try {
        const parsed = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        setEditorContent(parsed);
        setMetadata({
          created: data.created || 0,
          updated: data.updated || 0
        });
      } catch (e) {
        setEditorContent({ value: data.data });
      }
    }
  }, [data, effectiveLiveMode]);

  const goBack = () => {
    if (fromFilter) {
      // Navigate back to the proxy list view
      const params = new URLSearchParams();
      params.set('source', 'proxies');
      params.set('canRead', canRead ? '1' : '0');
      params.set('canWrite', canWrite ? '1' : '0');
      params.set('canDelete', canDelete ? '1' : '0');
      params.set('type', 'list');
      window.location.hash = '/proxy/view/' + encodeURIComponent(fromFilter) + '?' + params.toString();
    } else {
      window.location.hash = '/proxies';
    }
  };

  const switchToLive = () => {
    const params = new URLSearchParams();
    params.set('source', 'proxies');
    params.set('canRead', canRead ? '1' : '0');
    params.set('canWrite', canWrite ? '1' : '0');
    params.set('canDelete', canDelete ? '1' : '0');
    params.set('mode', 'live');
    window.location.hash = '/proxy/view/' + encodeURIComponent(keyPath) + '?' + params.toString();
  };

  const switchToEdit = () => {
    const params = new URLSearchParams();
    params.set('source', 'proxies');
    params.set('canRead', canRead ? '1' : '0');
    params.set('canWrite', canWrite ? '1' : '0');
    params.set('canDelete', canDelete ? '1' : '0');
    params.set('mode', 'edit');
    window.location.hash = '/proxy/view/' + encodeURIComponent(keyPath) + '?' + params.toString();
  };

  const handleSave = async () => {
    if (!canWrite || !editorRef.current) return;

    setSaving(true);
    setError(null);
    setSuccess(null);

    try {
      const content = editorRef.current.getContent();
      const body = content.json || JSON.parse(content.text);

      const resp = await fetch('/' + keyPath, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });

      if (!resp.ok) {
        throw new Error('Failed to save: ' + resp.status);
      }

      // Reload after save
      const refreshRes = await fetch('/' + keyPath);
      if (refreshRes.ok) {
        const refreshData = await refreshRes.json();
        setMetadata({
          created: refreshData.created || 0,
          updated: refreshData.updated || 0
        });
        let newContent = {};
        if (refreshData.data !== undefined) {
          if (typeof refreshData.data === 'object' && refreshData.data !== null) {
            newContent = JSON.parse(JSON.stringify(refreshData.data));
          } else if (typeof refreshData.data === 'string') {
            try { newContent = JSON.parse(atob(refreshData.data)); }
            catch (e) {
              try { newContent = JSON.parse(refreshData.data); }
              catch (e2) { newContent = { value: refreshData.data }; }
            }
          } else {
            newContent = { value: refreshData.data };
          }
        } else {
          newContent = JSON.parse(JSON.stringify(refreshData));
        }
        setEditorContent(newContent);
      }

      setSuccess('Saved successfully');
      setTimeout(() => setSuccess(null), 3000);
    } catch (e) {
      setError(e.message);
    } finally {
      setSaving(false);
    }
  };

  const confirmDelete = () => {
    setShowDeleteModal(true);
  };

  const handleDelete = async () => {
    if (!canDelete) return;

    setShowDeleteModal(false);
    setDeleting(true);
    setError(null);

    try {
      const resp = await fetch('/' + keyPath, {
        method: 'DELETE'
      });

      if (!resp.ok) {
        throw new Error('Failed to delete: ' + resp.status);
      }

      setSuccess('Deleted successfully');
      setTimeout(() => {
        goBack();
      }, 1000);
    } catch (e) {
      setError(e.message);
    } finally {
      setDeleting(false);
    }
  };

  if (loading) {
    return (
      <div className="container editor-page">
        <div className="edit-page-header">
          <button className="btn secondary" onClick={goBack}>
            <IconChevronLeft />
            Back
          </button>
          <span className="edit-page-title">Proxy: {keyPath}</span>
        </div>
        <div className="loading-container">
          <div className="spinner"></div>
          <div>Loading data...</div>
        </div>
      </div>
    );
  }

  // Determine what mode title to show
  const getModeTitle = () => {
    if (!hasAnyCaps) return 'Disabled';
    if (effectiveLiveMode) return 'Live';
    if (canWrite) return 'Edit';
    if (canDelete && !canRead) return 'Delete Only';
    return 'View';
  };

  return (
    <div className="container editor-page">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">
          {getModeTitle()}: {keyPath}
          {effectiveLiveMode && (
            <span className={`connection-status ${connected ? 'connected' : 'disconnected'}`}>
              {connected ? <IconWifi /> : <IconWifiOff />}
            </span>
          )}
        </span>
        <div className="header-right">
          <span className="proxy-caps">
            {canRead && <span className="cap-badge read">Read</span>}
            {canWrite && <span className="cap-badge write">Write</span>}
            {canDelete && <span className="cap-badge delete">Delete</span>}
            {!hasAnyCaps && <span className="cap-badge disabled">No Access</span>}
          </span>
          {canWrite && effectiveLiveMode && (
            <button className="btn secondary" onClick={switchToEdit} title="Switch to Edit Mode">
              <IconEdit />
              Edit
            </button>
          )}
          {canRead && !effectiveLiveMode && (
            <button className="btn secondary" onClick={switchToLive} title="Switch to Live Mode">
              <IconEye />
              Live
            </button>
          )}
        </div>
      </div>

      {error && (
        <div className="error-banner">
          {error}
        </div>
      )}

      {success && (
        <div className="success-banner">
          {success}
        </div>
      )}

      {wsError && effectiveLiveMode && (
        <div className="error-banner">
          Connection error: {wsError.message || 'Failed to connect'}
        </div>
      )}

      {/* No capabilities - show disabled state */}
      {!hasAnyCaps && (
        <div className="disabled-state">
          <p>This proxy has no capabilities enabled.</p>
          <p>Contact your administrator to enable read, write, or delete access.</p>
        </div>
      )}

      {/* Delete only - show only delete button, no JSON view */}
      {canDelete && !canRead && !canWrite && (
        <div className="delete-only-state">
          <p>This proxy only allows delete operations.</p>
          <p>Use the delete button below to remove this resource.</p>
        </div>
      )}

      {/* Write only (or write+delete) - show empty editor for creating data */}
      {writeOnlyMode && JsonEditorWrapper && (
        <div className="editor-wrapper">
          <p className="write-only-hint">Enter data to send to this resource:</p>
          <JsonEditorWrapper
            content={{}}
            editorRef={editorRef}
          />
        </div>
      )}
      {writeOnlyMode && !JsonEditorWrapper && (
        <div className="loading-state">Loading editor...</div>
      )}

      {/* Live mode: read-only view (only if we can read) */}
      {effectiveLiveMode && canRead && (
        <div className="editor-wrapper json-view-container">
          {editorContent !== null && ReactJson ? (
            <ReactJson
              src={editorContent}
              theme="monokai"
              displayDataTypes={false}
              displayObjectSize={true}
              enableClipboard={true}
              collapsed={false}
              collapseStringsAfterLength={100}
              name={false}
              groupArraysAfterLength={50}
              style={{
                backgroundColor: '#0d1117',
                padding: '16px',
                borderRadius: '10px',
                border: '1px solid #1e3a4a',
                height: '100%',
                overflow: 'auto'
              }}
            />
          ) : editorContent !== null ? (
            <pre className="json-fallback">{JSON.stringify(editorContent, null, 2)}</pre>
          ) : (
            <div className="loading-state">
              {connected ? 'Waiting for data...' : 'Connecting...'}
            </div>
          )}
        </div>
      )}

      {/* Edit mode: editable view (only if we can read or write) */}
      {!effectiveLiveMode && showEditor && JsonEditorWrapper && editorContent !== null && (
        <div className="editor-wrapper">
          <JsonEditorWrapper
            content={editorContent}
            editorRef={editorRef}
          />
        </div>
      )}
      {!effectiveLiveMode && showEditor && !JsonEditorWrapper && (
        <div className="loading-state">Loading editor...</div>
      )}

      {/* Action buttons - show in both live and edit modes based on capabilities */}
      {(canWrite || canDelete) && hasAnyCaps && (
        <div className="proxy-actions">
          {canWrite && !effectiveLiveMode && (
            <button
              className="btn primary"
              onClick={handleSave}
              disabled={saving}
            >
              <IconSave />
              {saving ? 'Saving...' : 'Save Changes'}
            </button>
          )}
          {canDelete && (
            <button
              className="btn danger"
              onClick={confirmDelete}
              disabled={deleting}
            >
              <IconTrash />
              {deleting ? 'Deleting...' : 'Delete'}
            </button>
          )}
        </div>
      )}

      {/* Metadata */}
      {showEditor && (metadata.created > 0 || metadata.updated > 0) && (
        <div className="meta-info">
          <span className="meta-item">Created: {metadata.created ? new Date(metadata.created / 1000000).toLocaleString() : '-'}</span>
          <span className="meta-item">Updated: {metadata.updated ? new Date(metadata.updated / 1000000).toLocaleString() : '-'}</span>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {showDeleteModal && (
        <div className="modal-overlay" onClick={() => setShowDeleteModal(false)}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Confirm Delete</h3>
              <button className="modal-close" onClick={() => setShowDeleteModal(false)}>&times;</button>
            </div>
            <div className="modal-body">
              <p>Are you sure you want to delete this resource?</p>
              <p className="modal-path-preview">
                <code>{keyPath}</code>
              </p>
            </div>
            <div className="modal-footer">
              <button className="btn secondary" onClick={() => setShowDeleteModal(false)}>Cancel</button>
              <button className="btn danger" onClick={handleDelete}>Delete</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

window.ProxyView = ProxyView;
