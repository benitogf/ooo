function ProxiesList({ clockConnected }) {
  const { useState, useEffect, useCallback, useRef } = React;
  const { IconRefresh, IconLink, IconChevronRight } = window.Icons;
  const prevConnected = useRef(clockConnected);

  const [proxies, setProxies] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [modalProxy, setModalProxy] = useState(null);
  const [paramValue, setParamValue] = useState('');

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const data = await Api.fetchProxies();
      setProxies(data || []);
    } catch (err) {
      console.error('Failed to load proxies:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  // Reload when clock reconnects
  useEffect(() => {
    if (clockConnected && !prevConnected.current) {
      loadData();
    }
    prevConnected.current = clockConnected;
  }, [clockConnected, loadData]);

  const filteredProxies = searchTerm
    ? proxies.filter(p => p.localPath.toLowerCase().includes(searchTerm.toLowerCase()))
    : proxies;

  const getTypeBadge = (type) => {
    switch (type) {
      case 'single': return <span className="badge-static">SINGLE</span>;
      case 'list': return <span className="badge-glob">LIST</span>;
      case 'vars': return <span className="badge-custom">VARS</span>;
      default: return <span className="badge-custom">{type.toUpperCase()}</span>;
    }
  };

  const getCapabilities = (proxy) => {
    const caps = [];
    if (proxy.canRead) caps.push('R');
    if (proxy.canWrite) caps.push('W');
    if (proxy.canDelete) caps.push('D');
    return caps.join('/');
  };

  // Check if path has dynamic parameter (glob * or needs user input)
  const needsParamInput = (localPath) => {
    return localPath.includes('*');
  };

  // Use shared validation from Api
  const isValidUrlParam = Api.isValidKeySegment;
  const getParamError = () => Api.getKeySegmentError(paramValue);

  const buildProxyUrl = (path, proxy, isList = false) => {
    const params = new URLSearchParams();
    params.set('source', 'proxies');
    params.set('canRead', proxy.canRead ? '1' : '0');
    params.set('canWrite', proxy.canWrite ? '1' : '0');
    params.set('canDelete', proxy.canDelete ? '1' : '0');
    // Default to edit mode if writable, live mode if read-only
    params.set('mode', proxy.canWrite ? 'edit' : 'live');
    if (isList) {
      params.set('type', 'list');
    }
    return '/proxy/view/' + encodeURIComponent(path) + '?' + params.toString();
  };

  const openProxy = (proxy) => {
    const { localPath, type } = proxy;
    
    // List type proxies go directly to list view
    if (type === 'list') {
      // For list proxies, we need to know the node ID first
      // Count wildcards - if more than 1, we need a param for the node ID
      const wildcardCount = (localPath.match(/\*/g) || []).length;
      if (wildcardCount > 1) {
        // Need node ID parameter - show modal
        setModalProxy(proxy);
        setParamValue('');
        return;
      }
      // Single wildcard list - open directly
      window.location.hash = buildProxyUrl(localPath, proxy, true);
      return;
    }
    
    // If path has wildcard, show modal for parameter input
    if (needsParamInput(localPath)) {
      setModalProxy(proxy);
      setParamValue('');
      return;
    }
    
    // Single proxies without wildcards open directly
    window.location.hash = buildProxyUrl(localPath, proxy);
  };

  const navigateWithParam = () => {
    if (!modalProxy || !paramValue.trim()) return;
    
    const { localPath, type } = modalProxy;
    // Replace first * with the user-provided value
    const resolvedPath = localPath.replace('*', paramValue.trim());
    
    // For list proxies with remaining wildcards, open as list view
    const isList = type === 'list' && resolvedPath.includes('*');
    
    // Navigate to the resolved path with capabilities
    window.location.hash = buildProxyUrl(resolvedPath, modalProxy, isList);
    setModalProxy(null);
    setParamValue('');
  };

  const closeModal = () => {
    setModalProxy(null);
    setParamValue('');
  };

  return (
    <div className="container">
      <div className="page-header">
        <div className="page-header-left">
          <h1 className="page-title">Proxy Routes</h1>
          <div className="stats-row">
            <div className="stat-item">
              <span className="dot purple"></span>
              <span>{proxies.length}</span> Proxies
            </div>
          </div>
        </div>
        <div className="header-actions">
          <input 
            type="text" 
            className="search-box" 
            placeholder="Search..." 
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
          />
          <button className="refresh-btn" onClick={loadData} title="Refresh">
            <IconRefresh />
          </button>
        </div>
      </div>

      <div className={`table-container ${loading ? 'loading' : ''}`}>
        <table className="data-table">
          <thead>
            <tr>
              <th>Local Path</th>
              <th>Type</th>
              <th>Capabilities</th>
            </tr>
          </thead>
          <tbody>
            {filteredProxies.length === 0 ? (
              <tr>
                <td colSpan="3">
                  <div className="empty-state">
                    <IconLink />
                    <div>No proxy routes registered</div>
                  </div>
                </td>
              </tr>
            ) : (
              filteredProxies.map((proxy, idx) => (
                <tr 
                  key={idx} 
                  onClick={() => openProxy(proxy)}
                  style={{ cursor: 'pointer' }}
                >
                  <td><span className="key-path">{proxy.localPath}</span></td>
                  <td>{getTypeBadge(proxy.type)}</td>
                  <td>
                    <span className="capabilities">
                      {proxy.canRead && <span className="cap-read" title="Read">R</span>}
                      {proxy.canWrite && <span className="cap-write" title="Write">W</span>}
                      {proxy.canDelete && <span className="cap-delete" title="Delete">D</span>}
                    </span>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Parameter Input Modal */}
      {modalProxy && (
        <div className="modal-overlay" onClick={closeModal}>
          <div className="modal-content" onClick={e => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Access Proxy Resource</h3>
              <button className="modal-close" onClick={closeModal}>&times;</button>
            </div>
            <div className="modal-body">
              <p className="modal-description">
                Enter the identifier to access a specific resource through this proxy.
              </p>
              <div className="modal-path-preview">
                <span className="path-label">Path:</span>
                <code>{modalProxy.localPath.replace('*', paramValue || '<id>')}</code>
              </div>
              <div className="form-group">
                <label>Resource ID</label>
                <input
                  type="text"
                  className={`form-input ${getParamError() ? 'input-error' : ''}`}
                  placeholder="Enter identifier (e.g., server-01, user-123)"
                  value={paramValue}
                  onChange={(e) => setParamValue(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && isValidUrlParam(paramValue) && navigateWithParam()}
                  autoFocus
                />
                {getParamError() && (
                  <span className="form-error">{getParamError()}</span>
                )}
              </div>
            </div>
            <div className="modal-footer">
              <button className="btn secondary" onClick={closeModal}>Cancel</button>
              <button 
                className="btn primary" 
                onClick={navigateWithParam}
                disabled={!isValidUrlParam(paramValue)}
              >
                View Resource
                <IconChevronRight />
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

window.ProxiesList = ProxiesList;
