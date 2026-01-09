function StatePage() {
  const { useState, useEffect, useCallback } = React;
  const { IconActivity, IconRefresh } = window.Icons;
  
  const [state, setState] = useState({ pools: [], totalConnections: 0 });
  const [loading, setLoading] = useState(false);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch('/?api=state');
      const data = await res.json();
      setState(data);
    } catch (err) {
      console.error('Failed to load state:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  return (
    <div className="container">
      <div className="page-header">
        <div className="page-header-left">
          <h1 className="page-title">Stream State</h1>
          <div className="stats-row">
            <div className="stat-item">
              <span className="dot green"></span>
              <span>{state.pools?.length || 0}</span> Active Pools
            </div>
            <div className="stat-item">
              <span className="dot blue"></span>
              <span>{state.totalConnections || 0}</span> Total Connections
            </div>
          </div>
        </div>
        <div className="header-actions">
          <button className="refresh-btn" onClick={loadData} title="Refresh">
            <IconRefresh />
          </button>
        </div>
      </div>

      <div className={`table-container ${loading ? 'loading' : ''}`}>
        <table className="data-table">
          <thead>
            <tr>
              <th>Key</th>
              <th style={{ textAlign: 'right' }}>Connections</th>
            </tr>
          </thead>
          <tbody>
            {(!state.pools || state.pools.length === 0) ? (
              <tr>
                <td colSpan="2">
                  <div className="empty-state">
                    <IconActivity />
                    <div>No active connections</div>
                  </div>
                </td>
              </tr>
            ) : (
              state.pools.map(pool => (
                <tr key={pool.key}>
                  <td><span className="key-path">{pool.key}</span></td>
                  <td style={{ textAlign: 'right' }}>
                    <span className="connection-count">{pool.connections}</span>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

window.StatePage = StatePage;
