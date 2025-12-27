function StorageKeys({ filterPath, refresh }) {
  const { useState, useEffect, useCallback, useMemo } = React;
  const { IconBox, IconChevronLeft, IconChevronRight, IconTrash, IconSend } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  
  const [items, setItems] = useState([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [modalVisible, setModalVisible] = useState(false);
  const [keyToDelete, setKeyToDelete] = useState('');
  const limit = 50;

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

  const loadData = useCallback(async () => {
    if (!filterPath) return;
    setLoading(true);
    setPage(1);
    try {
      const res = await fetch('/' + filterPath);
      const data = await res.json();
      const itemsArray = Array.isArray(data) ? data : [data];
      setItems(itemsArray.filter(item => item && item.index));
    } catch (err) {
      console.error('Failed to load data:', err);
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, [filterPath]);

  useEffect(() => {
    loadData();
  }, [loadData, refresh]);

  const dataColumns = useMemo(() => {
    const columns = new Set();
    items.forEach(item => {
      if (item.data && typeof item.data === 'object') {
        const flatData = flattenData(item.data);
        Object.keys(flatData).forEach(key => columns.add(key));
      }
    });
    return Array.from(columns).sort();
  }, [items]);

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
    setModalVisible(true);
  };

  const executeDelete = async () => {
    if (!keyToDelete) return;
    try {
      const res = await fetch('/' + keyToDelete, { method: 'DELETE' });
      if (!res.ok) throw new Error(await res.text());
      setKeyToDelete('');
      setModalVisible(false);
      loadData();
    } catch (err) {
      console.error('Failed to delete:', err);
    }
  };

  const renderCellValue = (value) => {
    if (value === null || value === undefined) return <span className="text-muted">-</span>;
    if (typeof value === 'boolean') return <span className={value ? 'text-success' : 'text-muted'}>{value ? 'true' : 'false'}</span>;
    if (typeof value === 'string' && value.length > 50) return <span title={value}>{value.substring(0, 50)}...</span>;
    return String(value);
  };

  const totalColumns = 3 + dataColumns.length;

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
          <button className="btn" onClick={pushData}>
            <IconSend />
            Push
          </button>
        </div>
      </div>

      <div className={`table-container table-scroll ${loading ? 'loading' : ''}`}>
        <table className="data-table">
          <thead>
            <tr>
              <th>Created</th>
              <th>Updated</th>
              {dataColumns.map(col => (
                <th key={col}>{col}</th>
              ))}
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
                    {dataColumns.map(col => (
                      <td key={col}>{renderCellValue(flatData[col])}</td>
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
        onCancel={() => setModalVisible(false)}
      />
    </div>
  );
}

window.StorageKeys = StorageKeys;
