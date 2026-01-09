function StorageKeysLive({ filterPath, fromFilter }) {
  const { useState, useEffect, useCallback, useMemo, useRef } = React;
  const { IconBox, IconChevronLeft, IconChevronRight, IconChevronDown, IconChevronUp, IconTrash, IconEdit, IconEye, IconSend, IconWifi, IconWifiOff, IconFilter } = window.Icons;
  const ConfirmModal = window.ConfirmModal;
  const PushDialog = window.PushDialog;
  const EditKeyModal = window.EditKeyModal;
  const PathNavigatorModal = window.PathNavigatorModal;
  const JsonTreeCell = window.JsonTreeCell;
  
  const [searchTerm, setSearchTerm] = useState('');
  const [page, setPage] = useState(1);
  const [modalVisible, setModalVisible] = useState(false);
  const [keyToDelete, setKeyToDelete] = useState('');
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [keyToEdit, setKeyToEdit] = useState('');
  const [pathNavVisible, setPathNavVisible] = useState(false);
  const getStorageKey = () => `ooo-collapsed-${filterPath}`;
  
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
  const [filterInfo, setFilterInfo] = useState(null);
  const [modalLoading, setModalLoading] = useState(false);
  const [modalError, setModalError] = useState('');
  const [pushDialogVisible, setPushDialogVisible] = useState(false);
  const [sortConfig, setSortConfig] = useState({ key: null, direction: 'asc' });
  const [selectedItems, setSelectedItems] = useState(new Set());
  const [batchDeleteVisible, setBatchDeleteVisible] = useState(false);
  const [batchDeleteLoading, setBatchDeleteLoading] = useState(false);
  const [batchDeleteError, setBatchDeleteError] = useState('');
  const [batchDeleteProgress, setBatchDeleteProgress] = useState({ done: 0, total: 0 });
  const prevItemsLengthRef = useRef(0);
  const tableContainerRef = useRef(null);
  const limit = 50;

  const { items, connected, error, lastUpdate, clearLastUpdate } = Api.useSubscribeGlob(filterPath);

  useEffect(() => {
    fetch('/?api=filters')
      .then(res => res.json())
      .then(data => {
        const info = (data.filters || []).find(f => f.path === filterPath);
        setFilterInfo(info);
        // Custom filters have no client access
        if (info && info.type === 'custom') {
          window.location.hash = '/storage';
        }
      })
      .catch(() => {});
  }, [filterPath]);

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
          // Only remove if the timestamp matches (avoid removing newer highlights)
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

  // Check if filter has multiple globs
  const hasMultipleGlobs = useMemo(() => {
    return (filterPath.match(/\*/g) || []).length > 1;
  }, [filterPath]);

  // Extract glob segment values from item.path based on filter path pattern
  const extractGlobValues = useCallback((itemPath) => {
    if (!itemPath) return '';
    const filterParts = filterPath.split('/');
    const pathParts = itemPath.split('/');
    const globValues = [];
    
    filterParts.forEach((part, idx) => {
      if (part === '*' && pathParts[idx]) {
        globValues.push(pathParts[idx]);
      }
    });
    
    return globValues.join('/');
  }, [filterPath]);

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

  // Initialize collapsed state when items first arrive - default to collapsed for unknown columns
  useEffect(() => {
    if (items.length > 0 && prevItemsLengthRef.current === 0) {
      const savedState = loadCollapsedState();
      const newCollapsedFields = { ...savedState };
      
      // Default all collapsible paths to collapsed if not in saved state
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

  // Save collapsed state to localStorage whenever it changes
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
    
    // Sort: basic types first, then collapsible/nested fields
    const uniqueResult = [...new Set(result)];
    
    // Helper to check if a column is or belongs to a collapsible path
    const isNestedOrCollapsible = (col) => {
      // Check if it's a collapsible path itself
      if (collapsiblePaths.includes(col)) return true;
      // Check if it's a child of any collapsible path
      for (const path of collapsiblePaths) {
        if (col.startsWith(path + '.')) return true;
      }
      return false;
    };
    
    return uniqueResult.sort((a, b) => {
      const aIsNested = isNestedOrCollapsible(a);
      const bIsNested = isNestedOrCollapsible(b);
      // Basic types come before nested/collapsible
      if (!aIsNested && bIsNested) return -1;
      if (aIsNested && !bIsNested) return 1;
      // Within same category, sort alphabetically
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
      // Expanding: only expand this level, collapse immediate children
      const immediateChildren = collapsiblePaths.filter(p => {
        if (!p.startsWith(path + '.')) return false;
        const remainder = p.slice(path.length + 1);
        return !remainder.includes('.');
      });
      
      setCollapsedFields(prev => {
        const next = { ...prev, [path]: false };
        // Collapse immediate children so we only expand one level
        immediateChildren.forEach(child => {
          next[child] = true;
        });
        return next;
      });
    } else {
      // Collapsing or simple toggle
      setCollapsedFields(prev => ({ ...prev, [path]: !prev[path] }));
    }
    
    // Scroll to make the column visible after collapse
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
          // Search in index
          if (item.index && item.index.toLowerCase().includes(searchLower)) return true;
          // Search in path (glob values)
          if (item.path && item.path.toLowerCase().includes(searchLower)) return true;
          // Search in extracted glob values
          const globValues = extractGlobValues(item.path);
          if (globValues && globValues.toLowerCase().includes(searchLower)) return true;
          // Search in data
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

  // Clear selected items that are no longer visible after search filter
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
        // Data column
        const aFlat = flattenData(a.data || {});
        const bFlat = flattenData(b.data || {});
        aVal = aFlat[sortConfig.key];
        bVal = bFlat[sortConfig.key];
      }
      
      // Handle null/undefined
      if (aVal === null || aVal === undefined) aVal = '';
      if (bVal === null || bVal === undefined) bVal = '';
      
      // Numeric comparison
      if (typeof aVal === 'number' && typeof bVal === 'number') {
        return sortConfig.direction === 'asc' ? aVal - bVal : bVal - aVal;
      }
      
      // String comparison
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
        // Toggle direction or clear
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
    if (fromFilter) {
      window.location.hash = '/storage/keys/live/' + encodeURIComponent(fromFilter);
    } else {
      window.location.hash = '/storage';
    }
  };


  const viewKeyLive = (e, item) => {
    e.stopPropagation();
    window.location.hash = '/storage/key/live/' + encodeURIComponent(item.path) + '?from=' + encodeURIComponent(filterPath);
  };

  const pushData = () => {
    setPushDialogVisible(true);
  };

  const closePushDialog = () => {
    setPushDialogVisible(false);
  };

  const openEditModal = (item) => {
    setKeyToEdit(item.path);
    setEditModalVisible(true);
  };

  const closeEditModal = () => {
    setEditModalVisible(false);
    setKeyToEdit('');
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

  // Selection handlers
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
    
    // Run all deletes in parallel
    const deletePromises = indices.map(async (index) => {
      const keyPath = filterPath.replace('*', index);
      try {
        const res = await fetch('/' + keyPath, { method: 'DELETE' });
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

  const totalColumns = 5 + visibleColumns.length; // checkbox + index + created + updated + actions + data columns

  return (
    <div className="container">
      <div className="edit-page-header">
        <button className="btn secondary" onClick={goBack}>
          <IconChevronLeft />
          Back
        </button>
        <span className="edit-page-title">
          {filterPath}
          <span className={`connection-status ${connected ? 'connected' : 'disconnected'}`}>
            {connected ? <IconWifi /> : <IconWifiOff />}
          </span>
        </span>
        <div className="header-right">
          {hasMultipleGlobs && items.length > 0 && (
            <button className="btn secondary" onClick={() => setPathNavVisible(true)} title="Navigate to sub-path">
              <IconFilter />
              Paths
            </button>
          )}
          {filterInfo && filterInfo.canDelete && selectedItems.size > 0 && (
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
          {filterInfo && filterInfo.type !== 'read-only' && (
            <button className="btn" onClick={pushData}>
              <IconSend />
              Push
            </button>
          )}
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
              {filterInfo && filterInfo.canDelete && (
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
                const isSortable = !canCollapse || !isCollapsed;
                
                if (canCollapse && isCollapsed) {
                  // Collapsed column - show expand button and collapse-up buttons for all ancestors
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
                  // Expanded column with collapsible ancestors - show collapse buttons for ALL ancestors
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
                  // Top-level column - sortable
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
                  <tr key={item.index} onClick={() => openEditModal(item)} className={`${highlightClass} ${isSelected ? 'selected' : ''}`}>
                    {filterInfo && filterInfo.canDelete && (
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
                        // Get the nested object for this collapsed path
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
                        <button className="btn ghost" title="View Live" onClick={(e) => viewKeyLive(e, item)}>
                          <IconEye />
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

      <PushDialog
        visible={pushDialogVisible}
        filterPath={filterPath}
        existingKeys={items.map(item => item.path)}
        onClose={closePushDialog}
        onEditKey={(keyPath) => {
          setKeyToEdit(keyPath);
          setEditModalVisible(true);
        }}
      />

      <EditKeyModal
        visible={editModalVisible}
        keyPath={keyToEdit}
        filterPath={filterPath}
        onClose={closeEditModal}
      />

      <PathNavigatorModal
        visible={pathNavVisible}
        filterPath={filterPath}
        items={items}
        onClose={() => setPathNavVisible(false)}
      />

      {/* Batch Delete Modal */}
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

window.StorageKeysLive = StorageKeysLive;
