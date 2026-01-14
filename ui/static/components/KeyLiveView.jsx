function KeyLiveView({ keyPath, fromFilter, source }) {
  const { useState, useEffect, useRef, useMemo } = React;
  const { IconChevronLeft, IconEdit, IconWifi, IconWifiOff, IconFileText, IconChevronDown, IconChevronRight, IconCopy, IconCheck } = window.Icons;
  const ReactJson = window.reactJsonView ? window.reactJsonView.default : null;

  const [editorContent, setEditorContent] = useState(null);
  const [filterInfo, setFilterInfo] = useState(null);
  const [schemaExpanded, setSchemaExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const { data, connected, error, version } = Api.useSubscribe(keyPath);

  useEffect(() => {
    // Check filter type and redirect if needed
    const checkPath = fromFilter || keyPath;
    if (checkPath) {
      fetch('/?api=filters')
        .then(res => res.json())
        .then(data => {
          const info = (data.filters || []).find(f => 
            f.path === checkPath || checkPath.match(new RegExp('^' + f.path.replace(/\*/g, '[^/]+') + '$'))
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
    // Return unique descriptions
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
          {mergedDescription && (
            <span className="filter-description-badge" title={mergedDescription}>
              {mergedDescription.length > 50 ? mergedDescription.substring(0, 50) + '...' : mergedDescription}
            </span>
          )}
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

window.KeyLiveView = KeyLiveView;
