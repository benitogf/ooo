function StorageList() {
  const { useState, useEffect, useCallback } = React;
  const { IconFilter, IconTrash, IconRefresh } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  
  const [filters, setFilters] = useState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [filterToClear, setFilterToClear] = useState('');

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch('/?api=filters');
      const data = await res.json();
      setFilters((data.paths || []).sort());
    } catch (err) {
      console.error('Failed to load filters:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const filteredFilters = searchTerm
    ? filters.filter(f => f.toLowerCase().includes(searchTerm.toLowerCase()))
    : filters;

  const openFilter = (path) => {
    const isGlob = path.includes('*');
    const type = isGlob ? 'glob' : 'static';
    window.location.hash = '/storage/key/' + type + '/' + encodeURIComponent(path);
  };

  const confirmClear = (e, path) => {
    e.stopPropagation();
    setFilterToClear(path);
    setModalVisible(true);
  };

  const executeClear = async () => {
    if (!filterToClear) return;
    try {
      const res = await fetch('/' + filterToClear, { method: 'DELETE' });
      if (!res.ok) throw new Error(await res.text());
      setFilterToClear('');
      setModalVisible(false);
      loadData();
    } catch (err) {
      console.error('Failed to clear:', err);
    }
  };

  return (
    <div className="container">
      <div className="page-header">
        <div className="page-header-left">
          <h1 className="page-title">Filters</h1>
          <div className="stats-row">
            <div className="stat-item">
              <span className="dot green"></span>
              <span>{filters.length}</span> Total
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
              <th>Type</th>
              <th style={{ textAlign: 'right' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {filteredFilters.length === 0 ? (
              <tr>
                <td colSpan="3">
                  <div className="empty-state">
                    <IconFilter />
                    <div>No filters registered</div>
                  </div>
                </td>
              </tr>
            ) : (
              filteredFilters.map(path => {
                const isGlob = path.includes('*');
                return (
                  <tr key={path} onClick={() => openFilter(path)}>
                    <td><span className="key-path">{path}</span></td>
                    <td>
                      <span className={isGlob ? 'badge-glob' : 'badge-static'}>
                        {isGlob ? 'GLOB' : 'STATIC'}
                      </span>
                    </td>
                    <td>
                      <div className="actions">
                        <button className="btn ghost" title="Clear" onClick={(e) => confirmClear(e, path)}>
                          <IconTrash />
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })
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
        onCancel={() => setModalVisible(false)}
      />
    </div>
  );
}

window.StorageList = StorageList;
