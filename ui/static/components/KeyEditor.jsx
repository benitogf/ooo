function KeyEditor({ keyPath, filterPath, isCreate }) {
  const { useState, useEffect, useRef, useMemo } = React;
  const { IconChevronLeft, IconEye, IconChevronDown, IconChevronRight, IconCopy, IconCheck } = window.Icons;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState({ type: '', message: '' });
  const [editorData, setEditorData] = useState(null);
  const [metadata, setMetadata] = useState({ created: 0, updated: 0 });
  const [filterInfo, setFilterInfo] = useState(null);
  const [schemaExpanded, setSchemaExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const editorRef = useRef(null);

  useEffect(() => {
    // Check filter type and redirect if needed
    const checkPath = filterPath || keyPath;
    if (checkPath) {
      fetch('/?api=filters')
        .then(res => res.json())
        .then(data => {
          const info = (data.filters || []).find(f => 
            f.path === checkPath || checkPath.match(new RegExp('^' + f.path.replace(/\*/g, '[^/]+') + '$'))
          );
          setFilterInfo(info);
          if (info) {
            if (info.type === 'read-only') {
              window.location.hash = '/storage/key/live/' + encodeURIComponent(keyPath) + (filterPath ? '?from=' + encodeURIComponent(filterPath) : '');
            } else if (info.type === 'custom') {
              window.location.hash = '/storage';
            }
          }
        })
        .catch(() => setFilterInfo(null));
    }
  }, [keyPath, filterPath]);

  useEffect(() => {
    if (isCreate) {
      setEditorData({});
      setMetadata({ created: 0, updated: 0 });
      return;
    }
    if (!keyPath) return;
    
    setLoading(true);
    fetch('/' + keyPath)
      .then(async res => {
        if (res.ok) {
          const data = await res.json();
          // Store metadata
          setMetadata({ 
            created: data.created || 0, 
            updated: data.updated || 0 
          });
          let content = {};
          if (data.data !== undefined) {
            if (typeof data.data === 'object' && data.data !== null) {
              content = JSON.parse(JSON.stringify(data.data));
            } else if (typeof data.data === 'string') {
              try { content = JSON.parse(atob(data.data)); } 
              catch (e) { 
                try { content = JSON.parse(data.data); } 
                catch (e2) { content = { value: data.data }; }
              }
            } else {
              content = { value: data.data };
            }
          } else {
            content = JSON.parse(JSON.stringify(data));
          }
          setEditorData(content);
        } else {
          setEditorData({});
          setMetadata({ created: 0, updated: 0 });
        }
      })
      .catch(() => {
        setEditorData({});
        setMetadata({ created: 0, updated: 0 });
      })
      .finally(() => setLoading(false));
  }, [keyPath, isCreate]);

  const goBack = () => {
    if (filterPath && filterPath.includes('*')) {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent(filterPath);
    } else {
      window.location.hash = '/storage';
    }
  };

  const switchToLive = () => {
    const params = filterPath ? '?from=' + encodeURIComponent(filterPath) : '';
    window.location.hash = '/storage/key/live/' + encodeURIComponent(keyPath) + params;
  };

  const save = async () => {
    if (!keyPath) {
      setStatus({ type: 'error', message: 'Key path is required' });
      return;
    }
    
    setSaving(true);
    setStatus({ type: '', message: '' });
    
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
      
      // Reload the data after save instead of navigating away
      const refreshRes = await fetch('/' + keyPath);
      if (refreshRes.ok) {
        const refreshData = await refreshRes.json();
        // Update metadata
        setMetadata({ 
          created: refreshData.created || 0, 
          updated: refreshData.updated || 0 
        });
        let content = {};
        if (refreshData.data !== undefined) {
          if (typeof refreshData.data === 'object' && refreshData.data !== null) {
            content = JSON.parse(JSON.stringify(refreshData.data));
          } else if (typeof refreshData.data === 'string') {
            try { content = JSON.parse(atob(refreshData.data)); } 
            catch (e) { 
              try { content = JSON.parse(refreshData.data); } 
              catch (e2) { content = { value: refreshData.data }; }
            }
          } else {
            content = { value: refreshData.data };
          }
        } else {
          content = JSON.parse(JSON.stringify(refreshData));
        }
        setEditorData(content);
      }
      setSaving(false);
      setStatus({ type: 'success', message: 'Saved successfully' });
      setTimeout(() => setStatus({ type: '', message: '' }), 2000);
    } catch (err) {
      setSaving(false);
      setStatus({ type: 'error', message: 'Failed: ' + err.message });
      setTimeout(() => setStatus({ type: '', message: '' }), 3000);
    }
  };

  // Get merged description from filter info
  const mergedDescription = useMemo(() => {
    if (!filterInfo) return null;
    const descriptions = [
      filterInfo.descWrite,
      filterInfo.descRead,
      filterInfo.descDelete,
      filterInfo.descAfterWrite,
      filterInfo.descLimit
    ].filter(Boolean);
    if (descriptions.length === 0) return null;
    const unique = [...new Set(descriptions)];
    return unique.join(' | ');
  }, [filterInfo]);

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

  if (loading) {
    return (
      <div className="container">
        <div className="edit-page-header">
          <button className="btn secondary" onClick={goBack}>
            <IconChevronLeft />
            Back
          </button>
          <span className="edit-page-title">{isCreate ? 'Create Key' : 'Edit: ' + keyPath}</span>
        </div>
        <div className="loading-container">
          <div className="spinner"></div>
          <div>Loading data...</div>
        </div>
      </div>
    );
  }

  if (saving) {
    return (
      <div className="container">
        <div className="edit-page-header">
          <button className="btn secondary" onClick={goBack}>
            <IconChevronLeft />
            Back
          </button>
          <span className="edit-page-title">{isCreate ? 'Create Key' : 'Edit: ' + keyPath}</span>
        </div>
        <div className="loading-container">
          <div className="spinner"></div>
          <div>Saving changes...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="container editor-page">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">
          {isCreate ? 'Create Key' : 'Edit: ' + keyPath}
          {mergedDescription && (
            <span className="filter-description-badge" title={mergedDescription}>
              {mergedDescription.length > 50 ? mergedDescription.substring(0, 50) + '...' : mergedDescription}
            </span>
          )}
        </span>
        {!isCreate && (
          <div className="header-right">
            <button className="btn secondary" onClick={switchToLive} title="Switch to Live Mode (Read-only)">
              <IconEye />
              Live
            </button>
          </div>
        )}
      </div>

      {hasSchema && (
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

      <div className="editor-wrapper">
        {editorData !== null && <JsonEditorWrapper content={editorData} editorRef={editorRef} />}
      </div>
      <div className="edit-page-actions">
        {status.message && <div className={`status ${status.type}`}>{status.message}</div>}
        <button className="btn" onClick={save}>
          {isCreate ? 'Create Key' : 'Save Changes'}
        </button>
      </div>

      {!isCreate && (
        <div className="meta-info">
          <span className="meta-item">Created: {metadata.created ? new Date(metadata.created / 1000000).toLocaleString() : '-'}</span>
          <span className="meta-item">Updated: {metadata.updated ? new Date(metadata.updated / 1000000).toLocaleString() : '-'}</span>
        </div>
      )}
    </div>
  );
}

window.KeyEditor = KeyEditor;
