function StorageKeys({ filterPath, refresh }) {
  const { useState, useEffect, useCallback, useMemo } = React;
  const { IconBox, IconChevronLeft, IconChevronRight, IconChevronDown, IconChevronUp, IconTrash, IconSend, IconLive } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  
  const [items, setItems] = useState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [modalVisible, setModalVisible] = useState(false);
  const [keyToDelete, setKeyToDelete] = useState('');
  const [collapsedFields, setCollapsedFields] = useState({});
  const [filterInfo, setFilterInfo] = useState(null);
  const [modalLoading, setModalLoading] = useState(false);
  const [modalError, setModalError] = useState('');
  const limit = 50;

  useEffect(() => {
    fetch('/?api=filters')
      .then(res => res.json())
      .then(data => {
        const info = (data.filters || []).find(f => f.path === filterPath);
        setFilterInfo(info);
        // Redirect read-only and custom filters to appropriate view
        if (info) {
          if (info.type === 'read-only') {
            window.location.hash = '/storage/keys/live/' + encodeURIComponent(filterPath);
          } else if (info.type === 'custom') {
            window.location.hash = '/storage';
          }
        }
      })
      .catch(() => {});
  }, [filterPath]);

  const formatTime = (timestamp) => {
    if (!timestamp) return '-';
    // Timestamps are in nanoseconds, convert to milliseconds
    const ms = Math.floor(timestamp / 1000000);
    const date = new Date(ms);
    return date.toLocaleString();
  };

  const flattenData = (data, prefix = '') => {
    if (!data || typeof data !== 'object') return {};
    const result = {};
    for (const [key, value] of Object.entries(data)) {
      const fullKey = prefix ? `${prefix}.${key}` : key;
      if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
        Object.assign(result, flattenData(value, fullKey));
      } else if (Array.isArray(value)) {
        value.forEach((item, idx) => {
          if (item !== null && typeof item === 'object') {
            Object.assign(result, flattenData(item, `${fullKey}.${idx}`));
          } else {
            result[`${fullKey}.${idx}`] = item;
          }
        });
      } else {
        result[fullKey] = value;
      }
    }
    return result;
  };

  // Helper to compute columns and collapsed state from items
  const computeInitialCollapsedState = (filteredItems) => {
    const columns = new Set();
    filteredItems.forEach(item => {
      if (item.data && typeof item.data === 'object') {
        const flatData = flattenData(item.data);
        Object.keys(flatData).forEach(key => columns.add(key));
      }
    });
    const allCols = Array.from(columns).sort();
    
    // Find collapsible paths
    const paths = new Set();
    allCols.forEach(col => {
      const parts = col.split('.');
      for (let i = 1; i <= parts.length; i++) {
        const path = parts.slice(0, i).join('.');
        const hasChildren = allCols.some(c => c !== path && c.startsWith(path + '.'));
        if (hasChildren) {
          paths.add(path);
        }
      }
    });
    
    // Collapse top-level paths
    const collapsed = {};
    Array.from(paths).filter(p => !p.includes('.')).forEach(path => {
      collapsed[path] = true;
    });
    return collapsed;
  };

  const loadData = useCallback(async () => {
    if (!filterPath) return;
    setLoading(true);
    setPage(1);
    
    // Minimum delay to avoid flashing and make transition smoother
    const minDelay = new Promise(r => setTimeout(r, 300));
    
    try {
      const [res] = await Promise.all([
        fetch('/' + filterPath),
        minDelay
      ]);
      const data = await res.json();
      const itemsArray = Array.isArray(data) ? data : [data];
      const filteredItems = itemsArray.filter(item => item && item.index);
      
      // Compute and set collapsed state BEFORE setting items
      if (filteredItems.length > 0) {
        const initialCollapsed = computeInitialCollapsedState(filteredItems);
        setCollapsedFields(prev => ({ ...prev, ...initialCollapsed }));
      }
      
      setItems(filteredItems);
      setLoading(false);
    } catch (err) {
      console.error('Failed to load data:', err);
      setItems([]);
      setLoading(false);
    }
  }, [filterPath]);

  useEffect(() => {
    loadData();
  }, [loadData, refresh]);

  const allDataColumns = useMemo(() => {
    const columns = new Set();
    items.forEach(item => {
      if (item.data && typeof item.data === 'object') {
        const flatData = flattenData(item.data);
        Object.keys(flatData).forEach(key => columns.add(key));
      }
    });
    return Array.from(columns).sort();
  }, [items]);

  const collapsiblePaths = useMemo(() => {
    const paths = new Set();
    allDataColumns.forEach(col => {
      const parts = col.split('.');
      for (let i = 1; i <= parts.length; i++) {
        const path = parts.slice(0, i).join('.');
        const hasChildren = allDataColumns.some(c => c !== path && c.startsWith(path + '.'));
        if (hasChildren) {
          paths.add(path);
        }
      }
    });
    return Array.from(paths).sort();
  }, [allDataColumns]);


  const isPathCollapsed = (path) => {
    const parts = path.split('.');
    for (let i = 1; i <= parts.length; i++) {
      const checkPath = parts.slice(0, i).join('.');
      if (collapsedFields[checkPath]) {
        return true;
      }
    }
    return false;
  };

  const visibleColumns = useMemo(() => {
    const result = [];
    const addedCollapsedPaths = new Set();
    
    allDataColumns.forEach(col => {
      const parts = col.split('.');
      let collapsed = false;
      
      for (let i = 1; i < parts.length; i++) {
        const parentPath = parts.slice(0, i).join('.');
        if (collapsedFields[parentPath]) {
          if (!addedCollapsedPaths.has(parentPath)) {
            result.push(parentPath);
            addedCollapsedPaths.add(parentPath);
          }
          collapsed = true;
          break;
        }
      }
      
      if (!collapsed) {
        result.push(col);
      }
    });
    
    return [...new Set(result)].sort();
  }, [allDataColumns, collapsedFields]);

  const getTopLevelParent = (col) => {
    const parts = col.split('.');
    if (parts.length > 1) {
      return parts[0];
    }
    return null;
  };

  const getCollapsibleAncestors = (col) => {
    const parts = col.split('.');
    const ancestors = [];
    for (let i = 1; i < parts.length; i++) {
      const path = parts.slice(0, i).join('.');
      if (collapsiblePaths.includes(path)) {
        ancestors.push(path);
      }
    }
    return ancestors;
  };

  const getHiddenCount = (path) => {
    return allDataColumns.filter(c => c.startsWith(path + '.')).length;
  };

  const toggleCollapse = (path) => {
    setCollapsedFields(prev => ({ ...prev, [path]: !prev[path] }));
  };

  const isCollapsible = (col) => collapsiblePaths.includes(col);

  const filteredItems = searchTerm
    ? items.filter(item => {
        const searchLower = searchTerm.toLowerCase();
        if (item.index && item.index.toLowerCase().includes(searchLower)) return true;
        if (item.data) {
          const flatData = flattenData(item.data);
          return Object.values(flatData).some(v => 
            String(v).toLowerCase().includes(searchLower)
          );
        }
        return false;
      })
    : items;

  const totalPages = Math.max(1, Math.ceil(filteredItems.length / limit));
  const start = (page - 1) * limit;
  const paginatedItems = filteredItems.slice(start, start + limit);

  const goBack = () => {
    window.location.hash = '/storage';
  };

  const switchToLive = () => {
    window.location.hash = '/storage/keys/live/' + encodeURIComponent(filterPath);
  };

  const editKey = (index) => {
    const keyPath = filterPath.replace('*', index);
    window.location.hash = '/storage/key/static/' + encodeURIComponent(keyPath) + '?from=' + encodeURIComponent(filterPath);
  };

  const pushData = () => {
    window.location.hash = '/storage/push/' + encodeURIComponent(filterPath);
  };

  const confirmDelete = (e, item) => {
    e.stopPropagation();
    const keyPath = filterPath.replace('*', item.index);
    setKeyToDelete(keyPath);
    setModalError('');
    setModalVisible(true);
  };

  const executeDelete = async () => {
    if (!keyToDelete || modalLoading) return;
    setModalLoading(true);
    setModalError('');
    try {
      const res = await fetch('/' + keyToDelete, { method: 'DELETE' });
      if (!res.ok) throw new Error(await res.text());
      setKeyToDelete('');
      setModalVisible(false);
      loadData();
    } catch (err) {
      setModalError('Failed to delete: ' + (err.message || 'Unknown error'));
    } finally {
      setModalLoading(false);
    }
  };

  const closeModal = () => {
    if (modalLoading) return;
    setModalVisible(false);
    setModalError('');
    setKeyToDelete('');
  };

  const renderCellValue = (value) => {
    if (value === null || value === undefined) return <span className="text-muted">-</span>;
    if (typeof value === 'boolean') return <span className={value ? 'text-bool-true' : 'text-bool-false'}>{value ? 'true' : 'false'}</span>;
    if (typeof value === 'number') {
      const isInt = Number.isInteger(value);
      return <span className={isInt ? 'text-int' : 'text-float'}>{value}</span>;
    }
    if (typeof value === 'string') {
      if (value.length > 50) return <span className="text-string" title={value}>{value.substring(0, 50)}...</span>;
      return <span className="text-string">{value}</span>;
    }
    return String(value);
  };

  const totalColumns = 3 + visibleColumns.length;

  return (
    <div className="container">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">{filterPath}</span>
        <div className="header-right">
          <input 
            type="text" 
            className="search-box" 
            placeholder="Search..." 
            value={searchTerm}
            onChange={(e) => { setSearchTerm(e.target.value); setPage(1); }}
            style={{ width: '200px' }}
          />
          <button className="btn secondary" onClick={switchToLive} title="Switch to Live Mode">
            <IconLive />
            Live
          </button>
          <button className="btn" onClick={pushData}>
            <IconSend />
            Push
          </button>
        </div>
      </div>

      {loading ? (
        <div className="table-container">
          <div className="loading-container">
            <div className="spinner"></div>
            <div>Loading data...</div>
          </div>
        </div>
      ) : (
      <div className="table-container table-scroll">
        <table className="data-table">
          <thead>
            <tr>
              <th>Created</th>
              <th>Updated</th>
              {visibleColumns.map(col => {
                const isCollapsed = collapsedFields[col];
                const canCollapse = isCollapsible(col);
                const ancestors = getCollapsibleAncestors(col);
                const immediateParent = ancestors.length > 0 ? ancestors[ancestors.length - 1] : null;
                
                if (canCollapse && isCollapsed) {
                  const parentOfCollapsed = getCollapsibleAncestors(col).filter(a => a !== col);
                  const canCollapseHigher = parentOfCollapsed.length > 0;
                  return (
                    <th key={col} className="collapsible-header">
                      <span className="collapse-toggle">
                        <span className="collapse-btn" onClick={(e) => { e.stopPropagation(); toggleCollapse(col); }} title="Expand">
                          <IconChevronRight />
                        </span>
                        {col}
                        <span className="collapsed-count">({getHiddenCount(col)})</span>
                        {canCollapseHigher && (
                          <span className="collapse-btn collapse-up" onClick={(e) => { e.stopPropagation(); toggleCollapse(parentOfCollapsed[parentOfCollapsed.length - 1]); }} title={`Collapse to ${parentOfCollapsed[parentOfCollapsed.length - 1]}`}>
                            <IconChevronUp />
                          </span>
                        )}
                      </span>
                    </th>
                  );
                } else if (immediateParent) {
                  return (
                    <th key={col} className="collapsible-header" onClick={(e) => { e.stopPropagation(); toggleCollapse(immediateParent); }}>
                      <span className="collapse-toggle">
                        <IconChevronDown />
                        {col}
                      </span>
                    </th>
                  );
                } else {
                  return <th key={col}>{col}</th>;
                }
              })}
              <th style={{ textAlign: 'right' }}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {paginatedItems.length === 0 ? (
              <tr>
                <td colSpan={totalColumns}>
                  <div className="empty-state">
                    <IconBox />
                    <div>No data found</div>
                  </div>
                </td>
              </tr>
            ) : (
              paginatedItems.map(item => {
                const flatData = flattenData(item.data || {});
                return (
                  <tr key={item.index} onClick={() => editKey(item.index)}>
                    <td><span className="text-muted">{formatTime(item.created)}</span></td>
                    <td><span className="text-muted">{formatTime(item.updated)}</span></td>
                    {visibleColumns.map(col => (
                      <td key={col}>
                        {isCollapsible(col) && collapsedFields[col] 
                          ? <span className="text-muted">...</span>
                          : renderCellValue(flatData[col])
                        }
                      </td>
                    ))}
                    <td>
                      <div className="actions">
                        <button className="btn ghost" title="Delete" onClick={(e) => confirmDelete(e, item)}>
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
      )}

      {items.length > 50 && (
        <div className="pagination">
          <button 
            className="btn secondary sm" 
            onClick={() => setPage(p => Math.max(1, p - 1))} 
            disabled={page <= 1}
          >
            <IconChevronLeft />
            Prev
          </button>
          <span className="page-info">Page {page} of {totalPages}</span>
          <button 
            className="btn secondary sm" 
            onClick={() => setPage(p => Math.min(totalPages, p + 1))} 
            disabled={page >= totalPages}
          >
            Next
            <IconChevronRight />
          </button>
        </div>
      )}

      <ConfirmModal
        visible={modalVisible}
        title="Delete Key"
        message={`Are you sure you want to delete: ${keyToDelete}?`}
        confirmText="Delete"
        danger={true}
        onConfirm={executeDelete}
        onCancel={closeModal}
        loading={modalLoading}
        error={modalError}
      />
    </div>
  );
}

window.StorageKeys = StorageKeys;
