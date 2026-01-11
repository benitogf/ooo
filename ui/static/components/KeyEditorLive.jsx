function KeyEditorLive({ keyPath, fromFilter, source }) {
  const { useState, useEffect, useRef } = React;
  const { IconChevronLeft, IconEdit, IconWifi, IconWifiOff } = window.Icons;
  const ReactJson = window.reactJsonView ? window.reactJsonView.default : null;

  const [editorContent, setEditorContent] = useState(null);
  const [filterInfo, setFilterInfo] = useState(null);
  const { data, connected, error, version } = Api.useSubscribe(keyPath);

  useEffect(() => {
    // Check filter type and redirect if needed
    const checkPath = fromFilter || keyPath;
    if (checkPath) {
      fetch('/?api=filters')
        .then(res => res.json())
        .then(data => {
          const info = (data.filters || []).find(f => 
            f.path === checkPath || checkPath.match(new RegExp('^' + f.path.replace('*', '[^/]+') + '$'))
          );
          setFilterInfo(info);
          // Custom filters have no client access
          if (info && info.type === 'custom') {
            window.location.hash = '/storage';
          }
        })
        .catch(() => {});
    }
  }, [keyPath, fromFilter]);

  useEffect(() => {
    if (data && data.data) {
      try {
        const parsed = typeof data.data === 'string' ? JSON.parse(data.data) : data.data;
        setEditorContent(parsed);
      } catch (e) {
        setEditorContent({ value: data.data });
      }
    }
  }, [data, version]);

  const goBack = () => {
    if (source === 'proxies') {
      window.location.hash = '/proxies';
    } else if (fromFilter) {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent(fromFilter);
    } else {
      window.location.hash = '/storage';
    }
  };

  const switchToEdit = () => {
    const params = fromFilter ? '?from=' + encodeURIComponent(fromFilter) : '';
    window.location.hash = '/storage/key/static/' + encodeURIComponent(keyPath) + params;
  };

  return (
    <div className="container editor-page">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">
          Live: {keyPath}
          <span className={`connection-status ${connected ? 'connected' : 'disconnected'}`}>
            {connected ? <IconWifi /> : <IconWifiOff />}
          </span>
        </span>
        <div className="header-right">
          {filterInfo && filterInfo.type !== 'read-only' && (
            <button className="btn secondary" onClick={switchToEdit} title="Switch to Edit">
              <IconEdit />
              Edit
            </button>
          )}
        </div>
      </div>

      {error && (
        <div className="error-banner">
          Connection error: {error.message || 'Failed to connect'}
        </div>
      )}

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

      {data && (
        <div className="meta-info">
          <span className="meta-item">Created: {data.created ? new Date(data.created / 1000000).toLocaleString() : '-'}</span>
          <span className="meta-item">Updated: {data.updated ? new Date(data.updated / 1000000).toLocaleString() : '-'}</span>
        </div>
      )}
    </div>
  );
}

window.KeyEditorLive = KeyEditorLive;
