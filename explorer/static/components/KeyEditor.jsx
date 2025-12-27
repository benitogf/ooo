function KeyEditor({ keyPath, filterPath, isCreate }) {
  const { useState, useEffect, useRef } = React;
  const { IconChevronLeft } = window.Icons;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState({ type: '', message: '' });
  const [editorData, setEditorData] = useState(null);
  const editorRef = useRef(null);

  useEffect(() => {
    if (isCreate) {
      setEditorData({});
      return;
    }
    if (!keyPath) return;
    
    setLoading(true);
    fetch('/' + keyPath)
      .then(async res => {
        if (res.ok) {
          const data = await res.json();
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
        }
      })
      .catch(() => setEditorData({}))
      .finally(() => setLoading(false));
  }, [keyPath, isCreate]);

  const goBack = () => {
    if (filterPath && filterPath.includes('*')) {
      window.location.hash = '/storage/key/glob/' + encodeURIComponent(filterPath);
    } else {
      window.location.hash = '/storage';
    }
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
      goBack();
    } catch (err) {
      setSaving(false);
      setStatus({ type: 'error', message: 'Failed: ' + err.message });
      setTimeout(() => setStatus({ type: '', message: '' }), 3000);
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
    <div className="container">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">{isCreate ? 'Create Key' : 'Edit: ' + keyPath}</span>
      </div>
      <div className="edit-page-content">
        {editorData !== null && <JsonEditorWrapper content={editorData} editorRef={editorRef} />}
      </div>
      <div className="edit-page-actions">
        {status.message && <div className={`status ${status.type}`}>{status.message}</div>}
        <button className="btn" onClick={save}>
          {isCreate ? 'Create Key' : 'Save Changes'}
        </button>
      </div>
    </div>
  );
}

window.KeyEditor = KeyEditor;
