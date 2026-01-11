function ProxyListView({ proxyPath, canRead, canWrite, canDelete }) {
  const { useState, useEffect, useCallback, useMemo, useRef } = React;
  const { IconBox, IconChevronLeft, IconChevronRight, IconChevronDown, IconChevronUp, IconTrash, IconEdit, IconEye, IconWifi, IconWifiOff } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  const EditKeyModal = window.EditKeyModal;
  const JsonTreeCell = window.JsonTreeCell;
  
  const [searchTerm, setSearchTerm] = useState('');
  const [page, setPage] = useState(1);
  const [modalVisible, setModalVisible] = useState(false);
  const [keyToDelete, setKeyToDelete] = useState('');
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [keyToEdit, setKeyToEdit] = useState('');
  const getStorageKey = () => `ooo-collapsed-proxy-${proxyPath}`;
  
  const loadCollapsedState = () => {
    try {
      const saved = localStorage.getItem(getStorageKey());
      return saved ? JSON.parse(saved) : {};
    } catch {
      return {};
    }
  };
  
  const [collapsedFields, setCollapsedFields] = useState(loadCollapsedState);
  const [tableReady, setTableReady] = useState(false);
  const [highlightedItems, setHighlightedItems] = useState({});
  const [modalLoading, setModalLoading] = useState(false);
  const [modalError, setModalError] = useState('');
  const [sortConfig, setSortConfig] = useState({ key: null, direction: 'asc' });
  const [selectedItems, setSelectedItems] = useState(new Set());
  const [batchDeleteVisible, setBatchDeleteVisible] = useState(false);
  const [batchDeleteLoading, setBatchDeleteLoading] = useState(false);
  const [batchDeleteError, setBatchDeleteError] = useState('');
  const [batchDeleteProgress, setBatchDeleteProgress] = useState({ done: 0, total: 0 });
  const prevItemsLengthRef = useRef(0);
  const tableContainerRef = useRef(null);
  const limit = 50;

  const { items, connected, error, lastUpdate, clearLastUpdate } = Api.useSubscribeGlob(proxyPath);

  useEffect(() => {
    if (lastUpdate.type && lastUpdate.index && lastUpdate.timestamp) {
      const { index, type, timestamp } = lastUpdate;
      
      setHighlightedItems(prev => ({
        ...prev,
        [index]: { type, timestamp }
      }));
      
      const timer = setTimeout(() => {
        setHighlightedItems(prev => {
          const next = { ...prev };
          if (next[index] && next[index].timestamp === timestamp) {
            delete next[index];
          }
          return next;
        });
        clearLastUpdate();
      }, 2000);
      
      return () => clearTimeout(timer);
    }
  }, [lastUpdate, clearLastUpdate]);

  const extractGlobValues = useCallback((itemPath) => {
    if (!itemPath) return '';
    const filterParts = proxyPath.split('/');
    const pathParts = itemPath.split('/');
    const globValues = [];
    
    filterParts.forEach((part, idx) => {
      if (part === '*' && pathParts[idx]) {
        globValues.push(pathParts[idx]);
      }
    });
    
    return globValues.join('/');
  }, [proxyPath]);

  // Build the local proxy path for an item using the proxyPath pattern and item index
  const buildLocalPath = useCallback((itemIndex) => {
    if (!itemIndex) return proxyPath;
    // Replace the last wildcard with the item index
    const parts = proxyPath.split('/');
    // Find the last wildcard and replace it
    for (let i = parts.length - 1; i >= 0; i--) {
      if (parts[i] === '*') {
        parts[i] = itemIndex;
        break;
      }
    }
    return parts.join('/');
  }, [proxyPath]);

  const formatTime = (timestamp) => {
    if (!timestamp) return '-';
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

  useEffect(() => {
    if (items.length > 0 && prevItemsLengthRef.current === 0) {
      const savedState = loadCollapsedState();
      const newCollapsedFields = { ...savedState };
      
      collapsiblePaths.forEach(path => {
        if (!(path in newCollapsedFields)) {
          newCollapsedFields[path] = true;
        }
      });
      
      setCollapsedFields(newCollapsedFields);
      requestAnimationFrame(() => {
        setTableReady(true);
      });
    }
    prevItemsLengthRef.current = items.length;
  }, [items.length, collapsiblePaths]);

  useEffect(() => {
    if (Object.keys(collapsedFields).length > 0) {
      try {
        localStorage.setItem(getStorageKey(), JSON.stringify(collapsedFields));
      } catch {}
    }
  }, [collapsedFields]);

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
    
    const uniqueResult = [...new Set(result)];
    
    const isNestedOrCollapsible = (col) => {
      if (collapsiblePaths.includes(col)) return true;
      for (const path of collapsiblePaths) {
        if (col.startsWith(path + '.')) return true;
      }
      return false;
    };
    
    return uniqueResult.sort((a, b) => {
      const aIsNested = isNestedOrCollapsible(a);
      const bIsNested = isNestedOrCollapsible(b);
      if (!aIsNested && bIsNested) return -1;
      if (aIsNested && !bIsNested) return 1;
      return a.localeCompare(b);
    });
  }, [allDataColumns, collapsedFields, collapsiblePaths]);

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

  const toggleCollapse = (path, expand = false) => {
    const isCurrentlyCollapsed = collapsedFields[path];
    
    if (isCurrentlyCollapsed && expand) {
      const immediateChildren = collapsiblePaths.filter(p => {
        if (!p.startsWith(path + '.')) return false;
        const remainder = p.slice(path.length + 1);
        return !remainder.includes('.');
      });
      
      setCollapsedFields(prev => {
        const next = { ...prev, [path]: false };
        immediateChildren.forEach(child => {
          next[child] = true;
        });
        return next;
      });
    } else {
      setCollapsedFields(prev => ({ ...prev, [path]: !prev[path] }));
    }
    
    if (!isCurrentlyCollapsed) {
      setTimeout(() => {
        const header = document.querySelector(`th[data-path="${path}"]`);
        if (header && tableContainerRef.current) {
          header.scrollIntoView({ behavior: 'smooth', inline: 'center', block: 'nearest' });
        }
      }, 50);
    }
  };

  const isCollapsible = (col) => collapsiblePaths.includes(col);

  const filteredItems = useMemo(() => {
    return searchTerm
      ? items.filter(item => {
          const searchLower = searchTerm.toLowerCase();
          if (item.index && item.index.toLowerCase().includes(searchLower)) return true;
          if (item.path && item.path.toLowerCase().includes(searchLower)) return true;
          const globValues = extractGlobValues(item.path);
          if (globValues && globValues.toLowerCase().includes(searchLower)) return true;
          if (item.data) {
            const flatData = flattenData(item.data);
            return Object.values(flatData).some(v => 
              String(v).toLowerCase().includes(searchLower)
            );
          }
          return false;
        })
      : items;
  }, [items, searchTerm, flattenData]);

  useEffect(() => {
    const visibleIndices = new Set(filteredItems.map(item => item.index));
    setSelectedItems(prev => {
      const newSelected = new Set([...prev].filter(idx => visibleIndices.has(idx)));
      if (newSelected.size !== prev.size) {
        return newSelected;
      }
      return prev;
    });
  }, [filteredItems]);

  const sortedItems = useMemo(() => {
    if (!sortConfig.key) return filteredItems;
    
    return [...filteredItems].sort((a, b) => {
      let aVal, bVal;
      
      if (sortConfig.key === 'index') {
        aVal = a.index || '';
        bVal = b.index || '';
      } else if (sortConfig.key === 'created') {
        aVal = a.created || 0;
        bVal = b.created || 0;
      } else if (sortConfig.key === 'updated') {
        aVal = a.updated || 0;
        bVal = b.updated || 0;
      } else {
        const aFlat = flattenData(a.data || {});
        const bFlat = flattenData(b.data || {});
        aVal = aFlat[sortConfig.key];
        bVal = bFlat[sortConfig.key];
      }
      
      if (aVal === null || aVal === undefined) aVal = '';
      if (bVal === null || bVal === undefined) bVal = '';
      
      if (typeof aVal === 'number' && typeof bVal === 'number') {
        return sortConfig.direction === 'asc' ? aVal - bVal : bVal - aVal;
      }
      
      const aStr = String(aVal).toLowerCase();
      const bStr = String(bVal).toLowerCase();
      if (aStr < bStr) return sortConfig.direction === 'asc' ? -1 : 1;
      if (aStr > bStr) return sortConfig.direction === 'asc' ? 1 : -1;
      return 0;
    });
  }, [filteredItems, sortConfig, flattenData]);

  const handleSort = (key) => {
    setSortConfig(prev => {
      if (prev.key === key) {
        if (prev.direction === 'asc') return { key, direction: 'desc' };
        return { key: null, direction: 'asc' };
      }
      return { key, direction: 'asc' };
    });
  };

  const getSortIndicator = (key) => {
    if (sortConfig.key !== key) return null;
    return sortConfig.direction === 'asc' ? ' ↑' : ' ↓';
  };

  const totalPages = Math.max(1, Math.ceil(sortedItems.length / limit));
  const start = (page - 1) * limit;
  const paginatedItems = sortedItems.slice(start, start + limit);

  const goBack = () => {
    window.location.hash = '/proxies';
  };

  const viewKeyLive = (e, item) => {
    e.stopPropagation();
    const localPath = buildLocalPath(item.index);
    const params = new URLSearchParams();
    params.set('source', 'proxies');
    params.set('canRead', canRead ? '1' : '0');
    params.set('canWrite', canWrite ? '1' : '0');
    params.set('canDelete', canDelete ? '1' : '0');
    params.set('mode', 'live');
    params.set('from', proxyPath);
    window.location.hash = '/proxy/view/' + encodeURIComponent(localPath) + '?' + params.toString();
  };

  const openEditModal = (item) => {
    if (!canWrite && !canRead) return;
    const localPath = buildLocalPath(item.index);
    setKeyToEdit(localPath);
    setEditModalVisible(true);
  };

  const closeEditModal = () => {
    setEditModalVisible(false);
    setKeyToEdit('');
  };

  const confirmDelete = (e, item) => {
    e.stopPropagation();
    const localPath = buildLocalPath(item.index);
    setKeyToDelete(localPath);
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

  const toggleSelectItem = (e, index) => {
    e.stopPropagation();
    setSelectedItems(prev => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  const toggleSelectAll = (e) => {
    e.stopPropagation();
    if (selectedItems.size === paginatedItems.length) {
      setSelectedItems(new Set());
    } else {
      setSelectedItems(new Set(paginatedItems.map(item => item.index)));
    }
  };

  const openBatchDelete = () => {
    if (selectedItems.size === 0) return;
    setBatchDeleteError('');
    setBatchDeleteProgress({ done: 0, total: selectedItems.size });
    setBatchDeleteVisible(true);
  };

  const closeBatchDelete = () => {
    if (batchDeleteLoading) return;
    setBatchDeleteVisible(false);
    setBatchDeleteError('');
  };

  const executeBatchDelete = async () => {
    if (batchDeleteLoading || selectedItems.size === 0) return;
    setBatchDeleteLoading(true);
    setBatchDeleteError('');
    
    const indices = Array.from(selectedItems);
    const total = indices.length;
    let done = 0;
    const errors = [];
    
    const deletePromises = indices.map(async (index) => {
      const localPath = buildLocalPath(index);
      try {
        const res = await fetch('/' + localPath, { method: 'DELETE' });
        if (!res.ok) {
          const text = await res.text();
          throw new Error(text || 'Failed');
        }
        done++;
        setBatchDeleteProgress({ done, total });
      } catch (err) {
        errors.push(`${index}: ${err.message}`);
      }
    });
    
    await Promise.all(deletePromises);
    
    setBatchDeleteLoading(false);
    
    if (errors.length > 0) {
      setBatchDeleteError(`Failed to delete ${errors.length} item(s): ${errors.slice(0, 3).join(', ')}${errors.length > 3 ? '...' : ''}`);
    } else {
      setSelectedItems(new Set());
      setBatchDeleteVisible(false);
    }
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

  const totalColumns = (canDelete ? 1 : 0) + 3 + visibleColumns.length + 1;

  return (
    <div className="container">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">
          {proxyPath}
          <span className={`connection-status ${connected ? 'connected' : 'disconnected'}`}>
            {connected ? <IconWifi /> : <IconWifiOff />}
          </span>
        </span>
        <div className="header-right">
          <span className="proxy-caps">
            {canRead && <span className="cap-badge read">Read</span>}
            {canWrite && <span className="cap-badge write">Write</span>}
            {canDelete && <span className="cap-badge delete">Delete</span>}
          </span>
          {canDelete && selectedItems.size > 0 && (
            <button className="btn danger" onClick={openBatchDelete}>
              <IconTrash color="#fff" />
              Delete ({selectedItems.size})
            </button>
          )}
          <input 
            type="text" 
            className="search-box" 
            placeholder="Search..." 
            value={searchTerm}
            onChange={(e) => { setSearchTerm(e.target.value); setPage(1); }}
            style={{ width: '200px' }}
          />
        </div>
      </div>

      {error && (
        <div className="error-banner">
          Connection error: {error.message || 'Failed to connect'}
        </div>
      )}

      {!tableReady && items.length > 0 ? (
        <div className="table-container">
          <div className="loading-container">
            <div className="spinner"></div>
            <div>Loading data...</div>
          </div>
        </div>
      ) : (
      <div className="table-container table-scroll" ref={tableContainerRef}>
        <table className="data-table">
          <thead>
            <tr>
              {canDelete && (
                <th className="checkbox-col sticky-col">
                  <input 
                    type="checkbox" 
                    checked={paginatedItems.length > 0 && selectedItems.size === paginatedItems.length}
                    onChange={toggleSelectAll}
                  />
                </th>
              )}
              <th className="sortable-header" onClick={() => handleSort('index')}>
                Index{getSortIndicator('index')}
              </th>
              <th className="sortable-header" onClick={() => handleSort('created')}>
                Created{getSortIndicator('created')}
              </th>
              <th className="sortable-header" onClick={() => handleSort('updated')}>
                Updated{getSortIndicator('updated')}
              </th>
              {visibleColumns.map(col => {
                const isCollapsed = collapsedFields[col];
                const canCollapse = isCollapsible(col);
                const ancestors = getCollapsibleAncestors(col);
                
                if (canCollapse && isCollapsed) {
                  const parentOfCollapsed = ancestors.filter(a => a !== col);
                  return (
                    <th key={col} data-path={col} className="collapsible-header collapsed" onClick={() => toggleCollapse(col, true)}>
                      <span className="collapse-toggle">
                        <span className="collapse-btn expand-btn" onClick={(e) => { e.stopPropagation(); toggleCollapse(col, true); }} title="Expand">
                          <IconChevronRight />
                        </span>
                        {col}
                        <span className="collapsed-count">({getHiddenCount(col)})</span>
                        {parentOfCollapsed.map(parent => (
                          <span key={parent} className="collapse-btn collapse-up" onClick={(e) => { e.stopPropagation(); toggleCollapse(parent); }} title={`Collapse to ${parent}`}>
                            <IconChevronUp />
                          </span>
                        ))}
                      </span>
                    </th>
                  );
                } else if (ancestors.length > 0) {
                  return (
                    <th key={col} data-path={col} className="collapsible-header expanded sortable-header">
                      <span className="collapse-toggle">
                        {ancestors.map((ancestor, idx) => (
                          <span key={ancestor} className={`collapse-btn ${idx === ancestors.length - 1 ? 'collapse-btn-down' : 'collapse-up'}`} onClick={(e) => { e.stopPropagation(); toggleCollapse(ancestor); }} title={`Collapse to ${ancestor}`}>
                            {idx === ancestors.length - 1 ? <IconChevronDown /> : <IconChevronUp />}
                          </span>
                        ))}
                        <span onClick={(e) => { e.stopPropagation(); handleSort(col); }}>{col}{getSortIndicator(col)}</span>
                      </span>
                    </th>
                  );
                } else {
                  return (
                    <th key={col} className="sortable-header" onClick={() => handleSort(col)}>
                      {col}{getSortIndicator(col)}
                    </th>
                  );
                }
              })}
              <th className="actions-col"><IconEye /></th>
            </tr>
          </thead>
          <tbody>
            {paginatedItems.length === 0 ? (
              <tr>
                <td colSpan={totalColumns}>
                  <div className="empty-state">
                    <IconBox />
                    <div>{connected ? 'No data found' : 'Connecting...'}</div>
                  </div>
                </td>
              </tr>
            ) : (
              paginatedItems.map(item => {
                const flatData = flattenData(item.data || {});
                const highlight = highlightedItems[item.index];
                const highlightClass = highlight ? `highlight-${highlight.type}` : '';
                const isSelected = selectedItems.has(item.index);
                return (
                  <tr key={item.index} onClick={() => openEditModal(item)} className={`${highlightClass} ${isSelected ? 'selected' : ''}`} style={{ cursor: canRead || canWrite ? 'pointer' : 'default' }}>
                    {canDelete && (
                      <td className="checkbox-col sticky-col" onClick={(e) => e.stopPropagation()}>
                        <input 
                          type="checkbox" 
                          checked={isSelected}
                          onChange={(e) => toggleSelectItem(e, item.index)}
                        />
                      </td>
                    )}
                    <td><span className="index-cell">{extractGlobValues(item.path)}</span></td>
                    <td><span className="text-muted">{formatTime(item.created)}</span></td>
                    <td><span className="text-muted">{formatTime(item.updated)}</span></td>
                    {visibleColumns.map(col => {
                      const isCollapsedCol = isCollapsible(col) && collapsedFields[col];
                      if (isCollapsedCol) {
                        const getNestedValue = (obj, path) => {
                          const parts = path.split('.');
                          let current = obj;
                          for (const part of parts) {
                            if (current === null || current === undefined) return undefined;
                            current = current[part];
                          }
                          return current;
                        };
                        const nestedData = getNestedValue(item.data, col);
                        return (
                          <td key={col}>
                            <JsonTreeCell data={nestedData} maxDepth={2} />
                          </td>
                        );
                      }
                      return (
                        <td key={col}>
                          {renderCellValue(flatData[col])}
                        </td>
                      );
                    })}
                    <td className="actions-col">
                      <div className="actions">
                        {canRead && (
                          <button className="btn ghost" title="View Live" onClick={(e) => viewKeyLive(e, item)}>
                            <IconEye />
                          </button>
                        )}
                        {canDelete && (
                          <button className="btn ghost" title="Delete" onClick={(e) => confirmDelete(e, item)}>
                            <IconTrash />
                          </button>
                        )}
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

      <EditKeyModal
        visible={editModalVisible}
        keyPath={keyToEdit}
        filterPath={proxyPath}
        onClose={closeEditModal}
        readOnly={!canWrite}
        isProxy={true}
      />

      {batchDeleteVisible && (
        <div className="modal-overlay" onClick={closeBatchDelete}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <div className="modal-title">Delete {selectedItems.size} Items</div>
            <div className="modal-message">
              Are you sure you want to delete {selectedItems.size} selected item(s)? This action cannot be undone.
            </div>
            {batchDeleteLoading && (
              <div className="batch-progress">
                <div className="progress-bar">
                  <div 
                    className="progress-fill" 
                    style={{ width: `${(batchDeleteProgress.done / batchDeleteProgress.total) * 100}%` }}
                  />
                </div>
                <div className="progress-text">
                  Deleting... {batchDeleteProgress.done} / {batchDeleteProgress.total}
                </div>
              </div>
            )}
            {batchDeleteError && (
              <div className="modal-error">{batchDeleteError}</div>
            )}
            <div className="modal-actions">
              <button className="btn-cancel" onClick={closeBatchDelete} disabled={batchDeleteLoading}>
                Cancel
              </button>
              <button className="btn-confirm btn-danger" onClick={executeBatchDelete} disabled={batchDeleteLoading}>
                {batchDeleteLoading ? 'Deleting...' : 'Delete All'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

window.ProxyListView = ProxyListView;
