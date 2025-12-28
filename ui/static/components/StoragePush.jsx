function StoragePush({ filterPath }) {
  const { useState, useRef } = React;
  const { IconChevronLeft } = window.Icons;
  const JsonEditorWrapper = window.JsonEditorWrapper;
  
  const [pushing, setPushing] = useState(false);
  const [status, setStatus] = useState({ type: '', message: '' });
  const editorRef = useRef(null);

  const goBack = () => {
    window.location.hash = '/storage/key/glob/' + encodeURIComponent(filterPath);
  };

  const push = async () => {
    setPushing(true);
    setStatus({ type: '', message: '' });
    
    const minDelay = new Promise(r => setTimeout(r, 500));
    
    try {
      const content = editorRef.current?.getContent() || { json: {} };
      const dataToSave = content.json !== undefined 
        ? JSON.stringify(content.json) 
        : content.text !== undefined ? content.text : '{}';
      
      const [res] = await Promise.all([
        fetch('/' + filterPath, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: dataToSave
        }),
        minDelay
      ]);
      
      if (!res.ok) throw new Error(await res.text());
      goBack();
    } catch (err) {
      setPushing(false);
      setStatus({ type: 'error', message: 'Failed: ' + err.message });
      setTimeout(() => setStatus({ type: '', message: '' }), 3000);
    }
  };

  if (pushing) {
    return (
      <div className="container">
        <div className="edit-page-header">
          <button className="btn secondary" onClick={goBack}>
            <IconChevronLeft />
            Back
          </button>
          <span className="edit-page-title">Push to: {filterPath}</span>
        </div>
        <div className="loading-container">
          <div className="spinner"></div>
          <div>Pushing data...</div>
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
        <span className="edit-page-title">Push to: {filterPath}</span>
      </div>
      <div className="editor-wrapper">
        <JsonEditorWrapper content={{}} editorRef={editorRef} />
      </div>
      <div className="edit-page-actions">
        {status.message && <div className={`status ${status.type}`}>{status.message}</div>}
        <button className="btn" onClick={push}>Push Data</button>
      </div>
    </div>
  );
}

window.StoragePush = StoragePush;
