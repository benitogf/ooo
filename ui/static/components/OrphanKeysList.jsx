function OrphanKeysList({ clockConnected }) {
  const { useState, useEffect, useCallback, useRef } = React;
  const { IconRefresh, IconWarning } = window.Icons;

  const [orphanKeys, setOrphanKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const prevConnected = useRef(clockConnected);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const data = await Api.fetchOrphanKeys();
      setOrphanKeys(data || []);
    } catch (err) {
      console.error('Failed to load orphan keys:', err);
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

  const filteredKeys = searchTerm
    ? orphanKeys.filter(k => k.toLowerCase().includes(searchTerm.toLowerCase()))
    : orphanKeys;

  return (
    <div className="container">
      <div className="page-header">
        <div className="page-header-left">
          <h1 className="page-title">Orphan Keys</h1>
          <div className="stats-row">
            <div className="stat-item">
              <span className="dot orange"></span>
              <span>{orphanKeys.length}</span> Keys without filters
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

      {orphanKeys.length > 0 && (
        <div className="warning-banner">
          <IconWarning />
          <span>These keys exist in storage but are not exposed through any filter. They cannot be read via the API.</span>
        </div>
      )}

      <div className={`table-container ${loading ? 'loading' : ''}`}>
        <table className="data-table">
          <thead>
            <tr>
              <th>Key Path</th>
            </tr>
          </thead>
          <tbody>
            {filteredKeys.length === 0 ? (
              <tr>
                <td>
                  <div className="empty-state success">
                    <div>No orphan keys found</div>
                    <div className="empty-state-sub">All stored keys are covered by filters</div>
                  </div>
                </td>
              </tr>
            ) : (
              filteredKeys.map((key, idx) => (
                <tr key={idx}>
                  <td><span className="key-path orphan">{key}</span></td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

window.OrphanKeysList = OrphanKeysList;
