function FiltersList({ clockConnected }) {
  const { useState, useEffect, useCallback, useRef } = React;
  const { IconFilter, IconTrash, IconRefresh } = window.Icons;
  const ConfirmModal = window.ConfirmModal;

  const [filtersInfo, setFiltersInfo] = useState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [filterToClear, setFilterToClear] = useState('');
  const [modalLoading, setModalLoading] = useState(false);
  const [modalError, setModalError] = useState('');
  const prevConnected = useRef(clockConnected);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch('/?api=filters');
      const data = await res.json();
      // Use detailed filters info if available, otherwise fall back to paths
      if (data.filters && data.filters.length > 0) {
        setFiltersInfo(data.filters.sort((a, b) => a.path.localeCompare(b.path)));
      } else {
        // Fallback for backwards compatibility
        const paths = (data.paths || []).sort();
        setFiltersInfo(paths.map(path => ({
          path,
          type: path.includes('*') ? 'open' : 'custom',
          isGlob: path.includes('*'),
          canRead: true,
          canWrite: true,
          canDelete: true
        })));
      }
    } catch (err) {
      console.error('Failed to load filters:', err);
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

  const filteredFilters = searchTerm
    ? filtersInfo.filter(f => f.path.toLowerCase().includes(searchTerm.toLowerCase()))
    : filtersInfo;

  const openFilter = (filter) => {
    const { path, isGlob, type } = filter;

    // Custom filters have no client access - don't navigate
    if (type === 'custom') {
      return;
    }

    // Glob filters always open in live mode
    if (isGlob) {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent(path);
      return;
    }

    // Read-only static filters go to live mode
    if (type === 'read-only') {
      window.location.hash = '/storage/key/live/' + encodeURIComponent(path);
      return;
    }

    // Other static filters go to edit mode
    window.location.hash = '/storage/key/static/' + encodeURIComponent(path);
  };

  const getFilterTypeBadge = (filter) => {
    const { type, limit, limitDynamic, canRead, canWrite } = filter;
    if (type === 'limit') {
      const limitDisplay = limitDynamic ? `${limit}*` : limit;
      return <span className="badge-limit" title={limitDynamic ? 'Dynamic limit (current value shown)' : 'Fixed limit'}>LIMIT ({limitDisplay})</span>;
    }
    if (type === 'open') {
      return <span className="badge-open">OPEN</span>;
    }
    if (type === 'read-only') {
      return <span className="badge-readonly">READ ONLY</span>;
    }
    if (type === 'write-only') {
      return <span className="badge-writeonly">WRITE ONLY</span>;
    }
    return <span className="badge-custom">CUSTOM</span>;
  };

  const confirmClear = (e, path) => {
    e.stopPropagation();
    setFilterToClear(path);
    setModalError('');
    setModalVisible(true);
  };

  const executeClear = async () => {
    if (!filterToClear || modalLoading) return;
    setModalLoading(true);
    setModalError('');
    try {
      const res = await fetch('/' + filterToClear, { method: 'DELETE' });
      if (!res.ok) throw new Error(await res.text());
      setFilterToClear('');
      setModalVisible(false);
      loadData();
    } catch (err) {
      setModalError('Failed to clear: ' + (err.message || 'Unknown error'));
    } finally {
      setModalLoading(false);
    }
  };

  const closeModal = () => {
    if (modalLoading) return;
    setModalVisible(false);
    setModalError('');
    setFilterToClear('');
  };

  return (
    <div className="container">
      <div className="page-header">
        <div className="page-header-left">
          <h1 className="page-title">Filters</h1>
          <div className="stats-row">
            <div className="stat-item">
              <span className="dot green"></span>
              <span>{filtersInfo.length}</span> Total
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
              <th>Filter Path</th>
              <th>Pattern</th>
              <th>Access</th>
              <th className="actions-col"><IconTrash color="#3a4a5a" /></th>
            </tr>
          </thead>
          <tbody>
            {filteredFilters.length === 0 ? (
              <tr>
                <td colSpan="4">
                  <div className="empty-state">
                    <IconFilter />
                    <div>No filters registered</div>
                  </div>
                </td>
              </tr>
            ) : (
              filteredFilters.map(filter => (
                <tr
                  key={filter.path}
                  onClick={() => openFilter(filter)}
                  className={filter.type === 'custom' ? 'row-disabled' : ''}
                  style={filter.type === 'custom' ? { cursor: 'default' } : {}}
                >
                  <td><span className="key-path">{filter.path}</span></td>
                  <td>
                    <span className={filter.isGlob ? 'badge-glob' : 'badge-static'}>
                      {filter.isGlob ? 'GLOB' : 'STATIC'}
                    </span>
                  </td>
                  <td>{getFilterTypeBadge(filter)}</td>
                  <td className="actions-col">
                    <div className="actions">
                      {filter.canDelete && (
                        <button className="btn ghost" title="Clear" onClick={(e) => confirmClear(e, filter.path)}>
                          <IconTrash />
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <ConfirmModal
        visible={modalVisible}
        title="Clear Filter"
        message={`Are you sure you want to clear all data for: ${filterToClear}?`}
        confirmText="Clear"
        danger={true}
        onConfirm={executeClear}
        onCancel={closeModal}
        loading={modalLoading}
        error={modalError}
      />
    </div>
  );
}

window.FiltersList = FiltersList;
